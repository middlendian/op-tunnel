# op-tunnel

Use the 1Password CLI (`op`) on remote machines, authenticated by your local desktop app.

1. SSH into a remote host.
2. Run commands like `op read`/`op item list` and they work exactly as if you ran them locally (including biometric
   auth on your local computer).
3. Profit.

## Install

On **both** machines (local and remote):

1. Install via Homebrew:

    ```bash
    brew install middlendian/tap/op-tunnel
    ```

On your **local machine** (the one hosting the 1Password desktop app):

1. Ensure CLI access in enabled in 1Password settings (probably already is if you're using `op`)
2. Add to `~/.ssh/config`:

    ```
    Host my-server
        Include ~/.local/share/op-tunnel/ssh.config
    ```

> [!CAUTION]
> Use specific host names, not `Host *`. This way, the included fragment applies RemoteForward **only** for
> trusted hosts where you opt in. Patterns like `Host *.local` or `Host *.example.com` may also be useful to trust hosts
> hosts in specific subdomains. See [ssh_config(5)](https://man.openbsd.org/ssh_config)

## How it works

`op` requires a local 1Password desktop connection for access to your vaults. On remote machines, there's no native way
to use it through a terminal session. The programs **op-tunnel-client** and **op-tunnel-server** work together to
forward `op` commands back to your local machine over a standard SSH socket — vaults stay local, the remote gets
results. No service accounts or static credentials required. Simple, static binaries on both sides.

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

`op-tunnel-server` listens on a Unix socket on your local machine. An SSH `RemoteForward` directive maps that socket to
the remote machine. `op-tunnel-client` is symlinked as `~/.local/bin/op` to serve as a wrapper. When `LC_OP_TUNNEL_SOCK`
is set, it sends the command over the socket; the server executes it locally and returns stdout, stderr, and exit code.
When `LC_OP_TUNNEL_SOCK` is unset, the local `op` binary is called directly.

Only allowlisted `OP_*` environment variables are forwarded. The socket is owner-only (`0600`) and unique per remote
user. The trust model is equivalent to SSH agent forwarding.

## Prerequisites

**Local machine (SSH client):**

- OpenSSH 7.3+ — standard on macOS 10.12+ and Ubuntu 18.04+
- 1Password desktop app running with CLI access enabled (for biometric auth according to your security settings)

**Remote machine (SSH server):**

- `AcceptEnv LC_OP_TUNNEL_SOCK` in `/etc/ssh/sshd_config`

  Stock Debian/Ubuntu already includes `AcceptEnv LANG LC_*`, which covers `LC_OP_TUNNEL_SOCK` — no change needed. On
  other systems (macOS, RHEL, Arch), run once after installing op-tunnel on the remote:

## Usage

```bash
# SSH as normal — op works transparently on the remote
ssh user@my-server

# On the remote:
op item list # This will prompt for authentication of "op-tunnel-server" on your local machine
op item get GitHub --fields password # Depending on your 1Password settings, re-prompts may be required at intervals
```

Local `op` usage is completely unaffected. When `LC_OP_TUNNEL_SOCK` is not set (i.e., outside an SSH session), the
client passes through to the real `op` binary.

## How it works

## License

GPLv3 — see [LICENSE](LICENSE).
