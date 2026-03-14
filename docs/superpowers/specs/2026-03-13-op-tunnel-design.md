# op-tunnel: Remote 1Password CLI over SSH

## Problem

The 1Password CLI (`op`) requires a local desktop app connection for authentication and credential access. There is no supported way to use `op` on a remote machine while authenticating against the local 1Password app. Existing alternatives (Connect Server, Service Accounts) are heavyweight or limited in scope.

## Solution

A socket-based command proxy that forwards `op` CLI invocations from a remote machine back to the local machine over an SSH-forwarded Unix domain socket. The local machine executes `op` commands natively (with full biometric/auth support) and returns results verbatim.

## Architecture

```
Remote Host (Beta)                          Local Host (Alpha)
┌──────────────────────┐                    ┌──────────────────────┐
│                      │   SSH RemoteForward│                      │
│  op-tunnel-client    │   (Unix socket)    │  op-tunnel-server    │
│  (symlinked as `op`) ├───────────────────►│       │              │
│       │              │                    │       ▼              │
│  if LC_OP_TUNNEL_SOCK│  ◄─────────────────│  exec real `op`     │
│  is set: use tunnel  │  stdout/stderr/rc  │  return results     │
│  else: call real `op`│                    │                      │
└──────────────────────┘                    └──────────────────────┘
```

## Components

### 1. `op-tunnel-server` (local host daemon)

- Listens on `~/.local/share/op-tunnel/server/op-tunnel.sock`
- Accepts connections, reads a framed JSON request, executes the real `op` binary with the provided args and allowlisted env vars, captures stdout/stderr/exit code, returns a framed JSON response
- Runs as a LaunchAgent (macOS) or systemd user service (Linux), started automatically on login
- Creates the socket directory if it does not exist, with `0700` permissions
- Cleans up stale sockets on startup

### 2. `op-tunnel-client` (remote host stub)

- Installed at a Homebrew-managed path and symlinked as `op` (overriding the native binary in PATH)
- Decision logic:
  - If `LC_OP_TUNNEL_SOCK` is set and the socket exists: connect to the tunnel socket, send the command, reproduce the response (stdout to fd 1, stderr to fd 2, exit with the returned exit code)
  - If `LC_OP_TUNNEL_SOCK` is **not** set: find and exec the real `op` binary (pass-through mode). Discovery: search `PATH` entries, skipping any directory containing the `op-tunnel-client` binary itself (i.e., skip its own symlink). This ensures local `op` usage is completely unaffected.
- If the socket is set but not connectable: print error `op-tunnel: tunnel not connected` to stderr, exit 1

### 3. SSH Configuration

Added to `~/.ssh/config` (or via `Include`):

```ssh_config
Host *
    # Forward op-tunnel socket from remote to local
    RemoteForward ~/.local/share/op-tunnel/client/op-tunnel.sock ~/.local/share/op-tunnel/server/op-tunnel.sock

    # Set LC_OP_TUNNEL_SOCK on the remote host so the client stub knows to use the tunnel.
    # This var only exists in SSH sessions — local shells never see it.
    SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock

    # Clean up stale remote sockets before binding
    StreamLocalBindUnlink yes

    # Keep connection alive
    ServerAliveInterval 30
    ServerAliveCountMax 6

    # Optional: fail fast if forwarding fails
    # ExitOnForwardFailure yes
```

**Key insight**: `SetEnv` ensures `LC_OP_TUNNEL_SOCK` is only present in SSH sessions. The client stub uses this as the signal to tunnel vs pass-through. Local shell sessions never have this var set, so `op` works natively without interference.

**Requirements:**
- `SetEnv` requires OpenSSH 7.8+ (released 2018). `RemoteForward` with `~` expansion for Unix socket paths requires OpenSSH 8.x+.
- The remote sshd must have `AcceptEnv LC_*` in its config. This is the default on macOS and stock Ubuntu/Debian (`AcceptEnv LANG LC_*`), but may be stripped on hardened or minimal Linux images. The `op-tunnel-client` install can verify this with `sshd -T | grep acceptenv`.

## Socket Paths

| Side   | Path                                                    | Purpose                        |
|--------|---------------------------------------------------------|--------------------------------|
| Local  | `~/.local/share/op-tunnel/server/op-tunnel.sock`        | Server listens here            |
| Remote | `~/.local/share/op-tunnel/client/op-tunnel.sock`        | SSH forwards to here           |

Each socket is the only file in its parent directory, enabling clean Docker volume mounts if needed (e.g., `-v ~/.local/share/op-tunnel/client:/run/op-tunnel`).

## Wire Protocol

Length-prefixed JSON over Unix domain socket. One request-response per connection.

### Framing

4-byte big-endian length prefix, followed by JSON payload. Same format for both request and response. Maximum payload size: 64MB. Messages exceeding this limit are rejected.

### Request (client → server)

```json
{
  "v": 1,
  "args": ["item", "get", "GitHub", "--fields", "password"],
  "env": {
    "OP_FORMAT": "json"
  },
  "tty": false
}
```

| Field  | Type     | Description                                          |
|--------|----------|------------------------------------------------------|
| `v`    | int      | Protocol version (currently `1`)                     |
| `args` | []string | Arguments to pass to `op` (everything after `op`)    |
| `env`  | object   | Allowlisted env vars that are set on the remote side |
| `tty`  | bool     | Whether the remote stdout is a TTY                   |

### Response (server → client)

```json
{
  "v": 1,
  "exitCode": 0,
  "stdout": "<base64>",
  "stderr": "<base64>",
  "error": ""
}
```

