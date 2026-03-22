package claude_test

// agent_test.go: unit tests for claude.Agent that do not require a real
// claude-agent-acp binary or network access. No //go:build integration tag.

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/swm8023/wheelmaker/internal/agent/claude"
)

// TestBackend_Connect_NotExecutable verifies that Connect() returns a
// descriptive error when the configured path resolves to a non-executable file.
// We use t.TempDir() as the ExePath: the directory exists on disk (os.Stat
// succeeds, so tools.ResolveBinary accepts it without PATH fallback), but
// exec.Command(dir).Start() fails because a directory cannot be executed.
func TestBackend_Connect_NotExecutable(t *testing.T) {
	a := claude.New(claude.Config{
		ExePath: t.TempDir(), // exists but not executable
	})

	ctx := context.Background()
	conn, err := a.Connect(ctx)
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("expected error when binary path is not executable, got nil")
	}
	if conn != nil {
		_ = conn.Close()
		t.Error("expected nil conn when Connect fails, got non-nil")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should mention 'claude', got: %v", err)
	}
}

// TestBackend_Connect_BinaryNotFound verifies that Connect() returns the
// "binary not found" error path when the binary cannot be located.
func TestBackend_Connect_BinaryNotFound(t *testing.T) {
	// Clear PATH to prevent exec.LookPath succeeding.
	t.Setenv("PATH", "")
	if runtime.GOOS == "windows" {
		t.Setenv("Path", "")
	}

	// Provide a non-existent ExePath so ResolveBinary skips option 1.
	a := claude.New(claude.Config{
		ExePath: filepath.Join(t.TempDir(), "nonexistent-claude-agent-acp"),
	})

	ctx := context.Background()
	conn, err := a.Connect(ctx)
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("expected error when binary cannot be found, got nil")
	}
	if conn != nil {
		_ = conn.Close()
		t.Error("expected nil conn when Connect fails, got non-nil")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should mention 'claude', got: %v", err)
	}
}

// TestBackend_Close_Unit verifies that Close() is a no-op and idempotent.
func TestBackend_Close_Unit(t *testing.T) {
	a := claude.New(claude.Config{})
	if err := a.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestBackend_Name verifies that Name() returns "claude".
func TestBackend_Name(t *testing.T) {
	a := claude.New(claude.Config{})
	if got := a.Name(); got != "claude" {
		t.Errorf("Name() = %q, want %q", got, "claude")
	}
}
