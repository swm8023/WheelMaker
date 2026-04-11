package sessionview

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/swm8023/wheelmaker/internal/hub/client"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type stubRuntime struct{}

func (stubRuntime) CreateSession(context.Context, string) (*client.Session, error) { return nil, nil }
func (stubRuntime) SendToSession(context.Context, string, any, any) error          { return nil }

func TestSessionViewAggregatesAssistantChunksIntoSingleMessage(t *testing.T) {
	store, err := client.NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	svc := New("proj1", store, nil)
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventSessionCreated, SessionID: "sess-1", Title: "New Session"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventAssistantChunk, SessionID: "sess-1", Role: "assistant", Kind: "text", Text: "hello"}); err != nil {
		t.Fatalf("RecordEvent chunk1: %v", err)
	}
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventAssistantChunk, SessionID: "sess-1", Role: "assistant", Kind: "text", Text: " world"}); err != nil {
		t.Fatalf("RecordEvent chunk2: %v", err)
	}
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventPromptFinished, SessionID: "sess-1"}); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	_, messages, err := svc.ReadSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].Text != "hello world" {
		t.Fatalf("messages[0].Text = %q, want %q", messages[0].Text, "hello world")
	}
}

func TestSessionViewListIncludesProjectionFields(t *testing.T) {
	store, err := client.NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	svc := New("proj1", store, nil)
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventSessionCreated, SessionID: "sess-1", Title: "Task"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventUserMessageAccepted, SessionID: "sess-1", Role: "user", Kind: "text", Text: "hello"}); err != nil {
		t.Fatalf("RecordEvent user message: %v", err)
	}

	sessions, err := svc.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
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

func TestSessionViewPreservesUserImageBlocksAndPermissionOptions(t *testing.T) {
	store, err := client.NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	svc := New("proj1", store, nil)
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventSessionCreated, SessionID: "sess-1", Title: "Images"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{
		Type:      client.SessionViewEventUserMessageAccepted,
		SessionID: "sess-1",
		Role:      "user",
		Kind:      "text",
		Text:      "Sent an image",
		Blocks:    []acp.ContentBlock{{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "abc123"}},
	}); err != nil {
		t.Fatalf("RecordEvent user image message: %v", err)
	}
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{
		Type:      client.SessionViewEventPermissionRequested,
		SessionID: "sess-1",
		Role:      "system",
		Kind:      "permission",
		Text:      "Run tool?",
		RequestID: 42,
		Options:   []acp.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: "allow_once"}},
	}); err != nil {
		t.Fatalf("RecordEvent permission requested: %v", err)
	}

	summary, messages, err := svc.ReadSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
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
	store, err := client.NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	svc := New("proj1", store, nil)
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventSessionCreated, SessionID: "sess-1", Title: "Tools"}); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventToolUpdated, SessionID: "sess-1", Role: "system", Kind: "tool", Text: "Running build", AggregateKey: "tool-1"}); err != nil {
		t.Fatalf("RecordEvent tool updated #1: %v", err)
	}
	if err := svc.RecordEvent(context.Background(), client.SessionViewEvent{Type: client.SessionViewEventToolUpdated, SessionID: "sess-1", Role: "system", Kind: "tool", Text: "Build finished", AggregateKey: "tool-1"}); err != nil {
		t.Fatalf("RecordEvent tool updated #2: %v", err)
	}

	summary, messages, err := svc.ReadSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
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
