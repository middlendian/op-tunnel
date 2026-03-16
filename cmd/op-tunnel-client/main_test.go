package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/middlendian/op-tunnel/oppath"
)

func TestClientSocketPath(t *testing.T) {
	t.Setenv(oppath.EnvTunnelID, "abcdef1234567890abcdef1234567890")
	t.Setenv("USER", "testuser")

	got := oppath.ClientSocketPath(os.Getenv("USER"), os.Getenv(oppath.EnvTunnelID))
	want := "/opt/op-tunnel/testuser/client/abcdef1234567890abcdef1234567890.sock"
	if got != want {
		t.Errorf("socket path = %q, want %q", got, want)
	}
}

func TestTunnelMode_DialFailure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "op-tunnel-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	sockPath := filepath.Join(tmpDir, "nonexistent.sock")
	_, err = net.DialTimeout("unix", sockPath, 100*1e6)
	if err == nil {
		t.Fatal("expected dial to fail on nonexistent socket")
	}
}

func TestTunnelMode_DialSuccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "op-tunnel-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	sockPath := filepath.Join(tmpDir, "test.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	conn, err := net.DialTimeout("unix", sockPath, 100*1e6)
	if err != nil {
		t.Fatalf("expected dial to succeed: %v", err)
	}
	_ = conn.Close()
}

func TestFindRealOp_ClientIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "op-tunnel-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	selfDir := filepath.Join(tmpDir, "self")
	aliasDir := filepath.Join(tmpDir, "alias")
	realDir := filepath.Join(tmpDir, "real")
	if err := os.MkdirAll(selfDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(aliasDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}

	selfBin := filepath.Join(selfDir, "op")
	if err := os.WriteFile(selfBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(selfBin, filepath.Join(aliasDir, "op")); err != nil {
		t.Fatal(err)
	}

	realBin := filepath.Join(realDir, "op")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\necho real\n"), 0755); err != nil {
		t.Fatal(err)
	}

	path := aliasDir + string(os.PathListSeparator) + realDir
	got := oppath.FindRealOp(selfBin, path)
	if got != realBin {
		t.Errorf("FindRealOp = %q, want %q", got, realBin)
	}
}
