# Streamline Setup Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move sockets to secure per-user directories, fix the client passthrough loop, and automate post-install setup.

**Architecture:** New shared `oppath` package provides socket path construction and binary lookup used by client, server, and doctor. Client switches from `LC_OP_TUNNEL_SOCK` env var to `LC_OP_TUNNEL_ID` + path construction. Setup script generates ssh.config from a template with `sed`. `/opt/op-tunnel/<user>/` replaces `/tmp/` for socket storage.

**Tech Stack:** Go 1.26, shell (bash/sh), sed, Homebrew (goreleaser)

**Spec:** `docs/superpowers/specs/2026-03-15-streamline-setup-design.md`

---

## Chunk 1: Shared Package and Core Logic

### Task 1: Create `oppath` package with path helpers

**Files:**
- Create: `oppath/oppath.go`
- Create: `oppath/oppath_test.go`

- [ ] **Step 1: Write tests for path helpers**

Create `oppath/oppath_test.go`:

```go
package oppath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientSocketPath(t *testing.T) {
	got := ClientSocketPath("alice", "abc123def456")
	want := "/opt/op-tunnel/alice/client/abc123def456.sock"
	if got != want {
		t.Errorf("ClientSocketPath = %q, want %q", got, want)
	}
}

func TestServerSocketPath(t *testing.T) {
	got := ServerSocketPath("bob")
	want := "/opt/op-tunnel/bob/server/op.sock"
	if got != want {
		t.Errorf("ServerSocketPath = %q, want %q", got, want)
	}
}

func TestConfigDir_Default(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	got := ConfigDir()
	want := filepath.Join(home, ".config", "op-tunnel")
	if got != want {
		t.Errorf("ConfigDir = %q, want %q", got, want)
	}
}

func TestConfigDir_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	got := ConfigDir()
	want := "/custom/config/op-tunnel"
	if got != want {
		t.Errorf("ConfigDir = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./oppath/...`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement `oppath.go`**

Create `oppath/oppath.go`:

```go
package oppath

import (
	"os"
	"path/filepath"
)

const (
	// BaseDir is the root directory for op-tunnel sockets.
	BaseDir = "/opt/op-tunnel"

	// EnvTunnelID is the environment variable set by SSH's SetEnv
	// to identify the local machine's tunnel.
	EnvTunnelID = "LC_OP_TUNNEL_ID"
)

// ClientSocketPath returns the path for a remote-forwarded client socket.
func ClientSocketPath(user, tunnelID string) string {
	return filepath.Join(BaseDir, user, "client", tunnelID+".sock")
}

// ServerSocketPath returns the path for the local op-tunnel-server socket.
func ServerSocketPath(user string) string {
	return filepath.Join(BaseDir, user, "server", "op.sock")
}

// ConfigDir returns the op-tunnel configuration directory,
// respecting $XDG_CONFIG_HOME (default ~/.config/op-tunnel).
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "op-tunnel")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "op-tunnel")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./oppath/...`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add oppath/
git commit -m "feat: add oppath package with socket path and config helpers"
```

---

### Task 2: Add `FindRealOp` to `oppath` with tests

**Files:**
- Modify: `oppath/oppath.go`
- Modify: `oppath/oppath_test.go`

- [ ] **Step 1: Write tests for FindRealOp**

Add to `oppath/oppath_test.go`:

```go
func TestFindRealOp_SkipsSelf(t *testing.T) {
	// Create temp dirs: one with "self" binary, one with symlink alias, one with real op
	tmpDir := t.TempDir()
	selfDir := filepath.Join(tmpDir, "self")
	aliasDir := filepath.Join(tmpDir, "alias")
	realDir := filepath.Join(tmpDir, "real")
	os.MkdirAll(selfDir, 0755)
	os.MkdirAll(aliasDir, 0755)
	os.MkdirAll(realDir, 0755)

	// Create a small executable as "self"
	selfBin := filepath.Join(selfDir, "op")
	os.WriteFile(selfBin, []byte("#!/bin/sh\n"), 0755)

	// Symlink alias in different dir pointing to self
	os.Symlink(selfBin, filepath.Join(aliasDir, "op"))

	// Create a different "real" op binary
	realBin := filepath.Join(realDir, "op")
	os.WriteFile(realBin, []byte("#!/bin/sh\necho real\n"), 0755)

	// PATH: alias dir first, then real dir
	path := aliasDir + string(os.PathListSeparator) + realDir

	got := FindRealOp(selfBin, path)
	if got != realBin {
		t.Errorf("FindRealOp = %q, want %q", got, realBin)
	}
}

func TestFindRealOp_NoneFound(t *testing.T) {
	tmpDir := t.TempDir()
	selfBin := filepath.Join(tmpDir, "op")
	os.WriteFile(selfBin, []byte("#!/bin/sh\n"), 0755)

	got := FindRealOp(selfBin, tmpDir)
	if got != "" {
		t.Errorf("FindRealOp = %q, want empty string", got)
	}
}

