# op-tunnel Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a command proxy that tunnels 1Password CLI (`op`) invocations over SSH-forwarded Unix domain sockets, so remote `op` commands authenticate against the local 1Password app.

**Architecture:** Two Go binaries sharing a protocol package. `op-tunnel-server` listens on a Unix socket, executes `op` locally, returns results. `op-tunnel-client` intercepts `op` calls on remote machines — tunneling when `LC_OP_TUNNEL_SOCK` is set, passing through to the real `op` otherwise.

**Tech Stack:** Go (stdlib + `golang.org/x/term`), Unix domain sockets, LaunchAgent/systemd for service management, Homebrew for distribution.

**Spec:** `docs/superpowers/specs/2026-03-13-op-tunnel-design.md`

---

## File Structure

```
op-tunnel/
├── go.mod                                  # module github.com/middlendian/op-tunnel
├── go.sum
├── Makefile                                # build, test, clean targets
├── protocol/
│   ├── protocol.go                         # types, constants, framing, env filtering
│   └── protocol_test.go                    # unit tests for protocol package
├── cmd/
│   ├── op-tunnel-server/
│   │   └── main.go                         # server daemon
│   └── op-tunnel-client/
│       └── main.go                         # client stub (tunnel + passthrough)
├── dist/
│   ├── com.middlendian.op-tunnel-server.plist  # macOS LaunchAgent
│   └── op-tunnel-server.service            # systemd user unit
├── docs/superpowers/specs/...              # (existing)
└── docs/superpowers/plans/...              # (this file)
```

---

## Chunk 1: Project Setup + Protocol Package

### Task 0: Create .gitignore

**Files:**
- Create: `.gitignore`

- [ ] **Step 1: Create .gitignore**

Create `.gitignore`:

```
/bin/
```

- [ ] **Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: add .gitignore for build output"
```

---

### Task 1: Initialize Go module

**Files:**
- Create: `go.mod`
- Create: `go.sum`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/greg/proj/middlendian/op-tunnel
go mod init github.com/middlendian/op-tunnel
```

- [ ] **Step 2: Add the only external dependency**

```bash
go get golang.org/x/term
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialize Go module with x/term dependency"
```

---

### Task 2: Protocol package — types and constants

**Files:**
- Create: `protocol/protocol.go`

- [ ] **Step 1: Write the protocol package with types, constants, and env allowlist**

Create `protocol/protocol.go`:

```go
package protocol

import (
	"os"
	"path/filepath"
)

const (
	ProtocolVersion = 1
	MaxPayloadSize  = 64 << 20 // 64MB
	SocketName      = "op-tunnel.sock"
	EnvTunnelSock   = "LC_OP_TUNNEL_SOCK"
	ServerSocketDir = ".local/share/op-tunnel/server"
	ClientSocketDir = ".local/share/op-tunnel/client"
)

var AllowedEnvVars = []string{
	"OP_ACCOUNT",
	"OP_CACHE",
	"OP_CONNECT_HOST",
	"OP_CONNECT_TOKEN",
	"OP_FORMAT",
	"OP_INCLUDE_ARCHIVE",
	"OP_ISO_TIMESTAMPS",
	"OP_RUN_NO_MASKING",
	"OP_SERVICE_ACCOUNT_TOKEN",
}

type Request struct {
	V    int               `json:"v"`
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
	TTY  bool              `json:"tty"`
}

type Response struct {
	V        int    `json:"v"`
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go build ./protocol/
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add protocol/protocol.go
git commit -m "feat: add protocol types and constants"
```

---

### Task 3: Protocol package — framing functions

**Files:**
- Modify: `protocol/protocol.go`
- Create: `protocol/protocol_test.go`

- [ ] **Step 1: Write failing tests for WriteMessage and ReadMessage**

Create `protocol/protocol_test.go`:

