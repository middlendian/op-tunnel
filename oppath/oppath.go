package oppath

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	// EnvTunnelID is the environment variable set by SSH's SetEnv
	// to identify the local machine's tunnel.
	EnvTunnelID = "LC_OP_TUNNEL_ID"
)

// UserDir returns the per-user runtime directory for op-tunnel sockets.
func UserDir(user string) string {
	return "/tmp/op-tunnel-" + user
}

// VerifyDirOwnership checks that dir exists and is owned by the current user.
func VerifyDirOwnership(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("unable to get ownership info for %s", dir)
	}
	if stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("%s is owned by uid %d, expected %d", dir, stat.Uid, os.Getuid())
	}
	return nil
}

// ClientSocketPath returns the path for a remote-forwarded client socket.
func ClientSocketPath(user, tunnelID string) string {
	return filepath.Join(UserDir(user), "client", tunnelID+".sock")
}

// ServerSocketPath returns the path for the local op-tunnel-server socket.
func ServerSocketPath(user string) string {
	return filepath.Join(UserDir(user), "server", "op.sock")
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