func TestFindRealOp_SkipsNonExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	selfDir := filepath.Join(tmpDir, "self")
	otherDir := filepath.Join(tmpDir, "other")
	os.MkdirAll(selfDir, 0755)
	os.MkdirAll(otherDir, 0755)

	selfBin := filepath.Join(selfDir, "op")
	os.WriteFile(selfBin, []byte("#!/bin/sh\n"), 0755)

	// Non-executable file named "op"
	os.WriteFile(filepath.Join(otherDir, "op"), []byte("data"), 0644)

	got := FindRealOp(selfBin, otherDir)
	if got != "" {
		t.Errorf("FindRealOp = %q, want empty string", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./oppath/... -run FindRealOp`
Expected: FAIL — `FindRealOp` not defined.

- [ ] **Step 3: Implement FindRealOp**

Add to `oppath/oppath.go`:

```go
// FindRealOp searches PATH for the real `op` binary, skipping any candidate
// that resolves to the same file as selfPath (via device+inode comparison).
// This prevents infinite loops when op-tunnel-client is symlinked as `op`.
func FindRealOp(selfPath, pathEnv string) string {
	selfInfo, err := os.Stat(selfPath)
	if err != nil {
		return ""
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "op")
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
			continue
		}
		if os.SameFile(selfInfo, info) {
			continue
		}
		return candidate
	}
	return ""
}
```

Note: `FindRealOp` takes `selfPath` and `pathEnv` as parameters rather than reading `os.Executable()` and `os.Getenv("PATH")` internally. This makes it testable without manipulating global state.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./oppath/... -run FindRealOp`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add oppath/
git commit -m "feat: add FindRealOp with os.SameFile to prevent symlink loops"
```

---

## Chunk 2: Update Protocol, Client, and Server

All three changes are made together so every commit leaves the codebase compilable.

### Task 3: Update protocol, server, and client in one pass

**Files:**
- Modify: `protocol/protocol.go`
- Modify: `cmd/op-tunnel-server/main.go`
- Modify: `cmd/op-tunnel-client/main.go`
- Modify: `cmd/op-tunnel-client/main_test.go`

- [ ] **Step 1: Run existing tests to confirm green baseline**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 2: Update `protocol/protocol.go`**

Remove constants `SocketName`, `EnvTunnelSock`, `ServerSocketDir`, `ClientSocketDir` and the `ExpandSocketPath` function. Keep everything else (protocol types, message framing, `FilterEnv`, `AllowedEnvVars`).

Note: `EnvTunnelID` is defined in `oppath`, not `protocol` — the protocol package deals with the wire format, while `oppath` owns path/env conventions.

- [ ] **Step 3: Update `cmd/op-tunnel-server/main.go`**

1. Add import `"github.com/middlendian/op-tunnel/oppath"`

2. Replace socket path resolution in `main()`:
   ```go
   // Old:
   socketPath, err := protocol.ExpandSocketPath(protocol.ServerSocketDir)
   if err != nil {
       log.Fatalf("resolving socket path: %v", err)
   }

   // New:
   user := os.Getenv("USER")
   if user == "" {
       log.Fatal("USER environment variable not set")
   }
   socketPath := oppath.ServerSocketPath(user)
   ```

3. Replace `exec.LookPath("op")` in `handleConnection()`:
   ```go
   // Old:
   opPath, err := exec.LookPath("op")
   if err != nil {
       resp := protocol.ErrorResponse("op binary not found in PATH")
       // ...
   }

   // New:
   self, _ := os.Executable()
   opPath := oppath.FindRealOp(self, os.Getenv("PATH"))
   if opPath == "" {
       resp := protocol.ErrorResponse("op binary not found in PATH")
       // ... (keep existing error response send logic, remove the err check)
   }
   ```

- [ ] **Step 4: Rewrite client tests**

Replace `cmd/op-tunnel-client/main_test.go` (removes `TestExpandTilde*`, adds new tests):

```go
package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/middlendian/op-tunnel/oppath"
)

func TestClientSocketPath(t *testing.T) {
	// Verify the client constructs the correct socket path from env vars
	t.Setenv(oppath.EnvTunnelID, "abcdef1234567890abcdef1234567890")
	t.Setenv("USER", "testuser")

	got := oppath.ClientSocketPath(os.Getenv("USER"), os.Getenv(oppath.EnvTunnelID))
	want := "/opt/op-tunnel/testuser/client/abcdef1234567890abcdef1234567890.sock"
	if got != want {
		t.Errorf("socket path = %q, want %q", got, want)
	}
}

func TestTunnelMode_DialFailure(t *testing.T) {
	// When LC_OP_TUNNEL_ID is set but socket doesn't exist,
	// DialTimeout should fail immediately (not hang).
	sockPath := filepath.Join(t.TempDir(), "nonexistent.sock")
	_, err := net.DialTimeout("unix", sockPath, 100*1e6) // 100ms
	if err == nil {
		t.Fatal("expected dial to fail on nonexistent socket")
	}
}

