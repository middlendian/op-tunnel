# Client Socket Location Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat `/tmp/op-tunnel-client.sock` socket with a per-user `/tmp/op-tunnel.<username>.sock`, exposed via a stable symlink at `~/.local/share/op-tunnel/client/op-tunnel.sock`.

**Architecture:** `ssh.config` forwards the remote socket to `/tmp/op-tunnel.<username>.sock` using the `%r` token in `RemoteForward`. `SetEnv` continues to point at the stable symlink path (`~/.local/share/op-tunnel/client/op-tunnel.sock`), which `op-tunnel-setup` creates on the remote host. `op-tunnel-client` follows the symlink transparently via its existing tilde-expansion logic — no Go code changes required.

**Tech Stack:** Shell (bash/sh), OpenSSH `ssh_config(5)`

**Spec:** `docs/superpowers/specs/2026-03-14-client-socket-location-design.md`

---

## Chunk 1: Update packaging files

### Task 1: Update `packaging/ssh.config`

**Files:**
- Modify: `packaging/ssh.config`

Current content:
```
RemoteForward /tmp/op-tunnel-client.sock %d/.local/share/op-tunnel/server/op-tunnel.sock
SetEnv LC_OP_TUNNEL_SOCK=/tmp/op-tunnel-client.sock
StreamLocalBindUnlink yes
ServerAliveInterval 30
```

- [ ] **Step 1: Update `RemoteForward` and `SetEnv`**

Replace `packaging/ssh.config` with:

```
RemoteForward /tmp/op-tunnel.%r.sock %d/.local/share/op-tunnel/server/op-tunnel.sock
SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock
StreamLocalBindUnlink yes
ServerAliveInterval 30
```

Two changes:
- `RemoteForward` socket path: `/tmp/op-tunnel-client.sock` → `/tmp/op-tunnel.%r.sock` (`%r` expands to the remote login username)
- `SetEnv` value: `/tmp/op-tunnel-client.sock` → `~/.local/share/op-tunnel/client/op-tunnel.sock` (stable symlink path, `~/` expanded by `op-tunnel-client` at runtime)

- [ ] **Step 2: Verify no other references to old socket path remain**

```bash
grep -r 'op-tunnel-client\.sock' .
```

Expected: no matches (if any remain outside of test fixtures or docs, update them too).

- [ ] **Step 3: Commit**

```bash
git add packaging/ssh.config
git commit -m "fix: update ssh.config to use per-user socket path via %r"
```

---

### Task 2: Add symlink creation step to `packaging/op-tunnel-setup`

**Files:**
- Modify: `packaging/op-tunnel-setup`

This adds a new Step 4 at the end of the script that creates the stable symlink on the **remote** host pointing to the per-user socket in `/tmp`.

- [ ] **Step 1: Add Step 4 to `op-tunnel-setup`**

Append the following block to `packaging/op-tunnel-setup`, immediately before the final `=== Setup complete ===` block:

```sh
# ─── Step 4: Create client socket symlink ─────────────────────────────────────
echo "=== Step 4: Create client socket symlink ==="
echo ""
echo "Why: The SSH RemoteForward creates the socket at /tmp/op-tunnel.\$USER.sock."
echo "     A stable symlink at ~/.local/share/op-tunnel/client/op-tunnel.sock"
echo "     lets LC_OP_TUNNEL_SOCK use a fixed path that survives reboots."
echo "     The symlink is dangling when no SSH session is active — that is expected."
echo ""

SYMLINK_DIR="$HOME/.local/share/op-tunnel/client"
SYMLINK_TARGET="/tmp/op-tunnel.$USER.sock"
SYMLINK_PATH="$SYMLINK_DIR/op-tunnel.sock"

echo "Action: mkdir -p $SYMLINK_DIR"
mkdir -p "$SYMLINK_DIR"
echo "Action: ln -sf $SYMLINK_TARGET $SYMLINK_PATH"
ln -sf "$SYMLINK_TARGET" "$SYMLINK_PATH"
echo "Created: $SYMLINK_PATH -> $SYMLINK_TARGET"

echo ""
echo "Step 4 complete."
echo ""
```

Also update the final "Next steps" block to mention re-running `op-tunnel-setup` on the remote host if upgrading from an earlier version:

Find:
```sh
echo "Next steps:"
echo "  - Start the server:  brew services start op-tunnel"
echo "  - Test the tunnel:   ssh <remote-host> op --version"
```

Replace with:
```sh
echo "Next steps:"
echo "  - Start the server:  brew services start op-tunnel"
echo "  - Test the tunnel:   ssh <remote-host> op --version"
echo ""
echo "Upgrading from an earlier version? Re-run op-tunnel-setup on each remote"
echo "host to create the client socket symlink (Step 4)."
```

- [ ] **Step 2: Run `make test` to confirm no regressions**

```bash
make test
```

Expected: all tests pass. The existing tilde-expansion test in `cmd/op-tunnel-client/main_test.go` (which checks `~/.local/share/op-tunnel/client/op-tunnel.sock`) continues to pass unchanged.

- [ ] **Step 3: Smoke-test the updated setup script for syntax errors**

```bash
sh -n packaging/op-tunnel-setup
```

Expected: no output (clean parse).

- [ ] **Step 4: Commit**

```bash
git add packaging/op-tunnel-setup
git commit -m "fix: add client socket symlink creation step to op-tunnel-setup"
```
