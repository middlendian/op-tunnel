# CI/CD, Release Pipeline, and op-tunnel-setup Redesign

**Date:** 2026-03-14
**Status:** Approved

## Overview

This spec covers three related areas:

1. GitHub Actions CI (PR testing with golangci-lint + gofmt + go test)
2. GoReleaser-based release pipeline (cross-platform tarballs, GitHub releases, Homebrew tap updates)
3. Redesign of `op-tunnel-setup` (three explicit steps, granular sudo, symlink-based op shadowing)

## Repository Layout Changes

### Rename `dist/` → `packaging/`

GoReleaser uses `dist/` as its default build output directory and recommends gitignoring it. Our source distribution files move to `packaging/` to avoid the collision. GoReleaser is configured with `dist: _dist`.

**Files removed** (replaced by `brew services`):
- `packaging/com.middlendian.op-tunnel-server.plist`
- `packaging/op-tunnel-server.service`

**Files kept** (platform-agnostic, included in every tarball):
- `packaging/ssh.config`
- `packaging/op-tunnel-sshd.conf`
- `packaging/op-tunnel-setup`

**New files:**
- `.goreleaser.yaml`
- `.golangci.yml`
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`

### Go Version

`go.mod` bumped from `1.25.6` to `1.26.1`. Both workflows use Go 1.26.1.

## GoReleaser Config (`.goreleaser.yaml`)

### Builds

Two named builds, both cross-compiled for four targets:

| Build ID          | Targets                                                    |
|-------------------|------------------------------------------------------------|
| `op-tunnel-server` | `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64` |
| `op-tunnel-client` | `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64` |

Flags: `-ldflags "-s -w"`, `CGO_ENABLED=0` for fully static binaries.

### Archives

One archive per OS/arch containing **both** binaries plus the `packaging/` files. The `packaging/` files land in a `dist/` subdirectory inside the tarball so `op-tunnel-setup` can locate `op-tunnel-sshd.conf` via the same relative path used in Homebrew installations.

- Name template: `op-tunnel-{{ .Version }}-{{ .Os }}-{{ .Arch }}`
- Format: `tar.gz`
- Contents per archive:
  - `op-tunnel-server`
  - `op-tunnel-client`
  - `dist/ssh.config` (from `packaging/ssh.config`)
  - `dist/op-tunnel-sshd.conf` (from `packaging/op-tunnel-sshd.conf`)
  - `dist/op-tunnel-setup` (from `packaging/op-tunnel-setup`)
- Checksums file auto-generated

### Homebrew Publisher (`brews`)

GoReleaser pushes the generated formula to `middlendian/homebrew-tap`, file `Formula/op-tunnel.rb`, committing directly (no PR). Requires `TAP_GITHUB_TOKEN` secret with write access to the tap repo.

### GoReleaser Output Directory

Configured as `_dist` (gitignored) to leave `dist/` free for GoReleaser convention clarity.

## GitHub Actions Workflows

### `ci.yml` — PR and main branch testing

Trigger: `pull_request`, `push` to `main`
Runner: `ubuntu-latest`
Go: `1.26.1`

Jobs (sequential in one job):
1. `go test ./...`
2. `go test -tags integration ./...`
3. `golangci-lint run` (via `golangci-lint-action`)

No secrets required.

### `release.yml` — Tag-triggered releases

Trigger: `push` to tags matching `v*`
Runner: `ubuntu-latest`
Go: `1.26.1`

Single job: `goreleaser release --clean`

Required secrets:
- `GITHUB_TOKEN` — built-in; creates the GitHub release and uploads artifacts
- `TAP_GITHUB_TOKEN` — PAT with `contents: write` on `middlendian/homebrew-tap`; stored as a repo secret

### `.golangci.yml`

Enabled linters (on top of defaults):
- `errcheck`
- `govet`
- `staticcheck`
- `unused`
- `gofmt`

## Homebrew Tap Formula

GoReleaser generates and maintains `middlendian/homebrew-tap/Formula/op-tunnel.rb`. Key design:

**No Go build dependency** — downloads a pre-built platform tarball.

**Dependencies:**
```ruby
depends_on "1password-cli"
```
Ensures the real `op` binary is present before `op-tunnel-client` tries to fall back to it.

**Install block:**
```ruby
def install
  bin.install "op-tunnel-server"
  bin.install "op-tunnel-client"
  (share/"op-tunnel").install "dist/op-tunnel-sshd.conf", "dist/ssh.config"
  bin.install "dist/op-tunnel-setup"
