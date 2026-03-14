# op-tunnel

Use the 1Password CLI (`op`) on remote machines, authenticated by your local desktop app.

`op` requires a local 1Password desktop connection for biometric auth. On remote machines, there's no native way to use it. op-tunnel forwards `op` commands back to your local machine over SSH — secrets stay local, the remote gets results.

```
Remote Host                                 Local Host
┌───────────────────────────┐               ┌───────────────────────────┐
│                           │  SSH socket   │                           │
│  op-tunnel-client         │  forward      │  op-tunnel-server         │
│  (installed as `op`)      ├──────────────►│       │                   │
│                           │               │       ▼                   │
│  LC_OP_TUNNEL_SOCK set?   │◄──────────────│  exec real `op`           │
│  yes → tunnel             │  stdout /     │  return results           │
│  no  → real `op`          │  stderr / rc  │                           │
└───────────────────────────┘               └───────────────────────────┘
```

Both binaries are ~5MB static Go executables with no runtime dependencies.

## Install

On **both** machines (local and remote):

```bash
brew tap middlendian/tap
brew install op-tunnel
```

Add to `~/.ssh/config`:

```
Host *
    RemoteForward ~/.local/share/op-tunnel/client/op-tunnel.sock ~/.local/share/op-tunnel/server/op-tunnel.sock
    SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock
    StreamLocalBindUnlink yes
    ServerAliveInterval 30
    ServerAliveCountMax 6
```

The server starts automatically at login (LaunchAgent on macOS, systemd user service on Linux).

> **Requirement:** Remote sshd must accept `LC_*` env vars — the default on macOS and stock Ubuntu/Debian (`AcceptEnv LANG LC_*`).

## Usage

```bash
# SSH as normal — op works transparently on the remote
ssh user@remote-server

# On the remote:
op item list
op item get GitHub --fields password   # biometric prompt appears on your local Mac
op run --env-file=.env -- ./deploy.sh
```

Local `op` usage is completely unaffected. When `LC_OP_TUNNEL_SOCK` is not set (i.e., outside an SSH session), the client passes through to the real `op` binary.

## How it works

`op-tunnel-server` listens on a Unix socket on your local machine. SSH's `RemoteForward` maps that socket to the remote machine. `op-tunnel-client` is symlinked as `op` on the remote — when `LC_OP_TUNNEL_SOCK` is set, it sends the command over the socket; the server executes it locally and returns stdout, stderr, and exit code.

Only allowlisted `OP_*` environment variables are forwarded. The socket is owner-only (`0600`). The trust model is equivalent to SSH agent forwarding.

## License

GPLv3 — see [LICENSE](LICENSE).