func TestTunnelMode_DialSuccess(t *testing.T) {
	// When socket exists and is listening, DialTimeout should succeed.
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	conn, err := net.DialTimeout("unix", sockPath, 100*1e6)
	if err != nil {
		t.Fatalf("expected dial to succeed: %v", err)
	}
	conn.Close()
}

func TestFindRealOp_ClientIntegration(t *testing.T) {
	// Verify FindRealOp skips symlink aliases and finds the real binary
	tmpDir := t.TempDir()
	selfDir := filepath.Join(tmpDir, "self")
	aliasDir := filepath.Join(tmpDir, "alias")
	realDir := filepath.Join(tmpDir, "real")
	os.MkdirAll(selfDir, 0755)
	os.MkdirAll(aliasDir, 0755)
	os.MkdirAll(realDir, 0755)

	selfBin := filepath.Join(selfDir, "op")
	os.WriteFile(selfBin, []byte("#!/bin/sh\n"), 0755)
	os.Symlink(selfBin, filepath.Join(aliasDir, "op"))

	realBin := filepath.Join(realDir, "op")
	os.WriteFile(realBin, []byte("#!/bin/sh\necho real\n"), 0755)

	path := aliasDir + string(os.PathListSeparator) + realDir
	got := oppath.FindRealOp(selfBin, path)
	if got != realBin {
		t.Errorf("FindRealOp = %q, want %q", got, realBin)
	}
}
```

- [ ] **Step 5: Rewrite client `main.go`**

Update `cmd/op-tunnel-client/main.go`:

1. Remove the `expandTilde` function entirely.
2. Remove the `findRealOp` function entirely.

3. Update imports:
   ```go
   import (
       "encoding/base64"
       "fmt"
       "net"
       "os"
       "os/signal"
       "syscall"
       "time"

       "golang.org/x/term"

       "github.com/middlendian/op-tunnel/oppath"
       "github.com/middlendian/op-tunnel/protocol"
   )
   ```

4. Replace `main()`:
   ```go
   func main() {
       tunnelID := os.Getenv(oppath.EnvTunnelID)
       if tunnelID != "" {
           user := os.Getenv("USER")
           if user == "" {
               fmt.Fprintln(os.Stderr, "op-tunnel: USER environment variable not set")
               os.Exit(1)
           }
           sockPath := oppath.ClientSocketPath(user, tunnelID)
           conn, err := net.DialTimeout("unix", sockPath, 100*time.Millisecond)
           if err != nil {
               fmt.Fprintln(os.Stderr, "op-tunnel: tunnel not connected")
               os.Exit(1)
           }
           tunnelMode(conn, os.Args[1:])
       } else {
           passthroughMode(os.Args[1:])
       }
   }
   ```

5. Replace `tunnelMode` — change signature from `(sockPath string, args []string)` to `(conn net.Conn, args []string)` and remove the `net.Dial` call since the connection is already open:
   ```go
   func tunnelMode(conn net.Conn, args []string) {
       // Close connection on signal
       sigCh := make(chan os.Signal, 1)
       signal.Notify(sigCh, syscall.SIGINT, syscall.SIGPIPE)
       go func() {
           <-sigCh
           _ = conn.Close()
           os.Exit(1)
       }()

       req := &protocol.Request{
           V:    protocol.ProtocolVersion,
           Args: args,
           Env:  protocol.FilterEnv(),
           TTY:  term.IsTerminal(int(os.Stdout.Fd())),
       }

       if err := protocol.SendRequest(conn, req); err != nil {
           fmt.Fprintf(os.Stderr, "op-tunnel: sending request: %v\n", err)
           os.Exit(1)
       }

       resp, err := protocol.ReadResponse(conn)
       if err != nil {
           fmt.Fprintf(os.Stderr, "op-tunnel: reading response: %v\n", err)
           os.Exit(1)
       }

       _ = conn.Close()

       if resp.Error != "" {
           fmt.Fprintf(os.Stderr, "op-tunnel: %s\n", resp.Error)
           os.Exit(1)
       }

       if resp.Stdout != "" {
           decoded, err := base64.StdEncoding.DecodeString(resp.Stdout)
           if err != nil {
               fmt.Fprintf(os.Stderr, "op-tunnel: decoding stdout: %v\n", err)
               os.Exit(1)
           }
           if _, err := os.Stdout.Write(decoded); err != nil {
               fmt.Fprintf(os.Stderr, "op-tunnel: writing stdout: %v\n", err)
               os.Exit(1)
           }
       }

       if resp.Stderr != "" {
           decoded, err := base64.StdEncoding.DecodeString(resp.Stderr)
           if err != nil {
               fmt.Fprintf(os.Stderr, "op-tunnel: decoding stderr: %v\n", err)
               os.Exit(1)
           }
           _, _ = os.Stderr.Write(decoded)
       }

       os.Exit(resp.ExitCode)
   }
   ```

6. Replace `passthroughMode`:
   ```go
   func passthroughMode(args []string) {
       self, err := os.Executable()
       if err != nil {
           fmt.Fprintln(os.Stderr, "op-tunnel: cannot determine own path")
           os.Exit(1)
       }
       realOp := oppath.FindRealOp(self, os.Getenv("PATH"))
       if realOp == "" {
           fmt.Fprintln(os.Stderr, "op-tunnel: op not found (install 1Password CLI or connect a tunnel)")
           os.Exit(1)
       }

       argv := append([]string{"op"}, args...)
       if err := syscall.Exec(realOp, argv, os.Environ()); err != nil {
           fmt.Fprintf(os.Stderr, "op-tunnel: exec %s: %v\n", realOp, err)
           os.Exit(1)
       }
   }
   ```

- [ ] **Step 6: Fix any protocol test references**

Run: `go test ./protocol/...`

Remove any tests that reference `ExpandSocketPath` or `EnvTunnelSock`. Protocol tests for `FilterEnv`, `AllowedEnvVars`, and wire format should still pass unchanged.

- [ ] **Step 7: Verify full build and test suite**

Run: `go build ./cmd/op-tunnel-server && go build ./cmd/op-tunnel-client && go test ./...`
Expected: All compile, all tests pass.

- [ ] **Step 8: Commit all together**

```bash
git add protocol/ cmd/op-tunnel-server/ cmd/op-tunnel-client/
git commit -m "feat: switch to LC_OP_TUNNEL_ID with oppath, fix findRealOp symlink loop

