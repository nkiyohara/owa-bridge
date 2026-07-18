# Repository instructions

## Scope

Build a local-first Outlook Web CLI and MCP server. Outlook mail and calendar
are in scope, including Teams join links provisioned as an Outlook calendar
event property. Teams chat, channels, calls, recordings, and meeting lifecycle
management are out of scope, as are Graph, hosted relays, unattended credential
login, and tenant-wide access.

## Architecture invariants

- Dependencies point inward: adapters and transports may depend on application
  ports; the domain must not import them.
- CLI and MCP must call the same typed application use cases.
- Authentication is interactive and browser-owned. Never request or persist a
  password and never introduce TLS interception.
- Consequential writes use the server-enforced preview/commit protocol.
- MCP annotations describe effects but never replace core policy checks.
- Live mailbox tests are opt-in and cannot run in the default test command or CI.
- Fixtures are synthetic and contain no credentials or personal data.

## Working agreement

- Keep commits narrow and use Conventional Commit messages.
- Update an ADR when changing an accepted architectural decision.
- Run `mise exec -- task verify` before committing.
- Never weaken a security invariant to make a test pass.
