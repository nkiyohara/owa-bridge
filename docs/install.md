# Install and verify

`owa-bridge` 0.1 is an early release over undocumented Outlook Web contracts.
Use only an authorized account and review the
[compatibility evidence](compatibility.md) before enabling writes.

## Release targets

Each release contains one native `owa` executable plus the license, security
policy, manual, shell completions, and essential documentation.

| Operating system | Architecture | Artifacts |
| --- | --- | --- |
| macOS | Intel, Apple silicon | `.tar.gz` |
| Linux | amd64, arm64 | `.tar.gz`, `.deb`, `.rpm`, `.apk` |
| Windows | amd64, arm64 | `.zip` |

Release assets include SHA-256 checksums and SPDX JSON and CycloneDX JSON SBOMs
for every archive and Linux package. Each archive and package includes the
third-party license material required by its linked dependencies.

## Download

Use the release page in a browser or GitHub CLI. For example:

```console
VERSION=v0.1.0
mkdir owa-release
gh release download "$VERSION" \
  --repo nkiyohara/owa-bridge \
  --dir owa-release
cd owa-release
```

Choose the archive matching `darwin`, `linux`, or `windows` and `amd64` or
`arm64`. Extract it, place `owa` or `owa.exe` on `PATH`, and run
`owa version --json` to record the version, source commit, build time, Go
version, operating system, and architecture.

On Linux, download the matching native package when preferred:

```console
gh release download "$VERSION" \
  --repo nkiyohara/owa-bridge \
  --pattern '*.deb'
sudo apt install ./owa-bridge_*.deb
```

Use the matching `.rpm` with `dnf install` or `.apk` with `apk add`. Review and
verify the package before invoking a privileged package manager.

## Verify checksums and provenance

Verify downloaded release assets before extracting or installing them:

```console
# Linux
sha256sum --ignore-missing --check checksums.txt

# macOS
shasum -a 256 --check checksums.txt
```

The release workflow signs `checksums.txt` with GitHub Actions keyless Sigstore
identity after verifying the complete artifact inventory. Verify the bundle
against the exact repository workflow:

```console
WORKFLOW_ID="https://github.com/nkiyohara/owa-bridge/"
WORKFLOW_ID="${WORKFLOW_ID}.github/workflows/release.yml@refs/tags/${VERSION}"
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "$WORKFLOW_ID" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

The binaries are not yet Apple-notarized or Windows Authenticode-signed. Do not
disable or weaken operating-system protection merely to run a download. If
local policy requires a platform signature, inspect and build from the tagged
source or wait for a future signed distribution.

## First run

Create and validate the secret-free local configuration before starting the
browser session owner:

```console
owa config init
owa config validate
owa doctor
```

Edit only the configured account alias and final HTTPS Outlook origin used
after sign-in. `owa login` opens a dedicated browser profile. Sign-in, MFA, and
Conditional Access remain inside that browser; the CLI does not accept a
password or persist an authorization header.

```console
owa login
owa doctor --online
```

For an interactive SSH session without a display server, the experimental
`owa login --terminal` command can relay ordinary text-based browser controls
through the TTY. CAPTCHA, passkeys, security keys, client certificates, and
native dialogs may still require visible login.

## Shell completion and manual

Homebrew metadata and native deb, RPM, and APK packages install `owa(1)` plus
Bash, Zsh, and Fish completions into platform-standard locations. Archive users
can activate a relocatable generated script directly:

```console
source <(owa completion bash)
source <(owa completion zsh)
owa completion fish | source
```

Persist only the command appropriate for the current shell. Completion derives
commands, flags, and enum values from the same CLI model and does not contact
Outlook.

## Configure an MCP client

After initializing `owa`, register it through each client's official CLI:

```console
owa mcp setup codex
codex mcp get owa

owa mcp setup claude-code
claude mcp get owa
```

Use `--dry-run` to inspect the exact process invocation first. Claude Code also
accepts `--scope local|project|user`. Neither setup path parses or rewrites
unrelated client settings.

For offline review, project configuration, or advanced Codex timeouts and
write-aware approval defaults, print a native document:

```console
owa mcp config codex
owa mcp config claude-code
```

See [MCP integration](mcp.md) and
[interactive authentication](authentication.md) before the first login.

## Package catalogs

GitHub release archives and Linux packages are the canonical install surface
for 0.1. Release builds render Homebrew Cask, Scoop, and WinGet manifests from
the same artifacts but do not publish them. Catalog publication requires
separate repositories, review, and least-privilege automation; it will never
silently rebuild an existing release.
