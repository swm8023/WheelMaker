package hub_monitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoreGetStatus(t *testing.T) {
	core := New(t.TempDir())
	status, err := core.GetServiceStatus()
	if err != nil {
		t.Fatalf("GetServiceStatus: %v", err)
	}
	if status.Timestamp == "" {
		t.Fatalf("timestamp should not be empty")
	}
}

func TestCoreGetLogs_NormalizesFileAndTail(t *testing.T) {
	base := t.TempDir()
	core := New(base)
	logPath := filepath.Join(base, "log", "hub.log")
	if err := writeFile(logPath, "line1\nline2\nline3\n"); err != nil {
		t.Fatalf("write log: %v", err)
	}
	res, err := core.GetLogs("hub", "", 2)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if res.File != "hub" {
		t.Fatalf("file=%q want hub", res.File)
	}
	if res.Total != 2 {
		t.Fatalf("total=%d want 2", res.Total)
	}
	if len(res.Entries) != 2 || res.Entries[0].Message != "line2" || res.Entries[1].Message != "line3" {
		t.Fatalf("unexpected entries: %#v", res.Entries)
	}
}

func TestCoreGetDBTables_NoDBReturnsErrorResult(t *testing.T) {
	core := New(t.TempDir())
	res := core.GetDBTables()
	if res.Error == "" {
		t.Fatalf("expected db error when client.sqlite3 missing")
	}
}

func TestCoreAction_UnsupportedAction(t *testing.T) {
	core := New(t.TempDir())
	if err := core.ExecuteAction("unknown-action"); err == nil {
		t.Fatalf("expected unsupported action error")
	}
}

func writeFile(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
