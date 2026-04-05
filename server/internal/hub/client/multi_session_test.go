package client

import (
	"context"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/im"
)

// ---------------------------------------------------------------------------
// Client-level session management: /new, /load, /list
// ---------------------------------------------------------------------------

func TestClientNewSession_CreatesAndBindsRoute(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	oldSess := c.activeSession
	if oldSess == nil {
		t.Fatal("expected default session")
	}

	newSess := c.ClientNewSession("route-1")
	if newSess == nil {
		t.Fatal("ClientNewSession returned nil")
	}
	if newSess.ID == oldSess.ID {
		t.Fatal("new session should have different ID from old")
	}
	if c.activeSession != newSess {
		t.Fatal("activeSession should be the new session")
	}

	c.mu.Lock()
	sessID := c.routeMap["route-1"]
	c.mu.Unlock()
	if sessID != newSess.ID {
		t.Fatalf("routeMap[route-1] = %q, want %q", sessID, newSess.ID)
	}

	// Old session should still exist in the sessions map.
	c.mu.Lock()
	_, oldExists := c.sessions[oldSess.ID]
	c.mu.Unlock()
	if !oldExists {
		t.Fatal("old session should still be in sessions map")
	}
}

func TestClientNewSession_SuspendsOldSession(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	// Map "route-1" to the default session first.
	c.mu.Lock()
	c.routeMap["route-1"] = "default"
	c.mu.Unlock()

	oldSess := c.activeSession

	_ = c.ClientNewSession("route-1")

	oldSess.mu.Lock()
	status := oldSess.Status
	oldSess.mu.Unlock()
	if status != SessionSuspended {
		t.Fatalf("old session status = %d, want SessionSuspended(%d)", status, SessionSuspended)
	}
}

func TestClientListSessions_MergesInMemory(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	// Create a second session.
	_ = c.ClientNewSession("route-1")

	entries, err := c.clientListSessions()
	if err != nil {
		t.Fatalf("clientListSessions: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}

	// All should be in-memory.
	for _, e := range entries {
		if !e.InMemory {
			t.Fatalf("entry %q should be in-memory", e.ID)
		}
	}
}

func TestClientListSessions_MergesPersistedSessions(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)

	// Persist a session.
	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "persisted-1",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		LastReply:    "hello",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActiveAt: time.Now().Add(-30 * time.Minute),
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save snap: %v", err)
	}

	entries, err := c.clientListSessions()
	if err != nil {
		t.Fatalf("clientListSessions: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.ID == "persisted-1" {
			found = true
			if e.InMemory {
				t.Fatal("persisted session should not be marked in-memory")
			}
			if e.Status != SessionPersisted {
				t.Fatalf("persisted session status = %d, want SessionPersisted", e.Status)
			}
		}
	}
	if !found {
		t.Fatal("persisted session not in list")
	}
}

func TestClientLoadSession_RestoresFromStore(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)

	// Persist a session.
	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "restore-me",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		LastReply:    "previous reply",
		ACPSessionID: "acp-999",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActiveAt: time.Now().Add(-10 * time.Minute),
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save snap: %v", err)
	}

	// List to get index.
	entries, err := c.clientListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	// Find the index of "restore-me".
	idx := -1
	for i, e := range entries {
		if e.ID == "restore-me" {
			idx = i + 1
			break
		}
	}
	if idx == -1 {
		t.Fatal("restore-me not in list")
	}

	loaded, err := c.ClientLoadSession("route-1", idx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ID != "restore-me" {
		t.Fatalf("loaded session ID = %q, want %q", loaded.ID, "restore-me")
	}
	if loaded.lastReply != "previous reply" {
		t.Fatalf("loaded lastReply = %q, want %q", loaded.lastReply, "previous reply")
	}
	if loaded.Status != SessionActive {
		t.Fatalf("loaded status = %d, want SessionActive", loaded.Status)
	}

	// Route should point to the loaded session.
	c.mu.Lock()
	routedID := c.routeMap["route-1"]
	c.mu.Unlock()
	if routedID != "restore-me" {
		t.Fatalf("route-1 -> %q, want %q", routedID, "restore-me")
	}
}

