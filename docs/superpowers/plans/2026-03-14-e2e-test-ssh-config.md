# E2E Test & SSH Config Distribution Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add SSH config distribution files, a sshd setup script, client tilde expansion, an end-to-end test script, and update the Homebrew formula and README to use the `Include`-based installation approach.

**Architecture:** Static dist files (`dist/ssh.config`, `dist/op-tunnel-sshd.conf`, `dist/op-tunnel-setup`) are installed by Homebrew and a Makefile target. The client binary expands `~/` in `LC_OP_TUNNEL_SOCK` at runtime. The E2E test spins up a Docker container with sshd, SSHes in with RemoteForward via the repo's own `dist/ssh.config`, and drops the user into an interactive shell.

**Tech Stack:** Go 1.21+, Bash, Docker, OpenSSH, Homebrew Ruby DSL.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `dist/ssh.config` | SSH client config fragment (RemoteForward + SetEnv + StreamLocalBindUnlink) |
| Create | `dist/op-tunnel-sshd.conf` | sshd drop-in: `AcceptEnv LC_OP_TUNNEL_SOCK` |
| Create | `dist/op-tunnel-setup` | Shell script: deploys sshd drop-in + reloads sshd |
| Create | `cmd/op-tunnel-client/main_test.go` | Unit test for `expandTilde` |
| Modify | `cmd/op-tunnel-client/main.go` | Add `expandTilde`, call it in `main()` |
| Modify | `dist/op-tunnel.rb` | Install ssh.config + sshd.conf + setup script; update post_install + caveats |
| Modify | `README.md` | Replace `Host *` block with `Include` approach; add Prerequisites section |
| Modify | `Makefile` | Add `install-ssh-config` target |
| Create | `AGENTS.md` | Project guide for AI-assisted development sessions |
| Create | `test/Dockerfile` | Debian image with sshd + op-tunnel-client symlinked as `op` |
| Create | `test/entrypoint.sh` | Writes authorized_keys from `$SSH_PUBKEY` env var, starts sshd |
| Create | `test/e2e.sh` | All-in-one E2E test script |

---

## Chunk 1: Dist files and client tilde expansion

### Task 1: Create `dist/ssh.config`

**Files:**
- Create: `dist/ssh.config`

- [ ] **Step 1: Create the file**

```
RemoteForward ~/.local/share/op-tunnel/client/op-tunnel.sock ~/.local/share/op-tunnel/server/op-tunnel.sock
SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock
StreamLocalBindUnlink yes
ServerAliveInterval 30
```

- [ ] **Step 2: Verify it contains no Host or Match blocks**

Run: `grep -E '^(Host|Match)' dist/ssh.config`
Expected: no output (zero matches)

- [ ] **Step 3: Commit**

```bash
git add dist/ssh.config
git commit -m "feat: add SSH client config fragment"
```

---

### Task 2: Create `dist/op-tunnel-sshd.conf` and `dist/op-tunnel-setup`

**Files:**
- Create: `dist/op-tunnel-sshd.conf`
- Create: `dist/op-tunnel-setup`

- [ ] **Step 1: Create `dist/op-tunnel-sshd.conf`**

```
AcceptEnv LC_OP_TUNNEL_SOCK
```

- [ ] **Step 2: Create `dist/op-tunnel-setup`**

```sh
#!/bin/sh
set -e

CONF_SRC="$(dirname "$0")/../share/op-tunnel/op-tunnel-sshd.conf"
CONF_DST="/etc/ssh/sshd_config.d/op-tunnel.conf"

install -m 644 "$CONF_SRC" "$CONF_DST"

# Reload sshd
if command -v systemctl >/dev/null 2>&1; then
    systemctl reload sshd 2>/dev/null || systemctl reload ssh  # sshd on RHEL/Arch, ssh on Debian/Ubuntu
elif launchctl kickstart -k system/com.openssh.sshd >/dev/null 2>&1; then
    : # macOS Ventura+
else
    echo "sshd config installed. Reload sshd manually to apply."
fi

echo "Done. sshd will now accept LC_OP_TUNNEL_SOCK from SSH clients."
```

