package client

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

func newSessionViewTestClient(t *testing.T) *Client {
	t.Helper()

	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	c := New(store, "proj1", t.TempDir())
	c.SetSessionViewSink(c)
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}

func addRuntimeSession(c *Client, sessionID, title, agent string, createdAt, lastActiveAt time.Time) {
	sess := c.newWiredSession(sessionID)
	sess.mu.Lock()
	sess.activeAgent = agent
	if state := sess.agentStateLocked(sess.currentAgentNameLocked()); state != nil {
		state.Title = title
	}
	sess.Status = SessionActive
	sess.createdAt = createdAt
	sess.lastActiveAt = lastActiveAt
	sess.mu.Unlock()

	c.mu.Lock()
	c.sessions[sess.ID] = sess
	c.mu.Unlock()
}

func TestSessionViewAggregatesAssistantChunksIntoSingleMessage(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventSessionCreated, SessionID: "sess-1", Title: "New Session"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventAssistantChunk, SessionID: "sess-1", Role: "assistant", Kind: "text", Text: "hello"}); err != nil {
		t.Fatalf("RecordEvent chunk1: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventAssistantChunk, SessionID: "sess-1", Role: "assistant", Kind: "text", Text: " world"}); err != nil {
		t.Fatalf("RecordEvent chunk2: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventPromptFinished, SessionID: "sess-1"}); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	_, messages, _, err := c.readSessionView(context.Background(), "sess-1", 0)
	if err != nil {
		t.Fatalf("readSessionView: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].Text != "hello world" {
		t.Fatalf("messages[0].Text = %q, want %q", messages[0].Text, "hello world")
	}
}

func TestSessionViewListIncludesProjectionFields(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventSessionCreated, SessionID: "sess-1", Title: "Task"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventUserMessageAccepted, SessionID: "sess-1", Role: "user", Kind: "text", Text: "hello"}); err != nil {
		t.Fatalf("RecordEvent user message: %v", err)
	}

	sessions, err := c.listSessionViews(context.Background())
	if err != nil {
		t.Fatalf("listSessionViews: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if sessions[0].Title != "Task" {
		t.Fatalf("sessions[0].Title = %q, want %q", sessions[0].Title, "Task")
	}
	if sessions[0].Preview != "hello" {
		t.Fatalf("sessions[0].Preview = %q, want %q", sessions[0].Preview, "hello")
	}
	if sessions[0].MessageCount != 1 {
		t.Fatalf("sessions[0].MessageCount = %d, want 1", sessions[0].MessageCount)
	}
}

func TestSessionViewListIncludesRuntimeClientSessions(t *testing.T) {
	c := newSessionViewTestClient(t)

	addRuntimeSession(
		c,
		"sess-runtime-1",
		"Runtime Session",
		"claude",
		mustRFC3339Time(t, "2026-04-12T10:00:00Z"),
		mustRFC3339Time(t, "2026-04-12T10:05:00Z"),
	)

	sessions, err := c.listSessionViews(context.Background())
	if err != nil {
		t.Fatalf("listSessionViews: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if sessions[0].SessionID != "sess-runtime-1" {
		t.Fatalf("sessions[0].SessionID = %q, want %q", sessions[0].SessionID, "sess-runtime-1")
	}
	if sessions[0].Title != "Runtime Session" {
		t.Fatalf("sessions[0].Title = %q, want %q", sessions[0].Title, "Runtime Session")
	}
	if sessions[0].Status != "active" {
		t.Fatalf("sessions[0].Status = %q, want %q", sessions[0].Status, "active")
	}
	if sessions[0].Agent != "claude" {
		t.Fatalf("sessions[0].Agent = %q, want %q", sessions[0].Agent, "claude")
	}
}

func TestSessionViewListPreservesStoredProjectionMetadataForRuntimeSessions(t *testing.T) {
	c := newSessionViewTestClient(t)

	ctx := context.Background()
	lastMessageAt := mustRFC3339Time(t, "2026-04-12T10:08:00Z")
	if err := c.store.SaveSession(ctx, &SessionRecord{
		ID:                 "sess-runtime-1",
		ProjectName:        "proj1",
		Status:             SessionSuspended,
		Title:              "Persisted Title",
		LastMessagePreview: "persisted preview",
		LastMessageAt:      lastMessageAt,
		MessageCount:       7,
		CreatedAt:          mustRFC3339Time(t, "2026-04-12T10:00:00Z"),
		LastActiveAt:       mustRFC3339Time(t, "2026-04-12T10:05:00Z"),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	addRuntimeSession(
		c,
		"sess-runtime-1",
		"Runtime Session",
		"claude",
		mustRFC3339Time(t, "2026-04-12T10:00:00Z"),
		mustRFC3339Time(t, "2026-04-12T10:05:00Z"),
	)

	sessions, err := c.listSessionViews(ctx)
	if err != nil {
		t.Fatalf("listSessionViews: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if sessions[0].Title != "Runtime Session" {
		t.Fatalf("sessions[0].Title = %q, want %q", sessions[0].Title, "Runtime Session")
	}
	if sessions[0].Preview != "persisted preview" {
		t.Fatalf("sessions[0].Preview = %q, want %q", sessions[0].Preview, "persisted preview")
	}
	if sessions[0].MessageCount != 7 {
		t.Fatalf("sessions[0].MessageCount = %d, want 7", sessions[0].MessageCount)
	}
	if sessions[0].UpdatedAt != lastMessageAt.Format(time.RFC3339) {
		t.Fatalf("sessions[0].UpdatedAt = %q, want %q", sessions[0].UpdatedAt, lastMessageAt.Format(time.RFC3339))
	}
}

func TestSessionViewPreservesUserImageBlocksAndPermissionOptions(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventSessionCreated, SessionID: "sess-1", Title: "Images"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{
		Type:      SessionViewEventUserMessageAccepted,
		SessionID: "sess-1",
		Role:      "user",
		Kind:      "text",
		Text:      "Sent an image",
		Blocks:    []acp.ContentBlock{{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "abc123"}},
	}); err != nil {
		t.Fatalf("RecordEvent user image message: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{
		Type:      SessionViewEventPermissionRequested,
		SessionID: "sess-1",
		Role:      "system",
		Kind:      "permission",
		Text:      "Run tool?",
		RequestID: 42,
		Options:   []acp.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: "allow_once"}},
	}); err != nil {
		t.Fatalf("RecordEvent permission requested: %v", err)
	}

	summary, messages, _, err := c.readSessionView(context.Background(), "sess-1", 0)
	if err != nil {
		t.Fatalf("readSessionView: %v", err)
	}
	if summary.MessageCount != 2 {
		t.Fatalf("summary.MessageCount = %d, want 2", summary.MessageCount)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if len(messages[0].Blocks) != 1 || messages[0].Blocks[0].Type != acp.ContentBlockTypeImage {
		t.Fatalf("messages[0].Blocks = %#v, want image block", messages[0].Blocks)
	}
	if len(messages[1].Options) != 1 || messages[1].Options[0].OptionID != "allow" {
		t.Fatalf("messages[1].Options = %#v, want allow option", messages[1].Options)
	}
	if messages[1].Status != "needs_action" {
		t.Fatalf("messages[1].Status = %q, want needs_action", messages[1].Status)
	}
}

func TestSessionViewToolUpdatesReuseSingleMessage(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventSessionCreated, SessionID: "sess-1", Title: "Tools"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventToolUpdated, SessionID: "sess-1", Role: "system", Kind: "tool", Text: "Running build", AggregateKey: "tool-1"}); err != nil {
		t.Fatalf("RecordEvent tool updated #1: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventToolUpdated, SessionID: "sess-1", Role: "system", Kind: "tool", Text: "Build finished", AggregateKey: "tool-1"}); err != nil {
		t.Fatalf("RecordEvent tool updated #2: %v", err)
	}

	summary, messages, _, err := c.readSessionView(context.Background(), "sess-1", 0)
	if err != nil {
		t.Fatalf("readSessionView: %v", err)
	}
	if summary.MessageCount != 1 {
		t.Fatalf("summary.MessageCount = %d, want 1", summary.MessageCount)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].Text != "Build finished" {
		t.Fatalf("messages[0].Text = %q, want %q", messages[0].Text, "Build finished")
	}
}

func TestSessionViewReadAfterIndexReturnsIncrementalMessages(t *testing.T) {
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

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterIndex": 1})
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
	if messages[0].Status != "done" {
		t.Fatalf("messages[0].Status = %q, want done", messages[0].Status)
	}
	if messages[0].SyncIndex != 2 {
		t.Fatalf("messages[0].SyncIndex = %d, want 2", messages[0].SyncIndex)
	}
	if got := body["lastIndex"].(int64); got != 2 {
		t.Fatalf("lastIndex = %d, want 2", got)
	}
}

func TestSessionViewStreamingChunksAdvanceSyncIndexBeforeFlush(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventSessionCreated, SessionID: "sess-1", Title: "Stream"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), SessionViewEvent{Type: SessionViewEventAssistantChunk, SessionID: "sess-1", Role: "assistant", Kind: "text", Text: "hello", Status: "streaming"}); err != nil {
		t.Fatalf("RecordEvent chunk: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterIndex": 0})
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
	if messages[0].Status != "streaming" {
		t.Fatalf("messages[0].Status = %q, want streaming", messages[0].Status)
	}
	if messages[0].SyncIndex != 1 {
		t.Fatalf("messages[0].SyncIndex = %d, want 1", messages[0].SyncIndex)
	}
}

func mustRFC3339Time(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q): %v", value, err)
	}
	return parsed
}
