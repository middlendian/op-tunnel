package oppath

import (
	"os"
	"path/filepath"
)

const (
	// BaseDir is the root directory for op-tunnel sockets.
	BaseDir = "/opt/op-tunnel"

	// EnvTunnelID is the environment variable set by SSH's SetEnv
	// to identify the local machine's tunnel.
	EnvTunnelID = "LC_OP_TUNNEL_ID"
)

// ClientSocketPath returns the path for a remote-forwarded client socket.
func ClientSocketPath(user, tunnelID string) string {
	return filepath.Join(BaseDir, user, "client", tunnelID+".sock")
}

// ServerSocketPath returns the path for the local op-tunnel-server socket.
func ServerSocketPath(user string) string {
	return filepath.Join(BaseDir, user, "server", "op.sock")
}

// ConfigDir returns the op-tunnel configuration directory,
// respecting $XDG_CONFIG_HOME (default ~/.config/op-tunnel).
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "op-tunnel")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "op-tunnel")
}