- [ ] **Step 3: Make the setup script executable**

```bash
chmod +x dist/op-tunnel-setup
```

- [ ] **Step 4: Verify the script is valid shell**

Run: `sh -n dist/op-tunnel-setup`
Expected: no output (no syntax errors)

- [ ] **Step 5: Commit**

```bash
git add dist/op-tunnel-sshd.conf dist/op-tunnel-setup
git commit -m "feat: add sshd drop-in config and op-tunnel-setup script"
```

---

### Task 3: Client tilde expansion

**Files:**
- Create: `cmd/op-tunnel-client/main_test.go`
- Modify: `cmd/op-tunnel-client/main.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/op-tunnel-client/main_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"~/.local/share/op-tunnel/client/op-tunnel.sock", filepath.Join(home, ".local/share/op-tunnel/client/op-tunnel.sock")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~notslash", "~notslash"},
		{"", ""},
	}

	for _, tt := range tests {
		got := expandTilde(tt.input)
		if got != tt.want {
			t.Errorf("expandTilde(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/op-tunnel-client/ -run TestExpandTilde -v`
Expected: compile error — `expandTilde` undefined

- [ ] **Step 3: Add `expandTilde` to `cmd/op-tunnel-client/main.go`**

Add the function before `main()` and call it in `main()`. The diff is:

In `main()`, change:
```go
sockPath := os.Getenv(protocol.EnvTunnelSock)
```
to:
```go
sockPath := expandTilde(os.Getenv(protocol.EnvTunnelSock))
```

Add the function after the import block (before `main`):
```go
// expandTilde expands a leading ~/ to the user's home directory.
// sshd does not expand ~ in SetEnv values; the SSH client may or may not
// depending on version. This ensures the path is always absolute.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
```

Note: `strings` and `filepath` are already imported.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/op-tunnel-client/ -run TestExpandTilde -v`
Expected:
```
=== RUN   TestExpandTilde
--- PASS: TestExpandTilde (0.00s)
PASS
```

- [ ] **Step 5: Run all tests to confirm nothing is broken**

Run: `go test ./...`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add cmd/op-tunnel-client/main.go cmd/op-tunnel-client/main_test.go
git commit -m "feat: expand tilde in LC_OP_TUNNEL_SOCK before dialing socket"
```

---

## Chunk 2: Homebrew formula, README, Makefile, and AGENTS.md

### Task 4: Update Homebrew formula

**Files:**
- Modify: `dist/op-tunnel.rb`

The existing formula installs two binaries and the LaunchAgent. Three changes needed:
1. `install` block: install `op-tunnel-sshd.conf` to `share/op-tunnel/` and `op-tunnel-setup` to `bin/`
2. `post_install` block: install `ssh.config` to `~/.local/share/op-tunnel/`; replace hardcoded SSH block with `Include`-based instructions
3. `caveats`: replace `AcceptEnv LC_*` note with reference to `op-tunnel-setup`

- [ ] **Step 1: Update the `install` block**

In `dist/op-tunnel.rb`, the `install` block currently ends after installing the service files. Add after the existing `prefix.install` lines:

```ruby
    # SSH distribution files — ssh.config is installed here so it is
    # available via share/"op-tunnel" in post_install (buildpath is nil there)
    (share/"op-tunnel").install "dist/op-tunnel-sshd.conf", "dist/ssh.config"
    bin.install "dist/op-tunnel-setup"
```

- [ ] **Step 2: Update the `post_install` block**

Replace the entire `post_install` block with:

