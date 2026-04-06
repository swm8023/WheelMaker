package im2

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteState_PersistAndReload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "im2_state.db")

	st, err := NewState(dbPath, "proj1")
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if err := st.BindActiveChat(context.Background(), "", "feishu", "chat-1", "session-1", true); err != nil {
		t.Fatalf("BindActiveChat: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := NewState(dbPath, "proj1")
	if err != nil {
		t.Fatalf("NewState(reload): %v", err)
	}
	defer reloaded.Close()

	sid, ok, err := reloaded.ResolveClientSessionID(context.Background(), "feishu:chat-1")
	if err != nil {
		t.Fatalf("ResolveClientSessionID: %v", err)
	}
	if !ok || sid != "session-1" {
		t.Fatalf("resolved=(%v,%q), want (true,session-1)", ok, sid)
	}
}

func TestSQLiteState_DebouncedFlush(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "im2_state_debounce.db")

	st, err := newSQLiteState(dbPath, "proj1", 30*time.Millisecond)
	if err != nil {
		t.Fatalf("newSQLiteState: %v", err)
	}
	defer st.Close()

	if err := st.BindActiveChat(context.Background(), "", "feishu", "chat-2", "session-2", false); err != nil {
		t.Fatalf("BindActiveChat(async): %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	sid, ok, err := st.ResolveClientSessionID(context.Background(), "feishu:chat-2")
	if err != nil {
		t.Fatalf("ResolveClientSessionID: %v", err)
	}
	if !ok || sid != "session-2" {
		t.Fatalf("resolved=(%v,%q), want (true,session-2)", ok, sid)
	}
}
