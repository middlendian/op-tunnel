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
- Supporting `LC_OP_TUNNEL_SOCK` override for custom paths (no change to existing behaviour).

## Design

### Socket layout

| Path | Description |
|------|-------------|
| `/tmp/op-tunnel.<username>.sock` | Actual Unix socket, created by SSH `RemoteForward`. Ephemeral — exists only while SSH is connected. Per-user by username in filename. |
| `~/.local/share/op-tunnel/client/op-tunnel.sock` | Symlink → `/tmp/op-tunnel.<username>.sock`. Persistent. Created once by `op-tunnel-setup` on the remote host. |

The symlink provides a stable, home-dir-relative path that survives reboots. It simply does not resolve until SSH connects and creates the socket. `op-tunnel-client` already expands `~/` in `LC_OP_TUNNEL_SOCK`, so it follows the symlink transparently.

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
ln -sf "/tmp/op-tunnel.$USER.sock" "$HOME/.local/share/op-tunnel/client/op-tunnel.sock"
```

The target directory (`~/.local/share/op-tunnel/client/`) is already created by the existing setup step. The symlink is idempotent (`-sf` overwrites if present).

### `protocol.go`

No changes required. `ClientSocketDir`, `SocketName`, `EnvTunnelSock`, and `ExpandSocketPath` remain as-is.

### `cmd/op-tunnel-client/main_test.go`

The existing test case for `~/.local/share/op-tunnel/client/op-tunnel.sock` tilde expansion remains valid and unchanged.

## Compatibility

- **macOS and Linux:** `/tmp` exists on both; `~/.local/share/` works on both.
- **OpenSSH version:** `%r` in `RemoteForward` socket paths has been supported since OpenSSH 6.7 (2014). No minimum version concern in practice.
- **Existing installs:** The old `/tmp/op-tunnel-client.sock` path stops working after `ssh.config` is updated. Users must re-run `op-tunnel-setup` on remote hosts to create the symlink.

## Docker / VM mounting

To expose the tunnel inside a container:

```sh
# Mount the socket file directly
docker run -v /tmp/op-tunnel.$(whoami).sock:/tmp/op-tunnel.$(whoami).sock \
           -e LC_OP_TUNNEL_SOCK=/tmp/op-tunnel.$(whoami).sock ...

# Or mount the whole client dir (symlink included; target must also be accessible)
docker run -v ~/.local/share/op-tunnel/client:/root/.local/share/op-tunnel/client \
           -v /tmp/op-tunnel.$(whoami).sock:/tmp/op-tunnel.$(whoami).sock ...
```

## Summary of changes

| File | Change |
|------|--------|
| `packaging/ssh.config` | `RemoteForward` uses `/tmp/op-tunnel.%r.sock`; `SetEnv` uses `~/.local/share/op-tunnel/client/op-tunnel.sock` |
| `packaging/op-tunnel-setup` | Add step to create symlink on remote host |
| `protocol/protocol.go` | No changes |
| `cmd/op-tunnel-client/` | No changes |
