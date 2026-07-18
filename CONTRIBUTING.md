# Contributing

Design changes begin with an issue or architecture decision record. Protocol
changes require synthetic fixtures and contract tests; live mailbox tests are
never part of the default test suite.

## Principles

- Keep domain behavior independent of CLI, MCP, browser, and OWA packages.
- Add one typed use case and test it before exposing it through an adapter.
- Preserve interactive authentication and the preview/commit safety boundary.
- Do not commit credentials, browser profiles, live mailbox data, or captures.
- Use synthetic identities and content in every test and example.

## Developer setup

The repository uses [mise](https://mise.jdx.dev/) to pin Go and every developer
tool for macOS, Linux, and Windows.

```console
mise trust
mise install
mise exec -- task verify
```

`task verify` checks formatting, Markdown, static analysis, unit tests, the race
detector, repository history and working-tree secrets, known reachable
vulnerabilities, linked dependency licenses, the binary build, release
configuration, and GitHub Actions security. Build the local binary with
`mise exec -- task build`.

Go 1.26 is the minimum supported compiler and the checked-in toolchain follows
the latest Go 1.26 patch release. This matches the current Chromium/CDP stack
instead of silently downloading a newer compiler behind an older CI matrix.