```go
package protocol

import (
	"bytes"
	"testing"
)

func TestWriteReadMessageRoundTrip(t *testing.T) {
	payload := []byte(`{"v":1,"args":["item","list"]}`)

	var buf bytes.Buffer
	if err := WriteMessage(&buf, payload); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	// Should be 4-byte length prefix + payload
	if buf.Len() != 4+len(payload) {
		t.Fatalf("expected %d bytes, got %d", 4+len(payload), buf.Len())
	}

	got, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, payload)
	}
}

func TestWriteMessageRejectsOversized(t *testing.T) {
	// Test the length check without allocating 64MB — craft a header with an oversized length
	// and verify ReadMessage rejects it. For WriteMessage, we verify the error path
	// by using a size just over the limit via a small wrapper.
	var buf bytes.Buffer
	// Write a fake header claiming MaxPayloadSize+1 bytes
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, MaxPayloadSize+1)
	buf.Write(header)
	_, err := ReadMessage(&buf)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
}

func TestWriteReadEmptyPayload(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMessage(&buf, []byte{}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	got, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty payload, got %d bytes", len(got))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go test ./protocol/ -v
```

Expected: FAIL — `WriteMessage` and `ReadMessage` undefined.

- [ ] **Step 3: Implement WriteMessage and ReadMessage**

Add these imports to `protocol/protocol.go`:

```go
import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)
```

Add the functions:

```go
// WriteMessage writes a length-prefixed message. Rejects payloads > MaxPayloadSize.
func WriteMessage(w io.Writer, payload []byte) error {
	if len(payload) > MaxPayloadSize {
		return fmt.Errorf("payload size %d exceeds maximum %d", len(payload), MaxPayloadSize)
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("writing payload: %w", err)
	}
	return nil
}

// ReadMessage reads a length-prefixed message. Rejects payloads > MaxPayloadSize.
func ReadMessage(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	size := binary.BigEndian.Uint32(header)
	if size > MaxPayloadSize {
		return nil, fmt.Errorf("payload size %d exceeds maximum %d", size, MaxPayloadSize)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("reading payload: %w", err)
	}
	return payload, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go test ./protocol/ -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add protocol/
git commit -m "feat: add wire protocol framing with length-prefix"
```

---

### Task 4: Protocol package — helper functions

**Files:**
- Modify: `protocol/protocol.go`
- Modify: `protocol/protocol_test.go`

- [ ] **Step 1: Write failing tests for ExpandSocketPath and FilterEnv**

Add to `protocol/protocol_test.go`:

```go
func TestExpandSocketPath(t *testing.T) {
	path, err := ExpandSocketPath(ServerSocketDir)
	if err != nil {
		t.Fatalf("ExpandSocketPath: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ServerSocketDir, SocketName)
	if path != expected {
		t.Fatalf("got %q, want %q", path, expected)
	}
}

func TestFilterEnv(t *testing.T) {
	// Set one allowed var and one disallowed var
	t.Setenv("OP_FORMAT", "json")
	t.Setenv("SECRET_KEY", "hunter2")

	filtered := FilterEnv()

	if v, ok := filtered["OP_FORMAT"]; !ok || v != "json" {
		t.Fatalf("expected OP_FORMAT=json, got %v", filtered)
	}
	if _, ok := filtered["SECRET_KEY"]; ok {
		t.Fatal("SECRET_KEY should not be in filtered env")
	}
}

func TestFilterEnvEmpty(t *testing.T) {
	// Ensure none of the allowed vars are set by setting them to empty
	// via t.Setenv (which restores originals on cleanup), then unsetting
	for _, v := range AllowedEnvVars {
		os.Unsetenv(v)
	}
	// t.Cleanup will not restore these since we didn't use t.Setenv,
	// but these are test-only env vars unlikely to be set in CI

	filtered := FilterEnv()
	if len(filtered) != 0 {
		t.Fatalf("expected empty map, got %v", filtered)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go test ./protocol/ -v -run "TestExpandSocketPath|TestFilterEnv"
```

Expected: FAIL — `ExpandSocketPath` and `FilterEnv` undefined.

