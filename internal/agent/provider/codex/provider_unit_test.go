package codex_test

// adapter_unit_test.go: unit tests for CodexProvider that do not require a real
// codex-acp binary or network access. No //go:build integration tag.

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/swm8023/wheelmaker/internal/agent/provider/codex"
)

// TestCodexProvider_Connect_NotExecutable verifies that Connect() returns a
// descriptive error when the configured path resolves to a non-executable file.
// We use t.TempDir() as the ExePath: the directory exists on disk (os.Stat
// succeeds, so tools.ResolveBinary accepts it without PATH fallback), but
// exec.Command(dir).Start() fails because a directory cannot be executed.
// This is deterministic regardless of what binaries are installed on PATH.
func TestCodexProvider_Connect_NotExecutable(t *testing.T) {
	a := codex.NewProvider(codex.Config{
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
	if !strings.Contains(err.Error(), "codex") {
		t.Errorf("error should mention 'codex', got: %v", err)
	}
}

// TestCodexProvider_Connect_BinaryNotFound verifies that Connect() returns the
// "binary not found" error path ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â i.e. tools.ResolveBinary itself fails,
// not just conn.Start(). This matches AC-4's negative test:
// "when the binary cannot be found, Connect() returns error".
//
// Strategy: clear PATH so exec.LookPath cannot find codex-acp, and supply a
// non-existent ExePath so ResolveBinary skips option 1 (explicit config path)
// and falls through all lookup steps to the "not found" error.
func TestCodexProvider_Connect_BinaryNotFound(t *testing.T) {
	// Clear PATH to prevent exec.LookPath succeeding.
	t.Setenv("PATH", "")
	if runtime.GOOS == "windows" {
		// Windows env vars are case-insensitive; clearing both forms is belt-and-suspenders.
		t.Setenv("Path", "")
	}

	// Provide a non-existent ExePath so option 1 in ResolveBinary is skipped.
	a := codex.NewProvider(codex.Config{
		ExePath: filepath.Join(t.TempDir(), "nonexistent-codex-acp"),
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
	// The error should mention "codex" (from the "codex: resolve binary: ..." wrapper).
	if !strings.Contains(err.Error(), "codex") {
		t.Errorf("error should mention 'codex', got: %v", err)
	}
}

// TestCodexProvider_Close_Unit verifies that Close() is a no-op and idempotent,
// regardless of whether Connect() was called.
func TestCodexProvider_Close_Unit(t *testing.T) {
	a := codex.NewProvider(codex.Config{})
	if err := a.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

