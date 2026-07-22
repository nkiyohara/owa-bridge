package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nkiyohara/owa-bridge/internal/buildinfo"
	"github.com/nkiyohara/owa-bridge/internal/config"
	"github.com/nkiyohara/owa-bridge/internal/updatecheck"
)

func TestUpdateCheckProducesHumanAndMachineReadableStatus(t *testing.T) {
	result := updatecheck.Result{
		Status: updatecheck.StatusAvailable, CurrentVersion: "0.3.2",
		LatestVersion: "v0.4.0", UpdateAvailable: true,
		ReleaseURL: "https://github.com/nkiyohara/owa-bridge/releases/tag/v0.4.0",
		CheckedAt:  time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
	for _, jsonOutput := range []bool{false, true} {
		var stdout bytes.Buffer
		app := updateTestRuntime(t, &stdout, result)
		command := updateCheckCommand{JSON: jsonOutput}
		if err := command.Run(app); err != nil {
			t.Fatalf("Run(JSON=%v) error = %v", jsonOutput, err)
		}
		if jsonOutput {
			var report updateReport
			if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
				t.Fatalf("decode JSON: %v", err)
			}
			if report.Status != updatecheck.StatusAvailable || strings.Contains(stdout.String(), "Update available:") {
				t.Fatalf("unexpected machine output: %s", stdout.String())
			}
		} else if !strings.Contains(stdout.String(), "Update available: owa 0.3.2 -> v0.4.0") ||
			!strings.Contains(stdout.String(), "brew upgrade owa-bridge") {
			t.Fatalf("unexpected human output: %s", stdout.String())
		}
	}
}

func TestAutomaticUpdateNoticeIsTTYOnlyAndHonorsOptOuts(t *testing.T) {
	result := updatecheck.Result{
		Status: updatecheck.StatusAvailable, CurrentVersion: "0.3.2",
		LatestVersion: "v0.4.0", UpdateAvailable: true,
	}
	var stdout bytes.Buffer
	app := updateTestRuntime(t, &stdout, result)
	var stderr bytes.Buffer
	app.stderr = &stderr
	app.interactiveOutput = func() bool { return true }
	app.maybeNotifyUpdate(t.Context())
	if !strings.Contains(stderr.String(), "Update available:") {
		t.Fatalf("TTY notice missing: %q", stderr.String())
	}

	configuration := config.Default()
	configuration.Updates.DisableAutomaticChecks = true
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(configPath, configuration); err != nil {
		t.Fatal(err)
	}
	app.configPath = configPath
	stderr.Reset()
	app.maybeNotifyUpdate(t.Context())
	if stderr.Len() != 0 {
		t.Fatalf("config opt-out emitted notice: %q", stderr.String())
	}

	configuration.Updates.DisableAutomaticChecks = false
	if err := config.Save(configPath, configuration); err != nil {
		t.Fatal(err)
	}
	app.lookupEnv = func(name string) (string, bool) {
		if name == "OWA_NO_UPDATE_CHECK" {
			return "1", true
		}
		return "", false
	}
	app.maybeNotifyUpdate(t.Context())
	if stderr.Len() != 0 {
		t.Fatalf("environment opt-out emitted notice: %q", stderr.String())
	}
}

func TestMachineSurfacesNeverOfferAutomaticNotice(t *testing.T) {
	tests := [][]string{
		{"mcp", "serve"},
		{"mcp", "config", "codex"},
		{"completion", "bash"},
		{"version", "--json"},
		{"doctor", "--json"},
		{"update", "check"},
		{"daemon", "run"},
	}
	for _, arguments := range tests {
		if shouldOfferAutomaticUpdateNotice(arguments) {
			t.Errorf("shouldOfferAutomaticUpdateNotice(%q) = true", arguments)
		}
	}
	if !shouldOfferAutomaticUpdateNotice([]string{"mail", "list"}) {
		t.Fatal("human-facing mail command did not allow a quiet notice")
	}
}

func updateTestRuntime(t *testing.T, stdout *bytes.Buffer, result updatecheck.Result) *runtime {
	t.Helper()
	app := newRuntime(context.Background(), filepath.Join(t.TempDir(), "missing.toml"), stdout, &bytes.Buffer{}, buildinfo.Current())
	app.checkUpdate = func(context.Context) (updatecheck.Result, error) { return result, nil }
	app.installMethod = func() updatecheck.InstallMethod { return updatecheck.InstallHomebrew }
	return app
}
