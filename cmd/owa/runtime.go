package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nkiyohara/owa-bridge/internal/browser"
	"github.com/nkiyohara/owa-bridge/internal/buildinfo"
	"github.com/nkiyohara/owa-bridge/internal/config"
	"github.com/nkiyohara/owa-bridge/internal/daemonapi"
	"github.com/nkiyohara/owa-bridge/internal/domain"
	"github.com/nkiyohara/owa-bridge/internal/localipc"
	"github.com/nkiyohara/owa-bridge/internal/paths"
	"github.com/nkiyohara/owa-bridge/internal/session"
	"github.com/nkiyohara/owa-bridge/internal/updatecheck"
)

type browserHandle interface {
	WaitForSession(context.Context) (session.Credentials, error)
	Apply(*http.Request) error
	Close() error
}

type terminalBrowserHandle interface {
	browserHandle
	CurrentSession() (session.Credentials, error)
	TerminalSnapshot(context.Context) (browser.TerminalView, error)
	TerminalAct(context.Context, browser.TerminalAction) error
}

type browserLauncher func(context.Context, browser.Options) (browserHandle, error)

type commandRunner func(context.Context, io.Writer, io.Writer, string, ...string) error

type runtime struct {
	context           context.Context
	configPath        string
	info              buildinfo.Info
	stdin             io.Reader
	stdout            io.Writer
	stderr            io.Writer
	launch            browserLauncher
	endpoint          func(string) (localipc.Endpoint, error)
	runCommand        commandRunner
	processID         int
	checkUpdate       func(context.Context) (updatecheck.Result, error)
	installMethod     func() updatecheck.InstallMethod
	interactiveOutput func() bool
	lookupEnv         func(string) (string, bool)
}

func newRuntime(
	ctx context.Context,
	configPath string,
	stdout, stderr io.Writer,
	info buildinfo.Info,
) *runtime {
	app := &runtime{
		context:    ctx,
		configPath: configPath,
		info:       info,
		stdin:      os.Stdin,
		stdout:     stdout,
		stderr:     stderr,
		launch: func(ctx context.Context, options browser.Options) (browserHandle, error) {
			return browser.Launch(ctx, options)
		},
		endpoint: localipc.Resolve,
		runCommand: func(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
			command := exec.CommandContext(ctx, name, args...) // #nosec G204 -- name and args come from typed client setup plans.
			command.Stdout = stdout
			command.Stderr = stderr
			return command.Run()
		},
		processID: os.Getpid(),
		lookupEnv: os.LookupEnv,
	}
	app.checkUpdate = func(ctx context.Context) (updatecheck.Result, error) {
		cachePath, err := paths.UpdateCachePath()
		if err != nil {
			return updatecheck.Result{}, err
		}
		return (updatecheck.Checker{
			CurrentVersion: app.info.Version,
			CachePath:      cachePath,
			Client:         &http.Client{Timeout: 5 * time.Second},
		}).Check(ctx)
	}
	app.installMethod = func() updatecheck.InstallMethod {
		executable, err := os.Executable()
		if err != nil {
			return updatecheck.InstallDirect
		}
		return updatecheck.DetectInstallation(executable)
	}
	app.interactiveOutput = func() bool { return outputIsTerminal(app.stderr) }
	return app
}

// openDaemon connects to the config-scoped session owner, starting it when
// absent. It never receives Outlook authorization material.
func (app *runtime) openDaemon(ctx context.Context) (*daemonapi.Client, daemonapi.Status, error) {
	if _, _, err := app.loadConfig(); err != nil {
		return nil, daemonapi.Status{}, err
	}
	configPath, err := app.resolvedConfigPath()
	if err != nil {
		return nil, daemonapi.Status{}, err
	}
	configDigest, err := config.Fingerprint(configPath)
	if err != nil {
		return nil, daemonapi.Status{}, err
	}
	endpoint, err := app.endpoint(configPath)
	if err != nil {
		return nil, daemonapi.Status{}, err
	}
	client, err := daemonapi.NewClient(endpoint)
	if err != nil {
		return nil, daemonapi.Status{}, err
	}
	probeContext, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	status, statusErr := client.Status(probeContext, app.caller())
	cancel()
	if statusErr == nil {
		if err := app.validateDaemonStatus(status, configDigest); err != nil {
			return nil, daemonapi.Status{}, errors.Join(err, client.Close())
		}
		return client, status, nil
	}
	if err := startDetachedDaemon(ctx, configPath); err != nil {
		return nil, daemonapi.Status{}, errors.Join(err, client.Close())
	}
	status, err = waitForDaemon(ctx, app, client, 5*time.Second)
	if err != nil {
		return nil, daemonapi.Status{}, errors.Join(err, client.Close())
	}
	if err := app.validateDaemonStatus(status, configDigest); err != nil {
		return nil, daemonapi.Status{}, errors.Join(err, client.Close())
	}
	return client, status, nil
}

func (app *runtime) validateDaemonStatus(status daemonapi.Status, configDigest string) error {
	if status.ConfigDigest != configDigest {
		return errors.New("session owner has stale configuration; run `owa daemon stop` and retry")
	}
	if status.Version != app.info.Version {
		return fmt.Errorf(
			"session owner version %s differs from CLI %s; run `owa daemon stop` and retry",
			status.Version, app.info.Version,
		)
	}
	return nil
}

func (app *runtime) resolvedConfigPath() (string, error) {
	if app.configPath == "" {
		return config.DefaultPath()
	}
	absolute, err := filepath.Abs(app.configPath)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	return filepath.Clean(absolute), nil
}

func (app *runtime) loadConfig() (config.Config, string, error) {
	path, err := app.resolvedConfigPath()
	if err != nil {
		return config.Config{}, "", err
	}
	configuration, err := config.Load(path)
	if err != nil {
		return config.Config{}, path, err
	}
	return configuration, path, nil
}

func (app *runtime) account(
	configuration config.Config,
	requested string,
) (domain.AccountID, error) {
	alias := requested
	if alias == "" {
		alias = configuration.DefaultAccount
	}
	_, exists := configuration.Accounts[alias]
	if !exists {
		return "", fmt.Errorf("account %q is not configured", alias)
	}
	return domain.AccountID(alias), nil
}

func (app *runtime) authenticate(
	ctx context.Context,
	configuration config.Config,
	accountID domain.AccountID,
	account config.Account,
) (browserHandle, session.Credentials, error) {
	profileDirectory, err := paths.ProfileDir(accountID)
	if err != nil {
		return nil, session.Credentials{}, err
	}
	if _, err := fmt.Fprintf(app.stderr, "Opening Outlook Web for account %q; complete sign-in in the browser.\n", accountID); err != nil {
		return nil, session.Credentials{}, err
	}
	handle, err := app.launch(ctx, browser.Options{
		Origin:     account.Origin,
		ProfileDir: profileDirectory,
		Executable: configuration.Browser.Executable,
	})
	if err != nil {
		return nil, session.Credentials{}, err
	}
	waitContext, cancel := context.WithTimeout(ctx, time.Duration(configuration.Browser.LoginTimeout))
	defer cancel()
	credentials, err := handle.WaitForSession(waitContext)
	if err != nil {
		closeErr := handle.Close()
		return nil, session.Credentials{}, errors.Join(err, closeErr)
	}
	return handle, credentials, nil
}

func (app *runtime) caller() domain.Caller {
	return domain.Caller{Surface: "cli", Instance: fmt.Sprintf("process-%d", app.processID)}
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
