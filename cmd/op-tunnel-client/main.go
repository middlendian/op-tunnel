package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/middlendian/op-tunnel/protocol"
)

// expandTilde expands a leading ~/ to the user's home directory.
// sshd does not expand ~ in SetEnv values; the SSH client may or may not
// depending on version. This ensures the path is always absolute.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "op-tunnel: warning: could not resolve home directory: %v\n", err)
		return path
	}
	return filepath.Join(home, path[2:])
}

func main() {
	sockPath := expandTilde(os.Getenv(protocol.EnvTunnelSock))
	if sockPath != "" {
		tunnelMode(sockPath, os.Args[1:])
	} else {
		passthroughMode(os.Args[1:])
	}
}

func tunnelMode(sockPath string, args []string) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "op-tunnel: tunnel not connected")
		os.Exit(1)
	}

	// Close connection on signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGPIPE)
	go func() {
		<-sigCh
		_ = conn.Close()
		os.Exit(1)
	}()

	req := &protocol.Request{
		V:    protocol.ProtocolVersion,
		Args: args,
		Env:  protocol.FilterEnv(),
		TTY:  term.IsTerminal(int(os.Stdout.Fd())),
	}

	if err := protocol.SendRequest(conn, req); err != nil {
		fmt.Fprintf(os.Stderr, "op-tunnel: sending request: %v\n", err)
		os.Exit(1)
	}

	resp, err := protocol.ReadResponse(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "op-tunnel: reading response: %v\n", err)
		os.Exit(1)
	}

	_ = conn.Close()

	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "op-tunnel: %s\n", resp.Error)
		os.Exit(1)
	}

	if resp.Stdout != "" {
		decoded, err := base64.StdEncoding.DecodeString(resp.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "op-tunnel: decoding stdout: %v\n", err)
			os.Exit(1)
		}
		if _, err := os.Stdout.Write(decoded); err != nil {
			fmt.Fprintf(os.Stderr, "op-tunnel: writing stdout: %v\n", err)
			os.Exit(1)
		}
	}

	if resp.Stderr != "" {
		decoded, err := base64.StdEncoding.DecodeString(resp.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "op-tunnel: decoding stderr: %v\n", err)
			os.Exit(1)
		}
		_, _ = os.Stderr.Write(decoded)
	}

	os.Exit(resp.ExitCode)
}

func passthroughMode(args []string) {
	realOp := findRealOp()
	if realOp == "" {
		fmt.Fprintln(os.Stderr, "op-tunnel: op not found (install 1Password CLI or connect a tunnel)")
		os.Exit(1)
	}

	argv := append([]string{"op"}, args...)
	if err := syscall.Exec(realOp, argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "op-tunnel: exec %s: %v\n", realOp, err)
		os.Exit(1)
	}
}

// findRealOp searches PATH for the real `op` binary, skipping our own directory.
func findRealOp() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return ""
	}
	selfDir := filepath.Dir(self)

	pathEnv := os.Getenv("PATH")
	for _, dir := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if dir == "" {
			continue
		}
		// Resolve symlinks on the PATH dir too, so we catch Homebrew symlink dirs
		resolvedDir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			resolvedDir = dir
		}
		if resolvedDir == selfDir {
			continue
		}
		candidate := filepath.Join(dir, "op")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate
		}
	}
	return ""
}
