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

func TestClientServerRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

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

	time.Sleep(10 * time.Millisecond)

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

func TestVersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

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
