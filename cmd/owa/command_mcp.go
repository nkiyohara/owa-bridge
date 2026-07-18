package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pelletier/go-toml/v2"

	"github.com/nkiyohara/owa-bridge/internal/mcpserver"
)

type mcpCommand struct {
	Serve  mcpServeCommand  `cmd:"" help:"Run the local MCP server."`
	Config mcpConfigCommand `cmd:"" help:"Generate client configuration without changing client files."`
	Setup  mcpSetupCommand  `cmd:"" help:"Register the server through a client's official CLI."`
}

type mcpServeCommand struct {
	Stdio bool `default:"true" help:"Use newline-delimited JSON over stdin/stdout."`
}

func (command *mcpServeCommand) Run(app *runtime) (returnErr error) {
	if !command.Stdio {
		return errors.New("stdio is the only supported MCP transport")
	}
	backend, err := newDaemonMCPBackend(app)
	if err != nil {
		return err
	}
	defer func() { returnErr = errors.Join(returnErr, backend.Close()) }()
	server, err := mcpserver.New(backend, mcpserver.Options{
		Version:  app.info.Version,
		Instance: fmt.Sprintf("process-%d", app.processID),
	})
	if err != nil {
		return err
	}
	return server.Run(app.context, &mcp.StdioTransport{})
}

type mcpConfigCommand struct {
	Codex      mcpCodexConfigCommand      `cmd:"" help:"Print a Codex config.toml fragment."`
	ClaudeCode mcpClaudeCodeConfigCommand `cmd:"" name:"claude-code" help:"Print a Claude Code MCP JSON document."`
}

type mcpSetupCommand struct {
	Codex      mcpCodexSetupCommand      `cmd:"" help:"Register with Codex using codex mcp add."`
	ClaudeCode mcpClaudeCodeSetupCommand `cmd:"" name:"claude-code" help:"Register with Claude Code using claude mcp add."`
}

type mcpClientConfigFlags struct {
	Name       string `default:"owa" help:"Client-side MCP server name."`
	Executable string `type:"path" help:"owa executable path; defaults to this process."`
}

type mcpCodexConfigCommand struct{ mcpClientConfigFlags }
type mcpClaudeCodeConfigCommand struct{ mcpClientConfigFlags }

type mcpSetupFlags struct {
	mcpClientConfigFlags
	DryRun bool `help:"Print the official client command without running it."`
}

type mcpCodexSetupCommand struct{ mcpSetupFlags }

type mcpClaudeCodeSetupCommand struct {
	mcpSetupFlags
	Scope string `default:"user" enum:"local,project,user" help:"Claude Code configuration scope."`
}

type codexMCPDocument struct {
	Servers map[string]codexMCPServer `toml:"mcp_servers"`
}

type codexMCPServer struct {
	Command         string   `toml:"command"`
	Arguments       []string `toml:"args"`
	StartupTimeout  int      `toml:"startup_timeout_sec"`
	ToolTimeout     int      `toml:"tool_timeout_sec"`
	DefaultApproval string   `toml:"default_tools_approval_mode"`
	Enabled         bool     `toml:"enabled"`
	Required        bool     `toml:"required"`
}

type claudeMCPDocument struct {
	Servers map[string]claudeMCPServer `json:"mcpServers"`
}

type claudeMCPServer struct {
	Type      string            `json:"type"`
	Command   string            `json:"command"`
	Arguments []string          `json:"args"`
	Env       map[string]string `json:"env"`
}

var mcpClientNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func (command *mcpCodexConfigCommand) Run(app *runtime) error {
	name, executable, arguments, err := resolveMCPClientConfig(app, command.Name, command.Executable)
	if err != nil {
		return err
	}
	document := codexMCPDocument{Servers: map[string]codexMCPServer{
		name: {
			Command:         executable,
			Arguments:       arguments,
			StartupTimeout:  30,
			ToolTimeout:     360,
			DefaultApproval: "writes",
			Enabled:         true,
			Required:        false,
		},
	}}
	encoded, err := toml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode Codex MCP config: %w", err)
	}
	_, err = app.stdout.Write(encoded)
	return err
}

