// Command owa provides the CLI, daemon, and MCP entry point for owa-bridge.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	kongcompletion "github.com/jotaen/kong-completion"

	"github.com/nkiyohara/owa-bridge/internal/buildinfo"
)

type cli struct {
	ConfigPath string            `name:"config" type:"path" env:"OWA_CONFIG" help:"Path to config.toml."`
	Version    versionCommand    `cmd:"" help:"Print version and build information."`
	Config     configCommand     `cmd:"" help:"Initialize and inspect configuration."`
	Doctor     doctorCommand     `cmd:"" help:"Diagnose local setup and opt-in OWA compatibility."`
	Login      loginCommand      `cmd:"" help:"Open the interactive Outlook Web sign-in."`
	Mail       mailCommand       `cmd:"" help:"Read and manage mail."`
	Calendar   calendarCommand   `cmd:"" help:"Read and manage calendar events."`
	Daemon     daemonCommand     `cmd:"" help:"Run and inspect the local session owner."`
	MCP        mcpCommand        `cmd:"" help:"Expose guarded Outlook tools over MCP."`
	Completion completionCommand `cmd:"" help:"Generate a shell completion script."`
}

type versionCommand struct {
	JSON bool `help:"Write machine-readable JSON."`
}

func (command *versionCommand) Run(app *runtime) error {
	if command.JSON {
		encoder := json.NewEncoder(app.stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(app.info)
	}

	_, err := fmt.Fprintf(
		app.stdout,
		"owa %s (commit %s, built %s, %s)\n",
		app.info.Version,
		app.info.Commit,
		app.info.BuildDate,
		app.info.GoVersion,
	)
	return err
}

func run(executionContext context.Context, arguments []string, stdout, stderr io.Writer) int {
	if !completionEnvironmentActive() && (len(arguments) == 0 ||
		(len(arguments) == 1 && (arguments[0] == "--help" || arguments[0] == "-h"))) {
		_, err := fmt.Fprint(stdout, `owa: Local-first Outlook Web mail and calendar.

Usage:
  owa <command> [flags]

Commands:
  config     Initialize and inspect configuration
  doctor     Diagnose local setup and opt-in OWA compatibility
  login      Open the interactive Outlook Web sign-in
  mail       Read and manage mail
  calendar   Read and manage calendar events
  daemon     Run and inspect the local session owner
  mcp        Expose guarded Outlook tools over MCP
  completion Generate a shell completion script
  version    Print version and build information

Run "owa <command> --help" for command-specific help.
`)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	var commandLine cli
	exitCode := -1
	parser, err := kong.New(
		&commandLine,
		kong.Name("owa"),
		kong.Description("Local-first Outlook Web mail and calendar."),
		kong.UsageOnError(),
		kong.Writers(stdout, stderr),
		kong.Exit(func(code int) { exitCode = code }),
	)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	var completionErr error
	kongcompletion.Register(
		parser,
		kongcompletion.WithExitFunc(func(code int) { exitCode = code }),
		kongcompletion.WithErrorHandler(func(err error) { completionErr = err }),
	)
	if completionErr != nil {
		_, _ = fmt.Fprintf(stderr, "initialize shell completion: %v\n", completionErr)
		return 1
	}
	if exitCode >= 0 {
		return exitCode
	}

	parsed, err := parser.Parse(arguments)
	if exitCode >= 0 {
		return exitCode
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}

	app := newRuntime(executionContext, commandLine.ConfigPath, stdout, stderr, buildinfo.Current())
	if err := parsed.Run(app); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func completionEnvironmentActive() bool {
	return os.Getenv("COMP_LINE") != ""
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
