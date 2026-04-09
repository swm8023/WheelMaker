package shared

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig_RejectsRemovedIMVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"projects":[{"name":"p","path":".","im":{"type":"feishu","version":2}}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "im.version has been removed") {
		t.Fatalf("err=%v, want removed im.version error", err)
	}
}

func TestLoadConfig_RejectsRemovedProjectDebug(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"projects":[{"name":"p","debug":true,"path":".","im":{"type":"app"}}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "projects[].debug has been removed") {
		t.Fatalf("err=%v, want removed project debug error", err)
	}
}

func TestLoadConfig_AllowsDebugLogLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"log":{"level":"debug"},"projects":[{"name":"p","path":".","im":{"type":"app"}}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("log level=%q, want %q", cfg.Log.Level, "debug")
	}
}

func TestLoadConfig_ConfigExampleIsValid(t *testing.T) {
	path := filepath.Join("..", "..", "config.example.json")
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig(config.example.json) error = %v", err)
	}
}

type panicStringer struct{}

func (panicStringer) String() string {
	panic("String() should not be called for filtered logs")
}

func TestLogger_FilteredLogsDoNotFormatArguments(t *testing.T) {
	var out bytes.Buffer
	if err := Setup(LoggerConfig{Level: LevelWarn}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer Close()
	SetOutput(&out)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("filtered log unexpectedly formatted arguments: %v", r)
		}
	}()

	Debug("%v", panicStringer{})
	Info("%v", panicStringer{})
}

func TestDeriveDebugLogPath(t *testing.T) {
	base := filepath.Join("tmp", "hub.log")
	got := deriveDebugLogPath(base)
	want := filepath.Join("tmp", "hub.debug.log")
	if got != want {
		t.Fatalf("deriveDebugLogPath(%q)=%q, want %q", base, got, want)
	}
}

func TestLoggerDebugRotator_RotateOnDayChange(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "hub.debug.log")

	now := time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC)
	r := newDebugDailyRotator(path, 1, func() time.Time { return now })
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

func TestLoggerDebugRotator_KeepOneDay(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "hub.debug.log")
	now := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	r := newDebugDailyRotator(path, 1, func() time.Time { return now })
	defer r.Close()

	for i := 1; i <= 4; i++ {
		d := now.AddDate(0, 0, -i)
		p := filepath.Join(base, "hub.debug."+d.Format("2006-01-02")+".log")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write archive: %v", err)
		}
	}
	if err := r.cleanupOldArchives(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	keep := filepath.Join(base, "hub.debug."+now.AddDate(0, 0, -1).Format("2006-01-02")+".log")
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("expected kept: %s", keep)
	}

	for i := 2; i <= 4; i++ {
		d := now.AddDate(0, 0, -i)
		p := filepath.Join(base, "hub.debug."+d.Format("2006-01-02")+".log")
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected removed: %s", p)
		}
	}
}
