package client

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
	_ "modernc.org/sqlite"
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

func TestStoreMigratesLegacyProjectsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacyDB.Exec(`
		CREATE TABLE projects (
			project_name TEXT PRIMARY KEY,
			yolo INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("create legacy projects table: %v", err)
	}
	if _, err := legacyDB.Exec(`
		INSERT INTO projects (project_name, yolo, created_at, updated_at)
		VALUES ('proj1', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("insert legacy project row: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	loaded, err := store.LoadProject(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProject after migration: %v", err)
	}
	if !loaded.YOLO {
		t.Fatal("YOLO = false, want true")
	}
	if got := len(loaded.AgentState); got != 0 {
		t.Fatalf("agent state size = %d, want 0", got)
	}

	next := ProjectConfig{
		YOLO: true,
		AgentState: map[string]ProjectAgentState{
			"codex": {
				AvailableCommands: []acp.AvailableCommand{{Name: "/help"}},
			},
		},
	}
	if err := store.SaveProject(context.Background(), "proj1", next); err != nil {
		t.Fatalf("SaveProject after migration: %v", err)
	}
}
