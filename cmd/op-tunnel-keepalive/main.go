package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/middlendian/op-tunnel/oppath"
)

// ANSI colors for stderr messages
var (
	green  = "\033[1;32m"
	yellow = "\033[1;33m"
	red    = "\033[1;31m"
	reset  = "\033[0m"
)

func init() {
	if os.Getenv("NO_COLOR") != "" || !isTerminal() {
		green, yellow, red, reset = "", "", "", ""
	}
}

func isTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func info(msg string) {
	fmt.Fprintf(os.Stderr, "%s✓%s %s\n", green, reset, msg)
}

func warn(msg string) {
	fmt.Fprintf(os.Stderr, "%s!%s %s\n", yellow, reset, msg)
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "%s✗%s %s\n", red, reset, msg)
	os.Exit(1)
}

func main() {
	sessionPID := flag.Int("session-pid", 0, "PID of sshd session process to monitor")
	tunnelID := flag.String("tunnel-id", "", "LC_OP_TUNNEL_ID value")
	flag.Parse()

	if *sessionPID == 0 || *tunnelID == "" {
		fmt.Fprintf(os.Stderr, "Usage: op-tunnel-keepalive --session-pid PID --tunnel-id ID\n")
		os.Exit(1)
	}

	user := os.Getenv("USER")
	if user == "" {
		die("USER environment variable not set")
	}

	socketPath := oppath.ClientSocketPath(user, *tunnelID)
	clientDir := filepath.Dir(socketPath)
	userDir := oppath.UserDir(user)

	// Daemon mode: monitor session PID and clean up on exit
	if os.Getenv("_OP_TUNNEL_KEEPALIVE_DAEMON") != "" {
		daemonLoop(*sessionPID, socketPath)
		return
	}

	// Foreground mode: check state, print status, daemonize if needed

	// Check/create directory
	if _, err := os.Stat(userDir); os.IsNotExist(err) {
		oldUmask := syscall.Umask(0077)
		err := os.MkdirAll(clientDir, 0700)
		syscall.Umask(oldUmask)
		if err != nil {
			die(fmt.Sprintf("creating directory: %v", err))
		}
		warn("op-tunnel: ready — reconnect to activate")
		os.Exit(0)
	}

	// Verify ownership
	if err := oppath.VerifyDirOwnership(userDir); err != nil {
		die(fmt.Sprintf("op-tunnel: %v", err))
	}

	// Ensure client subdirectory exists
	if err := os.MkdirAll(clientDir, 0700); err != nil {
		die(fmt.Sprintf("creating client directory: %v", err))
	}

	// Check socket state
	if _, err := os.Stat(socketPath); err == nil {
		// Socket file exists — check liveness
		conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
		if err == nil {
			// Socket is alive — RemoteForward succeeded (or another session owns it)
			_ = conn.Close()
			info("op-tunnel: active")
			daemonize()
			return
		}
		// Stale socket — clean it up, ask user to reconnect
		_ = os.Remove(socketPath)
		warn("op-tunnel: stale socket removed — reconnect to activate")
		os.Exit(0)
	}

	// Socket file doesn't exist yet — directory was just created
	// RemoteForward hasn't bound yet or failed; nothing to monitor
	warn("op-tunnel: ready — reconnect to activate")
	os.Exit(0)
}

func daemonize() {
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Env = append(os.Environ(), "_OP_TUNNEL_KEEPALIVE_DAEMON=1")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		die(fmt.Sprintf("daemonize: %v", err))
	}
	os.Exit(0)
}

func daemonLoop(sessionPID int, socketPath string) {
	for {
		time.Sleep(2 * time.Second)
		if err := syscall.Kill(sessionPID, 0); err != nil {
			// Session process is gone — only delete if socket is stale
			conn, dialErr := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
			if dialErr != nil {
				// Socket is stale — clean it up
				_ = os.Remove(socketPath)
			} else {
				// Socket is alive (another session took over) — leave it
				_ = conn.Close()
			}
			os.Exit(0)
		}
	}
}
