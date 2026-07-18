# Feature and evidence matrix

The CLI and MCP server call the same typed application use cases. This matrix
is the public contract for the current release: a missing row is not available
through an arbitrary protocol escape hatch.

## Mail

<!-- markdownlint-disable MD013 -->

| Capability | CLI | MCP | Stable result | Live observation |
| --- | --- | --- | --- | --- |
| Discover folders | `owa mail folders` | `mail_list_folders` | `MailFolderPage` | Observed |
| List metadata | `owa mail list` | `mail_list` | `MailPage` | Observed |
| Search metadata with AQS | `owa mail search` | `mail_search` | `MailPage` | Observed |
| Read one plain-text body | `owa mail body` | `mail_get_body` + commit when required | `MailBodyAccess` | Observed |
| Save a plain-text draft | `owa mail draft` | `mail_create_draft` + commit when required | `MailDraftAccess` | Observed |
| Send a new plain-text message | `owa mail send` + `--approve` | `mail_send` + `mail_send_commit` | `MailSendAccess` | Observed, self-recipient only |
| Move one message version | `owa mail move` | `mail_move` + commit when required | `MailMoveAccess` | Observed, including restore |
| Set read or unread | `owa mail mark` | `mail_set_read_state` + commit when required | `MailReadStateAccess` | Observed, including restore |

<!-- markdownlint-enable MD013 -->

Metadata listing excludes bodies and attachment content. Draft creation uses
`SaveOnly` and never sends. A new send always requires a separate exact commit
and is attempted once; a successful Outlook response may omit a Sent Items
identity, so `sent.id` and `sent.changeKey` are optional.

## Calendar

<!-- markdownlint-disable MD013 -->

| Capability | CLI | MCP | Stable result | Live observation |
| --- | --- | --- | --- | --- |
| List bounded event metadata | `owa calendar list` | `calendar_list` | `CalendarPage` | Observed |
| Create an appointment or meeting | `owa calendar create` + `--approve` | `calendar_create` + `calendar_create_commit` | `CalendarCreateAccess` | Observed |
| Add a Teams join link at creation | `--teams-meeting` | `teamsMeeting: true` | `created.onlineMeetingJoinUrl` | Observed, self-attendee only |
| Update supported fields | `owa calendar update` + `--approve` | `calendar_update` + `calendar_update_commit` | `CalendarUpdateAccess` | Observed |
| Cancel one event version | `owa calendar cancel` + `--approve` | `calendar_cancel` + `calendar_cancel_commit` | `CalendarCancelAccess` | Observed |

<!-- markdownlint-enable MD013 -->

Creation supports subject, plain-text body, start, end, location, required and
optional attendees, and one closed Teams option. Update is deliberately limited
to subject, plain-text body, start plus end, and location. Cancellation moves
the selected event version to Deleted Items and asks Outlook to notify meeting
attendees.

Outlook's calendar-view response may report `isOnlineMeeting: false` even for
an event whose creation response returned a Teams link. The authoritative
provisioning result is `calendar_create_commit.created`; bounded calendar lists
never return join URLs.

## Agent ergonomics and safety

- Every consequential operation has a review tool or CLI preview. Commit tools
  accept only a short-lived token bound to the originating process and exact
  operation digest.
- Tool names are narrow verbs, schemas reject unknown fields, and updates do
  not accept arbitrary property names or OWA actions.
- IDs and change keys are returned together wherever Outlook supplies them so
  an agent can perform stale-safe follow-up actions.
- Mail and calendar output is labelled private, untrusted external content.
  Message bodies and Teams join URLs carry the sensitive classification.
- Unknown write outcomes fail closed and are never retried automatically.

See the [JSON contract](json.md) for field-level output shapes and
[compatibility evidence](compatibility.md) for the limits of the live
observations.

## Intentionally not implemented

The current release does not expose reply, forward, HTML composition,
attachments, permanent deletion, mailbox rules, delegated access, recurrence
or all-day creation, reminders, attendee replacement, or a generic property
update API. Teams chat, channels, calls, recordings, and meeting lifecycle
management are outside the project boundary. Microsoft Graph, hosted relays,
unattended login, and tenant-wide access are also out of scope.

These omissions are deliberate. They avoid weak approximations and keep the
review surface small; they are not coverage gaps to fill without a concrete use
case and a typed safety contract.
