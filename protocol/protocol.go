package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	ProtocolVersion = 1
	MaxPayloadSize  = 64 << 20 // 64MB
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

// WriteMessage writes a length-prefixed message. Rejects payloads > MaxPayloadSize.
func WriteMessage(w io.Writer, payload []byte) error {
	if len(payload) > MaxPayloadSize {
		return fmt.Errorf("payload size %d exceeds maximum %d", len(payload), MaxPayloadSize)
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("writing payload: %w", err)
	}
	return nil
}

// ReadMessage reads a length-prefixed message. Rejects payloads > MaxPayloadSize.
func ReadMessage(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	size := binary.BigEndian.Uint32(header)
	if size > MaxPayloadSize {
		return nil, fmt.Errorf("payload size %d exceeds maximum %d", size, MaxPayloadSize)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("reading payload: %w", err)
	}
	return payload, nil
}

// SendRequest marshals and writes a framed request.
func SendRequest(w io.Writer, req *Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	return WriteMessage(w, data)
}

// ReadRequest reads and unmarshals a framed request.
func ReadRequest(r io.Reader) (*Request, error) {
	data, err := ReadMessage(r)
	if err != nil {
		return nil, err
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("unmarshaling request: %w", err)
	}
	return &req, nil
}

// SendResponse marshals and writes a framed response.
func SendResponse(w io.Writer, resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshaling response: %w", err)
	}
	return WriteMessage(w, data)
}

// ReadResponse reads and unmarshals a framed response.
func ReadResponse(r io.Reader) (*Response, error) {
	data, err := ReadMessage(r)
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}
	return &resp, nil
}

// ErrorResponse creates a tunnel-level error response.
func ErrorResponse(msg string) *Response {
	return &Response{V: ProtocolVersion, ExitCode: -1, Error: msg}
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
