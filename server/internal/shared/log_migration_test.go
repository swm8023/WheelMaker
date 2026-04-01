package shared

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyLogFile_MovesWhenTargetMissing(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "hub.log")
	target := filepath.Join(base, "log", "hub.log")
	if err := os.WriteFile(legacy, []byte("old"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if err := MigrateLegacyLogFile(legacy, target); err != nil {
		t.Fatalf("MigrateLegacyLogFile error: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy should be removed after migration")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("target content=%q, want %q", string(data), "old")
	}
}

func TestMigrateLegacyLogFile_NoOverwriteExistingTarget(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "registry.log")
	target := filepath.Join(base, "log", "registry.log")
	if err := os.WriteFile(legacy, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("new"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := MigrateLegacyLogFile(legacy, target); err != nil {
		t.Fatalf("MigrateLegacyLogFile error: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("target content=%q, want %q", string(data), "new")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy should remain when target already exists: %v", err)
	}
}
