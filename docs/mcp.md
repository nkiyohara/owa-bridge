# MCP integration

`owa-bridge` exposes the same application service used by the CLI over a local
MCP stdio server. The adapter cannot call the OWA transport directly, so MCP
calls retain the deterministic policy and content-free audit boundary.

## Start the server

```console
owa mcp serve
```

The process writes only newline-delimited MCP JSON to stdout. It connects over
authenticated local IPC to the config-scoped session owner, starting it when
absent. The first account operation opens that account's dedicated Outlook Web
browser profile; later calls and later MCP stdio processes reuse the daemon's
in-memory session. Outlook authorization never enters the MCP process.

The server exposes 24 tools. It uses the signed-in Outlook Web session directly;
no Microsoft Graph app registration or OAuth application permission enters the
MCP process.

- `mail_list_folders`: read-only, bounded folder discovery with opaque IDs and
  aggregate counts; folder names are private untrusted data.
- `mail_list`: read-only message metadata for a folder, with stable structured
  output shared with `owa mail list --json`.
- `mail_search`: read-only, folder-scoped Outlook AQS search returning the same
  metadata-only schema. Queries are private input, bounded to 1024 UTF-8 bytes,
  and are represented only by an operation digest in content-free audit data.
- `mail_move`: moves one exact message ID and change key to one destination
  discovered under the selected account. It either completes under policy or
  returns an immutable, caller-bound preview; it never retries after submission.
- `mail_move_commit`: consumes only the matching move preview. A token for a
  body read, draft, or send is rejected without consuming it.
- `mail_set_read_state`: sets only `read` or `unread` for one exact message ID
  and change key. There is no generic update-property surface.
- `mail_set_read_state_commit`: consumes only the matching caller-bound state
  preview when reversible-write policy requests one.
- `mail_delete`: prepares an irreversible hard-delete preview for one exact
  message ID and change key. It never deletes directly.
- `mail_delete_commit`: consumes only the matching destructive preview and
  attempts the hard delete once.
- `calendar_list`: read-only event metadata for a required, bounded RFC3339
  time window, shared with `owa calendar list --json`.
- `calendar_create`: prepares one exact bounded event, including optional
  all-day, Exchange time-zone, reminder, supported recurrence, attendees, and
  closed `teamsMeeting` settings, and returns a caller-bound mandatory preview.
  It never creates an event or sends an invitation directly.
- `calendar_create_commit`: consumes only the matching preview, creates the
  immutable event once, and sends invitations when the reviewed event contains
  attendees. A requested Teams meeting returns the single created event's join
  URL when Outlook provisions it. It never retries an ambiguous submission.
- `calendar_update`: prepares a stale-safe patch for one exact event ID and
  change key. Only subject, plain-text body, start plus end and time zone,
  location, all-day status, reminder, and complete attendee-list replacement
  are accepted; an empty provided string clears a text field.
- `calendar_update_commit`: consumes only the matching update preview, uses the
  exact change key and OWA's default event scope, and leaves attendee
  notification behavior to Outlook.
- `calendar_cancel`: prepares a destructive cancellation preview for one exact
  event version. It does not change Outlook itself.
- `calendar_cancel_commit`: consumes only the matching cancellation preview,
  moves the event to Deleted Items, and asks Outlook to notify all attendees.
  This is the only calendar tool annotated as destructive.
- `mail_get_body`: bounded plain text and attachment metadata for one exact
  message ID. It returns content immediately under the default policy or an
  in-memory approval preview when sensitive-read previews are enabled.
- `mail_get_body_commit`: consumes that short-lived, caller-bound preview.
- `mail_get_attachment`: retrieves one exact file-attachment ID, limited to
  2 MiB, as private untrusted base64 content.
- `mail_get_attachment_commit`: consumes the matching sensitive-read preview.
- `mail_create_draft`: creates one text or HTML new-message, reply, reply-all,
  or forward draft with OWA `SaveOnly`, optionally with bounded file
  attachments; it never sends mail. Policy may instead return a bounded review
  and approval token.
- `mail_create_draft_commit`: consumes the matching token and saves the exact
  immutable draft that was reviewed. A token issued for another operation
  class is rejected without consuming it.
- `mail_send`: prepares a text or HTML new message, reply, reply-all, or
  forward, optionally with bounded file attachments, and returns its normalized
  review plus a caller-bound approval token. It never sends directly.
- `mail_send_commit`: consumes only a matching `mail.send` token and submits
  the immutable message once. It never retries an ambiguous send.

Approval tokens are secret capabilities. Do not log or persist them. They expire
after two minutes by default, are usable once, and are stored only inside the
daemon that issued them. The caller binding includes the originating MCP process
instance, so restarting MCP cannot claim an earlier preview.

Mail and returned calendar fields are private, untrusted external data. Tool
annotations communicate that risk to clients, but they are not the security
boundary. Enforcement lives in the shared application Guard. Move, read-state,
draft, send, and calendar mutation tools are non-read-only open-world
operations. Calendar cancellation and message hard-delete commits are
destructive; their preview tools are not.
Calendar create/update and mail send remain external writes even though their
first tools only prepare reviews. If a write returns an unknown outcome,
inspect Outlook state before another action.

## Codex

Register through the supported Codex CLI without directly editing
`config.toml`:

```console
owa mcp setup codex
codex mcp get owa
```

`owa mcp setup codex --dry-run` prints the exact `codex mcp add` invocation
instead. Codex CLI, the Codex IDE extension, and the ChatGPT desktop app share
the same local Codex MCP configuration.

Generate a native `config.toml` fragment when the extended startup timeout,
tool timeout, and write-aware client approval mode are desired:

```console
owa mcp config codex
```

Copy the fragment into the user or trusted-project Codex `config.toml`. The
generated entry uses read/write-aware client approval mode and extends the tool
timeout so a first interactive browser sign-in can complete.

The equivalent manual registration command is:

```console
codex mcp add owa -- /absolute/path/to/owa \
  --config /absolute/path/to/config.toml mcp serve
```

## Claude Code

Register for the current user through the supported Claude Code CLI:

```console
owa mcp setup claude-code
claude mcp get owa
```

Use `--scope local`, `--scope project`, or `--scope user` to select Claude
Code's configuration scope. Project scope writes the standard `.mcp.json`
through Claude Code itself.

Generate a complete MCP JSON document:

```console
owa mcp config claude-code
```

It can be passed to Claude Code with `--mcp-config`, or its `owa` entry can be
merged into an existing `.mcp.json`. To register it directly for the current
user instead:

```console
claude mcp add owa --scope user -- /absolute/path/to/owa \
  --config /absolute/path/to/config.toml mcp serve
```

The generators only print configuration. Setup delegates to the installed
client's official command and never parses or serializes unrelated Codex or
Claude Code settings.