- [ ] **Step 3: Implement ExpandSocketPath and FilterEnv**

Add to `protocol/protocol.go`:

```go
// ExpandSocketPath resolves a relative socket dir (e.g., ServerSocketDir) to
// an absolute path like /Users/greg/.local/share/op-tunnel/server/op-tunnel.sock.
func ExpandSocketPath(relDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, relDir, SocketName), nil
}

// FilterEnv returns a map of only the allowlisted 1Password env vars
// that are currently set in the environment.
func FilterEnv() map[string]string {
	env := make(map[string]string)
	for _, key := range AllowedEnvVars {
		if val, ok := os.LookupEnv(key); ok {
			env[key] = val
		}
	}
	return env
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go test ./protocol/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add protocol/
git commit -m "feat: add socket path expansion and env var filtering"
```

---

### Task 5: Protocol package — JSON marshaling helpers

**Files:**
- Modify: `protocol/protocol.go`
- Modify: `protocol/protocol_test.go`

- [ ] **Step 1: Write failing test for SendRequest/ReadRequest and SendResponse/ReadResponse**

Add to `protocol/protocol_test.go`:

```go
func TestRequestRoundTrip(t *testing.T) {
	req := &Request{
		V:    1,
		Args: []string{"item", "get", "GitHub"},
		Env:  map[string]string{"OP_FORMAT": "json"},
		TTY:  true,
	}

	var buf bytes.Buffer
	if err := SendRequest(&buf, req); err != nil {
		t.Fatalf("SendRequest: %v", err)
	}

	got, err := ReadRequest(&buf)
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}

	if got.V != req.V || got.TTY != req.TTY || len(got.Args) != len(req.Args) {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, req)
	}
}

func TestResponseRoundTrip(t *testing.T) {
	resp := &Response{
		V:        1,
		ExitCode: 0,
		Stdout:   "aGVsbG8=", // "hello" in base64
		Stderr:   "",
	}

	var buf bytes.Buffer
	if err := SendResponse(&buf, resp); err != nil {
		t.Fatalf("SendResponse: %v", err)
	}

	got, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}

	if got.ExitCode != resp.ExitCode || got.Stdout != resp.Stdout {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, resp)
	}
}

func TestResponseWithError(t *testing.T) {
	resp := &Response{
		V:        1,
		ExitCode: -1,
		Error:    "op not found",
	}

	var buf bytes.Buffer
	if err := SendResponse(&buf, resp); err != nil {
		t.Fatalf("SendResponse: %v", err)
	}

	got, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}

	if got.Error != "op not found" || got.ExitCode != -1 {
		t.Fatalf("error response mismatch: got %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go test ./protocol/ -v -run "TestRequestRoundTrip|TestResponseRoundTrip|TestResponseWithError"
```

Expected: FAIL — functions undefined.

- [ ] **Step 3: Implement marshaling helpers**

Add `"encoding/json"` to the imports in `protocol/protocol.go`, then add:

```go
// SendRequest marshals and writes a framed request.
func SendRequest(w io.Writer, req *Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	return WriteMessage(w, data)
}

// ReadRequest reads and unmarshals a framed request.
func ReadRequest(r io.Reader) (*Request, error) {
	data, err := ReadMessage(r)
	if err != nil {
		return nil, err
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("unmarshaling request: %w", err)
	}
	return &req, nil
}

// SendResponse marshals and writes a framed response.
func SendResponse(w io.Writer, resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshaling response: %w", err)
	}
	return WriteMessage(w, data)
}

// ReadResponse reads and unmarshals a framed response.
func ReadResponse(r io.Reader) (*Response, error) {
	data, err := ReadMessage(r)
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}
	return &resp, nil
}

// ErrorResponse creates a tunnel-level error response.
func ErrorResponse(msg string) *Response {
	return &Response{V: ProtocolVersion, ExitCode: -1, Error: msg}
}
```

