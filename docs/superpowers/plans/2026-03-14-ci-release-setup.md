# CI/CD, Release Pipeline, and op-tunnel-setup Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add GitHub Actions CI/CD, GoReleaser-based cross-platform releases with Homebrew tap auto-update, and rewrite op-tunnel-setup as an explicit three-step setup script.

**Architecture:** GoReleaser builds two binaries per platform into a single tarball (with packaging files in `dist/` inside the tarball), publishes GitHub releases, and auto-updates the Homebrew tap formula. CI runs on PRs via golangci-lint + go test. The setup script runs on any machine and walks through three explicit, idempotent steps with granular sudo.

**Tech Stack:** Go 1.26.1, GoReleaser v2, GitHub Actions, golangci-lint, POSIX sh

**Spec:** `docs/superpowers/specs/2026-03-14-ci-release-setup-design.md`

---

## Chunk 1: Repository Layout + Go Version Bump

**Files:**
- Delete: `dist/com.middlendian.op-tunnel-server.plist`
- Delete: `dist/op-tunnel-server.service`
- Delete: `dist/op-tunnel.rb` (formula moves to tap repo, managed by GoReleaser)
- Rename: `dist/` → `packaging/`
- Modify: `Makefile` — update `dist/ssh.config` reference to `packaging/ssh.config`
- Modify: `go.mod` — bump Go version
- Modify: `.gitignore` — add `_dist/`
- Modify: `README.md` — update any `dist/` path references

### Task 1: Remove obsolete files

These files are replaced by `brew services` (plist/service) and GoReleaser tap management (formula).

- [ ] **Step 1: Delete the three obsolete files**

```bash
git rm dist/com.middlendian.op-tunnel-server.plist
git rm dist/op-tunnel-server.service
git rm dist/op-tunnel.rb
```

- [ ] **Step 2: Commit**

```bash
git commit -m "chore: remove plist, service, and formula files (replaced by brew services + goreleaser)"
```

### Task 2: Rename dist/ → packaging/

- [ ] **Step 1: Rename the directory**

```bash
git mv dist packaging
```

- [ ] **Step 2: Update Makefile**

In `Makefile`, change the `install-ssh-config` target:

```makefile
install-ssh-config:
	mkdir -p $(DATADIR)
	cp packaging/ssh.config $(DATADIR)/ssh.config
```

- [ ] **Step 3: Check README for dist/ references and update them**

```bash
grep -n "dist/" README.md
```

Replace any occurrence of `dist/` with `packaging/` where it refers to source files (not build output).

- [ ] **Step 4: Add _dist/ to .gitignore**

Append to `.gitignore`:
```
_dist/
```

- [ ] **Step 5: Verify nothing else references the old path**

```bash
grep -r "dist/" --include="*.go" --include="*.md" --include="Makefile" --include="*.sh" .
```

Review output. Any remaining `dist/` references in source files should be updated to `packaging/`.

- [ ] **Step 6: Commit**

```bash
git add Makefile .gitignore README.md
git commit -m "chore: rename dist/ to packaging/, add _dist/ to gitignore"
```

### Task 3: Bump Go version

- [ ] **Step 1: Update go.mod**

Change the `go` directive in `go.mod`:
```
go 1.26.1
```

- [ ] **Step 2: Update go.sum**

```bash
go mod tidy
```

Expected: no errors. `go.sum` may have updated entries.

- [ ] **Step 3: Verify build still works**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: bump Go version to 1.26.1"
```

---

## Chunk 2: Linting + CI Workflow

**Files:**
- Create: `.golangci.yml`
- Create: `.github/workflows/ci.yml`

### Task 4: Create golangci-lint config

- [ ] **Step 1: Create `.golangci.yml`**

```yaml
version: "2"

linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - deadcode
    - gofmt

linters-settings:
  gofmt:
    simplify: true
```

> Note: `unused` was renamed to `deadcode` in golangci-lint v2. If `deadcode` causes an "unknown linter" error, remove it — `staticcheck` covers most dead-code detection.

- [ ] **Step 2: If golangci-lint is installed locally, validate the config**

```bash
golangci-lint run ./...
```

Expected: either passes or shows real lint issues to fix. If golangci-lint is not installed locally, skip this step — CI will catch it.

- [ ] **Step 3: Commit**

```bash
git add .golangci.yml
git commit -m "chore: add golangci-lint config (errcheck, govet, staticcheck, deadcode, gofmt)"
```

### Task 5: Create CI workflow

- [ ] **Step 1: Create `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  pull_request:
  push:
    branches:
      - main

