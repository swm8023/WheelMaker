package client

// client_test.go: unit tests for client.Client using a mock agent.Session.
// Uses package-internal access to inject mock dependencies.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/im"
)

// --- mock Session ---

type mockSession struct {
	mu           sync.Mutex
	promptCalls  []string
	cancelCalls  int
	adapterN     string
	sessionN     string
	promptResult func(text string) (<-chan agent.Update, error)
}

func (m *mockSession) Prompt(ctx context.Context, text string) (<-chan agent.Update, error) {
	m.mu.Lock()
	m.promptCalls = append(m.promptCalls, text)
	fn := m.promptResult
	m.mu.Unlock()
	if fn != nil {
		return fn(text)
	}
	// Default: return single done update
	ch := make(chan agent.Update, 1)
	ch <- agent.Update{Type: agent.UpdateDone, Content: "end_turn", Done: true}
	close(ch)
	return ch, nil
}

func (m *mockSession) Cancel() error {
	m.mu.Lock()
	m.cancelCalls++
	m.mu.Unlock()
	return nil
}

func (m *mockSession) SetMode(_ context.Context, modeID string) error { return nil }

func (m *mockSession) AdapterName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.adapterN
}

func (m *mockSession) SessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionN
}

func (m *mockSession) Close() error { return nil }

// compile-time check
var _ agent.Session = (*mockSession)(nil)

// --- mock Store ---

type mockStore struct {
	mu    sync.Mutex
	state *State
	saved []*State
}

func (s *mockStore) Load() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return defaultState(), nil
	}
	return s.state, nil
}

func (s *mockStore) Save(st *State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, st)
	return nil
}

// --- helper ---

// newTestClient creates a Client for testing with an injected mock session.
// c.ag is nil (no concrete agent), so switchAdapter skips Switch().
func newTestClient(mock *mockSession) *Client {
	store := &mockStore{state: defaultState()}
	c := New(store, nil)
	c.state = defaultState()
	c.session = mock
	return c
}

// captureReplies redirects Client replies to a string slice for inspection.
// It uses a pair of functions: inject into c and collect messages.
func captureReplies(c *Client) *[]string {
	messages := &[]string{}
	// Override via a fake IM adapter.
	c.imRun = &captureAdapter{messages: messages}
	return messages
}

type captureAdapter struct {
	messages *[]string
}

func (a *captureAdapter) OnMessage(_ im.MessageHandler) {}
func (a *captureAdapter) SendText(_ string, text string) error {
	*a.messages = append(*a.messages, text)
	return nil
}
func (a *captureAdapter) SendCard(_ string, _ im.Card) error             { return nil }
func (a *captureAdapter) SendReaction(_, _ string) error                 { return nil }
func (a *captureAdapter) Run(_ context.Context) error                    { return nil }

// --- Tests: command routing ---

func TestHandleMessage_Cancel(t *testing.T) {
	mock := &mockSession{adapterN: "codex", sessionN: "sess-1"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/cancel"})

	if mock.cancelCalls != 1 {
		t.Errorf("Cancel called %d times, want 1", mock.cancelCalls)
	}
	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "Cancelled") {
		t.Errorf("reply = %v, want Cancelled", *msgs)
	}
}

func TestHandleMessage_Cancel_NoSession(t *testing.T) {
	store := &mockStore{state: defaultState()}
	c := New(store, nil)
	c.state = defaultState()
	// c.session is nil
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/cancel"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "No active session") {
		t.Errorf("reply = %v, want no active session message", *msgs)
	}
}

func TestHandleMessage_Status(t *testing.T) {
	mock := &mockSession{adapterN: "codex", sessionN: "sess-abc"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/status"})

	if len(*msgs) == 0 {
		t.Fatal("no reply received")
	}
	reply := (*msgs)[0]
	if !strings.Contains(reply, "codex") {
		t.Errorf("status reply %q does not contain adapter name", reply)
	}
	if !strings.Contains(reply, "sess-abc") {
		t.Errorf("status reply %q does not contain session ID", reply)
	}
}

func TestHandleMessage_Status_NoSession(t *testing.T) {
	store := &mockStore{state: defaultState()}
	c := New(store, nil)
	c.state = defaultState()
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/status"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "No active session") {
		t.Errorf("reply = %v, want no active session message", *msgs)
	}
}

func TestHandleMessage_UnknownCommand(t *testing.T) {
	mock := &mockSession{adapterN: "codex"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/foobar"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "Unknown command") {
		t.Errorf("reply = %v, want Unknown command", *msgs)
	}
}

func TestHandleMessage_Use_UnknownAdapter(t *testing.T) {
	mock := &mockSession{adapterN: "codex"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use nonexistent"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "Switch error") {
		t.Errorf("reply = %v, want Switch error message", *msgs)
	}
}

func TestHandleMessage_Use_MissingName(t *testing.T) {
	mock := &mockSession{adapterN: "codex"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "Usage:") {
		t.Errorf("reply = %v, want Usage: message", *msgs)
	}
}

func TestHandleMessage_EmptyMessage(t *testing.T) {
	mock := &mockSession{adapterN: "codex"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "   "})

	if len(*msgs) != 0 {
		t.Errorf("expected no reply for empty message, got %v", *msgs)
	}
}

// --- Tests: streaming prompt ---