- [ ] **Step 4: Run all protocol tests**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go test ./protocol/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add protocol/
git commit -m "feat: add request/response marshaling helpers"
```

---

## Chunk 2: op-tunnel-server

### Task 6: Server — core implementation

**Files:**
- Create: `cmd/op-tunnel-server/main.go`

- [ ] **Step 1: Write the server**

Create `cmd/op-tunnel-server/main.go`:

```go
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/middlendian/op-tunnel/protocol"
)

const defaultTimeout = 5 * time.Minute

func main() {
	socketPath, err := protocol.ExpandSocketPath(protocol.ServerSocketDir)
	if err != nil {
		log.Fatalf("resolving socket path: %v", err)
	}

	// Create socket directory with restrictive permissions
	socketDir := filepath.Dir(socketPath)
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		log.Fatalf("creating socket directory: %v", err)
	}

	// Remove stale socket
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Fatalf("removing stale socket: %v", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("listening on %s: %v", socketPath, err)
	}
	defer listener.Close()

	// Set socket permissions
	if err := os.Chmod(socketPath, 0600); err != nil {
		log.Fatalf("setting socket permissions: %v", err)
	}

	log.Printf("op-tunnel-server: listening on %s", socketPath)

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	go func() {
		<-ctx.Done()
		log.Println("op-tunnel-server: shutting down")
		listener.Close()
		os.Remove(socketPath)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break // shutting down
			}
			log.Printf("accept error: %v", err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			handleConnection(ctx, conn)
		}()
	}

	wg.Wait()
}

func handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	req, err := protocol.ReadRequest(conn)
	if err != nil {
		log.Printf("reading request: %v", err)
		return
	}

	if req.V != protocol.ProtocolVersion {
		resp := protocol.ErrorResponse(fmt.Sprintf("unsupported protocol version: %d", req.V))
		protocol.SendResponse(conn, resp)
		return
	}

	// Find op binary
	opPath, err := exec.LookPath("op")
	if err != nil {
		resp := protocol.ErrorResponse("op binary not found in PATH")
		protocol.SendResponse(conn, resp)
		return
	}

	// Execute with timeout; also cancel if client disconnects
	cmdCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// Monitor connection: if client disconnects, cancel the command
	go func() {
		buf := make([]byte, 1)
		conn.Read(buf) // blocks until EOF or error (client disconnect)
		cancel()
	}()

	cmd := exec.CommandContext(cmdCtx, opPath, req.Args...)

	// Build clean environment with only allowed vars
	cmd.Env = buildEnv(req.Env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	exitCode := 0
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			resp := protocol.ErrorResponse("command timed out")
			protocol.SendResponse(conn, resp)
			return
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			resp := protocol.ErrorResponse(fmt.Sprintf("executing op: %v", err))
			protocol.SendResponse(conn, resp)
			return
		}
	}

	resp := &protocol.Response{
		V:        protocol.ProtocolVersion,
		ExitCode: exitCode,
		Stdout:   base64.StdEncoding.EncodeToString(stdout.Bytes()),
		Stderr:   base64.StdEncoding.EncodeToString(stderr.Bytes()),
	}
	protocol.SendResponse(conn, resp)
}

func buildEnv(reqEnv map[string]string) []string {
	// Start with essential vars from current process
	env := []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"USER=" + os.Getenv("USER"),
	}

	// Overlay allowlisted vars from request
	allowed := make(map[string]bool)
	for _, k := range protocol.AllowedEnvVars {
		allowed[k] = true
	}
	for k, v := range reqEnv {
		if allowed[k] {
			env = append(env, k+"="+v)
		}
	}
	return env
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go build ./cmd/op-tunnel-server/
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add cmd/op-tunnel-server/
git commit -m "feat: add op-tunnel-server daemon"
```

---

### Task 7: Server — smoke test

- [ ] **Step 1: Build and start the server**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go build -o bin/op-tunnel-server ./cmd/op-tunnel-server/
./bin/op-tunnel-server &
SERVER_PID=$!
```

