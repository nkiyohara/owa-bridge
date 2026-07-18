# ADR 0002: Interactive browser-owned authentication

- Status: accepted
- Date: 2026-07-17

## Context

Graph application access may be unavailable, while the user is authorized to
use Outlook Web. Direct credential submission fails modern SSO, MFA, and
Conditional Access requirements. TLS interception expands the trust boundary.

## Decision

Launch or attach to an isolated Chromium profile and let the user complete the
normal interactive flow. Do not request the password and do not intercept TLS.
Keep authorization material in the session owner and prefer execution inside
the browser security context when practical.

## Consequences

Initial login requires a visible browser. The approach preserves the identity
provider's controls but depends on undocumented Outlook Web behavior. Protocol
capability probes and fixtures are mandatory.