func TestHandleMessage_Prompt_TextStreaming(t *testing.T) {
	mock := &mockSession{
		adapterN: "codex",
		promptResult: func(text string) (<-chan agent.Update, error) {
			ch := make(chan agent.Update, 4)
			ch <- agent.Update{Type: agent.UpdateText, Content: "hello "}
			ch <- agent.Update{Type: agent.UpdateText, Content: "world"}
			ch <- agent.Update{Type: agent.UpdateDone, Content: "end_turn", Done: true}
			close(ch)
			return ch, nil
		},
	}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "hi there"})

	if len(mock.promptCalls) != 1 || mock.promptCalls[0] != "hi there" {
		t.Errorf("Prompt called with %v, want [hi there]", mock.promptCalls)
	}
	if len(*msgs) == 0 {
		t.Fatal("no reply received")
	}
	if (*msgs)[0] != "hello world" {
		t.Errorf("reply = %q, want 'hello world'", (*msgs)[0])
	}
}

func TestHandleMessage_Prompt_CurrentPromptChCleared(t *testing.T) {
	mock := &mockSession{adapterN: "codex"}
	c := newTestClient(mock)
	captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "test prompt"})

	c.mu.Lock()
	ch := c.currentPromptCh
	c.mu.Unlock()
	if ch != nil {
		t.Error("currentPromptCh should be nil after prompt completes")
	}
}

func TestHandleMessage_Prompt_NoSession(t *testing.T) {
	store := &mockStore{state: defaultState()}
	c := New(store, nil)
	c.state = defaultState()
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "hello"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "No active session") {
		t.Errorf("reply = %v, want no active session", *msgs)
	}
}

// --- Tests: state persistence (JSONStore + migration) ---

func TestJSONStore_DefaultState(t *testing.T) {
	dir := t.TempDir()
	store := NewJSONStore(filepath.Join(dir, "state.json"))

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.ActiveAdapter != "codex" {
		t.Errorf("ActiveAdapter = %q, want codex", state.ActiveAdapter)
	}
	if state.Adapters == nil {
		t.Error("Adapters should not be nil")
	}
	if state.SessionIDs == nil {
		t.Error("SessionIDs should not be nil")
	}
}

func TestJSONStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewJSONStore(filepath.Join(dir, "state.json"))

	original := &State{
		ActiveAdapter: "codex",
		Adapters: map[string]AdapterConfig{
			"codex": {ExePath: "/usr/bin/codex-acp"},
		},
		SessionIDs: map[string]string{
			"codex": "session-xyz-123",
		},
	}

	if err := store.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ActiveAdapter != "codex" {
		t.Errorf("ActiveAdapter = %q, want codex", loaded.ActiveAdapter)
	}
	if loaded.SessionIDs["codex"] != "session-xyz-123" {
		t.Errorf("SessionIDs[codex] = %q, want session-xyz-123", loaded.SessionIDs["codex"])
	}
}

func TestJSONStore_MigratesLegacyActiveAgent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write a file with the old hub.State format.
	legacy := map[string]any{
		"active_agent": "codex",
		"agents": map[string]any{
			"codex": map[string]any{"exe_path": "/bin/codex"},
		},
		"acp_session_ids": map[string]string{
			"codex": "legacy-session-id",
		},
	}
	data, _ := json.MarshalIndent(legacy, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	store := NewJSONStore(path)
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state.ActiveAdapter != "codex" {
		t.Errorf("ActiveAdapter = %q, want codex (migrated from active_agent)", state.ActiveAdapter)
	}
	if state.SessionIDs["codex"] != "legacy-session-id" {
		t.Errorf("SessionIDs[codex] = %q, want legacy-session-id (migrated from acp_session_ids)", state.SessionIDs["codex"])
	}
}

func TestJSONStore_SaveWritesNewKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := NewJSONStore(path)

	state := &State{
		ActiveAdapter: "myagent",
		SessionIDs:    map[string]string{"myagent": "sess-123"},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(path)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal saved file: %v", err)
	}

	// New keys must be present.
	if _, ok := raw["activeAdapter"]; !ok {
		t.Error("saved file missing 'activeAdapter' key")
	}
	if _, ok := raw["session_ids"]; !ok {
		t.Error("saved file missing 'session_ids' key")
	}
	// Old keys must NOT be present.
	if _, ok := raw["active_agent"]; ok {
		t.Error("saved file should not contain legacy 'active_agent' key")
	}
	if _, ok := raw["acp_session_ids"]; ok {
		t.Error("saved file should not contain legacy 'acp_session_ids' key")
	}
}

func TestJSONStore_NewKeysTakePrecedenceOverLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// File has both old and new keys (e.g., partial migration); new keys should win.
	mixed := map[string]any{
		"activeAdapter":   "new-codex",
		"active_agent":    "old-codex",
		"session_ids":     map[string]string{"new-codex": "new-sess"},
		"acp_session_ids": map[string]string{"old-codex": "old-sess"},
	}
	data, _ := json.MarshalIndent(mixed, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := NewJSONStore(path)
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state.ActiveAdapter != "new-codex" {
		t.Errorf("ActiveAdapter = %q, want new-codex", state.ActiveAdapter)
	}
	if state.SessionIDs["new-codex"] != "new-sess" {
		t.Errorf("SessionIDs[new-codex] = %q, want new-sess", state.SessionIDs["new-codex"])
	}
}