- Remove socket path constants from protocol (moved to oppath)
- Server uses oppath.FindRealOp instead of exec.LookPath
- Client uses LC_OP_TUNNEL_ID + oppath.ClientSocketPath
- Client tunnelMode takes pre-connected net.Conn
- Remove expandTilde (no longer needed)
- Fix infinite loop: os.SameFile replaces directory comparison"
```

---

## Chunk 3: Packaging and Setup

### Task 6: Create ssh.config template

**Files:**
- Create: `packaging/ssh.config.tmpl`
- Remove: `packaging/ssh.config`

- [ ] **Step 1: Create the template file**

Create `packaging/ssh.config.tmpl`:

```
# Auto-generated by op-tunnel-setup. Do not edit manually.
# Delete ~/.config/op-tunnel/ and re-run op-tunnel-setup to regenerate.
RemoteForward /opt/op-tunnel/%r/client/@@TUNNEL_ID@@.sock /opt/op-tunnel/@@LOCAL_USER@@/server/op.sock
SetEnv LC_OP_TUNNEL_ID=@@TUNNEL_ID@@
StreamLocalBindUnlink yes
ServerAliveInterval 30
```

- [ ] **Step 2: Remove old static ssh.config**

```bash
git rm packaging/ssh.config
```

- [ ] **Step 3: Update sshd.conf**

Replace contents of `packaging/op-tunnel-sshd.conf` with:

```
AcceptEnv LC_OP_TUNNEL_ID
StreamLocalBindUnlink yes
```

- [ ] **Step 4: Commit**

```bash
git add packaging/ssh.config.tmpl packaging/op-tunnel-sshd.conf
git commit -m "feat: replace static ssh.config with sed template, simplify sshd.conf"
```

---

### Task 7: Rewrite `op-tunnel-setup`

**Files:**
- Modify: `packaging/op-tunnel-setup`

- [ ] **Step 1: Rewrite the setup script**

Replace `packaging/op-tunnel-setup` with the new version. The script must:

1. Locate `ssh.config.tmpl` (Homebrew layout: `../share/op-tunnel/`, tarball layout: sibling)
2. Create `~/.config/op-tunnel/` (respecting `$XDG_CONFIG_HOME`)
3. Generate tunnel ID if `tunnel-id` file doesn't exist: `head -c 16 /dev/urandom | xxd -p -c 32`
4. Substitute `@@TUNNEL_ID@@` and `@@LOCAL_USER@@` via `sed`, write to config dir
5. Create `~/.local/bin/op` symlink
6. Print sudo explanation, run privileged steps:
   - `sudo mkdir -p /opt/op-tunnel/$USER`
   - `sudo chown $USER /opt/op-tunnel/$USER`
   - `sudo chmod 700 /opt/op-tunnel/$USER`
   - `sudo install -m 644 op-tunnel-sshd.conf /etc/ssh/sshd_config.d/op-tunnel.conf`
   - Reload sshd (systemctl on Linux, launchctl on macOS)
7. Create client/server subdirs (no sudo)
8. Print colored caveats (SSH config Include instruction, PATH warning)

Use ANSI colors only when stdout is a terminal (`[ -t 1 ]`).

Exit with clear error if sudo fails.

The full script:

```sh
#!/bin/sh
set -e

