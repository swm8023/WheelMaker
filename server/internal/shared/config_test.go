package shared

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_IMVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"projects":[{"name":"p","path":".","im":{"type":"feishu","version":2}}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Projects[0].IM.Version; got != 2 {
		t.Fatalf("im.version=%d, want 2", got)
	}
}