func (command *mcpClaudeCodeConfigCommand) Run(app *runtime) error {
	name, executable, arguments, err := resolveMCPClientConfig(app, command.Name, command.Executable)
	if err != nil {
		return err
	}
	document := claudeMCPDocument{Servers: map[string]claudeMCPServer{
		name: {
			Type:      "stdio",
			Command:   executable,
			Arguments: arguments,
			Env:       map[string]string{},
		},
	}}
	encoder := json.NewEncoder(app.stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}

func (command *mcpCodexSetupCommand) Run(app *runtime) error {
	name, executable, arguments, err := resolveMCPSetup(app, command.Name, command.Executable)
	if err != nil {
		return err
	}
	clientArguments := make([]string, 0, 5+len(arguments))
	clientArguments = append(clientArguments, "mcp", "add", name, "--", executable)
	clientArguments = append(clientArguments, arguments...)
	return applyMCPSetup(app, "Codex", "codex", name, clientArguments, command.DryRun)
}

func (command *mcpClaudeCodeSetupCommand) Run(app *runtime) error {
	name, executable, arguments, err := resolveMCPSetup(app, command.Name, command.Executable)
	if err != nil {
		return err
	}
	clientArguments := make([]string, 0, 7+len(arguments))
	clientArguments = append(clientArguments, "mcp", "add", "--scope", command.Scope, name, "--", executable)
	clientArguments = append(clientArguments, arguments...)
	return applyMCPSetup(app, "Claude Code", "claude", name, clientArguments, command.DryRun)
}

func resolveMCPSetup(app *runtime, name, executable string) (string, string, []string, error) {
	if _, _, err := app.loadConfig(); err != nil {
		return "", "", nil, fmt.Errorf("load owa configuration before MCP setup (run `owa config init` first): %w", err)
	}
	name, executable, arguments, err := resolveMCPClientConfig(app, name, executable)
	if err != nil {
		return "", "", nil, err
	}
	info, err := os.Stat(executable)
	if err != nil {
		return "", "", nil, fmt.Errorf("inspect owa executable %s: %w", executable, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", nil, fmt.Errorf("owa executable is not a regular file: %s", executable)
	}
	return name, executable, arguments, nil
}

func applyMCPSetup(
	app *runtime,
	clientLabel, clientCommand, name string,
	arguments []string,
	dryRun bool,
) error {
	if dryRun {
		_, err := fmt.Fprintf(app.stdout, "%s\n", formatCommand(clientCommand, arguments))
		return err
	}
	if err := app.runCommand(app.context, app.stdout, app.stderr, clientCommand, arguments...); err != nil {
		return fmt.Errorf("register MCP server with %s: %w", clientLabel, err)
	}
	_, err := fmt.Fprintf(
		app.stdout,
		"Registered MCP server %q with %s; verify it with `%s mcp get %s`.\n",
		name,
		clientLabel,
		clientCommand,
		name,
	)
	return err
}

func formatCommand(name string, arguments []string) string {
	parts := make([]string, 0, len(arguments)+1)
	parts = append(parts, quoteCommandArgument(name))
	for _, argument := range arguments {
		parts = append(parts, quoteCommandArgument(argument))
	}
	return strings.Join(parts, " ")
}

func quoteCommandArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n\\\"'") {
		return value
	}
	return strconv.Quote(value)
}

func resolveMCPClientConfig(app *runtime, name, executable string) (string, string, []string, error) {
	if !mcpClientNamePattern.MatchString(name) {
		return "", "", nil, errors.New("MCP client name must contain only letters, numbers, underscores, and hyphens")
	}
	if executable == "" {
		resolved, err := os.Executable()
		if err != nil {
			return "", "", nil, fmt.Errorf("resolve owa executable: %w", err)
		}
		executable = resolved
	}
	absoluteExecutable, err := filepath.Abs(executable)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolve owa executable: %w", err)
	}
	configPath, err := app.resolvedConfigPath()
	if err != nil {
		return "", "", nil, err
	}
	arguments := []string{"--config", configPath, "mcp", "serve"}
	return name, filepath.Clean(absoluteExecutable), arguments, nil
}
