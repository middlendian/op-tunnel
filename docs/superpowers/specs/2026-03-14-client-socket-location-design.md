# Client Socket Location Design

**Date:** 2026-03-14
**Status:** Approved

## Problem

The current client-side socket is placed at `/tmp/op-tunnel-client.sock` — a flat file in `/tmp` with no per-user isolation. This causes two issues:

1. Multiple users on the same remote host would collide on the same socket path.
2. There is no dedicated directory, making it awkward to bind-mount into Docker containers or VMs.

## Goals

- Per-user socket isolation on shared remote hosts.
- A stable, predictable home-dir path suitable for use in `SetEnv` and for bind-mounting.
- No root required at runtime; no reboot-persistence complexity.
- Minimal changes to existing code.

## Non-Goals

- Changing the server socket location (`~/.local/share/op-tunnel/server/op-tunnel.sock`).
- Supporting `LC_OP_TUNNEL_SOCK` override for custom paths (the env var mechanism is unchanged; only its default value changes).

## Design

### Socket layout

| Path | Description |
|------|-------------|
| `/tmp/op-tunnel.<username>.sock` | Actual Unix socket, created by SSH `RemoteForward`. Ephemeral — exists only while SSH is connected. Per-user by username in filename. |
| `~/.local/share/op-tunnel/client/op-tunnel.sock` | Symlink → `/tmp/op-tunnel.<username>.sock`. Persistent. Created once by `op-tunnel-setup` on the remote host. |

The symlink provides a stable, home-dir-relative path that survives reboots. It simply does not resolve until SSH connects and creates the socket. `op-tunnel-client` already expands `~/` in `LC_OP_TUNNEL_SOCK`, so it follows the symlink transparently.

When `LC_OP_TUNNEL_SOCK` is set but the socket does not exist (symlink dangling — i.e., no active SSH session), `op-tunnel-client` will fail to connect and print `op-tunnel: tunnel not connected`, then exit non-zero. This is the expected failure mode and is unchanged from current behaviour.

### `ssh.config` changes

```
RemoteForward /tmp/op-tunnel.%r.sock %d/.local/share/op-tunnel/server/op-tunnel.sock
SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock
StreamLocalBindUnlink yes
ServerAliveInterval 30
```

- `%r` is the remote login username — supported in `RemoteForward` Unix socket paths by all modern OpenSSH clients.
- `%r` is **not** expanded in `SetEnv` (verified by testing). The stable symlink path is used instead.
- `%d` is the local home directory (existing usage, unchanged).

### `op-tunnel-setup` changes

Add a step on the **remote** host (no `sudo` required):

```sh
mkdir -p "$HOME/.local/share/op-tunnel/client"
ln -sf "/tmp/op-tunnel.$USER.sock" "$HOME/.local/share/op-tunnel/client/op-tunnel.sock"
```

`mkdir -p` is included so the step is self-contained regardless of whether the directory was previously created. The symlink is idempotent (`-sf` overwrites if present).

**Constraint:** `$USER` must match the SSH login username (`%r`). This is true in the normal case. It breaks if the user SSHes with `ssh -l otheruser host` or has a `User` directive in `~/.ssh/config` that differs from their local username. In those cases, the symlink target will not match the forwarded socket name. This is an acceptable limitation — `op-tunnel-setup` should be run as the same user that will receive SSH connections.

### `protocol.go`

No changes required. `ClientSocketDir`, `SocketName`, `EnvTunnelSock`, and `ExpandSocketPath` remain as-is.

### `cmd/op-tunnel-client/main_test.go`

The existing test case for `~/.local/share/op-tunnel/client/op-tunnel.sock` tilde expansion remains valid and unchanged.

## Compatibility

- **macOS and Linux:** `/tmp` exists on both; `~/.local/share/` works on both.
- **OpenSSH version:** `%r` in `RemoteForward` socket paths has been supported since OpenSSH 6.7 (2014). No minimum version concern in practice.
- **Existing installs:** The old `/tmp/op-tunnel-client.sock` path stops working after `ssh.config` is updated. Users must re-run `op-tunnel-setup` on remote hosts to create the symlink.

## Docker / VM mounting

To expose the tunnel inside a container, mount the socket file directly and set the env var explicitly (simplest and most reliable):

```sh
docker run -v /tmp/op-tunnel.$(whoami).sock:/tmp/op-tunnel.$(whoami).sock \
           -e LC_OP_TUNNEL_SOCK=/tmp/op-tunnel.$(whoami).sock ...
```

Mounting the symlink directory alone is not sufficient — the symlink's target (`/tmp/op-tunnel.<user>.sock`) must also be reachable inside the container, which means mounting the socket file directly anyway. The simplest approach is always to mount and reference the socket file directly.

## Summary of changes

| File | Change |
|------|--------|
| `packaging/ssh.config` | `RemoteForward` uses `/tmp/op-tunnel.%r.sock`; `SetEnv` uses `~/.local/share/op-tunnel/client/op-tunnel.sock` |
| `packaging/op-tunnel-setup` | Add step to create symlink on remote host |
| `protocol/protocol.go` | No changes |
| `cmd/op-tunnel-client/` | No changes |
