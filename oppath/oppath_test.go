package oppath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientSocketPath(t *testing.T) {
	got := ClientSocketPath("alice", "abc123def456")
	want := "/opt/op-tunnel/alice/client/abc123def456.sock"
	if got != want {
		t.Errorf("ClientSocketPath = %q, want %q", got, want)
	}
}

func TestServerSocketPath(t *testing.T) {
	got := ServerSocketPath("bob")
	want := "/opt/op-tunnel/bob/server/op.sock"
	if got != want {
		t.Errorf("ServerSocketPath = %q, want %q", got, want)
	}
}

func TestConfigDir_Default(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	got := ConfigDir()
	want := filepath.Join(home, ".config", "op-tunnel")
	if got != want {
		t.Errorf("ConfigDir = %q, want %q", got, want)
	}
}

func TestConfigDir_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	got := ConfigDir()
	want := "/custom/config/op-tunnel"
	if got != want {
		t.Errorf("ConfigDir = %q, want %q", got, want)
	}
}

func TestFindRealOp_SkipsSelf(t *testing.T) {
	tmpDir := t.TempDir()
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

	got := FindRealOp(selfBin, path)
	if got != realBin {
		t.Errorf("FindRealOp = %q, want %q", got, realBin)
	}
}

func TestFindRealOp_NoneFound(t *testing.T) {
	tmpDir := t.TempDir()
	selfBin := filepath.Join(tmpDir, "op")
	os.WriteFile(selfBin, []byte("#!/bin/sh\n"), 0755)

	got := FindRealOp(selfBin, tmpDir)
	if got != "" {
		t.Errorf("FindRealOp = %q, want empty string", got)
	}
}

func TestFindRealOp_SkipsNonExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	selfDir := filepath.Join(tmpDir, "self")
	otherDir := filepath.Join(tmpDir, "other")
	os.MkdirAll(selfDir, 0755)
	os.MkdirAll(otherDir, 0755)

	selfBin := filepath.Join(selfDir, "op")
	os.WriteFile(selfBin, []byte("#!/bin/sh\n"), 0755)

	os.WriteFile(filepath.Join(otherDir, "op"), []byte("data"), 0644)

	got := FindRealOp(selfBin, otherDir)
	if got != "" {
		t.Errorf("FindRealOp = %q, want empty string", got)
	}
}
