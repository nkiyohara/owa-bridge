# Configuration

`owa-bridge` uses strict TOML in the platform user configuration directory:

- Linux: `$XDG_CONFIG_HOME/owa-bridge/config.toml`, normally
  `~/.config/owa-bridge/config.toml`;
- macOS: `~/Library/Application Support/owa-bridge/config.toml`;
- Windows: `%AppData%\owa-bridge\config.toml`.

The application creates the project directory with owner-only permissions and
atomically replaces the file with mode `0600` where the operating system
supports Unix permissions.

```toml
version = 1
default_account = "work"

[accounts.work]
origin = "https://outlook.cloud.microsoft"

[policy]
mode = "guarded"
preview_sensitive_reads = false
preview_reversible_writes = false
max_recipients = 20
max_attendees = 50

[browser]
login_timeout = "5m0s"
```

Unknown fields, non-HTTPS origins, URL credentials, unsupported policy modes,
and out-of-range limits are errors. There is deliberately no unguarded-write
mode.

`origin` is an exact authorization boundary, not a discovery hint or wildcard.
If a normal browser ends on a different Outlook host after sign-in, configure
that final HTTPS origin with no path. Do not add the identity-provider origin,
tenant vanity aliases that merely redirect elsewhere, or multiple origins in an
attempt to make capture succeed. Sovereign, hybrid, and on-premises deployments
must use the actual OWA service origin observed by an authorized user.

The configuration schema cannot represent a password, OAuth token, cookie,
canary, or refresh token. Browser session material belongs to the dedicated
browser profile and the in-memory session owner, never this file.

The daemon publishes a SHA-256 digest of the exact secret-free config it loaded.
CLI and MCP compare it before every new connection and fail closed if the file
or executable version changed. Run `owa daemon stop` and retry to apply an edit;
the next command starts a fresh owner with the new policy.