# ─── Helpers ──────────────────────────────────────────────────────────────────

# Colors (only when stdout is a terminal)
if [ -t 1 ]; then
    BOLD='\033[1m'
    YELLOW='\033[1;33m'
    GREEN='\033[1;32m'
    RED='\033[1;31m'
    RESET='\033[0m'
else
    BOLD='' YELLOW='' GREEN='' RED='' RESET=''
fi

info()  { printf "${GREEN}✓${RESET} %s\n" "$1"; }
warn()  { printf "${YELLOW}!${RESET} %s\n" "$1"; }
fail()  { printf "${RED}✗${RESET} %s\n" "$1"; exit 1; }

# Locate a file shipped alongside this script.
find_dist_file() {
    name="$1"
    brew_path="$(dirname "$0")/../share/op-tunnel/$name"
    tarball_path="$(dirname "$0")/$name"
    if [ -f "$brew_path" ]; then
        echo "$brew_path"
    elif [ -f "$tarball_path" ]; then
        echo "$tarball_path"
    else
        fail "Cannot find $name (looked in share/op-tunnel/ and next to this script)"
    fi
}

# ─── Configuration ───────────────────────────────────────────────────────────

CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/op-tunnel"
TUNNEL_ID_FILE="$CONFIG_DIR/tunnel-id"
SSH_CONFIG_FILE="$CONFIG_DIR/ssh.config"

echo ""
echo "${BOLD}=== op-tunnel setup ===${RESET}"
echo ""

# ─── Step 1: Generate tunnel ID ──────────────────────────────────────────────

mkdir -p "$CONFIG_DIR"

if [ -f "$TUNNEL_ID_FILE" ]; then
    TUNNEL_ID="$(cat "$TUNNEL_ID_FILE")"
    info "Tunnel ID: $TUNNEL_ID (existing)"
else
    TUNNEL_ID="$(head -c 16 /dev/urandom | xxd -p -c 32)"
    printf '%s' "$TUNNEL_ID" > "$TUNNEL_ID_FILE"
    info "Tunnel ID: $TUNNEL_ID (generated)"
fi

# ─── Step 2: Generate ssh.config ─────────────────────────────────────────────

TEMPLATE="$(find_dist_file ssh.config.tmpl)"

sed -e "s/@@TUNNEL_ID@@/$TUNNEL_ID/g" \
    -e "s/@@LOCAL_USER@@/$USER/g" \
    "$TEMPLATE" > "$SSH_CONFIG_FILE"
info "Generated: $SSH_CONFIG_FILE"

# ─── Step 3: Create op symlink ───────────────────────────────────────────────

CLIENT_BIN="$(command -v op-tunnel-client 2>/dev/null || true)"
if [ -z "$CLIENT_BIN" ]; then
    warn "op-tunnel-client not found in PATH. Skipping symlink."
else
    mkdir -p "$HOME/.local/bin"
    ln -sf "$CLIENT_BIN" "$HOME/.local/bin/op"
    info "Symlink: ~/.local/bin/op -> $CLIENT_BIN"
fi

# ─── Step 4: Privileged setup (sudo) ─────────────────────────────────────────

echo ""
echo "${BOLD}Administrator access required (one-time setup)${RESET}"
echo "Creating private socket directory and configuring sshd."
echo ""

if ! sudo -v 2>/dev/null; then
    fail "Setup cannot continue without administrator access. The socket directory and sshd configuration are required for op-tunnel to function."
fi

sudo mkdir -p "/opt/op-tunnel/$USER"
sudo chown "$USER" "/opt/op-tunnel/$USER"
sudo chmod 700 "/opt/op-tunnel/$USER"
info "Directory: /opt/op-tunnel/$USER/ (owner-only)"

SSHD_CONF="$(find_dist_file op-tunnel-sshd.conf)"
sudo install -m 644 "$SSHD_CONF" /etc/ssh/sshd_config.d/op-tunnel.conf
info "sshd config: /etc/ssh/sshd_config.d/op-tunnel.conf"

# Reload sshd
if command -v systemctl >/dev/null 2>&1; then
    sudo systemctl reload sshd 2>/dev/null || sudo systemctl reload ssh 2>/dev/null || true
    info "sshd reloaded (systemctl)"
elif sudo launchctl kickstart -k system/com.openssh.sshd 2>/dev/null; then
    info "sshd reloaded (launchctl)"
else
    warn "Could not reload sshd automatically. Please reload it manually."
fi

# ─── Step 5: Create subdirectories (no sudo) ─────────────────────────────────

mkdir -p "/opt/op-tunnel/$USER/client" "/opt/op-tunnel/$USER/server"
info "Subdirectories: client/ server/"

# ─── Caveats ──────────────────────────────────────────────────────────────────

echo ""
echo "${BOLD}=== Next steps ===${RESET}"
echo ""

