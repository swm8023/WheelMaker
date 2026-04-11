package client

import (
	"context"
	"path/filepath"
	"testing"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func TestStoreProjectAgentStateRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	cfg := ProjectConfig{
		YOLO: true,
		AgentState: map[string]ProjectAgentState{
			"codex": {
				ConfigOptions: []acp.ConfigOption{
					{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
					{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
					{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
				},
				AvailableCommands: []acp.AvailableCommand{{Name: "/status"}},
				UpdatedAt:         "2026-04-11T00:00:00Z",
			},
		},
	}
	if err := store.SaveProject(context.Background(), "proj1", cfg); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	loaded, err := store.LoadProject(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if !loaded.YOLO {
		t.Fatal("YOLO = false, want true")
	}
	codex := loaded.AgentState["codex"]
	if got := len(codex.ConfigOptions); got != 3 {
		t.Fatalf("config options = %d, want 3", got)
	}
	if got := len(codex.AvailableCommands); got != 1 {
		t.Fatalf("commands = %d, want 1", got)
	}
}
