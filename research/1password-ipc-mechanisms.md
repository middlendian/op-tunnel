# 1Password IPC & Communication Mechanisms Research

## 1. CLI <-> Desktop App Integration (Primary Target for Tunneling)

### IPC Mechanisms by Platform

| Platform | IPC Type | Path/Identifier |
|----------|----------|-----------------|
| **Linux** | Unix domain socket | `/run/user/<UID>/1Password-BrowserSupport.sock` (confirmed via strace) |
| **macOS** | XPC (Mach ports) | `1Password Browser Helper` XPC service via `NSXPCConnection` |
| **Windows** | Named pipe | Opened by the 1Password app; Authenticode signature verified |

### Linux Socket Details (Most Tunnelable)

- Socket path: `$XDG_RUNTIME_DIR/1Password-BrowserSupport.sock` (typically `/run/user/1000/1Password-BrowserSupport.sock`)
- Socket owned by current user/group
- **GID verification**: `op` CLI binary is owned by `onepassword-cli` group with `set-gid` bit. The 1Password app checks if the connecting process's GID matches `onepassword-cli`. If not, **connection is reset**.
- Protocol: JSON-based messages, e.g. `{"callbackId":1,"invocation":{"type":"NmRequestAccounts","content":{"version":1,"userRequested":true,...}}}`
- The "NmRequestAccounts" type suggests it reuses the Native Messaging protocol designed for browser extensions.

### macOS XPC Details (Harder to Tunnel)

- Uses `NSXPCConnection` XPC API
- `1Password Browser Helper` acts as XPC server and message relay
- Both CLI and Browser Helper have code signatures verified
- XPC is kernel-mediated and **not socket-based** -- cannot be trivially forwarded over SSH

### Security Model

- Authorization via biometrics/password prompt in desktop app
- 10-minute inactivity timeout, 12-hour hard limit
- Per-account, per-terminal-session (CLI) or per-process (SDK)
- Session credentials stored in environment variable `OP_BIOMETRIC_UNLOCK_CICD` (undocumented, seen in some CI contexts)

## 2. SSH Agent Socket (Already Tunnelable -- Reference Architecture)

### Socket Paths

| Platform | Path |
|----------|------|
| **Linux** | `~/.1password/agent.sock` |
| **macOS** | `~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock` |

### How Forwarding Works

- Standard SSH agent forwarding (`ssh -A` or `ForwardAgent yes`)
- `SSH_AUTH_SOCK` on remote points to a forwarded socket
- Requests flow: remote SSH client -> forwarded socket -> local 1Password SSH agent -> biometric auth -> response
- This is the **proven model** for tunneling 1Password functionality remotely

## 3. 1Password Connect Server (REST API -- Already Remote)

### Architecture

- Two Docker containers: `connect-sync` + `connect-api`
- Shared encrypted volume for cached data
- REST API on port 8080
- Auth via bearer tokens

### CLI Integration

- Set `OP_CONNECT_HOST` and `OP_CONNECT_TOKEN` environment variables
- `op` CLI automatically uses Connect instead of desktop app
- **This is 1Password's official solution for remote/headless `op` CLI usage**
- Limitation: requires deploying and managing Connect server infrastructure

### Priority Order for `op` CLI Authentication

1. `OP_CONNECT_HOST` + `OP_CONNECT_TOKEN` (Connect server)
2. `OP_SERVICE_ACCOUNT_TOKEN` (service account, no desktop app needed)
3. Desktop app integration (biometric, local only)
4. Manual sign-in (`op signin`)

## 4. Service Accounts (Token-Based -- Already Remote)

- `OP_SERVICE_ACCOUNT_TOKEN` env var
- Works headlessly, no desktop app
- Limited to specific vaults configured at creation
- No biometric auth -- token is the credential
- Available since CLI 2.18.0

## 5. SDK <-> Desktop App (Same IPC as CLI)

### Communication

- Same platform-native IPC channels as CLI (Mach ports / named pipes / Unix sockets)
- SDKs embed a core library that handles IPC internally
- Process identification via PID lookup -> executable name + path
- User sees authorization prompt with process name
- Available for Go, JavaScript, Python

## 6. Browser Extension <-> Desktop App

- Uses Chrome/Firefox Native Messaging protocol
- JSON manifest registers `1Password Browser Helper` as native messaging host
- Extension sends JSON messages via stdin/stdout to the helper process
- Helper relays to desktop app via same IPC channels
- Cannot work across network boundaries (requires local native messaging host)

## 7. Existing Solutions & Community Approaches

### What Exists

- **SSH Agent Forwarding**: Official, works well, but only for SSH keys
- **1Password Connect**: Official, works for secrets, but heavyweight (Docker deployment)
- **Service Accounts**: Official, works for secrets, but limited vault access and no personal vaults
- **WSL Integration**: Uses `npiperelay.exe` to bridge Windows named pipes to WSL Unix sockets (relevant pattern!)

### What Doesn't Exist (The Gap)

- **No way to tunnel the `op` CLI desktop app integration remotely**
- The community member SylvainLeroux asked about this exact use case (container -> host socket) with no official solution
- The GID check on Linux is a significant barrier: even if you forward the socket, the remote `op` binary won't have the right GID

## 8. Tunneling Feasibility Analysis

### Linux: Most Feasible

**Socket**: `/run/user/<UID>/1Password-BrowserSupport.sock`

**Approach**: SSH remote port forwarding of Unix socket:
```bash
# Forward remote socket to local 1Password socket
ssh -R /run/user/1000/1Password-BrowserSupport.sock:/run/user/1000/1Password-BrowserSupport.sock remote-host
```

**Challenges**:
1. **GID verification**: The 1Password app checks that the connecting process GID == `onepassword-cli` group GID. On the remote machine, the `op` binary needs the same group setup.
2. **Process verification**: The app may try to look up the PID of the connecting process, which won't work across machines.
3. **Biometric prompt**: Authorization still happens on the local machine (which is actually desirable).

**Potential workaround for GID**: Install `op` CLI on remote with proper group setup, or use a proxy that connects with correct GID.

### macOS: Difficult

- XPC/Mach ports are kernel IPC, not socket-based
- Cannot be forwarded over SSH
- Would need a local proxy that bridges XPC to a Unix socket, then tunnel the socket
- The `1Password Browser Helper` is the intermediary -- could potentially be leveraged

### Windows: Moderate

- Named pipes can potentially be forwarded (similar to WSL npiperelay pattern)
- More complex than Unix socket forwarding

## 9. Key Protocol Details

From the strace of the Linux socket communication:

```
connect(7, {sa_family=AF_UNIX, sun_path="/run/user/1000/1Password-BrowserSupport.sock"}, 47)
write(7, "\201\0\0\0{\"callbackId\":1,\"invocation\":{\"type\":\"NmRequestAccounts\",...}}")
```

- Binary frame header followed by JSON payload
- Uses the same "Nm" (Native Messaging) protocol as browser extension
- Message types include: `NmRequestAccounts`, and likely others for item access, vault listing, etc.

## 10. Sources

- https://developer.1password.com/docs/cli/app-integration-security/
- https://developer.1password.com/docs/sdks/desktop-app-integrations/
- https://developer.1password.com/docs/connect/
- https://developer.1password.com/docs/cli/app-integration/
- https://developer.1password.com/docs/ssh/agent/forwarding/
- https://developer.1password.com/docs/service-accounts/use-with-1password-cli/
- https://github.com/NixOS/nixpkgs/issues/258139
- https://www.1password.community/discussions/developers/link-the-1password-cli-in-a-container-to-the-1password-application-on-the-host/167032
