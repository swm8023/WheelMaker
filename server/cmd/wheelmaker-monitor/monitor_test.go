package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLogFilePath_PrefersLogDir(t *testing.T) {
	base := t.TempDir()
	m := NewMonitor(base)

	newPath := filepath.Join(base, "log", "hub.log")
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write new log: %v", err)
	}
	oldPath := filepath.Join(base, "hub.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old log: %v", err)
	}

	got := m.resolveLogFilePath("hub")
	if got != newPath {
		t.Fatalf("resolveLogFilePath(hub)=%q, want %q", got, newPath)
	}
}

func TestResolveLogFilePath_FallbackOldRoot(t *testing.T) {
	base := t.TempDir()
	m := NewMonitor(base)

	oldPath := filepath.Join(base, "registry.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old log: %v", err)
	}

	got := m.resolveLogFilePath("registry")
	if got != oldPath {
		t.Fatalf("resolveLogFilePath(registry)=%q, want %q", got, oldPath)
	}
}
