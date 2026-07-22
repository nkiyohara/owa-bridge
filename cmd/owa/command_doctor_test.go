package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nkiyohara/owa-bridge/internal/buildinfo"
	"github.com/nkiyohara/owa-bridge/internal/config"
	"github.com/nkiyohara/owa-bridge/internal/localipc"
)

func TestDoctorOfflineProducesContentFreeHealthyReport(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	executable := filepath.Join(root, "chromium")
	// #nosec G306 -- the owner-only test fixture must be executable.
	if err := os.WriteFile(executable, []byte("synthetic executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	configuration := config.Default()
	configuration.Browser.Executable = executable
	configPath := filepath.Join(root, "config.toml")
	if err := config.Save(configPath, configuration); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	app := newRuntime(context.Background(), configPath, &stdout, &bytes.Buffer{}, buildinfo.Current())
	app.endpoint = func(path string) (localipc.Endpoint, error) {
		return localipc.ResolveInState(path, filepath.Join(root, "state"))
	}
	command := doctorCommand{JSON: true}
	if err := command.Run(app); err != nil {
		t.Fatalf("doctor.Run() error = %v", err)
	}

	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor output: %v", err)
	}
	if !report.Healthy || report.Online || report.Account != "work" {
		t.Fatalf("unexpected doctor report: %+v", report)
	}
	if len(report.Checks) != 6 || report.Checks[2].Name != "update" || report.Checks[5].Status != "skip" {
		t.Fatalf("unexpected doctor checks: %+v", report.Checks)
	}
}

func TestDoctorReportsInvalidConfigBeforeOnlineWork(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	app := newRuntime(
		context.Background(),
		filepath.Join(t.TempDir(), "missing.toml"),
		&stdout,
		&bytes.Buffer{},
		buildinfo.Current(),
	)
	command := doctorCommand{Online: true, JSON: true}
	if err := command.Run(app); err == nil {
		t.Fatal("doctor.Run() unexpectedly accepted a missing config")
	}
	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor output: %v", err)
	}
	if report.Healthy || len(report.Checks) != 1 || report.Checks[0].Name != "config" {
		t.Fatalf("unexpected doctor failure report: %+v", report)
	}
}
