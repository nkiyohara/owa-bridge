package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nkiyohara/owa-bridge/internal/config"
	"github.com/nkiyohara/owa-bridge/internal/updatecheck"
)

type updateCommand struct {
	Check updateCheckCommand `cmd:"" help:"Check the latest stable public release."`
}

type updateCheckCommand struct {
	JSON bool `help:"Write machine-readable JSON."`
}

type updateReport struct {
	updatecheck.Result
	InstallMethod updatecheck.InstallMethod `json:"installMethod,omitempty"`
	Upgrade       string                    `json:"upgrade,omitempty"`
}

func (command *updateCheckCommand) Run(app *runtime) error {
	ctx, cancel := context.WithTimeout(app.context, 5*time.Second)
	defer cancel()
	report, _ := app.updateReport(ctx)
	if command.JSON {
		return writeJSON(app.stdout, report)
	}
	switch report.Status {
	case updatecheck.StatusAvailable:
		_, err := fmt.Fprintf(
			app.stdout,
			"Update available: owa %s -> %s\nInstall method: %s\nUpgrade: %s\nRelease: %s\n",
			report.CurrentVersion,
			report.LatestVersion,
			report.InstallMethod,
			report.Upgrade,
			report.ReleaseURL,
		)
		return err
	case updatecheck.StatusCurrent:
		_, err := fmt.Fprintf(
			app.stdout,
			"owa %s is current; latest stable is %s (checked %s).\n",
			report.CurrentVersion,
			report.LatestVersion,
			report.CheckedAt,
		)
		return err
	case updatecheck.StatusDevelopment:
		_, err := fmt.Fprintf(app.stdout, "owa %s is a development build; automatic version comparison is skipped.\n", report.CurrentVersion)
		return err
	case updatecheck.StatusUnavailable:
		_, err := fmt.Fprintln(app.stdout, "Update status is temporarily unavailable; normal owa operations are unaffected.")
		return err
	}
	return errors.New("unknown update status")
}

func (app *runtime) updateReport(ctx context.Context) (updateReport, error) {
	result, err := app.checkUpdate(ctx)
	report := updateReport{Result: result}
	if result.LatestVersion != "" {
		report.InstallMethod = app.installMethod()
		report.Upgrade = updatecheck.UpgradeAdvice(report.InstallMethod, result.LatestVersion)
	}
	return report, err
}

func (app *runtime) maybeNotifyUpdate(parent context.Context) {
	if app.interactiveOutput == nil || !app.interactiveOutput() || !app.automaticUpdateChecksEnabled(nil) {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 750*time.Millisecond)
	defer cancel()
	report, err := app.updateReport(ctx)
	if err != nil || report.Status != updatecheck.StatusAvailable {
		return
	}
	_, _ = fmt.Fprintf(
		app.stderr,
		"\nUpdate available: owa %s -> %s. %s\n",
		report.CurrentVersion,
		report.LatestVersion,
		report.Upgrade,
	)
}

func (app *runtime) automaticUpdateChecksEnabled(configuration *config.Config) bool {
	if value, exists := app.lookupEnv("OWA_NO_UPDATE_CHECK"); exists && disablesUpdateCheck(value) {
		return false
	}
	if configuration != nil {
		return !configuration.Updates.DisableAutomaticChecks
	}
	loaded, _, err := app.loadConfig()
	if err == nil {
		return !loaded.Updates.DisableAutomaticChecks
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return false
}

func disablesUpdateCheck(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func shouldOfferAutomaticUpdateNotice(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--json" || strings.HasPrefix(argument, "--json=") {
			return false
		}
	}
	command := rootCommand(arguments)
	switch command {
	case "", "completion", "daemon", "doctor", "mcp", "update":
		return false
	default:
		return !completionEnvironmentActive()
	}
}

func rootCommand(arguments []string) string {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--config" {
			index++
			continue
		}
		if strings.HasPrefix(argument, "-") {
			continue
		}
		return argument
	}
	return ""
}
