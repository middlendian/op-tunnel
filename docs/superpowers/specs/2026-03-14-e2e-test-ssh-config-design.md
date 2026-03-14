# E2E Test Script & SSH Config Distribution Design

**Date:** 2026-03-14
**Status:** Approved

## Overview

Two related improvements:

1. **SSH config fragment** (`dist/ssh.config`) — a static, installable snippet users include inside trusted `Host` blocks in `~/.ssh/config` to activate op-tunnel forwarding for those hosts.
2. **E2E test script** (`test/e2e.sh`) — a single shell script that spins up a Docker container with sshd, SSHes into it with RemoteForward configured using the repo's own `dist/ssh.config`, and drops the user into an interactive shell to run real `op` commands end-to-end.
3. **AGENTS.md** — top-level file describing the project for AI-assisted development sessions.

---

## Section 1: `dist/ssh.config`

A static file checked into the repo. No templating or substitution required.

```
RemoteForward ~/.local/share/op-tunnel/client/op-tunnel.sock ~/.local/share/op-tunnel/server/op-tunnel.sock
SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock
StreamLocalBindUnlink yes
ServerAliveInterval 30
```

**Key design decisions:**

- `~` is used on both sides of `RemoteForward`. The local `~` is expanded by the SSH client; the remote `~` is expanded by sshd.
- `SetEnv LC_OP_TUNNEL_SOCK` also uses `~`. Since sshd passes this value verbatim (no shell expansion), the `op-tunnel-client` binary is responsible for expanding a leading `~/` to `$HOME/` when reading this variable.
- `StreamLocalBindUnlink yes` ensures stale sockets from previous sessions are cleaned up automatically.
- `ServerAliveInterval 30` keeps the SSH connection alive to maintain the forwarded socket.

**Installation:**

The Makefile and Homebrew formula copy this file to `~/.local/share/op-tunnel/ssh.config`.

Users add a single line inside each trusted `Host` block in `~/.ssh/config`:

```
Host myserver
    HostName myserver.example.com
    Include ~/.local/share/op-tunnel/ssh.config
```

Using `Include` inside a `Host` block (rather than at the top level) scopes these settings to specific trusted hosts only. Users should not use a wildcard `Host *` with this include.

---

## Section 2: Client tilde expansion

In `cmd/op-tunnel-client/main.go`, when reading `LC_OP_TUNNEL_SOCK`, expand a leading `~/` to the user's home directory before dialing the socket. This is a small, self-contained function — no external dependencies needed (`os.UserHomeDir()` suffices).

---

## Section 3: E2E test script

### Files

- `test/Dockerfile` — container image with sshd and op-tunnel-client
- `test/e2e.sh` — single all-in-one test script

### `test/Dockerfile`

- Base: Debian slim
- Installs: `openssh-server`
- Copies: `op-tunnel-client` binary (provided at build time via `--build-arg` or build context)
- Symlinks: `/usr/local/bin/op -> op-tunnel-client`
- Configures sshd: `AcceptEnv LC_OP_TUNNEL_SOCK`, `StreamLocalBindUnlink yes`, creates `/run/sshd`

### `test/e2e.sh` flow

1. **Build binaries** — `go build` both `op-tunnel-server` and `op-tunnel-client` into a temp directory.
2. **Build Docker image** — `docker build` copies the client binary into the image.
3. **Generate SSH keypair** — throwaway ed25519 key in a temp directory (no passphrase).
4. **Start container** — `docker run` with sshd listening on `127.0.0.1:2222`, authorized_keys set to the generated public key.
5. **Check server** — verify `op-tunnel-server` is running locally (check for socket at `~/.local/share/op-tunnel/server/op-tunnel.sock`); print a clear error and exit if not.
6. **Write temp SSH config** — a `Host op-tunnel-test` block pointing to `127.0.0.1:2222` with `Include <repo>/dist/ssh.config`:
   ```
   Host op-tunnel-test
       HostName 127.0.0.1
       Port 2222
       User root
       IdentityFile /tmp/op-tunnel-e2e-key
       StrictHostKeyChecking no
       UserKnownHostsFile /dev/null
       Include <repo>/dist/ssh.config
   ```
7. **SSH in** — `ssh -F <temp config> op-tunnel-test` — interactive shell. The user runs `op` commands; biometric prompts appear on the local machine; results print in the container.
8. **Cleanup** — on SSH exit (trap on EXIT): `docker stop` + `docker rm` the container, remove temp files.

### What the test validates

- `dist/ssh.config` is syntactically valid and activates forwarding correctly
- `RemoteForward` successfully tunnels the Unix socket through SSH
- `op-tunnel-client` (symlinked as `op`) connects via the forwarded socket
- `op-tunnel-server` executes the real `op` binary with biometric auth on the local machine
- The response (stdout/stderr/exit code) is correctly returned to the container

### Prerequisites printed by the script

- Docker must be running
- `op-tunnel-server` must be running locally (via LaunchAgent or manually)
- The real `op` binary must be installed and authenticated on the local machine

---

## Section 4: AGENTS.md

A top-level `AGENTS.md` describing the project for AI-assisted development sessions:

- What the project does and why
- Architecture overview (server, client, protocol)
- Key files and their purposes
- Important conventions (no shell injection, allowlisted env vars, socket permissions model)
- Where specs and plans live (`docs/superpowers/`)
- How to build and test

---

## Scope: what is NOT changing

- The wire protocol (`protocol/protocol.go`) — no changes
- The server binary (`cmd/op-tunnel-server/main.go`) — no changes
- The existing integration tests — no changes
- Socket path constants — no changes