Expected: `op-tunnel-server: listening on /Users/greg/.local/share/op-tunnel/server/op-tunnel.sock`

- [ ] **Step 2: Test with a simple socket connection**

```bash
# Use a small Go test program or socat to send a request
# This will be validated more thoroughly in the integration test task
kill $SERVER_PID
```

- [ ] **Step 3: Verify socket cleanup**

```bash
ls -la ~/.local/share/op-tunnel/server/
```

Expected: directory exists, socket file removed after server shutdown.

---

## Chunk 3: op-tunnel-client

### Task 8: Client — tunnel mode

**Files:**
- Create: `cmd/op-tunnel-client/main.go`

- [ ] **Step 1: Write the client**

Create `cmd/op-tunnel-client/main.go`:

```go
package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/middlendian/op-tunnel/protocol"
)

func main() {
	sockPath := os.Getenv(protocol.EnvTunnelSock)
	if sockPath != "" {
		tunnelMode(sockPath, os.Args[1:])
	} else {
		passthroughMode(os.Args[1:])
	}
}

func tunnelMode(sockPath string, args []string) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "op-tunnel: tunnel not connected")
		os.Exit(1)
	}

	// Close connection on signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGPIPE)
	go func() {
		<-sigCh
		conn.Close()
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

	conn.Close()

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
		os.Stdout.Write(decoded)
	}

	if resp.Stderr != "" {
		decoded, err := base64.StdEncoding.DecodeString(resp.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "op-tunnel: decoding stderr: %v\n", err)
			os.Exit(1)
		}
		os.Stderr.Write(decoded)
	}

	os.Exit(resp.ExitCode)
}

func passthroughMode(args []string) {
	realOp := findRealOp()
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

// findRealOp searches PATH for the real `op` binary, skipping our own directory.
func findRealOp() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return ""
	}
	selfDir := filepath.Dir(self)

	pathEnv := os.Getenv("PATH")
	for _, dir := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if dir == "" {
			continue
		}
		// Resolve symlinks on the PATH dir too, so we catch Homebrew symlink dirs
		resolvedDir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			resolvedDir = dir
		}
		if resolvedDir == selfDir {
			continue
		}
		candidate := filepath.Join(dir, "op")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate
		}
	}
	return ""
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go build ./cmd/op-tunnel-client/
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add cmd/op-tunnel-client/
git commit -m "feat: add op-tunnel-client with tunnel and passthrough modes"
```

---

## Chunk 4: Integration Tests

### Task 9: Integration test — client ↔ server round-trip

**Files:**
- Create: `protocol/integration_test.go`

This test starts a real server on a temp socket, then runs the client logic against it with a mock `op` script.

- [ ] **Step 1: Write the integration test**

Create `protocol/integration_test.go`:

