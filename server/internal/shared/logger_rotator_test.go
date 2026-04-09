package shared

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoggerDebugRotator_RotateOnDayChange(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "hub.debug.log")

	now := time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC)
	r := newDebugDailyRotator(path, 7, func() time.Time { return now })
	defer r.Close()

	if _, err := r.Write([]byte("day1\n")); err != nil {
		t.Fatalf("write day1: %v", err)
	}
	now = now.Add(24 * time.Hour)
	if _, err := r.Write([]byte("day2\n")); err != nil {
		t.Fatalf("write day2: %v", err)
	}

	archived := filepath.Join(base, "hub.debug.2026-04-08.log")
	if _, err := os.Stat(archived); err != nil {
		t.Fatalf("expected archived file %s: %v", archived, err)
	}
}

func TestLoggerDebugRotator_KeepSevenDays(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "hub.debug.log")
	now := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	r := newDebugDailyRotator(path, 7, func() time.Time { return now })
	defer r.Close()

	for i := 1; i <= 10; i++ {
		d := now.AddDate(0, 0, -i)
		p := filepath.Join(base, "hub.debug."+d.Format("2006-01-02")+".log")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write archive: %v", err)
		}
	}
	if err := r.cleanupOldArchives(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	for i := 8; i <= 10; i++ {
		d := now.AddDate(0, 0, -i)
		p := filepath.Join(base, "hub.debug."+d.Format("2006-01-02")+".log")
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected removed: %s", p)
		}
	}
}