func TestClientLoadSession_IndexOutOfRange(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	_, err := c.ClientLoadSession("route-1", 999)
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestClientLoadSession_InMemoryRebind(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	// Create 2 sessions.
	s1 := c.ClientNewSession("route-1")
	_ = c.ClientNewSession("route-1") // s2 now active, s1 suspended

	// List and find s1.
	entries, err := c.clientListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	idx := -1
	for i, e := range entries {
		if e.ID == s1.ID {
			idx = i + 1
			break
		}
	}
	if idx == -1 {
		t.Fatalf("s1 not in list")
	}

	loaded, err := c.ClientLoadSession("route-1", idx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ID != s1.ID {
		t.Fatalf("loaded ID = %q, want %q", loaded.ID, s1.ID)
	}
}

// ---------------------------------------------------------------------------
// Timer-driven eviction
// ---------------------------------------------------------------------------

func TestEvictSuspendedSessions(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)
	c.suspendTimeout = 0 // instant eviction

	// Create and suspend a session.
	c.mu.Lock()
	sess := c.newWiredSession("evict-me")
	sess.Status = SessionSuspended
	sess.lastActiveAt = time.Now().Add(-time.Minute)
	c.sessions["evict-me"] = sess
	c.mu.Unlock()

	c.evictSuspendedSessions()

	// Session should be removed from memory.
	c.mu.Lock()
	_, exists := c.sessions["evict-me"]
	c.mu.Unlock()
	if exists {
		t.Fatal("evicted session should not be in sessions map")
	}

	// Should be in SQLite.
	ctx := context.Background()
	snap, err := store.Load(ctx, "evict-me")
	if err != nil {
		t.Fatalf("load from store: %v", err)
	}
	if snap == nil {
		t.Fatal("evicted session not found in store")
	}
	if snap.ID != "evict-me" {
		t.Fatalf("snap ID = %q, want %q", snap.ID, "evict-me")
	}
}

func TestEvictSuspendedSessions_ActiveNotEvicted(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)
	c.suspendTimeout = 0

	// Default session is active, should not be evicted.
	c.evictSuspendedSessions()

	c.mu.Lock()
	_, exists := c.sessions["default"]
	c.mu.Unlock()
	if !exists {
		t.Fatal("active session should not be evicted")
	}
}

func TestEvictSuspendedSessions_RespectsTimeout(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)
	c.suspendTimeout = 1 * time.Hour // won't expire

	c.mu.Lock()
	sess := c.newWiredSession("not-yet")
	sess.Status = SessionSuspended
	sess.lastActiveAt = time.Now() // just suspended
	c.sessions["not-yet"] = sess
	c.mu.Unlock()

	c.evictSuspendedSessions()

	c.mu.Lock()
	_, exists := c.sessions["not-yet"]
	c.mu.Unlock()
	if !exists {
		t.Fatal("recently suspended session should not be evicted yet")
	}
}

// ---------------------------------------------------------------------------
// resolveSession: restore evicted sessions
// ---------------------------------------------------------------------------

func TestResolveSession_RestoresEvictedSession(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)

	// Persist a session to SQLite.
	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "evicted-sess",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		LastReply:    "restored content",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActiveAt: time.Now().Add(-5 * time.Minute),
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Set up routeMap pointing to the evicted session ID.
	c.mu.Lock()
	c.routeMap["route-x"] = "evicted-sess"
	c.mu.Unlock()

	// resolveSession should restore from SQLite.
	msg := im.Message{RouteKey: "route-x"}
	sess := c.resolveSession(msg)

	if sess.ID != "evicted-sess" {
		t.Fatalf("resolved session ID = %q, want %q", sess.ID, "evicted-sess")
	}
	if sess.lastReply != "restored content" {
		t.Fatalf("restored lastReply = %q, want %q", sess.lastReply, "restored content")
	}

	// Should now be in memory.
	c.mu.Lock()
	_, exists := c.sessions["evicted-sess"]
	c.mu.Unlock()
	if !exists {
		t.Fatal("restored session should be in sessions map")
	}
}

// ---------------------------------------------------------------------------
// Session ID generation
// ---------------------------------------------------------------------------

func TestNextSessionID_Unique(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		c.mu.Lock()
		id := c.nextSessionID()
		c.mu.Unlock()
		if ids[id] {
			t.Fatalf("duplicate session ID: %s", id)
		}
		ids[id] = true
	}
}
