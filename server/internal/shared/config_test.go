package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
