# Changelog

All notable user-facing changes are recorded here. The project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.3.1 - 2026-07-20

### Package catalogs

- Add a source-building Homebrew Formula, a Scoop bucket manifest, and WinGet
  manifests bound to the verified release checksum inventory.
- Preserve catalog metadata separately from release publication so catalog
  updates cannot rebuild or replace a published artifact.
- Add a tagged source archive for Homebrew builds and document package-manager
  installation without asking users to bypass macOS Gatekeeper.

## 0.3.0 - 2026-07-20

### Mail composition and attachments

- Add reviewed reply, reply-all, and forward composition for drafts and sends,
  with exact source message IDs and change keys.
- Add text or HTML bodies plus bounded file attachments whose sizes and SHA-256
  digests are visible in the review.
- Return bounded attachment metadata with body reads and retrieve one explicit
  file attachment through a separate sensitive-read tool.
- Add mandatory destructive preview and commit for one exact message hard
  delete.

### Calendar fields

- Add all-day creation, explicit Exchange/Windows time-zone IDs, reminders,
  and bounded daily, weekly, absolute-monthly, and absolute-yearly recurrence.
- Add versioned updates for all-day status, reminders, and complete required/
  optional attendee-list replacement.

### Mailbox routing and public documentation

- Add explicit shared/delegated mailbox aliases for mailboxes the interactive
  Outlook Web user is already authorized to access.
- Advance local IPC to protocol version 10 and expand the MCP surface to 24
  typed tools.
- Clarify on GitHub Pages and in the README that owa-bridge is a local Outlook
  MCP that does not require a Microsoft Graph app registration or hosted relay.

### Compatibility limits

- Keep new OWA contracts marked deterministic-only until a separately
  authorized live observation is recorded.
- Continue to omit Inbox-rule mutation because Exchange warns that updating
  rules can remove client-only rules, as well as recurrence editing,
  delegate-permission management, and generic property mutation.

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
