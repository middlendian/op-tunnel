# op-tunnel — Agent Guide

This file describes the project for AI-assisted development sessions.

## What this project does

op-tunnel lets remote SSH users run the 1Password CLI (`op`) on their local machine with full biometric auth. It forwards `op` commands from a remote host back to the local machine over an SSH-tunneled Unix socket.

## Architecture

```
Remote Host                                 Local Host
┌───────────────────────────┐               ┌───────────────────────────┐
│  op-tunnel-client         │  SSH socket   │  op-tunnel-server         │
│  (installed as `op`)      ├──────────────►│       │                   │
│                           │  forward      │       ▼                   │
│  LC_OP_TUNNEL_SOCK set?   │◄──────────────│  exec real `op`           │
│  yes → tunnel             │  stdout/      │  return results           │
│  no  → real `op`          │  stderr/rc    │                           │
└───────────────────────────┘               └───────────────────────────┘
```

## Key files

| Path | Purpose |
|------|---------|
| `cmd/op-tunnel-server/main.go` | Local daemon: listens on Unix socket, executes real `op` |
| `cmd/op-tunnel-client/main.go` | Remote stub: tunnel mode if `LC_OP_TUNNEL_SOCK` set, else passthrough |
| `protocol/protocol.go` | Wire protocol: JSON over Unix socket, 4-byte big-endian length prefix |
| `dist/ssh.config` | SSH client config fragment (RemoteForward + SetEnv + StreamLocalBindUnlink) |
| `dist/op-tunnel-sshd.conf` | sshd drop-in: `AcceptEnv LC_OP_TUNNEL_SOCK` |
| `dist/op-tunnel-setup` | Shell script: installs sshd drop-in and reloads sshd (run with sudo on remote) |
| `dist/op-tunnel.rb` | Homebrew formula |
| `test/e2e.sh` | End-to-end test via Docker + SSH |

## Wire protocol

Request (client → server):
```json
{ "v": 1, "args": ["item", "list"], "env": {"OP_ACCOUNT": "..."}, "tty": false }
```

Response (server → client):
```json
{ "v": 1, "exitCode": 0, "stdout": "<base64>", "stderr": "<base64>", "error": "" }
```

Stdout and stderr are base64-encoded. Exit code `-1` indicates a tunnel-level error (not an `op` error).

## Important conventions

- **No shell injection**: args are passed as an array to `exec`, never through a shell.
- **Allowlisted env vars**: only specific `OP_*` vars are forwarded (see `protocol.AllowedEnvVars`).
- **Socket permissions**: `0700` for the socket directory, `0600` for the socket file (owner-only).
- **Trust model**: equivalent to SSH agent forwarding — whoever can write to the socket can execute `op` as you.
- **Tilde expansion**: `op-tunnel-client` expands `~/` in `LC_OP_TUNNEL_SOCK` because sshd may not.

## Build and test

```bash
make build              # builds bin/op-tunnel-server and bin/op-tunnel-client
make test               # runs unit tests
make test-integration   # runs integration tests (require no external deps)
make install-ssh-config # copies dist/ssh.config to ~/.local/share/op-tunnel/ssh.config
bash test/e2e.sh        # end-to-end test (requires Docker + op-tunnel-server running locally)
```

## Specs and plans

Design specs live in `docs/superpowers/specs/`.
Implementation plans live in `docs/superpowers/plans/`.