```go
//go:build integration

package protocol_test

import (
	"encoding/base64"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/middlendian/op-tunnel/protocol"
)

// TestClientServerRoundTrip starts a mock server, sends a request, verifies the response.
func TestClientServerRoundTrip(t *testing.T) {
	// Create temp socket
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	// Mock server: reads request, returns canned response
	expectedStdout := "hello from op"
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		req, err := protocol.ReadRequest(conn)
		if err != nil {
			t.Errorf("server: reading request: %v", err)
			return
		}

		if req.V != 1 || len(req.Args) == 0 || req.Args[0] != "--version" {
			t.Errorf("unexpected request: %+v", req)
		}

		resp := &protocol.Response{
			V:        1,
			ExitCode: 0,
			Stdout:   base64.StdEncoding.EncodeToString([]byte(expectedStdout)),
			Stderr:   "",
		}
		protocol.SendResponse(conn, resp)
	}()

	// Give server a moment to start
	time.Sleep(10 * time.Millisecond)

	// Client side: connect, send request, read response
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := &protocol.Request{
		V:    1,
		Args: []string{"--version"},
		Env:  map[string]string{},
		TTY:  false,
	}

	if err := protocol.SendRequest(conn, req); err != nil {
		t.Fatalf("send request: %v", err)
	}

	resp, err := protocol.ReadResponse(conn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if resp.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", resp.ExitCode)
	}

	decoded, err := base64.StdEncoding.DecodeString(resp.Stdout)
	if err != nil {
		t.Fatalf("decoding stdout: %v", err)
	}

	if string(decoded) != expectedStdout {
		t.Fatalf("expected stdout %q, got %q", expectedStdout, string(decoded))
	}
}

// TestVersionMismatch verifies server rejects unknown protocol versions.
func TestVersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	// Server that validates version (mimics real server behavior)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		req, err := protocol.ReadRequest(conn)
		if err != nil {
			return
		}

		if req.V != protocol.ProtocolVersion {
			resp := protocol.ErrorResponse(fmt.Sprintf("unsupported protocol version: %d", req.V))
			protocol.SendResponse(conn, resp)
			return
		}
	}()

	time.Sleep(10 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := &protocol.Request{V: 99, Args: []string{"test"}}
	protocol.SendRequest(conn, req)

	resp, err := protocol.ReadResponse(conn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if resp.Error == "" {
		t.Fatal("expected error for version mismatch")
	}
	if resp.ExitCode != -1 {
		t.Fatalf("expected exit code -1, got %d", resp.ExitCode)
	}
}
```

- [ ] **Step 2: Run integration tests**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && go test ./protocol/ -v -tags integration
```

Expected: all integration tests PASS.

- [ ] **Step 3: Commit**

```bash
git add protocol/integration_test.go
git commit -m "test: add client-server integration tests"
```

---

## Chunk 5: Build System + Service Files

### Task 10: Makefile

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Write the Makefile**

Create `Makefile`:

```makefile
BINDIR := ./bin
LDFLAGS := -s -w

.PHONY: build clean test test-integration

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-server ./cmd/op-tunnel-server
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/op-tunnel-client ./cmd/op-tunnel-client

test:
	go test ./...

test-integration:
	go test ./... -tags integration

clean:
	rm -rf $(BINDIR)
```

- [ ] **Step 2: Verify `make build` works**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && make build
```

Expected: builds both binaries to `./bin/`.

- [ ] **Step 3: Verify `make test` works**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && make test
```

Expected: all unit tests pass.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile with build, test, clean targets"
```

---

### Task 11: LaunchAgent plist

**Files:**
- Create: `dist/com.middlendian.op-tunnel-server.plist`

- [ ] **Step 1: Write the LaunchAgent plist**

Create `dist/com.middlendian.op-tunnel-server.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.middlendian.op-tunnel-server</string>
    <key>ProgramArguments</key>
    <array>
        <string>HOMEBREW_PREFIX/bin/op-tunnel-server</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/op-tunnel-server.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/op-tunnel-server.log</string>
</dict>
</plist>
```

Note: `HOMEBREW_PREFIX` is replaced by the Homebrew formula at install time.

- [ ] **Step 2: Commit**

```bash
git add dist/com.middlendian.op-tunnel-server.plist
git commit -m "chore: add macOS LaunchAgent plist"
```

---

### Task 12: systemd user service

**Files:**
- Create: `dist/op-tunnel-server.service`

- [ ] **Step 1: Write the systemd unit file**

Create `dist/op-tunnel-server.service`:

