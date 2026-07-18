package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/nkiyohara/owa-bridge/internal/buildinfo"
	"github.com/nkiyohara/owa-bridge/internal/config"
)

func TestMCPConfigGeneratorsProduceNativeClientDocuments(t *testing.T) {
	t.Parallel()

	executable := filepath.Join(t.TempDir(), "owa")
	configPath := filepath.Join(t.TempDir(), "config.toml")
	tests := []struct {
		name      string
		arguments []string
		decode    func(*testing.T, []byte)
	}{
		{
			name: "Codex TOML",
			arguments: []string{
				"--config", configPath, "mcp", "config", "codex",
				"--name", "outlook_work", "--executable", executable,
			},
			decode: func(t *testing.T, data []byte) {
				t.Helper()
				var document codexMCPDocument
				if err := toml.Unmarshal(data, &document); err != nil {
					t.Fatalf("decode Codex document: %v", err)
				}
				server := document.Servers["outlook_work"]
				if server.Command != executable || server.DefaultApproval != "writes" || server.ToolTimeout != 360 {
					t.Fatalf("unexpected Codex server: %+v", server)
				}
				if len(server.Arguments) != 4 || server.Arguments[1] != configPath {
					t.Fatalf("unexpected Codex arguments: %#v", server.Arguments)
				}
			},
		},
		{
			name: "Claude Code JSON",
			arguments: []string{
				"--config", configPath, "mcp", "config", "claude-code",
				"--name", "outlook_work", "--executable", executable,
			},
			decode: func(t *testing.T, data []byte) {
				t.Helper()
				var document claudeMCPDocument
				if err := json.Unmarshal(data, &document); err != nil {
					t.Fatalf("decode Claude Code document: %v", err)
				}
				server := document.Servers["outlook_work"]
				if server.Type != "stdio" || server.Command != executable {
					t.Fatalf("unexpected Claude Code server: %+v", server)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := run(context.Background(), test.arguments, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
			}
			test.decode(t, stdout.Bytes())
		})
	}
}

func TestMCPConfigRejectsUnsafeClientName(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newRuntime(t.Context(), "", &stdout, &stderr, buildinfo.Current())
	command := mcpCodexConfigCommand{mcpClientConfigFlags: mcpClientConfigFlags{Name: "bad.name"}}
	if err := command.Run(app); err == nil {
		t.Fatal("Run() unexpectedly accepted unsafe MCP client name")
	}
}

func TestMCPSetupUsesOfficialClientCommands(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(configPath, config.Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	executable := filepath.Join(t.TempDir(), "owa")
	if err := os.WriteFile(executable, []byte("test executable"), 0o600); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	tests := []struct {
		name        string
		client      string
		wantArgs    []string
		run         func(*runtime) error
		wantMessage string
	}{
		{
			name:   "Codex",
			client: "codex",
			wantArgs: []string{
				"mcp", "add", "outlook_work", "--", executable,
				"--config", configPath, "mcp", "serve",
			},
			run: func(app *runtime) error {
				command := mcpCodexSetupCommand{mcpSetupFlags: mcpSetupFlags{
					mcpClientConfigFlags: mcpClientConfigFlags{Name: "outlook_work", Executable: executable},
				}}
				return command.Run(app)
			},
			wantMessage: "Registered MCP server \"outlook_work\" with Codex",
		},
		{
			name:   "Claude Code",
			client: "claude",
			wantArgs: []string{
				"mcp", "add", "--scope", "project", "outlook_work", "--", executable,
				"--config", configPath, "mcp", "serve",
			},
			run: func(app *runtime) error {
				command := mcpClaudeCodeSetupCommand{
					mcpSetupFlags: mcpSetupFlags{mcpClientConfigFlags: mcpClientConfigFlags{
						Name: "outlook_work", Executable: executable,
					}},
					Scope: "project",
				}
				return command.Run(app)
			},
			wantMessage: "Registered MCP server \"outlook_work\" with Claude Code",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			app := newRuntime(t.Context(), configPath, &stdout, &stderr, buildinfo.Current())
			var gotClient string
			var gotArgs []string
			app.runCommand = func(
				_ context.Context,
				_, _ io.Writer,
				name string,
				arguments ...string,
			) error {
				gotClient = name
				gotArgs = append([]string(nil), arguments...)
				return nil
			}
			if err := test.run(app); err != nil {
				t.Fatalf("setup Run() error = %v", err)
			}
			if gotClient != test.client || !reflect.DeepEqual(gotArgs, test.wantArgs) {
				t.Fatalf("client invocation = %q %#v, want %q %#v", gotClient, gotArgs, test.client, test.wantArgs)
			}
			if !strings.Contains(stdout.String(), test.wantMessage) {
				t.Fatalf("stdout = %q, want message %q", stdout.String(), test.wantMessage)
			}
		})
	}
}

func TestMCPSetupDryRunDoesNotInvokeClient(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(configPath, config.Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	executable := filepath.Join(t.TempDir(), "owa binary")
	if err := os.WriteFile(executable, []byte("test executable"), 0o600); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(t.Context(), []string{
		"--config", configPath,
		"mcp", "setup", "codex",
		"--name", "owa",
		"--executable", executable,
		"--dry-run",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "codex mcp add owa -- \"") || !strings.Contains(got, "mcp serve") {
		t.Fatalf("dry-run output = %q", got)
	}
}

func TestMCPSetupRequiresInitializedConfiguration(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newRuntime(t.Context(), filepath.Join(t.TempDir(), "missing.toml"), &stdout, &stderr, buildinfo.Current())
	command := mcpCodexSetupCommand{mcpSetupFlags: mcpSetupFlags{
		mcpClientConfigFlags: mcpClientConfigFlags{Name: "owa", Executable: os.Args[0]},
	}}
	if err := command.Run(app); err == nil || !strings.Contains(err.Error(), "owa config init") {
		t.Fatalf("Run() error = %v, want config initialization guidance", err)
	}
}
