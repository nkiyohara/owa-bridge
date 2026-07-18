# Interactive authentication

`owa-bridge` never asks for a Microsoft username, password, MFA code, OAuth
client secret, or tenant consent. Authentication belongs to the same visible
browser flow the user already trusts for Outlook Web.

Google Chrome, Chromium, and Microsoft Edge are supported. An explicit
`browser.executable` is resolved exactly and never falls back to a different
browser; otherwise `owa-bridge` discovers a platform-native installation and
reports a clear prerequisite failure through `owa doctor`.

## Lifecycle

1. The local session owner creates a dedicated Chromium profile with owner-only
   permissions.
2. It launches a visible browser at the configured HTTPS Outlook origin.
3. The user completes SSO, MFA, Conditional Access, and any organization notice
   directly in the browser.
4. A Chrome DevTools Protocol network observer watches only requests whose
   origin exactly matches the configured Outlook origin.
5. When Outlook itself sends bearer authorization, the session owner retains a
   minimal header snapshot in memory.
6. OWA requests receive the current snapshot only after another exact-origin
   check. An already-authorized request is rejected rather than overwritten.

The observer supports both orderings of CDP's `requestWillBeSent` and
`requestWillBeSentExtraInfo` events; the protocol does not guarantee which one
arrives first. It discards correlation state when requests finish and bounds
early-event memory.

## Stored state

Chromium may persist its own browser session in the dedicated profile using the
platform browser's protections. `owa-bridge` does not create a token cache. The
captured bearer value and selected routing headers exist only in the session
owner's memory and have no JSON, text, or logging representation.

The selected headers are limited to authorization and the OWA routing/session
headers known to be needed by the protocol adapter. Cookies, request bodies,
response bodies, and unrelated headers are not copied by the observer.

## Origin policy

Origins must use HTTPS and contain no path, query, fragment, or URL user
information. Matching includes the full host and optional port; suffix matches
are forbidden. For example, authorization observed for
`outlook.cloud.microsoft.example` cannot satisfy a configuration for
`outlook.cloud.microsoft`.

Redirects through an identity provider are expected, but authorization from
those origins is ignored. Supporting an additional Outlook API origin requires
an explicit configuration and a separate session boundary.

## Testing

Default tests exercise header filtering, origin confusion, malformed bearer
values, event ordering, lifecycle, and concurrent access with synthetic data.
They never start a browser or access a live mailbox. `owa doctor --online` is
the explicit opt-in browser and mailbox contract smoke test.
