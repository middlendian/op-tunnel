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
        Include ~/.config/op-tunnel/ssh.config
    ```

> [!CAUTION]
> Use specific host names, not `Host *`. This way, the included fragment applies `RemoteForward` **only** for
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
│  LC_OP_TUNNEL_ID set?     │◄──────────────│  exec real `op`           │
│  yes → tunnel             │  stdout /     │  return results           │
│  no  → real `op`          │  stderr / rc  │                           │
└───────────────────────────┘               └───────────────────────────┘
```

`op-tunnel-server` listens on a Unix socket on your local machine. An SSH `RemoteForward` directive maps that socket to
the remote machine, where `op-tunnel-client` is symlinked as `~/.local/bin/op` to serve as a wrapper. When `LC_OP_TUNNEL_ID` is set (inside an SSH session), it sends the command over the socket; the server executes `op` locally and returns stdout, stderr, and
exit code. When `LC_OP_TUNNEL_ID` is not set, `op-tunnel-client` is a passthrough for the local `op` binary.

An `op-tunnel-keepalive` process runs alongside each SSH session to monitor the connection and clean up stale sockets when the session disconnects.

### Security model

The trust model is equivalent to SSH agent forwarding between two trusted hosts. Only allowlisted `OP_*` environment
variables are forwarded. The socket is owner-only (`0600`) and lives in a per-user directory under `/tmp/op-tunnel-$USER/`. Access to 1Password items is
governed by the same local security settings as the `op` CLI itself.

### Supported `op` CLI features

Linux and macOS are supported for both local and remote machines, and either `arm64` or `x86_64` architectures.

| Feature                         | Supported on remote machine?            |
|---------------------------------|-----------------------------------------|
| Creating vault items/documents  | Yes                                     |
| Reading vault items/documents   | Yes                                     |
| Updating vault items/documents  | Yes                                     |
| Deleting vault items/documents  | Yes                                     |
| Using `op` in scripts           | Yes, assuming they use standard `$PATH` |
| `op inject`                     | Not yet                                 |
| User management and `op signin` | Use your local `op` CLI for this        |
| `op run --` wrappers            | Probably not                            |
| Direct 1Password SDK usage      | Probably not                            |
| Other advanced functions        | Mileage may vary                        |

## Usage

```bash
# SSH as normal — op works transparently on the remote
ssh user@my-server

# On the remote:
op item list # This will prompt for authentication of "op-tunnel-server" on your local machine
op item get GitHub --fields password # Depending on your 1Password settings, re-prompts may be required at intervals
```

Local `op` usage is completely unaffected. When the remote socket is inactive (i.e., outside an SSH session), the
`op-tunnel-client` passes through to the real `op` binary locally.

## Diagnostics

If something isn't working, run:

```bash
op-tunnel-doctor
```

It checks the server, tunnel connection, symlink, PATH, SSH config, directory permissions, and config files. Each failing check shows a recommended fix.

## Known limitations

- op-tunnel configures `~/.ssh/rc` on each machine. If you use X11 forwarding (`ssh -X`), you may need to add xauth handling to `~/.ssh/rc` — see `sshd(8)`. Op-tunnel targets terminal sessions; for GUI access, use the 1Password desktop app directly.

## License

GPLv3 — see [LICENSE](LICENSE).
