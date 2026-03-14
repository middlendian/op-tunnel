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

// ExpandSocketPath resolves a relative socket dir (e.g., ServerSocketDir) to
// an absolute path like /Users/greg/.local/share/op-tunnel/server/op-tunnel.sock.
func ExpandSocketPath(relDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
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
