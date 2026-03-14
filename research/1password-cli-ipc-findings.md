# 1Password CLI IPC Research Findings

## 1. IPC Mechanism by Platform

### macOS
- **Mechanism**: `NSXPCConnection` (XPC API)
- **Architecture**: The 1Password app sets up a service called **"1Password Browser Helper"** that acts as an XPC server. Both `op` CLI and the 1Password app connect to this server. Authenticity of both is confirmed by verifying the code signature. The helper acts as a **message relay** between the app and CLI.
- **Socket path**: Not a Unix socket -- uses macOS XPC, which is mediated by launchd.

### Linux
- **Mechanism**: Unix domain socket
- **Socket path**: `$XDG_RUNTIME_DIR/1Password-BrowserSupport.sock`
  - Typically: `/run/user/<UID>/1Password-BrowserSupport.sock`
- **Authentication**: The `op` binary is owned by the `onepassword-cli` group and has the **set-gid bit** set. The 1Password app verifies authenticity by checking if the GID of the connecting process matches the `onepassword-cli` group. If not, the connection is reset.

### Windows
- **Mechanism**: Named pipe
- **Authentication**: Authenticode signature verification of the connecting process's executable.

### SDK Integrations (different from CLI)
- macOS: Mach ports
- Linux: Unix domain sockets
- Windows: Named pipes

## 2. Wire Protocol (from strace)

The protocol is **JSON over a binary-framed Unix socket**. From the NixOS strace:

```
write(7, "\201\0\0\0{\"callbackId\":1,\"invocation\":{\"type\":\"NmRequestAccounts\",\"content\":{\"version\":1,\"userRequested\":true,\"supportsDelegation\":true}}}", 133)
```

Key observations:
- **Binary header**: First 4 bytes appear to be a length-prefixed frame (`\201\0\0\0` = 0x81000000 or similar framing)
- **JSON payload**: The body is JSON with fields:
  - `callbackId`: Integer, sequential request ID
  - `invocation.type`: String, e.g. `"NmRequestAccounts"` (Nm = Native Messaging)
  - `invocation.content`: Object with request parameters
- **Protocol heritage**: The "Nm" prefix and "BrowserSupport" socket name suggest this reuses the **Chrome/Firefox Native Messaging protocol** that 1Password uses for browser extension communication. The CLI piggybacks on the same IPC channel.

## 3. Socket Paths Summary

| Purpose | macOS Path | Linux Path |
|---------|-----------|------------|
| CLI <-> Desktop App | XPC (launchd-mediated, not a file path) | `/run/user/<UID>/1Password-BrowserSupport.sock` |
| op CLI daemon | `~/.config/op/op-daemon.sock` (confirmed on this machine) | `~/.config/op/op-daemon.sock` |
| SSH Agent | `~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock` | `~/.1password/agent.sock` |
| SSH Agent (alt) | Can symlink to `~/.1password/agent.sock` | Some distros: `$XDG_RUNTIME_DIR/1Password/agent.sock` |

**Note on `op-daemon.sock`**: Found at `~/.config/op/op-daemon.sock` on this Linux machine. This is the `op` CLI's own daemon socket -- the `op` binary spawns a background daemon process for caching/session management. This is **not** the same as the `1Password-BrowserSupport.sock` used for desktop app integration. The daemon socket is internal to the CLI tool itself. A community member reported Dropbox complaining about this file on macOS, confirming it exists on macOS too at the same relative path.

## 4. Environment Variables (Complete List from Official Docs)

