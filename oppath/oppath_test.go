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

func TestFindRealOp_SkipsClientSymlink(t *testing.T) {
	// ~/.local/bin/op -> op-tunnel-client should be skipped;
	// the real op binary (named "op") in another dir should be returned.
	tmpDir := t.TempDir()
	clientDir := filepath.Join(tmpDir, "client")
	aliasDir := filepath.Join(tmpDir, "alias")
	realDir := filepath.Join(tmpDir, "real")
	for _, d := range []string{clientDir, aliasDir, realDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// op-tunnel-client binary
	clientBin := filepath.Join(clientDir, "op-tunnel-client")
	if err := os.WriteFile(clientBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	// alias: op -> op-tunnel-client (like ~/.local/bin/op)
	if err := os.Symlink(clientBin, filepath.Join(aliasDir, "op")); err != nil {
		t.Fatal(err)
	}

	// real op binary named "op"
	realBin := filepath.Join(realDir, "op")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\necho real\n"), 0755); err != nil {
		t.Fatal(err)
	}

	path := aliasDir + string(os.PathListSeparator) + realDir
	got := FindRealOp(path)
	if got != realBin {
		t.Errorf("FindRealOp = %q, want %q", got, realBin)
	}
}

func TestFindRealOp_SkipsClientSymlinkAsHombrew(t *testing.T) {
	// Homebrew cask op: /opt/homebrew/bin/op -> Caskroom/.../op (named "op", not "op-tunnel-client")
	// Should NOT be skipped.
	tmpDir := t.TempDir()
	cellarDir := filepath.Join(tmpDir, "cellar")
	brewBinDir := filepath.Join(tmpDir, "brewbin")
	for _, d := range []string{cellarDir, brewBinDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// The real binary inside Cellar, named "op"
	cellarBin := filepath.Join(cellarDir, "op")
	if err := os.WriteFile(cellarBin, []byte("#!/bin/sh\necho op\n"), 0755); err != nil {
		t.Fatal(err)
	}
	// Homebrew bin symlink: op -> Cellar/op
	if err := os.Symlink(cellarBin, filepath.Join(brewBinDir, "op")); err != nil {
		t.Fatal(err)
	}

	got := FindRealOp(brewBinDir)
	if got != filepath.Join(brewBinDir, "op") {
		t.Errorf("FindRealOp = %q, want %q", got, filepath.Join(brewBinDir, "op"))
	}
}

func TestFindRealOp_NoneFound(t *testing.T) {
	tmpDir := t.TempDir()
	// Only an op-tunnel-client symlink, no real op
	clientBin := filepath.Join(tmpDir, "op-tunnel-client")
	if err := os.WriteFile(clientBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(clientBin, filepath.Join(tmpDir, "op")); err != nil {
		t.Fatal(err)
	}

	got := FindRealOp(tmpDir)
	if got != "" {
		t.Errorf("FindRealOp = %q, want empty string", got)
	}
}

func TestFindRealOp_SkipsNonExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "op"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	got := FindRealOp(tmpDir)
	if got != "" {
		t.Errorf("FindRealOp = %q, want empty string", got)
	}
}
