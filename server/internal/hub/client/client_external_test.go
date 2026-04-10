package client_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swm8023/wheelmaker/internal/hub/client"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type mockSession struct {
	promptCalls []string
	cancelCalls int
	agentName   string
	sessionID   string
	promptFn    func(string) (<-chan acp.SessionUpdateParams, acp.SessionPromptResult, error)
}

func newTestClient(t *testing.T, mock *mockSession) *client.Client {
	t.Helper()
	store, err := client.NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	c := client.New(store, "test", t.TempDir())
	c.InjectForwarder(mock.agentName, mock.sessionID, func(_ context.Context, text string) (<-chan acp.SessionUpdateParams, acp.SessionPromptResult, error) {
		mock.promptCalls = append(mock.promptCalls, text)
		if mock.promptFn != nil {
			return mock.promptFn(text)
		}
		ch := make(chan acp.SessionUpdateParams)
		close(ch)
		return ch, acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn}, nil
	}, func() error {
		mock.cancelCalls++
		return nil
	})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func captureReplies(c *client.Client) *[]string {
	router := client.NewTestCaptureRouter()
	c.SetIMRouter(router)
	return &router.Messages
}

func TestStart_LoadsRouteBindingsWithoutRestoringSessions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db", "client.sqlite3")

	store, err := client.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveProject(ctx, "proj-a", client.ProjectConfig{YOLO: false}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := store.SaveRouteBinding(ctx, "proj-a", "im:feishu:chat-1", "sess-1"); err != nil {
		t.Fatalf("SaveRouteBinding() error = %v", err)
	}
	if err := store.SaveSession(ctx, &client.SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj-a",
		Status:       client.SessionPersisted,
		ACPSessionID: "acp-1",
		AgentsJSON:   `{"claude":{"acpSessionId":"acp-1"}}`,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	c := client.New(store, "proj-a", dir)
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if got := c.RouteSessionIDForTest("im:feishu:chat-1"); got != "sess-1" {
		t.Fatalf("route binding = %q, want sess-1", got)
	}
	if c.HasSessionInMemoryForTest("sess-1") {
		t.Fatal("persisted session should not be eagerly restored during Start()")
	}
}

func TestResolveSession_RejectsEmptyRouteKey(t *testing.T) {
	store, err := client.NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	c := client.New(store, "proj-a", t.TempDir())
	if _, err := c.ResolveSessionForTest(""); err == nil {
		t.Fatal(`ResolveSessionForTest("") should fail`)
	}
}

func TestHandleMessage_Cancel(t *testing.T) {
	mock := &mockSession{agentName: "claude", sessionID: "sess-1"}
	c := newTestClient(t, mock)
	msgs := captureReplies(c)

	c.HandleMessage(client.Message{ChatID: "chat1", Text: "/cancel"})

	if mock.cancelCalls != 1 {
		t.Fatalf("Cancel called %d times, want 1", mock.cancelCalls)
	}
	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "Cancelled") {
		t.Fatalf("reply = %v, want Cancelled", *msgs)
	}
}

func TestHandleMessage_Status(t *testing.T) {
	mock := &mockSession{agentName: "codex", sessionID: "sess-abc"}
	c := newTestClient(t, mock)
	msgs := captureReplies(c)

	c.HandleMessage(client.Message{ChatID: "chat1", Text: "/status"})

	if len(*msgs) == 0 {
		t.Fatal("no reply received")
	}
	reply := (*msgs)[0]
	if !strings.Contains(reply, "codex") || !strings.Contains(reply, "sess-abc") {
		t.Fatalf("status reply %q missing expected data", reply)
	}
}

func TestHandleMessage_PromptTextStreaming(t *testing.T) {
	mock := &mockSession{
		agentName: "codex",
		sessionID: "sess-1",
		promptFn: func(_ string) (<-chan acp.SessionUpdateParams, acp.SessionPromptResult, error) {
			ch := make(chan acp.SessionUpdateParams, 2)
			content1, _ := json.Marshal(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello "})
			content2, _ := json.Marshal(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "world"})
			ch <- acp.SessionUpdateParams{Update: acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: content1}}
			ch <- acp.SessionUpdateParams{Update: acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: content2}}
			close(ch)
			return ch, acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn}, nil
		},
	}
	c := newTestClient(t, mock)
	msgs := captureReplies(c)

	c.HandleMessage(client.Message{ChatID: "chat1", Text: "hi there"})

	if len(mock.promptCalls) != 1 || mock.promptCalls[0] != "hi there" {
		t.Fatalf("Prompt called with %v, want [hi there]", mock.promptCalls)
	}
	// msgs[0] is the session-info system message; the streamed text follows.
	found := false
	for _, m := range *msgs {
		if m == "hello world" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("reply = %v, want a message containing 'hello world'", *msgs)
	}
}

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