```ruby
  def post_install
    # Symlink client as `op` — overrides native op binary
    bin.install_symlink bin/"op-tunnel-client" => "op"

    # Install LaunchAgent with correct paths
    plist = prefix/"com.middlendian.op-tunnel-server.plist"
    inreplace plist, "HOMEBREW_PREFIX", HOMEBREW_PREFIX
    launch_agent_dir = Pathname.new("#{Dir.home}/Library/LaunchAgents")
    launch_agent_dir.mkpath
    ln_sf plist, launch_agent_dir/"com.middlendian.op-tunnel-server.plist"

    # Install SSH client config fragment
    op_tunnel_dir = Pathname.new("#{Dir.home}/.local/share/op-tunnel")
    op_tunnel_dir.mkpath
    cp share/"op-tunnel"/"ssh.config", op_tunnel_dir/"ssh.config"

    ohai "op-tunnel installed!"
    puts <<~EOS
      The SSH config fragment has been installed to:
        ~/.local/share/op-tunnel/ssh.config

      Add the following inside each Host block in ~/.ssh/config for
      hosts where you want op-tunnel active (requires OpenSSH 7.3+):

        Host myserver
            Include ~/.local/share/op-tunnel/ssh.config

      On each remote host, run once to configure sshd:
        sudo op-tunnel-setup
        (Skip on stock Debian/Ubuntu — AcceptEnv LANG LC_* already covers LC_OP_TUNNEL_SOCK)

      The server LaunchAgent has been installed and will start on next login.
      To start it now:
        launchctl load ~/Library/LaunchAgents/com.middlendian.op-tunnel-server.plist
    EOS
  end
```

- [ ] **Step 3: Update `caveats`**

Replace the existing `caveats` block with:

```ruby
  def caveats
    <<~EOS
      op-tunnel-client has been symlinked as `op`.
      When LC_OP_TUNNEL_SOCK is set (via SSH), op commands are tunneled.
      Otherwise, the real op binary is called directly.

      To activate tunneling for a remote host, add inside its Host block
      in ~/.ssh/config:
        Include ~/.local/share/op-tunnel/ssh.config

      On remote hosts (except stock Debian/Ubuntu), configure sshd once:
        sudo op-tunnel-setup
    EOS
  end
```

- [ ] **Step 4: Verify the formula Ruby syntax**

Run: `ruby -c dist/op-tunnel.rb`
Expected: `Syntax OK`

- [ ] **Step 5: Commit**

```bash
git add dist/op-tunnel.rb
git commit -m "feat: update Homebrew formula to install ssh.config and op-tunnel-setup"
```

---

### Task 5: Update README and Makefile

**Files:**
- Modify: `README.md`
- Modify: `Makefile`

- [ ] **Step 1: Update the SSH config block in README Install section**

Replace the current SSH config block:
```
Host *
    RemoteForward ~/.local/share/op-tunnel/client/op-tunnel.sock ~/.local/share/op-tunnel/server/op-tunnel.sock
    SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock
    StreamLocalBindUnlink yes
    ServerAliveInterval 30
    ServerAliveCountMax 6
```

With:
```
Host myserver
    Include ~/.local/share/op-tunnel/ssh.config
```

Add a note beneath it:
> Use specific host names, not `Host *`. The included fragment applies RemoteForward and SetEnv only to the hosts where you opt in.

- [ ] **Step 2: Add Prerequisites section after the Install section**

Insert a new `## Prerequisites` section after `## Install` and before `## Usage`:

```markdown
## Prerequisites

**Local machine (SSH client):**
- OpenSSH 7.3+ — standard on macOS 10.12+ and Ubuntu 18.04+
- 1Password desktop app running with biometric auth enabled
- `op-tunnel-server` running (auto-started by LaunchAgent/systemd after install)

**Remote machine (SSH server):**
- `AcceptEnv LC_OP_TUNNEL_SOCK` in `/etc/ssh/sshd_config`

  Stock Debian/Ubuntu already includes `AcceptEnv LANG LC_*`, which covers `LC_OP_TUNNEL_SOCK` — no change needed. On other systems (macOS, RHEL, Arch), run once after installing op-tunnel on the remote:

  ```bash
  sudo op-tunnel-setup
  ```
```

- [ ] **Step 3: Remove the existing AcceptEnv footnote**

The current README has this line beneath the SSH config block:

```
> **Requirement:** Remote sshd must accept `LC_*` env vars — the default on macOS and stock Ubuntu/Debian (`AcceptEnv LANG LC_*`).
```

Remove it — the new Prerequisites section covers this more clearly.

