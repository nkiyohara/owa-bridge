package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nkiyohara/owa-bridge/internal/buildinfo"
)

func TestRunShowsHelpWithoutArguments(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(context.Background(), nil, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Local-first Outlook Web") {
		t.Fatalf("help output did not contain description: %q", stdout.String())
	}
}

func TestRunShowsCommandGroupHelp(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{
		{"mcp", "--help"},
		{"mcp", "setup", "--help"},
		{"mail", "--help"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := run(context.Background(), arguments, &stdout, &stderr); code != 0 {
			t.Errorf("run(%q) code = %d, stderr = %q", arguments, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Usage:") {
			t.Errorf("run(%q) stdout = %q, want usage", arguments, stdout.String())
		}
	}
}

func TestRunInitializesAndValidatesConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	arguments := []string{"--config", path, "config", "init", "--json"}
	if code := run(context.Background(), arguments, &stdout, &stderr); code != 0 {
		t.Fatalf("config init code = %d, stderr = %q", code, stderr.String())
	}
	var initialized struct {
		Created bool   `json:"created"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &initialized); err != nil {
		t.Fatalf("decode config init output: %v", err)
	}
	if !initialized.Created || initialized.Path != path {
		t.Fatalf("unexpected config init output: %+v", initialized)
	}

	stdout.Reset()
	stderr.Reset()
	arguments = []string{"--config", path, "config", "validate", "--json"}
	if code := run(context.Background(), arguments, &stdout, &stderr); code != 0 {
		t.Fatalf("config validate code = %d, stderr = %q", code, stderr.String())
	}
	var validated struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &validated); err != nil {
		t.Fatalf("decode config validate output: %v", err)
	}
	if !validated.Valid {
		t.Fatalf("unexpected config validate output: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--config", path, "config", "init"}, &stdout, &stderr); code != 1 {
		t.Fatalf("second config init code = %d, want 1", code)
	}
}

func TestRunPrintsVersionAsJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"version", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}

	var info buildinfo.Info
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("decode version output: %v", err)
	}
	if info.Version == "" || info.Commit == "" || info.GoVersion == "" {
		t.Fatalf("incomplete build info: %+v", info)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unexpected argument") {
		t.Fatalf("stderr did not explain parse error: %q", stderr.String())
	}
}
