package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/middlendian/op-tunnel/protocol"
)

const defaultTimeout = 5 * time.Minute

func main() {
	socketPath, err := protocol.ExpandSocketPath(protocol.ServerSocketDir)
	if err != nil {
		log.Fatalf("resolving socket path: %v", err)
	}

	// Create socket directory with restrictive permissions
	socketDir := filepath.Dir(socketPath)
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		log.Fatalf("creating socket directory: %v", err)
	}

	// Remove stale socket
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Fatalf("removing stale socket: %v", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("listening on %s: %v", socketPath, err)
	}
	defer func() { _ = listener.Close() }()

	// Set socket permissions
	if err := os.Chmod(socketPath, 0600); err != nil {
		log.Fatalf("setting socket permissions: %v", err)
	}

	log.Printf("op-tunnel-server: listening on %s", socketPath)

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	go func() {
		<-ctx.Done()
		log.Println("op-tunnel-server: shutting down")
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break // shutting down
			}
			log.Printf("accept error: %v", err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			handleConnection(ctx, conn)
		}()
	}

	wg.Wait()
}

func handleConnection(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	req, err := protocol.ReadRequest(conn)
	if err != nil {
		log.Printf("reading request: %v", err)
		return
	}

	if req.V != protocol.ProtocolVersion {
		resp := protocol.ErrorResponse(fmt.Sprintf("unsupported protocol version: %d", req.V))
		protocol.SendResponse(conn, resp)
		return
	}

	// Find op binary
	opPath, err := exec.LookPath("op")
	if err != nil {
		resp := protocol.ErrorResponse("op binary not found in PATH")
		protocol.SendResponse(conn, resp)
		return
	}

	// Execute with timeout; also cancel if client disconnects
	cmdCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// Monitor connection: if client disconnects, cancel the command
	go func() {
		buf := make([]byte, 1)
		conn.Read(buf) // blocks until EOF or error (client disconnect)
		cancel()
	}()

	cmd := exec.CommandContext(cmdCtx, opPath, req.Args...)

	// Build clean environment with only allowed vars
	cmd.Env = buildEnv(req.Env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	exitCode := 0
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			resp := protocol.ErrorResponse("command timed out")
			protocol.SendResponse(conn, resp)
			return
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			resp := protocol.ErrorResponse(fmt.Sprintf("executing op: %v", err))
			protocol.SendResponse(conn, resp)
			return
		}
	}

	resp := &protocol.Response{
		V:        protocol.ProtocolVersion,
		ExitCode: exitCode,
		Stdout:   base64.StdEncoding.EncodeToString(stdout.Bytes()),
		Stderr:   base64.StdEncoding.EncodeToString(stderr.Bytes()),
	}
	protocol.SendResponse(conn, resp)
}

func buildEnv(reqEnv map[string]string) []string {
	// Start with essential vars from current process
	env := []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"USER=" + os.Getenv("USER"),
	}

	// Overlay allowlisted vars from request
	allowed := make(map[string]bool)
	for _, k := range protocol.AllowedEnvVars {
		allowed[k] = true
	}
	for k, v := range reqEnv {
		if allowed[k] {
			env = append(env, k+"="+v)
		}
	}
	return env
}