printf "${YELLOW}!${RESET} ${BOLD}SSH config:${RESET} Add this to ${BOLD}~/.ssh/config${RESET} for each remote host:\n"
echo ""
echo "    Host my-server"
echo "        Include ~/.config/op-tunnel/ssh.config"
echo ""

# Check if ~/.local/bin is in PATH
case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *)
        printf "${YELLOW}!${RESET} ${BOLD}PATH:${RESET} ~/.local/bin is not in your PATH. Add it:\n"
        case "$SHELL" in
            */zsh)  echo "    echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc" ;;
            */fish) echo "    echo 'fish_add_path \$HOME/.local/bin' >> ~/.config/fish/config.fish" ;;
            *)      echo "    echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc" ;;
        esac
        echo ""
        ;;
esac

echo "${GREEN}Setup complete.${RESET}"
echo ""
```

- [ ] **Step 2: Make it executable**

```bash
chmod +x packaging/op-tunnel-setup
```

- [ ] **Step 3: Commit**

```bash
git add packaging/op-tunnel-setup
git commit -m "feat: rewrite op-tunnel-setup as automated post-install script"
```

---

### Task 8: Update `.goreleaser.yaml`

**Files:**
- Modify: `.goreleaser.yaml`

- [ ] **Step 1: Add op-tunnel-doctor build**

Add a new build entry after the `op-tunnel-client` build:

```yaml
  - id: op-tunnel-doctor
    main: ./cmd/op-tunnel-doctor
    binary: op-tunnel-doctor
    env:
      - CGO_ENABLED=0
    ldflags:
      - -s -w
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
```

- [ ] **Step 2: Update archives to include op-tunnel-doctor and new template**

Update the `archives` section `ids` to include `op-tunnel-doctor`. Update `files` to include `ssh.config.tmpl` instead of `ssh.config`:

```yaml
archives:
  - id: op-tunnel
    ids:
      - op-tunnel-server
      - op-tunnel-client
      - op-tunnel-doctor
    # ...
    files:
      - src: LICENSE
      - src: packaging/ssh.config.tmpl
        dst: dist
        strip_parent: true
      - src: packaging/op-tunnel-sshd.conf
        dst: dist
        strip_parent: true
      - src: packaging/op-tunnel-setup
        dst: dist
        strip_parent: true
```

- [ ] **Step 3: Update brews section**

Update `install`, `post_install`, and `caveats`:

```yaml
    install: |
      bin.install "op-tunnel-server"
      bin.install "op-tunnel-client"
      bin.install "op-tunnel-doctor"
      (share/"op-tunnel").install "dist/op-tunnel-sshd.conf", "dist/ssh.config.tmpl"
      bin.install "dist/op-tunnel-setup"
    post_install: |
      system bin/"op-tunnel-setup"
    caveats: |
      op-tunnel-setup has been run automatically.

      If you need to reconfigure, run:
        op-tunnel-setup

      See https://github.com/middlendian/op-tunnel for full instructions.
```

- [ ] **Step 4: Add `op-tunnel-doctor` to the `test` block**

Add to the `test` section of the brews config:
```yaml
    test: |
      assert_predicate bin/"op-tunnel-server", :executable?
      assert_predicate bin/"op-tunnel-client", :executable?
      assert_predicate bin/"op-tunnel-doctor", :executable?
```

- [ ] **Step 5: Run goreleaser check**

Run: `goreleaser check 2>&1 | head -5`
Expected: No errors (deprecation warning about `brews` is expected and intentional).

- [ ] **Step 6: Commit**

```bash
git add .goreleaser.yaml
git commit -m "feat: add op-tunnel-doctor build, post_install hook, update packaging"
```

---

### Task 9: Update Makefile

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Update Makefile**

1. Add `op-tunnel-doctor` to the `build` target
2. Add `op-tunnel-doctor` to `build-mac` and `build-linux` targets (all arch variants)
3. Remove the `DATADIR` variable and `install-ssh-config` target (ssh.config is now generated by `op-tunnel-setup`)
4. Update `.PHONY` list

Changes to `build`:

```makefile
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-server ./cmd/op-tunnel-server
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-client ./cmd/op-tunnel-client
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-doctor ./cmd/op-tunnel-doctor
```

Add to `build-mac` (both arm64 and amd64 stanzas):
```makefile
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-arm64/op-tunnel-doctor ./cmd/op-tunnel-doctor
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/darwin-amd64/op-tunnel-doctor ./cmd/op-tunnel-doctor
```

Add to `build-linux` (both arm64 and amd64 stanzas):
```makefile
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-arm64/op-tunnel-doctor ./cmd/op-tunnel-doctor
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/linux-amd64/op-tunnel-doctor ./cmd/op-tunnel-doctor
```

Remove the `DATADIR` and `install-ssh-config` lines entirely. Update `.PHONY` to remove `install-ssh-config`.

- [ ] **Step 2: Run make build (will fail until doctor exists)**

Run: `make build`
Expected: Fails on `op-tunnel-doctor` — built in next task. Server and client should compile.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "refactor: update Makefile for op-tunnel-doctor, remove install-ssh-config"
```

