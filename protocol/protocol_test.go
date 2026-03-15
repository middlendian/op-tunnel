package protocol

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadMessageRoundTrip(t *testing.T) {
	payload := []byte(`{"v":1,"args":["item","list"]}`)

	var buf bytes.Buffer
	if err := WriteMessage(&buf, payload); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	if buf.Len() != 4+len(payload) {
		t.Fatalf("expected %d bytes, got %d", 4+len(payload), buf.Len())
	}

	got, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, payload)
	}
}

func TestWriteMessageRejectsOversized(t *testing.T) {
	var buf bytes.Buffer
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, MaxPayloadSize+1)
	buf.Write(header)
	_, err := ReadMessage(&buf)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
}

func TestWriteReadEmptyPayload(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMessage(&buf, []byte{}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	got, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty payload, got %d bytes", len(got))
	}
}

func TestExpandSocketPath(t *testing.T) {
	path, err := ExpandSocketPath(ServerSocketDir)
	if err != nil {
		t.Fatalf("ExpandSocketPath: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ServerSocketDir, SocketName)
	if path != expected {
		t.Fatalf("got %q, want %q", path, expected)
	}
}

func TestFilterEnv(t *testing.T) {
	t.Setenv("OP_FORMAT", "json")
	t.Setenv("SECRET_KEY", "hunter2")

	filtered := FilterEnv()

	if v, ok := filtered["OP_FORMAT"]; !ok || v != "json" {
		t.Fatalf("expected OP_FORMAT=json, got %v", filtered)
	}
	if _, ok := filtered["SECRET_KEY"]; ok {
		t.Fatal("SECRET_KEY should not be in filtered env")
	}
}

func TestFilterEnvEmpty(t *testing.T) {
	for _, v := range AllowedEnvVars {
		if err := os.Unsetenv(v); err != nil {
			t.Fatalf("Unsetenv(%q): %v", v, err)
		}
	}

	filtered := FilterEnv()
	if len(filtered) != 0 {
		t.Fatalf("expected empty map, got %v", filtered)
	}
}

func TestRequestRoundTrip(t *testing.T) {
	req := &Request{
		V:    1,
		Args: []string{"item", "get", "GitHub"},
		Env:  map[string]string{"OP_FORMAT": "json"},
		TTY:  true,
	}

	var buf bytes.Buffer
	if err := SendRequest(&buf, req); err != nil {
		t.Fatalf("SendRequest: %v", err)
	}

	got, err := ReadRequest(&buf)
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}

	if got.V != req.V || got.TTY != req.TTY || len(got.Args) != len(req.Args) {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, req)
	}
}

func TestResponseRoundTrip(t *testing.T) {
	resp := &Response{
		V:        1,
		ExitCode: 0,
		Stdout:   "aGVsbG8=",
		Stderr:   "",
	}

	var buf bytes.Buffer
	if err := SendResponse(&buf, resp); err != nil {
		t.Fatalf("SendResponse: %v", err)
	}

	got, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}

	if got.ExitCode != resp.ExitCode || got.Stdout != resp.Stdout {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, resp)
	}
}

func TestResponseWithError(t *testing.T) {
	resp := &Response{
		V:        1,
		ExitCode: -1,
		Error:    "op not found",
	}

	var buf bytes.Buffer
	if err := SendResponse(&buf, resp); err != nil {
		t.Fatalf("SendResponse: %v", err)
	}

	got, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}

	if got.Error != "op not found" || got.ExitCode != -1 {
		t.Fatalf("error response mismatch: got %+v", got)
	}
}