```ini
[Unit]
Description=op-tunnel server - proxy 1Password CLI over SSH tunnels

[Service]
Type=simple
ExecStart=%h/.local/bin/op-tunnel-server
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

- [ ] **Step 2: Commit**

```bash
git add dist/op-tunnel-server.service
git commit -m "chore: add systemd user service unit"
```

---

### Task 13: Homebrew formula

**Files:**
- Create: `dist/op-tunnel.rb`

- [ ] **Step 1: Write the Homebrew formula**

Create `dist/op-tunnel.rb`:

```ruby
class OpTunnel < Formula
  desc "Tunnel 1Password CLI (op) commands over SSH"
  homepage "https://github.com/middlendian/op-tunnel"
  url "https://github.com/middlendian/op-tunnel/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER"
  license "MIT"

  depends_on "go" => :build

  def install
    system "make", "build"
    bin.install "bin/op-tunnel-server"
    bin.install "bin/op-tunnel-client"

    # Service files
    prefix.install "dist/com.middlendian.op-tunnel-server.plist"
    prefix.install "dist/op-tunnel-server.service"
  end

  def post_install
    # Symlink client as `op` — overrides native op binary
    bin.install_symlink bin/"op-tunnel-client" => "op"

    # Install LaunchAgent with correct paths
    plist = prefix/"com.middlendian.op-tunnel-server.plist"
    inreplace plist, "HOMEBREW_PREFIX", HOMEBREW_PREFIX
    launch_agent_dir = Pathname.new("#{Dir.home}/Library/LaunchAgents")
    launch_agent_dir.mkpath
    ln_sf plist, launch_agent_dir/"com.middlendian.op-tunnel-server.plist"

    # Print setup instructions
    ohai "op-tunnel installed!"
    puts <<~EOS
      Add to your ~/.ssh/config:

        Host *
            RemoteForward ~/.local/share/op-tunnel/client/op-tunnel.sock ~/.local/share/op-tunnel/server/op-tunnel.sock
            SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock
            StreamLocalBindUnlink yes
            ServerAliveInterval 30
            ServerAliveCountMax 6

      The server LaunchAgent has been installed and will start on next login.
      To start it now:
        launchctl load ~/Library/LaunchAgents/com.middlendian.op-tunnel-server.plist
    EOS
  end

  def caveats
    <<~EOS
      op-tunnel-client has been symlinked as `op`.
      When LC_OP_TUNNEL_SOCK is set (via SSH), op commands are tunneled.
      Otherwise, the real op binary is called directly.

      Ensure the remote sshd has: AcceptEnv LC_*
    EOS
  end

  test do
    # Verify binaries exist and are runnable
    assert_predicate bin/"op-tunnel-server", :executable?
    assert_predicate bin/"op-tunnel-client", :executable?
  end
end
```

- [ ] **Step 2: Commit**

```bash
git add dist/op-tunnel.rb
git commit -m "chore: add Homebrew formula"
```

---

## Chunk 6: End-to-End Smoke Test + Final Verification

### Task 14: Manual end-to-end verification

- [ ] **Step 1: Build both binaries**

```bash
cd /Users/greg/proj/middlendian/op-tunnel && make clean && make build
```

- [ ] **Step 2: Start the server**

```bash
./bin/op-tunnel-server &
```

Expected: `op-tunnel-server: listening on /Users/<user>/.local/share/op-tunnel/server/op-tunnel.sock`

- [ ] **Step 3: Test tunnel mode directly (no SSH needed)**

```bash
export LC_OP_TUNNEL_SOCK="$HOME/.local/share/op-tunnel/server/op-tunnel.sock"
./bin/op-tunnel-client --version
```

Expected: prints the `op` version string (same as running `op --version` locally).

- [ ] **Step 4: Test passthrough mode**

```bash
unset LC_OP_TUNNEL_SOCK
./bin/op-tunnel-client --version
```

Expected: same output — finds and execs the real `op` binary.

- [ ] **Step 5: Test error case — socket not available**

```bash
export LC_OP_TUNNEL_SOCK="/tmp/nonexistent.sock"
./bin/op-tunnel-client --version
```

Expected: `op-tunnel: tunnel not connected` on stderr, exit code 1.

- [ ] **Step 6: Stop the server and clean up**

```bash
kill %1
```

- [ ] **Step 7: Run all tests**

```bash
make test && make test-integration
```

Expected: all tests pass.

- [ ] **Step 8: Final commit with any fixes**

If any fixes were needed during testing, commit them.
