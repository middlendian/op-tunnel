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
	defer os.RemoveAll(tmpDir)
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
	defer os.RemoveAll(tmpDir)
	sockPath := filepath.Join(tmpDir, "test.sock")
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
	tmpDir, err := os.MkdirTemp("", "op-tunnel-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)
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
