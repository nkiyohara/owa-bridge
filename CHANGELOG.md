# Changelog

All notable user-facing changes are recorded here. The project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.2.0 - 2026-07-19

### Authentication

- Add experimental `owa login --terminal` authentication for interactive SSH
  sessions without a display server.
- Project a bounded text view and numbered controls from a dedicated headless
  Chromium profile over caller-bound, authenticated local IPC.
- Relay one key at a time without accepting piped input or returning complete
  form values; mask sensitive browser fields and support refresh, back, and
  cancellation controls.
- Advance the local IPC protocol to version 8 for terminal-login requests,
  events, input, and cancellation.

### Compatibility evidence

- Observe headless Google Chrome reaching the Microsoft sign-in page on Linux
  amd64, rendering its text and controls, focusing the email field, returning
  with Escape, and cancelling cleanly.
- Keep full authentication, MFA, Conditional Access, and session capture marked
  unobserved because no credentials or MFA values were entered during the live
  check.

### Terminal authentication limits

- CAPTCHA, passkeys, security keys, client certificates, native dialogs, and
  custom graphical authentication may still require a visible browser.

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
