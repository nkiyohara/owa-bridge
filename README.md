# owa-bridge

Local-first Outlook Web mail and calendar for humans, scripts, and AI agents.

`owa-bridge` is a cross-platform CLI, local session owner, and MCP server that
works through the interactive Outlook Web session you already use. It is built
for environments where Microsoft Graph application access is unavailable.

[Website](https://nkiyohara.github.io/owa-bridge/) ·
[Latest release](https://github.com/nkiyohara/owa-bridge/releases/latest) ·
[Install](docs/install.md) · [Feature matrix](docs/features.md) ·
[JSON contract](docs/json.md)

> [!WARNING]
> `owa-bridge` 0.2 is an early release over undocumented Outlook Web contracts.
> Use only an account you are authorized to access, review every write, and
> keep Outlook available for reconciliation after an unknown outcome.

## Why this exists

Outlook automation usually starts with Microsoft Graph. That remains the right
choice when an organization permits app registration and consent. Some do not.
`owa-bridge` serves that narrower case without asking a user to defeat MFA,
Conditional Access, or another sign-in control.

The user signs in interactively in a dedicated browser profile. A local daemon
owns that session and exposes one typed application core to two adapters:

```text
Claude Code / Codex ── MCP stdio ─┐
                                  ├── local IPC ── session owner ── OWA
Humans / scripts ───── CLI ────────┘                  │
                                             application / policy
```

## Design promises

- **Local first.** Mailbox data and authorization are not sent to a
  project-run cloud service.
- **Interactive authentication.** The browser completes normal SSO, MFA, and
  Conditional Access. The CLI never requests a password.
- **One core, two interfaces.** CLI and MCP execute the same typed use cases
  and return the same stable result shapes.
- **Safe by construction.** Reads are metadata-first. Sending and calendar
  changes use exact, caller-bound `preview -> commit` operations.
- **No automatic write retry.** Ambiguous outcomes fail closed to avoid
  duplicate mail, events, or invitations.
- **No ambient network listener.** MCP uses stdio; daemon IPC uses a Unix
  socket or Windows named pipe.

See the [architecture](docs/architecture.md),
[authentication design](docs/authentication.md),
[protocol boundary](docs/protocol.md), [threat model](docs/threat-model.md), and
[compatibility evidence](docs/compatibility.md).

## Quick start

Download the archive for your operating system from the
[latest release](https://github.com/nkiyohara/owa-bridge/releases/latest), then
verify `checksums.txt` before running it. Full platform instructions and
Sigstore verification are in [docs/install.md](docs/install.md).

```console
owa config init
# Set only the final HTTPS Outlook origin used after interactive sign-in.
owa config validate
owa login                    # visible browser
owa login --terminal         # experimental text-only SSH relay
owa doctor --online
```

Metadata-first reads:

```console
owa mail folders
owa mail list --limit 25
owa mail search --query 'subject:"Quarterly plan" from:reader'
owa calendar list \
  --start 2026-07-20T00:00:00Z \
  --end 2026-07-21T00:00:00Z
```

Reviewed writes:

```console
printf 'Synthetic draft body.\n' | \
  owa mail draft \
    --to reader@example.invalid \
    --subject 'Draft example' \
    --body-file -

printf 'Synthetic agenda.\n' | \
  owa calendar create \
    --subject 'Design review' \
    --start 2026-07-20T09:00:00Z \
    --end 2026-07-20T10:00:00Z \
    --required-attendee reader@example.invalid \
    --teams-meeting \
    --body-file -
```

The first call shows the exact normalized review and changes nothing when
approval is required. Repeat the same CLI command with `--approve` only after
checking every recipient and field. MCP keeps preview and commit as separate
tools and binds the token to the originating process.

## AI agents

Register through each client's supported CLI:

```console
owa mcp setup codex
codex mcp get owa

owa mcp setup claude-code
claude mcp get owa
```

The MCP server exposes twenty narrow mail and calendar tools. Descriptions and
annotations identify private untrusted content, side effects, and destructive
commits; application policy still enforces every operation. Read the
[MCP guide](docs/mcp.md) and [JSON contract](docs/json.md) for the complete
agent-facing surface.

## Current scope

Mail supports folder discovery, metadata list and AQS search, one explicit
plain-text body read, save-only drafts, new-message sending, versioned moves,
and read/unread updates.

Calendar supports bounded metadata list, non-recurring event creation with
required and optional attendees, optional Teams join-link provisioning,
versioned subject/body/time/location updates, and cancellation to Deleted
Items.

Reply, forward, HTML composition, attachments, permanent deletion, recurrence
editing, reminders, attendee replacement, and generic property mutation are
not implemented. General Teams chat, channels, calls, recordings, and meeting
lifecycle management are out of scope, as are Microsoft Graph, hosted relays,
unattended login, and tenant-wide access.

## Distribution

Releases contain macOS, Linux, and Windows binaries for amd64 and arm64. Linux
also receives deb, RPM, and APK packages. Every archive and package is covered
by SHA-256 checksums and SPDX and CycloneDX SBOMs; the checksum manifest carries
a keyless Sigstore bundle.

The binaries are not yet Apple-notarized or Windows Authenticode-signed. Do not
weaken operating-system security controls merely to run a download; verify the
release or build from the reviewed source when local policy requires signing.

## Development

The complete toolchain is checksummed and pinned with `mise`:

```console
mise trust
mise install
mise exec -- task verify
mise exec -- task release:snapshot
```

Live mailbox tests are opt-in, never run in the default test command or CI, and
must use authorized data. See [CONTRIBUTING.md](CONTRIBUTING.md) and the
[manual test checklist](docs/manual-test-checklist.md).

## Responsible use

Use `owa-bridge` only with mailboxes you are authorized to access and according
to your organization's policies. The project does not bypass authentication or
grant permissions the signed-in user does not already have in Outlook Web.

`owa-bridge` is independent and is not affiliated with, endorsed by, or
sponsored by Microsoft. Microsoft, Outlook, Microsoft 365, and Teams are
trademarks of the Microsoft group of companies.

## License

Apache-2.0. See [LICENSE](LICENSE).
