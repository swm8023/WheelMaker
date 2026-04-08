package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestGetLogs_DebugOmitsTimeLevelAndDedupsSessionID(t *testing.T) {
	base := t.TempDir()
	m := NewMonitor(base)

	logDir := filepath.Join(base, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	sid := "019d6db0-3e60-7cf3-85c6-d2bf7e2a6f8a"
	line := "2026/04/09 06:44:32 DEBUG [acp] < {" + sid + " session/update} {\"sessionId\":\"" + sid + "\",\"update\":{\"sessionUpdate\":\"agent_message_chunk\"}}"
	if err := os.WriteFile(filepath.Join(logDir, "hub.debug.log"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write debug log: %v", err)
	}

	res, err := m.GetLogs("debug", "", 100)
	if err != nil {
		t.Fatalf("GetLogs(debug): %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("entries=%d, want 1", len(res.Entries))
	}
	entry := res.Entries[0]
	if entry.Time != "" {
		t.Fatalf("debug time should be hidden, got %q", entry.Time)
	}
	if entry.Level != "" {
		t.Fatalf("debug level should be hidden, got %q", entry.Level)
	}
	if strings.Contains(entry.Message, "\"sessionId\":\""+sid+"\"") {
		t.Fatalf("duplicate sessionId should be removed from debug json payload: %q", entry.Message)
	}
	if !strings.Contains(entry.Message, "{019d6db0..6f8a session/update}") {
		t.Fatalf("session id should be shortened in debug prefix: %q", entry.Message)
	}
}
