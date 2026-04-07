package hub

import (
	"context"
	"path/filepath"
	"testing"

	shared "github.com/swm8023/wheelmaker/internal/shared"
)

func TestBuildClient_WiresIM2WithoutTouchingIMPackage(t *testing.T) {
	cfg := &shared.AppConfig{Projects: []shared.ProjectConfig{{
		Name: "proj",
		Path: ".",
		IM:   shared.IMConfig{Type: "console"},
	}}}

	h := New(cfg, filepath.Join(t.TempDir(), "state.json"))
	c, err := h.buildClient(context.Background(), cfg.Projects[0])
	if err != nil {
		t.Fatalf("buildClient: %v", err)
	}
	if c == nil || !c.HasIM2Router() {
		t.Fatal("expected client with IM2 router")
	}

	h.clients = append(h.clients, c)
	if err := h.Close(); err != nil {
		t.Fatalf("close hub: %v", err)
	}
}
