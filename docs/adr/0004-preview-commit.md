# ADR 0004: Preview and commit all consequential writes

- Status: accepted
- Date: 2026-07-17

## Context

Model hosts can misunderstand user intent, mailbox content can contain prompt
injection, and protocol timeouts can make write outcomes ambiguous. MCP tool
annotations communicate risk but cannot enforce application policy.

## Decision

Sending, deleting, inviting, responding, and cancelling are two-stage use
cases. Preview normalizes the operation and returns a short-lived, single-use
token bound to its cryptographic hash. Commit accepts the token rather than a
second mutable copy of the operation.

## Consequences

Callers need one additional round trip. In return, CLI and MCP share identical
server-enforced safety, and approval cannot be silently reused for altered
content or recipients.
