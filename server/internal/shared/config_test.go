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
