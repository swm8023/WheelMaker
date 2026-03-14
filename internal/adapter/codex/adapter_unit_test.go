package codex_test

// adapter_unit_test.go: unit tests for CodexAdapter that do not require a real
// codex-acp binary or network access. No //go:build integration tag.

import (
	"context"
	"strings"
	"testing"

	"github.com/swm8023/wheelmaker/internal/adapter/codex"
)

// TestCodexAdapter_Connect_MissingBinary verifies that Connect() returns a
// descriptive error when the configured path cannot be executed.
// We use t.TempDir() as the ExePath: the directory exists on disk (so
// tools.ResolveBinary accepts it), but executing a directory always fails,
// giving us a deterministic error regardless of binaries on PATH.
func TestCodexAdapter_Connect_MissingBinary(t *testing.T) {
	a := codex.NewAdapter(codex.Config{
		ExePath: t.TempDir(), // exists but not executable
	})

	ctx := context.Background()
	conn, err := a.Connect(ctx)
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("expected error when binary path does not exist, got nil")
	}
	if conn != nil {
		_ = conn.Close()
		t.Error("expected nil conn when Connect fails, got non-nil")
	}
	// Verify the error message is informative.
	if !strings.Contains(err.Error(), "codex") {
		t.Errorf("error should mention 'codex', got: %v", err)
	}
}

// TestCodexAdapter_Close_Unit verifies that Close() is a no-op and idempotent,
// regardless of whether Connect() was called.
func TestCodexAdapter_Close_Unit(t *testing.T) {
	a := codex.NewAdapter(codex.Config{})
	if err := a.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