| Variable | Purpose |
|----------|---------|
| `OP_ACCOUNT` | Default 1Password account (sign-in address or ID). `--account` flag takes precedence. |
| `OP_BIOMETRIC_UNLOCK_ENABLED` | Toggle desktop app integration on/off (`true`/`false`) |
| `OP_CACHE` | Toggle cached information on/off (`true`/`false`). Default: `true`. |
| `OP_CONFIG_DIR` | Custom configuration directory. `--config` flag takes precedence. |
| `OP_CONNECT_HOST` | URL of a 1Password Connect server (self-hosted REST API) |
| `OP_CONNECT_TOKEN` | Access token for 1Password Connect server |
| `OP_DEBUG` | Toggle debug mode (`true`/`false`). Default: `false`. |
| `OP_FORMAT` | Output format: `human-readable` or `json`. Default: `human-readable`. |
| `OP_INCLUDE_ARCHIVE` | Allow archived items in `op item get` / `op document get` (`true`/`false`). |
| `OP_ISO_TIMESTAMPS` | Format timestamps as ISO 8601 / RFC 3339 (`true`/`false`). |
| `OP_RUN_NO_MASKING` | Disable output masking for `op run`. |
| `OP_SESSION` | Stores session token for manual sign-in (`op signin`). |
| `OP_SERVICE_ACCOUNT_TOKEN` | Authenticate as a service account (token prefix: `ops_`). Bypasses desktop app entirely. |
| `SSH_AUTH_SOCK` | Path to SSH agent socket (set to 1Password's agent.sock for SSH integration) |
| `HTTP_PROXY` / `HTTPS_PROXY` | Network proxy for `op` CLI (standard Go proxy support) |

**Precedence**: `OP_CONNECT_HOST` + `OP_CONNECT_TOKEN` > `OP_SERVICE_ACCOUNT_TOKEN` > desktop app integration > manual sign-in.

**Notable**: There is **no** env var to control which IPC mechanism the CLI uses (e.g., no way to force Unix socket on macOS instead of XPC). `OP_CONFIG_DIR` controls config file location but not IPC behavior. `OP_BIOMETRIC_UNLOCK_CIPC` does not appear in official docs -- likely does not exist (was a speculative name).

## 5. Session Management

- Authorization is per-account, per-terminal-session
- Session credential = TTY ID + start time (macOS/Linux) or PID + start time (Windows)
- Expires after 10 minutes of inactivity, hard limit of 12 hours
- Locking the 1Password app revokes all sessions

## 6. Authentication Approaches for Remote/Headless Use

### Option A: Service Accounts
- Set `OP_SERVICE_ACCOUNT_TOKEN` environment variable
- No desktop app needed
- Token authenticates directly with 1Password servers
- Works on servers, CI/CD, containers

### Option B: 1Password Connect Server
- Self-hosted Docker container that provides a REST API
- Applications authenticate with bearer tokens
- Set `OP_CONNECT_HOST` and `OP_CONNECT_TOKEN`
- `op` CLI can use Connect as a backend

### Option C: Desktop App Integration (local only)
- Requires the desktop app running and unlocked
- Uses platform-native IPC (XPC on macOS, Unix socket on Linux)
- Requires biometric/password authorization
- **Cannot be tunneled** -- the IPC channel is local-only by design

## 7. Docker/Container Challenges

- The CLI-to-desktop-app socket (`1Password-BrowserSupport.sock`) is **not officially documented** as mountable into containers
- The GID check (Linux) means the container process must have the `onepassword-cli` group GID
- Community members have requested this feature but it remains unsupported
- The SSH agent socket (`agent.sock`) CAN be mounted into containers for SSH-only use

## 8. Community Requests & Projects

### Feature Requests (Exact Use Case)
- **"Feature request: forward op cli socket over SSH"** (1password.community/discussion/141918): User gclawes (Aug 2023) asked for an agent-socket-forwarding mechanism for `op` CLI similar to SSH agent forwarding. The request specifically notes that running `op` remotely requires storing the account secret key in cleartext at `~/.config/op/config`. **Zero replies from 1Password staff.**
- **"Possible to use 'op' via SSH tunnel?"** (1password.community/discussions/developers/.../26869): User chmod700 wants to SSH into a remote build machine and have `op` on that machine talk back to local 1Password. Also considered `op signin` remotely but uses a Yubikey and has no physical access. **No solution provided.**
- **"CLI over SSH?"** (1password.community/discussion/130714): Same pattern -- users want remote `op` CLI backed by local desktop app.

### Existing Projects
- **op-proxy** (github.com/carey404/op-proxy): Intercepts HTTP requests and replaces `op://` secret references with actual values using mitmproxy
- **npiperelay** (used in WSL): Bridges Windows named pipes to Unix sockets -- relevant architectural pattern
- No known projects that tunnel the `op` CLI IPC channel remotely

## 9. Key Implications for op-tunnel

1. **Linux is the tractable target**: Unix domain socket at a known path can be forwarded (e.g., via SSH socket forwarding or socat)
2. **macOS is harder**: XPC is launchd-mediated, not a simple socket file. Would need a local proxy that bridges XPC to a Unix socket.
3. **GID verification is a hurdle**: On Linux, the 1Password app checks the connecting process's GID. A forwarded socket would need the remote process to somehow present the right GID, or the verification would need to be spoofed/bypassed.
4. **Protocol is JSON-based**: The native messaging JSON protocol could potentially be proxied/relayed.
5. **Service accounts are the intended remote solution**: 1Password designed service accounts (`OP_SERVICE_ACCOUNT_TOKEN`) for exactly this use case, but they have limitations (no interactive items, token management overhead).

## 10. Config Directories

| Platform | Path | Contents |
|----------|------|----------|
| Linux/macOS | `~/.config/op/` | `config` (JSON with accounts), `op-daemon.sock` |
| macOS (app) | `~/Library/Group Containers/2BUA8C4S2C.com.1password/` | App data, SSH agent socket |
| macOS (app) | `~/Library/Application Support/1Password/` | App support files |

The `~/.config/op/config` file on a manually-signed-in machine contains: `latest_signin`, `device` UUID, and an `accounts` array with `shorthand`, `accountUUID`, `url`, `email`, `accountKey`, `userUUID`, `dsecret` -- all in **cleartext JSON**. This is the file that the community feature request called out as the security concern motivating socket forwarding.

## 11. Source Code Availability

- The `op` CLI is **closed source** (written in Go, compiled binary)
- No official decompilation or reverse-engineering efforts found in public
- The NixOS strace (Issue #258139) is the best available view into the wire protocol
- 1Password has open-source SDKs (Go, JS, Python) but those handle IPC internally via compiled core libraries

## Sources

- https://developer.1password.com/docs/cli/app-integration-security/
- https://developer.1password.com/docs/cli/app-integration/
- https://developer.1password.com/docs/cli/environment-variables/
- https://developer.1password.com/docs/sdks/desktop-app-integrations/
- https://developer.1password.com/docs/ssh/agent/
- https://developer.1password.com/docs/ssh/agent/forwarding/
- https://developer.1password.com/docs/service-accounts/
- https://developer.1password.com/docs/service-accounts/use-with-1password-cli/
- https://developer.1password.com/docs/connect/
- https://developer.1password.com/docs/connect/get-started/
- https://github.com/NixOS/nixpkgs/issues/258139
- https://www.1password.community/discussions/developers/link-the-1password-cli-in-a-container-to-the-1password-application-on-the-host/167032
- https://1password.community/discussion/141918/feature-request-forward-op-cli-socket-over-ssh
- https://www.1password.community/discussions/developers/possible-to-use-op-via-ssh-tunnel/26869
- https://1password.community/discussion/130714/cli-over-ssh
- https://www.1password.community/discussions/developers/where-is-op-daemon-sock-created-on-macos-so-dropbox-can-be-told-to-ignore-it-/95808