---

## Chunk 4: op-tunnel-doctor

### Task 10: Implement `op-tunnel-doctor`

**Files:**
- Create: `cmd/op-tunnel-doctor/main.go`

- [ ] **Step 1: Create the doctor binary**

Create `cmd/op-tunnel-doctor/main.go`:

```go
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/middlendian/op-tunnel/oppath"
)

// ANSI colors
var (
	green  = "\033[1;32m"
	red    = "\033[1;31m"
	yellow = "\033[1;33m"
	bold   = "\033[1m"
	reset  = "\033[0m"
)

func init() {
	if os.Getenv("NO_COLOR") != "" || !isTerminal() {
		green, red, yellow, bold, reset = "", "", "", "", ""
	}
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func pass(msg string)            { fmt.Printf("%s✓%s %s\n", green, reset, msg) }
func fail(msg, fix string)       { fmt.Printf("%s✗%s %s\n  → %s%s%s\n", red, reset, msg, yellow, fix, reset) }
func warn(msg, fix string)       { fmt.Printf("%s!%s %s\n  → %s%s%s\n", yellow, reset, msg, yellow, fix, reset) }

func main() {
	fmt.Printf("\n%sop-tunnel doctor%s\n\n", bold, reset)

	user := os.Getenv("USER")
	failures := 0

	// 1. Server socket
	serverSock := oppath.ServerSocketPath(user)
	if conn, err := net.DialTimeout("unix", serverSock, 100*time.Millisecond); err == nil {
		conn.Close()
		pass("Server is running")
	} else {
		fail("Server is not running", "brew services restart op-tunnel")
		failures++
	}

	// 2. Client socket (only if tunnel ID set)
	tunnelID := os.Getenv(oppath.EnvTunnelID)
	if tunnelID != "" {
		clientSock := oppath.ClientSocketPath(user, tunnelID)
		if conn, err := net.DialTimeout("unix", clientSock, 100*time.Millisecond); err == nil {
			conn.Close()
			pass("Client tunnel is active")
		} else {
			fail("Client tunnel is not connected", "Reconnect your SSH session")
			failures++
		}
	} else {
		pass("Client tunnel check skipped (not in SSH session)")
	}

	// 3. Real op binary
	self, _ := os.Executable()
	realOp := oppath.FindRealOp(self, os.Getenv("PATH"))
	if realOp != "" {
		pass(fmt.Sprintf("Real op binary: %s", realOp))
	} else {
		fail("Real op binary not found", "brew install --cask 1password-cli")
		failures++
	}

	// 4. Symlink
	symlink := filepath.Join(os.Getenv("HOME"), ".local", "bin", "op")
	if target, err := os.Readlink(symlink); err == nil {
		if clientPath, err := exec.LookPath("op-tunnel-client"); err == nil {
			resolved, _ := filepath.EvalSymlinks(symlink)
			resolvedClient, _ := filepath.EvalSymlinks(clientPath)
			if resolved == resolvedClient {
				pass(fmt.Sprintf("Symlink: %s -> %s", symlink, target))
			} else {
				fail(fmt.Sprintf("Symlink points to wrong target: %s", target), "brew reinstall op-tunnel")
				failures++
			}
		} else {
			fail("op-tunnel-client not found in PATH", "brew reinstall op-tunnel")
			failures++
		}
	} else {
		fail("~/.local/bin/op symlink missing", "brew reinstall op-tunnel")
		failures++
	}

	// 5. PATH
	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	localBin := filepath.Join(os.Getenv("HOME"), ".local", "bin")
	inPath := false
	for _, d := range pathDirs {
		if d == localBin {
			inPath = true
			break
		}
	}
	if inPath {
		pass("~/.local/bin is in PATH")
	} else {
		warn("~/.local/bin is not in PATH", "Add 'export PATH=\"$HOME/.local/bin:$PATH\"' to your shell rc file")
		failures++
	}

	// 6. SSH config
	sshConfig := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
	if data, err := os.ReadFile(sshConfig); err == nil {
		if strings.Contains(string(data), "op-tunnel/ssh.config") {
			pass("SSH config includes op-tunnel")
		} else {
			fail("SSH config does not include op-tunnel", "Add 'Include ~/.config/op-tunnel/ssh.config' under a Host block in ~/.ssh/config")
			failures++
		}
	} else {
		fail("~/.ssh/config not found", "Create ~/.ssh/config with op-tunnel Include")
		failures++
	}

	// 7. sshd drop-in
	sshdConf := "/etc/ssh/sshd_config.d/op-tunnel.conf"
	if _, err := os.Stat(sshdConf); err == nil {
		pass("sshd drop-in installed")
	} else {
		fail("sshd drop-in missing", "brew reinstall op-tunnel")
		failures++
	}

	// 8. Directory permissions
	userDir := filepath.Join(oppath.BaseDir, user)
	if info, err := os.Stat(userDir); err == nil {
		if info.Mode().Perm() == 0700 {
			pass(fmt.Sprintf("Directory permissions: %s (0700)", userDir))
		} else {
			fail(fmt.Sprintf("Directory permissions: %s (%04o, want 0700)", userDir, info.Mode().Perm()), "brew reinstall op-tunnel")
			failures++
		}
	} else {
		fail(fmt.Sprintf("Directory missing: %s", userDir), "brew reinstall op-tunnel")
		failures++
	}

	// 9. Config directory
	configDir := oppath.ConfigDir()
	tunnelIDFile := filepath.Join(configDir, "tunnel-id")
	sshConfigFile := filepath.Join(configDir, "ssh.config")
	if _, err := os.Stat(tunnelIDFile); err == nil {
		if _, err := os.Stat(sshConfigFile); err == nil {
			pass(fmt.Sprintf("Config: %s", configDir))
		} else {
			fail("ssh.config missing from config dir", "brew reinstall op-tunnel")
			failures++
		}
	} else {
		fail("tunnel-id missing from config dir", "brew reinstall op-tunnel")
		failures++
	}

	// Summary
	fmt.Println()
	if failures == 0 {
		fmt.Printf("%s✓ All checks passed.%s\n\n", green, reset)
	} else {
		fmt.Printf("%s✗ %d issue(s) found.%s\n\n", red, failures, reset)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/op-tunnel-doctor`
