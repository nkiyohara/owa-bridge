# ADR 0005: Treat calendar-hosted Teams links as calendar scope

- Status: accepted
- Date: 2026-07-18

## Context

An Outlook event can provision an online meeting and return its Teams join URL.
This is useful to both CLI users and MCP agents, but describing it as general
Teams integration would blur the product boundary and imply unrelated chat,
channel, call, recording, or meeting-management capabilities.

Join URLs are also more sensitive than ordinary calendar-list metadata. Bulk
calendar reads must not expose every meeting link merely because one event can
be created as an online meeting.

## Decision

Calendar creation accepts one closed `teamsMeeting` boolean. A true value is
shown in the mandatory preview and maps only to OWA's `IsOnlineMeeting` flag and
the closed `TeamsForBusiness` provider through the specialized
`CreateCalendarEvent` action. The commit result may return the join URL for the
single event it just created.

Calendar list remains metadata-only and excludes join URLs. The bridge does not
expose a provider string, arbitrary online-meeting fields, Graph, or any Teams
chat, channel, call, recording, or post-creation meeting-management operation.

## Consequences

Humans and agents get a simple preview/commit flow and can use the returned URL
without broadening calendar reads. The bridge depends on the signed-in user's
calendar supporting Teams and reports the OWA result rather than silently
falling back to another provider. Wider Teams features require a separate ADR
and must not be inferred from this calendar capability.