- [ ] **Step 4: Add `install-ssh-config` target to Makefile**

Make the following two edits to `Makefile`:

1. Add `DATADIR` variable and `install-ssh-config` target at the end of the file:

```makefile
DATADIR := $(HOME)/.local/share/op-tunnel

install-ssh-config:
	mkdir -p $(DATADIR)
	cp dist/ssh.config $(DATADIR)/ssh.config
```

2. Update the existing `.PHONY` line (line 4) — extend it to include `install-ssh-config`:

```makefile
.PHONY: build clean test test-integration install-ssh-config
```

Do NOT add a second `.PHONY` line — the existing one must be edited in place.

- [ ] **Step 5: Commit**

```bash
git add README.md Makefile
git commit -m "docs: update README to use Include-based SSH config with prerequisites section"
```

---

### Task 6: Create AGENTS.md

**Files:**
- Create: `AGENTS.md`

- [ ] **Step 1: Create the file**

```markdown
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
```

- [ ] **Step 2: Commit**

```bash
git add AGENTS.md
git commit -m "docs: add AGENTS.md project guide for AI-assisted development"
```

---

## Chunk 3: E2E test script

### Task 7: Create Dockerfile, entrypoint, and e2e.sh

**Files:**
- Create: `test/Dockerfile`
- Create: `test/entrypoint.sh`
- Create: `test/e2e.sh`

- [ ] **Step 1: Create `test/entrypoint.sh`**

This script runs as PID 1 inside the container. It writes the authorized_keys from the `$SSH_PUBKEY` env var and starts sshd.

```sh
#!/bin/sh
set -e

# Write the caller's public key into root's authorized_keys
mkdir -p /root/.ssh
chmod 700 /root/.ssh
echo "$SSH_PUBKEY" > /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys

exec /usr/sbin/sshd -D
```

Make it executable: `chmod +x test/entrypoint.sh`

- [ ] **Step 2: Create `test/Dockerfile`**

```dockerfile
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        openssh-server \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd

# sshd configuration
RUN echo 'AcceptEnv LC_OP_TUNNEL_SOCK' >> /etc/ssh/sshd_config \
    && echo 'StreamLocalBindUnlink yes' >> /etc/ssh/sshd_config \
    && echo 'PermitRootLogin yes' >> /etc/ssh/sshd_config

# Install op-tunnel-client, symlinked as `op`
COPY op-tunnel-client /usr/local/bin/op-tunnel-client
RUN chmod +x /usr/local/bin/op-tunnel-client \
    && ln -s /usr/local/bin/op-tunnel-client /usr/local/bin/op

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 22
CMD ["/entrypoint.sh"]
```

- [ ] **Step 3: Create `test/e2e.sh`**

