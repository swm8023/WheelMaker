package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
)

func tempDBPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

func TestSQLiteSessionStore_SaveLoad(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	snap := &SessionSnapshot{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       SessionSuspended,
		ActiveAgent:  "claude",
		LastReply:    "hello world",
		ACPSessionID: "acp-123",
		CreatedAt:    now.Add(-time.Hour),
		LastActiveAt: now,
		Agents: map[string]*SessionAgentState{
			"claude": {
				ACPSessionID: "acp-123",
				ConfigOptions: []acp.ConfigOption{
					{ID: "mode", CurrentValue: "code"},
				},
				Title:     "Test Session",
				UpdatedAt: "2025-01-01T00:00:00Z",
			},
		},
	}

	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load(ctx, "sess-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded is nil")
	}
	if loaded.ID != "sess-1" {
		t.Errorf("id=%q, want sess-1", loaded.ID)
	}
	if loaded.Status != SessionSuspended {
		t.Errorf("status=%d, want %d", loaded.Status, SessionSuspended)
	}
	if loaded.ActiveAgent != "claude" {
		t.Errorf("activeAgent=%q, want claude", loaded.ActiveAgent)
	}
	if loaded.LastReply != "hello world" {
		t.Errorf("lastReply=%q, want hello world", loaded.LastReply)
	}
	if loaded.ACPSessionID != "acp-123" {
		t.Errorf("acpSessionID=%q, want acp-123", loaded.ACPSessionID)
	}
	if as := loaded.Agents["claude"]; as == nil {
		t.Error("missing agent claude")
	} else {
		if as.Title != "Test Session" {
			t.Errorf("agent title=%q, want 'Test Session'", as.Title)
		}
		if len(as.ConfigOptions) != 1 || as.ConfigOptions[0].ID != "mode" {
			t.Errorf("agent configOptions=%v, want [{ID:mode}]", as.ConfigOptions)
		}
	}
}

func TestSQLiteSessionStore_LoadNonExistent(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	loaded, err := store.Load(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil for nonexistent, got %+v", loaded)
	}
}

func TestSQLiteSessionStore_List(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	for i, id := range []string{"a", "b", "c"} {
		snap := &SessionSnapshot{
			ID:           id,
			ProjectName:  "proj1",
			ActiveAgent:  "claude",
			CreatedAt:    now.Add(time.Duration(-3+i) * time.Hour),
			LastActiveAt: now.Add(time.Duration(-3+i) * time.Hour),
		}
		if err := store.Save(ctx, snap); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	entries, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("list len=%d, want 3", len(entries))
	}
	// Should be ordered by last_active DESC.
	if entries[0].ID != "c" || entries[1].ID != "b" || entries[2].ID != "a" {
		t.Errorf("order: %s, %s, %s — want c, b, a", entries[0].ID, entries[1].ID, entries[2].ID)
	}
}

func TestSQLiteSessionStore_Delete(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "del-me",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
		Agents: map[string]*SessionAgentState{
			"claude": {ACPSessionID: "acp-1"},
		},
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := store.Delete(ctx, "del-me"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	loaded, err := store.Load(ctx, "del-me")
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if loaded != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestSQLiteSessionStore_Upsert(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "upsert-me",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		LastReply:    "first",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save 1: %v", err)
	}

	snap.LastReply = "second"
	snap.ActiveAgent = "copilot"
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save 2: %v", err)
	}

	loaded, err := store.Load(ctx, "upsert-me")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.LastReply != "second" {
		t.Errorf("lastReply=%q, want second", loaded.LastReply)
	}
	if loaded.ActiveAgent != "copilot" {
		t.Errorf("activeAgent=%q, want copilot", loaded.ActiveAgent)
	}
}

func TestSQLiteSessionStore_ProjectIsolation(t *testing.T) {
	dbPath := tempDBPath(t)

	store1, err := NewSQLiteSessionStore(dbPath, "proj1")
	if err != nil {
		t.Fatalf("new store1: %v", err)
	}
	defer store1.Close()

	store2, err := NewSQLiteSessionStore(dbPath, "proj2")
	if err != nil {
		t.Fatalf("new store2: %v", err)
	}
	defer store2.Close()

	ctx := context.Background()
	_ = store1.Save(ctx, &SessionSnapshot{ID: "s1", ProjectName: "proj1", CreatedAt: time.Now(), LastActiveAt: time.Now()})
	_ = store2.Save(ctx, &SessionSnapshot{ID: "s2", ProjectName: "proj2", CreatedAt: time.Now(), LastActiveAt: time.Now()})

	// proj1 should only see s1.
	list1, _ := store1.List(ctx)
	if len(list1) != 1 || list1[0].ID != "s1" {
		t.Errorf("proj1 list: %v", list1)
	}

	// proj2 should only see s2.
	list2, _ := store2.List(ctx)
	if len(list2) != 1 || list2[0].ID != "s2" {
		t.Errorf("proj2 list: %v", list2)
	}

	// proj1 cannot load s2.
	loaded, _ := store1.Load(ctx, "s2")
	if loaded != nil {
		t.Error("proj1 should not load proj2's session")
	}
}

func TestSQLiteSessionStore_FileCreation(t *testing.T) {
	dbPath := tempDBPath(t)
	store, err := NewSQLiteSessionStore(dbPath, "test")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}
