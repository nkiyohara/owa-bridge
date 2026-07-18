# Changelog

All notable user-facing changes are recorded here. The project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.1.0 - 2026-07-18

Initial public release.

### Mail

- Discover Outlook folders and list or AQS-search bounded message metadata.
- Read one explicit plain-text body through configurable sensitive-read review.
- Save plain-text drafts without sending and send new messages only after an
  exact, caller-bound preview.
- Move one exact message version and set its read or unread state.

### Calendar

- List bounded calendar windows without bodies, attendees, or join URLs.
- Create reviewed appointments and meetings with required and optional
  attendees.
- Ask Outlook to provision a Microsoft Teams join link at event creation.
- Update supported fields or cancel one exact event version with mandatory
  preview and commit.

### Runtime and distribution

- Share the same typed application use cases across the CLI and twenty MCP
  tools for Codex and Claude Code.
- Keep interactive Outlook authentication in a dedicated browser-owned session
  behind authenticated local IPC.
- Ship macOS, Linux, and Windows archives, Linux native packages, SHA-256
  checksums, SPDX and CycloneDX SBOMs, and a Sigstore checksum bundle.
- Deploy concise documentation through GitHub Pages.

### Known limits

- Outlook Web actions are undocumented and can drift between deployments.
- Binaries are not Apple-notarized or Windows Authenticode-signed.
- Reply, forward, HTML composition, attachments, permanent deletion,
  recurrence editing, attendee replacement, and general Teams access are not
  implemented.