jobs:
  test:
    name: Test and Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26.1"
          cache: true

      - name: Run tests
        run: go test ./...

      - name: Run integration tests
        run: go test -tags integration ./...

      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v1.64.0
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add GitHub Actions CI workflow (test + golangci-lint)"
```

---

## Chunk 3: GoReleaser + Release Workflow

**Files:**
- Create: `.goreleaser.yaml`
- Create: `.github/workflows/release.yml`

### Task 6: Create GoReleaser config

The archives must contain both binaries plus the three `packaging/` files placed under `dist/` inside the tarball. This makes the relative paths in `op-tunnel-setup` consistent between Homebrew and tarball installations.

- [ ] **Step 1: Create `.goreleaser.yaml`**

```yaml
version: 2

project_name: op-tunnel
dist: _dist

before:
  hooks:
    - go mod tidy

builds:
  - id: op-tunnel-server
    main: ./cmd/op-tunnel-server
    binary: op-tunnel-server
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

  - id: op-tunnel-client
    main: ./cmd/op-tunnel-client
    binary: op-tunnel-client
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

archives:
  - id: op-tunnel
    builds:
      - op-tunnel-server
      - op-tunnel-client
    name_template: "op-tunnel-{{ .Version }}-{{ .Os }}-{{ .Arch }}"
    format: tar.gz
    files:
      - src: packaging/ssh.config
        dst: dist
        strip_parent: true
      - src: packaging/op-tunnel-sshd.conf
        dst: dist
        strip_parent: true
      - src: packaging/op-tunnel-setup
        dst: dist
        strip_parent: true

checksum:
  name_template: "checksums.txt"

brews:
  - name: op-tunnel
    repository:
      owner: middlendian
      name: homebrew-tap
      token: "{{ .Env.TAP_GITHUB_TOKEN }}"
    commit_author:
      name: goreleaserbot
      email: bot@goreleaser.com
    homepage: "https://github.com/middlendian/op-tunnel"
    description: "Tunnel 1Password CLI (op) commands over SSH"
    license: "MIT"
    dependencies:
      - name: 1password-cli
    service: |
      run [opt_bin/"op-tunnel-server"]
      keep_alive true
      log_path var/"log/op-tunnel-server.log"
      error_log_path var/"log/op-tunnel-server.log"
    install: |
      bin.install "op-tunnel-server"
      bin.install "op-tunnel-client"
      (share/"op-tunnel").install "dist/op-tunnel-sshd.conf", "dist/ssh.config"
      bin.install "dist/op-tunnel-setup"
    caveats: |
      Run op-tunnel-setup to complete configuration:
        op-tunnel-setup

      See https://github.com/middlendian/op-tunnel for full instructions.
    test: |
      assert_predicate bin/"op-tunnel-server", :executable?
      assert_predicate bin/"op-tunnel-client", :executable?
```

- [ ] **Step 2: Install goreleaser if not already installed**

```bash
brew install goreleaser
```

- [ ] **Step 3: Validate the GoReleaser config**

```bash
goreleaser check
```

Expected: `• config is valid` with no errors. Fix any validation errors before proceeding.

- [ ] **Step 4: Do a local snapshot build to verify archive layout**

```bash
goreleaser release --snapshot --clean
```

Expected: `_dist/` created with tarballs like `op-tunnel-0.0.0-next-darwin-arm64.tar.gz`. Inspect one tarball to verify both binaries and `dist/` files are present:

```bash
tar -tzf _dist/op-tunnel-*-darwin-arm64.tar.gz
```

Expected output should include:
```
op-tunnel-<version>-darwin-arm64/op-tunnel-server
op-tunnel-<version>-darwin-arm64/op-tunnel-client
op-tunnel-<version>-darwin-arm64/dist/ssh.config
op-tunnel-<version>-darwin-arm64/dist/op-tunnel-sshd.conf
op-tunnel-<version>-darwin-arm64/dist/op-tunnel-setup
```

If the layout is wrong (e.g., `packaging/` prefix instead of `dist/`), adjust the `files` section's `dst` and `strip_parent` values accordingly and re-run until correct.

- [ ] **Step 5: Clean up snapshot build**

```bash
rm -rf _dist/
```

- [ ] **Step 6: Commit**

```bash
git add .goreleaser.yaml
git commit -m "chore: add goreleaser config (cross-platform builds, homebrew tap)"
```

### Task 7: Create release workflow

- [ ] **Step 1: Create `.github/workflows/release.yml`**

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

jobs:
  release:
    name: GoReleaser
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26.1"
          cache: true

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAP_GITHUB_TOKEN: ${{ secrets.TAP_GITHUB_TOKEN }}
```

