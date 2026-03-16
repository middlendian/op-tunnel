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