Expected: Success.

- [ ] **Step 3: Run make build to verify all three binaries compile**

Run: `make build`
Expected: Success — all three binaries (`op-tunnel-server`, `op-tunnel-client`, `op-tunnel-doctor`) built.

- [ ] **Step 4: Run full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/op-tunnel-doctor/
git commit -m "feat: add op-tunnel-doctor diagnostic tool"
```

---

## Chunk 5: Documentation and Cleanup

### Task 11: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update key references in CLAUDE.md**

1. Update the architecture diagram: change `LC_OP_TUNNEL_SOCK` to `LC_OP_TUNNEL_ID`
2. Update the key files table:
   - Change `packaging/ssh.config` to `packaging/ssh.config.tmpl` with updated description
   - Add `oppath/oppath.go` — shared socket path construction and binary lookup
   - Add `cmd/op-tunnel-doctor/main.go` — diagnostic tool
3. Update Wire Protocol section: note that `LC_OP_TUNNEL_ID` replaces `LC_OP_TUNNEL_SOCK`
4. Update conventions:
   - Remove tilde expansion bullet
   - Update socket permissions to reference `/opt/op-tunnel/<user>/`
   - Update env var reference to `LC_OP_TUNNEL_ID`
5. Update build commands: add `op-tunnel-doctor` to build output list

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for new socket paths and env var"
```

---

### Task 12: Update README.md

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README**

1. Update install instructions: change `Include` path from `~/.local/share/op-tunnel/ssh.config` to `~/.config/op-tunnel/ssh.config`
2. Update "How it works" section: reference `LC_OP_TUNNEL_ID` instead of "remote socket is active"
3. Update architecture diagram if it references old paths
4. Mention `op-tunnel-doctor` as a diagnostic tool

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update README for new socket paths and setup flow"
```

---

### Task 13: Update e2e test

**Files:**
- Modify: `test/e2e.sh`

- [ ] **Step 1: Review current e2e.sh for references to update**

Read `test/e2e.sh` and identify all references to:
- `LC_OP_TUNNEL_SOCK`
- `~/.local/share/op-tunnel/`
- `/tmp/op-tunnel` socket paths
- `op-tunnel-sshd.conf` content expectations

- [ ] **Step 2: Update e2e.sh**

Update all references to use the new paths and env vars:
- `LC_OP_TUNNEL_SOCK` → `LC_OP_TUNNEL_ID`
- Socket paths → `/opt/op-tunnel/<user>/...`
- SSH config → generated from template
- sshd config → new simplified content

Also update the Docker test setup to create `/opt/op-tunnel/<user>/` directories.

- [ ] **Step 3: Commit**

```bash
git add test/
git commit -m "test: update e2e test for new socket paths and env vars"
```

---

### Task 14: Final verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 2: Run lint**

Run: `make lint`
Expected: PASS (or only pre-existing warnings).

- [ ] **Step 3: Run goreleaser check**

Run: `goreleaser check 2>&1 | head -10`
Expected: No new errors.

- [ ] **Step 4: Verify all three binaries build cleanly**

Run: `make clean && make build`
Expected: Three binaries in `bin/`.

- [ ] **Step 5: Final commit if any cleanup needed**

```bash
git status
# If any unstaged changes, commit them
```
