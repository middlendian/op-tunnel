package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/middlendian/op-tunnel/oppath"
)

// ANSI colors
var (
	green  = "\033[1;32m"
	red    = "\033[1;31m"
	yellow = "\033[1;33m"
	bold   = "\033[1m"
	reset  = "\033[0m"
)

func init() {
	if os.Getenv("NO_COLOR") != "" || !isTerminal() {
		green, red, yellow, bold, reset = "", "", "", "", ""
	}
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func pass(msg string) {
	fmt.Printf("%s✓%s %s\n", green, reset, msg)
}

func fail(msg, fix string) {
	fmt.Printf("%s✗%s %s\n  → %s%s%s\n", red, reset, msg, yellow, fix, reset)
}

func warn(msg, fix string) {
	fmt.Printf("%s!%s %s\n  → %s%s%s\n", yellow, reset, msg, yellow, fix, reset)
}

func main() {
	fmt.Printf("\n%sop-tunnel doctor%s\n\n", bold, reset)

	user := os.Getenv("USER")
	failures := 0

	// 1. Server socket
	serverSock := oppath.ServerSocketPath(user)
	if conn, err := net.DialTimeout("unix", serverSock, 100*time.Millisecond); err == nil {
		_ = conn.Close()
		pass("Server is running")
	} else {
		fail("Server is not running", "brew services restart op-tunnel")
		failures++
	}

	// 2. Client socket (only if tunnel ID set)
	tunnelID := os.Getenv(oppath.EnvTunnelID)
	if tunnelID != "" {
		clientSock := oppath.ClientSocketPath(user, tunnelID)
		if conn, err := net.DialTimeout("unix", clientSock, 100*time.Millisecond); err == nil {
			_ = conn.Close()
			pass("Client tunnel is active")
		} else {
			fail("Client tunnel is not connected", "Reconnect your SSH session")
			failures++
		}
	} else {
		pass("Client tunnel check skipped (not in SSH session)")
	}

	// 3. Real op binary
	// FindRealOp must skip op-tunnel-client (the op wrapper), not the doctor itself.
	// Use exec.LookPath to find op-tunnel-client's path; fall back to the doctor's
	// own path if op-tunnel-client isn't found (catches fewer aliases, but safe).
	clientBin, err := exec.LookPath("op-tunnel-client")
	if err != nil {
		clientBin, _ = os.Executable()
	}
	realOp := oppath.FindRealOp(clientBin, os.Getenv("PATH"))
	if realOp != "" {
		pass(fmt.Sprintf("Real op binary: %s", realOp))
	} else {
		fail("Real op binary not found", "brew install --cask 1password-cli")
		failures++
	}

	// 4. Symlink
	symlink := filepath.Join(os.Getenv("HOME"), ".local", "bin", "op")
	if target, err := os.Readlink(symlink); err == nil {
		if clientPath, err := exec.LookPath("op-tunnel-client"); err == nil {
			resolved, _ := filepath.EvalSymlinks(symlink)
			resolvedClient, _ := filepath.EvalSymlinks(clientPath)
			if resolved == resolvedClient {
				pass(fmt.Sprintf("Symlink: %s -> %s", symlink, target))
			} else {
				fail(fmt.Sprintf("Symlink points to wrong target: %s", target), "brew reinstall op-tunnel")
				failures++
			}
		} else {
			fail("op-tunnel-client not found in PATH", "brew reinstall op-tunnel")
			failures++
		}
	} else {
		fail("~/.local/bin/op symlink missing", "brew reinstall op-tunnel")
		failures++
	}

	// 5. PATH
	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	localBin := filepath.Join(os.Getenv("HOME"), ".local", "bin")
	inPath := false
	for _, d := range pathDirs {
		if d == localBin {
			inPath = true
			break
		}
	}
	if inPath {
		pass("~/.local/bin is in PATH")
	} else {
		warn("~/.local/bin is not in PATH", "Add 'export PATH=\"$HOME/.local/bin:$PATH\"' to your shell rc file")
		failures++
	}

	// 6. SSH config
	sshConfig := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
	if data, err := os.ReadFile(sshConfig); err == nil {
		if strings.Contains(string(data), "op-tunnel/ssh.config") {
			pass("SSH config includes op-tunnel")
		} else {
			fail("SSH config does not include op-tunnel", "Add 'Include ~/.config/op-tunnel/ssh.config' under a Host block in ~/.ssh/config")
			failures++
		}
	} else {
		fail("~/.ssh/config not found", "Create ~/.ssh/config with op-tunnel Include")
		failures++
	}

	// 7. sshd drop-in
	sshdConf := "/etc/ssh/sshd_config.d/op-tunnel.conf"
	if _, err := os.Stat(sshdConf); err == nil {
		pass("sshd drop-in installed")
	} else {
		fail("sshd drop-in missing", "brew reinstall op-tunnel")
		failures++
	}

	// 8. Directory permissions
	userDir := filepath.Join(oppath.BaseDir, user)
	if info, err := os.Stat(userDir); err == nil {
		if info.Mode().Perm() == 0700 {
			pass(fmt.Sprintf("Directory permissions: %s (0700)", userDir))
		} else {
			fail(fmt.Sprintf("Directory permissions: %s (%04o, want 0700)", userDir, info.Mode().Perm()), "brew reinstall op-tunnel")
			failures++
		}
	} else {
		fail(fmt.Sprintf("Directory missing: %s", userDir), "brew reinstall op-tunnel")
		failures++
	}

	// 9. Config directory
	configDir := oppath.ConfigDir()
	tunnelIDFile := filepath.Join(configDir, "tunnel-id")
	sshConfigFile := filepath.Join(configDir, "ssh.config")
	if _, err := os.Stat(tunnelIDFile); err == nil {
		if _, err := os.Stat(sshConfigFile); err == nil {
			pass(fmt.Sprintf("Config: %s", configDir))
		} else {
			fail("ssh.config missing from config dir", "brew reinstall op-tunnel")
			failures++
		}
	} else {
		fail("tunnel-id missing from config dir", "brew reinstall op-tunnel")
		failures++
	}

	// Summary
	fmt.Println()
	if failures == 0 {
		fmt.Printf("%s✓ All checks passed.%s\n\n", green, reset)
	} else {
		fmt.Printf("%s✗ %d issue(s) found.%s\n\n", red, failures, reset)
		os.Exit(1)
	}
}