| Field      | Type   | Description                                    |
|------------|--------|------------------------------------------------|
| `v`        | int    | Protocol version                               |
| `exitCode` | int    | Exit code from the `op` process (-1 if error)  |
| `stdout`   | string | Base64-encoded stdout bytes                    |
| `stderr`   | string | Base64-encoded stderr bytes                    |
| `error`    | string | Tunnel-level error (e.g., `op` not found, execution timeout). Empty on success. When set, `exitCode` is -1 and stdout/stderr may be empty. |

Base64 encoding handles binary output cleanly (e.g., `op document get`).

### Env Var Allowlist

Only these documented 1Password CLI environment variables are forwarded:

- `OP_ACCOUNT` — Account shorthand
- `OP_CACHE` — Enable/disable caching
- `OP_CONNECT_HOST` — Connect server host
- `OP_CONNECT_TOKEN` — Connect server token
- `OP_FORMAT` — Output format (`json` / `human-readable`)
- `OP_INCLUDE_ARCHIVE` — Include archived items
- `OP_ISO_TIMESTAMPS` — ISO 8601 timestamp formatting
- `OP_RUN_NO_MASKING` — Disable `op run` output masking
- `OP_SERVICE_ACCOUNT_TOKEN` — Service account authentication

All other environment variables are ignored.

## Security

- **Socket permissions**: Created with `0700` directory / `0600` socket (owner-only)
- **Trust model**: Equivalent to SSH agent forwarding. Anyone with access to the socket can run `op` commands as the tunnel owner. Same risk profile users accept with `SSH_AUTH_SOCK`.
- **No command filtering**: The server executes whatever args the client sends. The real `op` binary and 1Password app handle authorization (biometric prompts, vault access controls).
- **No shell interpretation**: Args are passed as an array directly to `execve`-style invocation. No shell injection risk.
- **Env var allowlist**: Prevents injection of arbitrary environment variables into the local `op` process.

## Auth Behavior

The tunnel faithfully proxies the native 1Password auth flow:

- If 1Password is locked: biometric/password prompt appears on the local machine
- If already unlocked: commands execute immediately with no prompt
- Session timeouts are governed by the local 1Password app settings
- No session management in the tunnel itself

## Distribution

### Homebrew Tap

Published as a Homebrew formula via a tap (e.g., `middlendian/tap`).

**Formula installs:**
- `op-tunnel-server` binary
- `op-tunnel-client` binary

**Postinstall:**
- Symlinks `op-tunnel-client` as `op` in the Homebrew bin directory, overriding the native `op` binary in PATH
- Registers the LaunchAgent for `op-tunnel-server`

### LaunchAgent (macOS)

`~/Library/LaunchAgents/com.middlendian.op-tunnel-server.plist`:

- Runs `op-tunnel-server` on login
- `KeepAlive: true` (restart on crash)
- `StandardOutPath` / `StandardErrorPath` for logging

### systemd User Service (Linux)

`~/.config/systemd/user/op-tunnel-server.service`:

- `Type=simple`, runs `op-tunnel-server`
- `Restart=on-failure`
- Enabled with `systemctl --user enable op-tunnel-server`

## Implementation Language

**Go** for both components:
- Single static binary, no runtime dependencies
- Excellent Unix socket support
- Easy cross-compile for macOS arm64/amd64 and Linux
- Small binary size (~5MB each)
- Both components are ~200-300 lines each

## Usage

### First-time setup (both machines)

```bash
brew tap middlendian/tap
brew install op-tunnel
```

Add to `~/.ssh/config` (or the formula prints this during postinstall):

```ssh_config
Host *
    RemoteForward ~/.local/share/op-tunnel/client/op-tunnel.sock ~/.local/share/op-tunnel/server/op-tunnel.sock
    SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock
    StreamLocalBindUnlink yes
    ServerAliveInterval 30
    ServerAliveCountMax 6
```

### Daily use

```bash
# Just SSH as normal — op works transparently on the remote host
ssh user@remote-server

# On remote:
op item list                          # tunneled to local 1Password
op item get GitHub --fields password  # biometric prompt on local Mac
op run --env-file=.env -- ./deploy.sh # secrets injected via tunnel
```

### Local use (unaffected)

```bash
# When not in an SSH session, LC_OP_TUNNEL_SOCK is not set.
# op-tunnel-client passes through to the real op binary.
op item list  # works normally, talks to local 1Password app directly
```

## Protocol Versioning

The `v` field in requests and responses enables forward compatibility. Behavior:
- Server rejects requests with an unsupported version, returning an `error` response: `"unsupported protocol version: N"`
- v1 is the only supported version initially
- Future versions can add fields (unknown fields are ignored for backwards compatibility)

## Known Limitations

- **Concurrent SSH sessions**: Multiple SSH sessions to the same remote host will compete for the same `RemoteForward` socket path. `StreamLocalBindUnlink yes` means the last session wins. This matches the behavior of SSH agent forwarding and is acceptable for v1.
- **Cross-platform home directories**: Current `~` expansion assumes same-structure home paths. For macOS→Linux tunneling, may need separate path configuration.

## Future Considerations

- **Docker integration**: The `client/` socket directory can be volume-mounted into containers running the Linux `op` binary.
- **Streaming**: For large `op document get` responses, consider streaming instead of buffering the full response. Not needed for v1.
- **Multiple concurrent commands**: The server handles one connection at a time in v1. Could add concurrency if needed.