> **Required setup before first release:** Create a GitHub PAT with `contents: write` scope on `middlendian/homebrew-tap` and add it as a repo secret named `TAP_GITHUB_TOKEN` in `middlendian/op-tunnel` Settings → Secrets.

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow (goreleaser on tag push)"
```

---

## Chunk 4: op-tunnel-setup Rewrite

**Files:**
- Modify: `packaging/op-tunnel-setup` — full rewrite

### Task 8: Rewrite op-tunnel-setup

The script runs on any machine (local or remote) and performs three explicit, idempotent steps. Only Step 1 uses `sudo`.

- [ ] **Step 1: Replace `packaging/op-tunnel-setup` with the following**

```sh
#!/bin/sh
set -e

# Locate a file shipped alongside this script.
# Tries Homebrew layout first (script in bin/, file in share/op-tunnel/),
# then tarball layout (script and file are siblings inside dist/).
find_dist_file() {
    name="$1"
    brew_path="$(dirname "$0")/../share/op-tunnel/$name"
    tarball_path="$(dirname "$0")/$name"
    if [ -f "$brew_path" ]; then
        echo "$brew_path"
    elif [ -f "$tarball_path" ]; then
        echo "$tarball_path"
    else
        echo "Error: cannot find $name (looked in share/op-tunnel/ and next to this script)" >&2
        exit 1
    fi
}

echo ""
echo "=== op-tunnel setup ==="
echo ""

# ─── Step 1: Configure sshd ───────────────────────────────────────────────────
echo "=== Step 1: Configure sshd ==="
echo ""
echo "Why: SSH servers must allow clients to pass LC_OP_TUNNEL_SOCK."
echo "     This environment variable tells op-tunnel-client where to find"
echo "     the forwarded Unix socket."
echo ""

CONF_SRC="$(find_dist_file op-tunnel-sshd.conf)"
CONF_DST="/etc/ssh/sshd_config.d/op-tunnel.conf"

echo "Action: sudo install -m 644 $CONF_SRC $CONF_DST"
sudo install -m 644 "$CONF_SRC" "$CONF_DST"
echo "Written: $CONF_DST"

echo "Action: reload sshd"
if command -v systemctl >/dev/null 2>&1; then
    sudo systemctl reload sshd 2>/dev/null || sudo systemctl reload ssh
    echo "sshd reloaded via systemctl."
elif sudo launchctl kickstart -k system/com.openssh.sshd >/dev/null 2>&1; then
    echo "sshd reloaded via launchctl."
else
    echo "Note: could not reload sshd automatically. Please reload it manually."
fi

echo ""
echo "Step 1 complete."
echo ""

# ─── Step 2: Configure SSH client ─────────────────────────────────────────────
echo "=== Step 2: Configure SSH client ==="
echo ""
echo "Why: When you SSH to a remote host, your SSH client needs to forward"
echo "     the op-tunnel socket and set LC_OP_TUNNEL_SOCK in the remote session."
echo ""

printf "Which hosts should use op-tunnel? [default: *.local]: "
read -r HOST_PATTERN
if [ -z "$HOST_PATTERN" ]; then
    HOST_PATTERN="*.local"
fi

OP_TUNNEL_DIR="$HOME/.local/share/op-tunnel"
SSH_CONFIG="$HOME/.ssh/config"
INCLUDE_LINE="Include ~/.local/share/op-tunnel/ssh.config"

echo "Action: create socket directories"
mkdir -p "$OP_TUNNEL_DIR/server" "$OP_TUNNEL_DIR/client"
echo "Created: $OP_TUNNEL_DIR/{server,client}"

echo "Action: copy ssh.config"
SSH_TEMPLATE="$(find_dist_file ssh.config)"
cp "$SSH_TEMPLATE" "$OP_TUNNEL_DIR/ssh.config"
echo "Copied ssh.config to $OP_TUNNEL_DIR/ssh.config"

mkdir -p "$HOME/.ssh"
touch "$SSH_CONFIG"

if grep -qF "$INCLUDE_LINE" "$SSH_CONFIG" 2>/dev/null; then
    echo "~/.ssh/config already contains the op-tunnel Include. Skipping."
else
    printf "\nHost %s\n    %s\n" "$HOST_PATTERN" "$INCLUDE_LINE" >> "$SSH_CONFIG"
    echo "Added to ~/.ssh/config:"
    echo "  Host $HOST_PATTERN"
    echo "      $INCLUDE_LINE"
fi

echo ""
echo "Step 2 complete."
echo ""

