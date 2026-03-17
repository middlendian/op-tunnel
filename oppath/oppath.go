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

// FindRealOp searches PATH for the real `op` binary, skipping any candidate
// that resolves (via symlink) to a file named "op-tunnel-client".
// This handles the case where op-tunnel-client is symlinked as `op` (including
// Homebrew cask installs of the real op, which are also symlinks).
func FindRealOp(pathEnv string) string {
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "op")
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		if filepath.Base(resolved) == "op-tunnel-client" {
			continue
		}
		return candidate
	}
	return ""
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
