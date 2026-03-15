package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandTildeNoHome(t *testing.T) {
	orig := os.Getenv("HOME")
	if err := os.Unsetenv("HOME"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	defer func() { _ = os.Setenv("HOME", orig) }()

	// On systems where HOME is the only source, UserHomeDir will fail.
	// expandTilde should return the path unchanged (with a warning to stderr).
	result := expandTilde("~/foo")
	if result != "~/foo" {
		t.Errorf("expandTilde with no HOME: got %q, want %q", result, "~/foo")
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"~/.local/share/op-tunnel/client/op-tunnel.sock", filepath.Join(home, ".local/share/op-tunnel/client/op-tunnel.sock")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~notslash", "~notslash"},
		{"", ""},
	}

	for _, tt := range tests {
		got := expandTilde(tt.input)
		if got != tt.want {
			t.Errorf("expandTilde(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
