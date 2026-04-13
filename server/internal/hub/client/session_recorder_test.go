package client

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSessionReadAfterSubIndexReturnsUpdatedMessage(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventSessionCreated, SessionID: "sess-1", Title: "Task"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventPermissionRequested, SessionID: "sess-1", Role: "system", Kind: "permission", Text: "Run tool?", RequestID: 42}); err != nil {
		t.Fatalf("RecordEvent permission requested: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventPermissionResolved, SessionID: "sess-1", RequestID: 42, Status: "done", UpdatedAt: mustRFC3339Time(t, "2026-04-12T10:02:00Z")}); err != nil {
		t.Fatalf("RecordEvent permission resolved: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterIndex": 1, "afterSubIndex": 0})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(context.Background(), "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	messages := body["messages"].([]sessionViewMessage)
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].SyncIndex != 1 {
		t.Fatalf("messages[0].SyncIndex = %d, want 1", messages[0].SyncIndex)
	}
	if messages[0].SubIndex != 1 {
		t.Fatalf("messages[0].SubIndex = %d, want 1", messages[0].SubIndex)
	}
	if messages[0].Status != "done" {
		t.Fatalf("messages[0].Status = %q, want done", messages[0].Status)
	}
	if got := body["lastIndex"].(int64); got != 1 {
		t.Fatalf("lastIndex = %d, want 1", got)
	}
	if got := body["lastSubIndex"].(int64); got != 1 {
		t.Fatalf("lastSubIndex = %d, want 1", got)
	}
}
