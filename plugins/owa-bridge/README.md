# OWA Bridge agent plugin

This plugin teaches compatible coding agents when and how to use the local
`owa-bridge` Outlook mail and calendar MCP server. It contains no credentials,
mailbox data, or bundled authentication.

Register the MCP server first:

```console
owa mcp setup codex
# or: owa mcp setup claude-code
# or: owa mcp setup github-copilot
# or: owa mcp setup gemini-cli
# or: owa mcp setup qwen-code
# or: owa mcp setup qoder
```

Claude Code can install the shared Skill from this repository:

```console
claude plugin marketplace add nkiyohara/owa-bridge
claude plugin install owa-bridge@owa-bridge
```

Start a new agent session, then simply ask:

```text
Check Outlook and summarize the messages that need my attention.
```

See the project [MCP guide](../../docs/mcp.md) for Codex, Claude Code, GitHub
Copilot CLI, Gemini CLI, Qwen Code, Qoder, Kimi Code CLI, manual
configuration, migration, and troubleshooting.
