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
GOOS=linux GOARCH=amd64 go build -o "$TMPDIR_E2E/op-tunnel-client" "$REPO_ROOT/cmd/op-tunnel-client"
GOOS=linux GOARCH=amd64 go build -o "$TMPDIR_E2E/op-tunnel-doctor" "$REPO_ROOT/cmd/op-tunnel-doctor"
GOOS=linux GOARCH=amd64 go build -o "$TMPDIR_E2E/op-tunnel-keepalive" "$REPO_ROOT/cmd/op-tunnel-keepalive"

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
TEST_TUNNEL_ID="e2e_test_00000000000000000000"
cat > "$SSH_CONFIG" <<EOF
Host op-tunnel-test
    HostName 127.0.0.1
    Port 2222
    User root
    IdentityFile $TMPDIR_E2E/id_ed25519
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    RemoteForward /tmp/op-tunnel-root/client/${TEST_TUNNEL_ID}.sock /tmp/op-tunnel-$USER/server/op.sock
    SetEnv LC_OP_TUNNEL_ID=${TEST_TUNNEL_ID}
    StreamLocalBindUnlink yes
    ServerAliveInterval 30
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

echo "==> Creating socket directories in container..."
ssh -F "$SSH_CONFIG" -o BatchMode=yes op-tunnel-test \
    'mkdir -p /tmp/op-tunnel-root/client /tmp/op-tunnel-root/server && chmod 700 /tmp/op-tunnel-root'

# --- 6. Check op-tunnel-server ---
SERVER_SOCK="/tmp/op-tunnel-$USER/server/op.sock"
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
echo "==> DEBUG: sshd container logs..."
docker logs "$CONTAINER_NAME" 2>&1 | sed 's/^/    /' || true

echo "==> DEBUG: SSH RemoteForward negotiation..."
ssh -F "$SSH_CONFIG" -v -o BatchMode=yes op-tunnel-test true 2>&1 \
    | grep -iE "forward|socket|stream|remote|channel|request" \
    | sed 's/^/    /' || true

echo "==> Checking tunnel forwarding..."
PREFLIGHT=$(ssh -F "$SSH_CONFIG" -o BatchMode=yes op-tunnel-test \
    'echo "DEBUG: HOME=$HOME"
    echo "DEBUG: LC_OP_TUNNEL_ID=${LC_OP_TUNNEL_ID:-<unset>}"
    if [ -z "${LC_OP_TUNNEL_ID:-}" ]; then
        echo "FAIL: LC_OP_TUNNEL_ID is not set (check AcceptEnv in sshd_config)"
    else
        SOCK="/tmp/op-tunnel-$USER/client/${LC_OP_TUNNEL_ID}.sock"
        echo "DEBUG: expected socket: $SOCK"
        if ! test -S "$SOCK"; then
            echo "FAIL: socket not found at $SOCK (is op-tunnel-server running?)"
        else
            echo "PASS: socket at $SOCK"
        fi
    fi')
echo "$PREFLIGHT" | sed 's/^/    /'
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