end
```

**No `post_install` symlink** — the `op` symlink is no longer created automatically. Ownership moves to `op-tunnel-setup` (see below). This avoids a Homebrew bin directory conflict between `1password-cli` (which installs `op`) and `op-tunnel`.

**Service block** (replaces plist and systemd unit files):
```ruby
service do
  run [opt_bin/"op-tunnel-server"]
  keep_alive true
  log_path var/"log/op-tunnel-server.log"
  error_log_path var/"log/op-tunnel-server.log"
end
```

**Caveats:** Instructs the user to run `op-tunnel-setup` to complete configuration.

**Test block:**
```ruby
test do
  assert_predicate bin/"op-tunnel-server", :executable?
  assert_predicate bin/"op-tunnel-client", :executable?
end
```

## op-tunnel-setup Script Redesign

The script runs on **any machine** participating in the tunnel — both the local machine (SSH client, op-tunnel-server host) and remote machines (SSH server, op-tunnel-client host). It performs three explicit, labeled steps and prints the intent and reason for each action before executing it.

Only Step 1 uses `sudo`. Steps 2 and 3 operate entirely on user-owned files.

### Step 1 — Configure sshd

**Why printed:** "Remote SSH servers must allow clients to set `LC_OP_TUNNEL_SOCK`. This environment variable tells `op-tunnel-client` where to find the forwarded Unix socket."

**Actions:**
- Locates `op-tunnel-sshd.conf` relative to the script:
  - First tries `../share/op-tunnel/op-tunnel-sshd.conf` (Homebrew layout)
  - Falls back to `./dist/op-tunnel-sshd.conf` (tarball layout)
- `sudo install -m 644 <conf> /etc/ssh/sshd_config.d/op-tunnel.conf`
- Reloads sshd:
  - Linux: `sudo systemctl reload sshd` or `sudo systemctl reload ssh`
  - macOS: `sudo launchctl kickstart -k system/com.openssh.sshd`
- Prints the path written and confirmation that sshd was reloaded.

### Step 2 — Configure SSH client

**Why printed:** "When you SSH to a remote host, your SSH client needs to forward the op-tunnel socket and set `LC_OP_TUNNEL_SOCK` in the remote session."

**Actions:**
- Prompts: `Which hosts should use op-tunnel? [default: *.local]`
- Creates `~/.local/share/op-tunnel/` directories (server/ and client/)
- Copies `ssh.config` to `~/.local/share/op-tunnel/ssh.config`
- Appends to `~/.ssh/config` (idempotent — skips if the block already exists for that pattern):
  ```
  Host <pattern>
      Include ~/.local/share/op-tunnel/ssh.config
  ```
- Prints the exact lines written to `~/.ssh/config`.

### Step 3 — Shadow `op` with a symlink

**Why printed:** "`op-tunnel-client` transparently proxies `op` commands over the tunnel when available, falling back to the real `op` binary. A symlink (unlike a shell alias) works reliably from scripts, Makefiles, IDEs, and other tools."

**Actions:**
- Creates `~/.local/bin/op` → `$(which op-tunnel-client)` symlink
- Detects shell from `$SHELL` (zsh → `.zshrc`, bash → `.bashrc`, fish → `~/.config/fish/config.fish`)
- Appends `export PATH="$HOME/.local/bin:$PATH"` to the shell init file **only if** `~/.local/bin` is not already present in the file (idempotent)
- Prints the symlink path and any changes made to the shell init file
- Notes that the PATH change takes effect in new shell sessions

### Script principles

- POSIX sh (`#!/bin/sh`)
- `set -e` — abort on any error
- Each step prefixed with a numbered header: `=== Step N: <name> ===`
- Prints intent before acting, confirmation after
- Idempotent: re-running is safe
- `sudo` used only for sshd config install and reload in Step 1