```bash
#!/bin/bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMPDIR_E2E="$(mktemp -d)"
CONTAINER_NAME="op-tunnel-e2e-$$"
IMAGE_NAME="op-tunnel-e2e"

cleanup() {
    echo ""
    echo "==> Cleaning up..."
    docker stop "$CONTAINER_NAME" 2>/dev/null || true
    docker rm   "$CONTAINER_NAME" 2>/dev/null || true
    rm -rf "$TMPDIR_E2E"
}
trap cleanup EXIT

# --- 1. Build binaries ---
echo "==> Building binaries..."
go build -o "$TMPDIR_E2E/op-tunnel-server" "$REPO_ROOT/cmd/op-tunnel-server"
go build -o "$TMPDIR_E2E/op-tunnel-client" "$REPO_ROOT/cmd/op-tunnel-client"

# --- 2. Build Docker image ---
echo "==> Building Docker image..."
cp "$TMPDIR_E2E/op-tunnel-client" "$REPO_ROOT/test/op-tunnel-client"
docker build -t "$IMAGE_NAME" "$REPO_ROOT/test" --quiet
rm -f "$REPO_ROOT/test/op-tunnel-client"

# --- 3. Generate throwaway SSH keypair ---
echo "==> Generating SSH keypair..."
ssh-keygen -t ed25519 -f "$TMPDIR_E2E/id_ed25519" -N "" -q

# --- 4. Write temp SSH config ---
# Written before docker run so the retry loop below can reference $SSH_CONFIG
SSH_CONFIG="$TMPDIR_E2E/ssh_config"
cat > "$SSH_CONFIG" <<EOF
Host op-tunnel-test
    HostName 127.0.0.1
    Port 2222
    User root
    IdentityFile $TMPDIR_E2E/id_ed25519
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    Include $REPO_ROOT/dist/ssh.config
EOF

# --- 5. Start container ---
echo "==> Starting container..."
docker run -d \
    --name "$CONTAINER_NAME" \
    -p 127.0.0.1:2222:22 \
    -e "SSH_PUBKEY=$(cat "$TMPDIR_E2E/id_ed25519.pub")" \
    "$IMAGE_NAME" >/dev/null

# Wait for sshd to be ready (retry up to 10s)
echo -n "==> Waiting for sshd..."
for i in $(seq 1 20); do
    ssh -F "$SSH_CONFIG" -o BatchMode=yes -o ConnectTimeout=1 op-tunnel-test exit 0 2>/dev/null && break
    echo -n "."
    sleep 0.5
done
echo ""

# --- 6. Check op-tunnel-server ---
SERVER_SOCK="$HOME/.local/share/op-tunnel/server/op-tunnel.sock"
if [ ! -S "$SERVER_SOCK" ]; then
    echo ""
    echo "ERROR: op-tunnel-server is not running (no socket at $SERVER_SOCK)."
    echo ""
    echo "Start it with one of:"
    echo "  launchctl load ~/Library/LaunchAgents/com.middlendian.op-tunnel-server.plist"
    echo "  op-tunnel-server &"
    exit 1
fi
echo "==> op-tunnel-server is running."

# --- 7. Pre-flight check ---
echo "==> Checking tunnel forwarding..."
PREFLIGHT=$(ssh -F "$SSH_CONFIG" -o BatchMode=yes op-tunnel-test \
    'if [ -z "${LC_OP_TUNNEL_SOCK:-}" ]; then
        echo "FAIL: LC_OP_TUNNEL_SOCK is not set (check AcceptEnv in sshd_config)"
    elif ! test -S "$LC_OP_TUNNEL_SOCK"; then
        echo "FAIL: socket not found at $LC_OP_TUNNEL_SOCK (is op-tunnel-server running?)"
    else
        echo "PASS: socket at $LC_OP_TUNNEL_SOCK"
    fi' 2>/dev/null)
echo "    $PREFLIGHT"
if echo "$PREFLIGHT" | grep -q "^FAIL"; then
    echo ""
    echo "Pre-flight failed. Fix the issue above and re-run."
    exit 1
fi

# --- 8. Interactive shell ---

echo ""
echo "==> Tunnel is working. Dropping into container shell."
echo "    Try: op whoami"
echo "    Biometric prompt will appear on your local machine."
echo "    Type 'exit' or press Ctrl-D to finish."
echo ""
ssh -F "$SSH_CONFIG" op-tunnel-test
```

Make it executable: `chmod +x test/e2e.sh`

- [ ] **Step 4: Verify shell scripts have no syntax errors**

Run:
```bash
sh -n test/entrypoint.sh
bash -n test/e2e.sh
```
Expected: no output (no errors)

- [ ] **Step 5: Add `test/` to .gitignore for the temp client binary**

Append to `.gitignore` (or create it if absent):
```
test/op-tunnel-client
```

- [ ] **Step 6: Verify Docker builds successfully**

Run:
```bash
go build -o /tmp/op-tunnel-client-test ./cmd/op-tunnel-client
cp /tmp/op-tunnel-client-test test/op-tunnel-client
docker build -t op-tunnel-e2e test/
rm test/op-tunnel-client
```
Expected: `Successfully built ...` (or similar — no errors)

- [ ] **Step 7: Commit**

```bash
git add test/Dockerfile test/entrypoint.sh test/e2e.sh .gitignore
git commit -m "feat: add E2E test script with Docker-based SSH tunnel"
```
