package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/middlendian/op-tunnel/oppath"
	"github.com/middlendian/op-tunnel/protocol"
)

func main() {
	tunnelID := os.Getenv(oppath.EnvTunnelID)
	if tunnelID != "" {
		user := os.Getenv("USER")
		if user == "" {
			fmt.Fprintln(os.Stderr, "op-tunnel: USER environment variable not set")
			os.Exit(1)
		}
		sockPath := oppath.ClientSocketPath(user, tunnelID)
		conn, err := net.DialTimeout("unix", sockPath, 100*time.Millisecond)
		if err != nil {
			fmt.Fprintln(os.Stderr, "op-tunnel: tunnel not connected")
			os.Exit(1)
		}
		tunnelMode(conn, os.Args[1:])
	} else {
		passthroughMode(os.Args[1:])
	}
}

func tunnelMode(conn net.Conn, args []string) {
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
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "op-tunnel: cannot determine own path")
		os.Exit(1)
	}
	realOp := oppath.FindRealOp(self, os.Getenv("PATH"))
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