# ─── Step 3: Shadow op with a symlink ─────────────────────────────────────────
echo "=== Step 3: Shadow 'op' with a symlink ==="
echo ""
echo "Why: op-tunnel-client proxies op commands over the tunnel when available,"
echo "     falling back to the real op binary. A symlink (unlike a shell alias)"
echo "     works reliably from scripts, Makefiles, IDEs, and other tools."
echo ""

CLIENT_BIN="$(command -v op-tunnel-client 2>/dev/null || true)"
if [ -z "$CLIENT_BIN" ]; then
    echo "Warning: op-tunnel-client not found in PATH. Skipping."
    echo "Re-run this step after op-tunnel-client is installed."
else
    mkdir -p "$HOME/.local/bin"
    ln -sf "$CLIENT_BIN" "$HOME/.local/bin/op"
    echo "Created: ~/.local/bin/op -> $CLIENT_BIN"

    # Detect shell and rc file
    case "$SHELL" in
        */zsh)  RC_FILE="$HOME/.zshrc" ;;
        */bash) RC_FILE="$HOME/.bashrc" ;;
        */fish) RC_FILE="$HOME/.config/fish/config.fish" ;;
        *)      RC_FILE="$HOME/.profile" ;;
    esac

    SHELL_NAME="$(basename "$SHELL")"

    if [ "$SHELL_NAME" = "fish" ]; then
        FISH_LINE="fish_add_path $HOME/.local/bin"
        if grep -qF ".local/bin" "$RC_FILE" 2>/dev/null; then
            echo "$RC_FILE already adds ~/.local/bin to PATH. Skipping."
        else
            mkdir -p "$(dirname "$RC_FILE")"
            echo "$FISH_LINE" >> "$RC_FILE"
            echo "Added to $RC_FILE: $FISH_LINE"
        fi
    else
        PATH_LINE='export PATH="$HOME/.local/bin:$PATH"'
        if grep -qF ".local/bin" "$RC_FILE" 2>/dev/null; then
            echo "$RC_FILE already adds ~/.local/bin to PATH. Skipping."
        else
            echo "$PATH_LINE" >> "$RC_FILE"
            echo "Added to $RC_FILE: $PATH_LINE"
        fi
    fi

    echo ""
    echo "Note: PATH changes take effect in new shell sessions."
    echo "To apply now: source $RC_FILE"
fi

echo ""
echo "Step 3 complete."
echo ""

echo "=== Setup complete ==="
echo ""
echo "Next steps:"
echo "  - Start the server:  brew services start op-tunnel"
echo "  - Test the tunnel:   ssh <remote-host> op --version"
echo ""
```

- [ ] **Step 2: Make it executable**

```bash
chmod +x packaging/op-tunnel-setup
```

- [ ] **Step 3: Smoke-test the script in dry-run fashion**

Read through the script and verify:
- `find_dist_file` is called for both `op-tunnel-sshd.conf` (Step 1) and `ssh.config` (Step 2)
- `sudo` appears only in Step 1 (`sudo install` and `sudo systemctl`/`sudo launchctl`)
- Each step prints "Why:", "Action:", and a completion message
- The fish PATH line uses `fish_add_path`, not `export PATH=`

- [ ] **Step 4: Run shellcheck if available**

```bash
shellcheck packaging/op-tunnel-setup
```

Expected: no errors or warnings. If shellcheck is not installed: `brew install shellcheck`.

- [ ] **Step 5: Commit**

```bash
git add packaging/op-tunnel-setup
git commit -m "feat: rewrite op-tunnel-setup with three explicit steps and granular sudo"
```

---

## Final Verification

- [ ] **Verify the full build still works**

```bash
go build ./...
go test ./...
```

- [ ] **Verify goreleaser snapshot still produces correct archives**

```bash
goreleaser release --snapshot --clean
tar -tzf _dist/op-tunnel-*-darwin-arm64.tar.gz | sort
rm -rf _dist/
```

Expected: both binaries + `dist/ssh.config`, `dist/op-tunnel-sshd.conf`, `dist/op-tunnel-setup` inside the archive.

- [ ] **Final commit if anything was missed**

```bash
git status
# commit any stray changes
```

---

## Pre-Release Checklist (before tagging v1.0.0)

1. Add `TAP_GITHUB_TOKEN` secret to `middlendian/op-tunnel` repo settings
2. Ensure `middlendian/homebrew-tap` repo exists with a `Formula/` directory
3. Push a test tag: `git tag v0.1.0 && git push origin v0.1.0`
4. Watch the release workflow run in GitHub Actions
5. Verify the GitHub release has tarballs + checksums attached
6. Verify `middlendian/homebrew-tap/Formula/op-tunnel.rb` was updated
7. Test install: `brew install middlendian/tap/op-tunnel`
