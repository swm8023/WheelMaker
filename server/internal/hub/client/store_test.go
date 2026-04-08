package client_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/swm8023/wheelmaker/internal/hub/client"
)

func TestSQLiteStore_ProjectRouteAndSessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db", "client.sqlite3")

	store, err := client.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveProject(ctx, "proj-a", client.ProjectConfig{YOLO: true}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := store.SaveRouteBinding(ctx, "proj-a", "im:feishu:chat-1", "sess-1"); err != nil {
		t.Fatalf("SaveRouteBinding() error = %v", err)
	}
	if err := store.SaveSession(ctx, &client.SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj-a",
		Status:       client.SessionSuspended,
		ACPSessionID: "acp-1",
		AgentsJSON:   `{"claude":{"acpSessionId":"acp-1","title":"Persisted"}}`,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	cfg, err := store.LoadProject(ctx, "proj-a")
	if err != nil {
		t.Fatalf("LoadProject() error = %v", err)
	}
	if !cfg.YOLO {
		t.Fatalf("LoadProject().YOLO = false, want true")
	}

	bindings, err := store.LoadRouteBindings(ctx, "proj-a")
	if err != nil {
		t.Fatalf("LoadRouteBindings() error = %v", err)
	}
	if got := bindings["im:feishu:chat-1"]; got != "sess-1" {
		t.Fatalf("binding = %q, want sess-1", got)
	}

	rec, err := store.LoadSession(ctx, "proj-a", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if rec == nil || rec.ID != "sess-1" {
		t.Fatalf("LoadSession() = %+v, want sess-1", rec)
	}

	entries, err := store.ListSessions(ctx, "proj-a")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListSessions() len = %d, want 1", len(entries))
	}
	if entries[0].Agent != "claude" {
		t.Fatalf("ListSessions()[0].Agent = %q, want claude", entries[0].Agent)
	}
	if entries[0].Title != "Persisted" {
		t.Fatalf("ListSessions()[0].Title = %q, want Persisted", entries[0].Title)
	}
}

func TestSQLiteStore_RejectsEmptyRouteKey(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db", "client.sqlite3")

	store, err := client.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	err = store.SaveRouteBinding(context.Background(), "proj-a", "", "sess-1")
	if err == nil {
		t.Fatal("SaveRouteBinding() should reject empty route keys")
	}
}
