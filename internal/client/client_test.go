package client

// client_test.go: unit tests for client.Client using a mock agent.Session.
// Uses package-internal access to inject mock dependencies.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/swm8023/wheelmaker/internal/adapter"
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/agent/acp"
	"github.com/swm8023/wheelmaker/internal/im"
)

// TestMain intercepts the test binary to act as a minimal ACP mock server when
// GO_CLIENT_ACP_MOCK=1 is set. This allows client tests to use real acp.Conn
// connections without depending on an external binary.
func TestMain(m *testing.M) {
	if os.Getenv("GO_CLIENT_ACP_MOCK") == "1" {
		runClientMockAgent()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

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

func TestJSONStore_MigratesLegacyAgentsExePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write old hub.State format with "agents[name].exe_path".
	legacy := map[string]any{
		"active_agent": "codex",
		"agents": map[string]any{
			"codex": map[string]any{"exe_path": "/usr/local/bin/codex-acp"},
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

	if state.Adapters["codex"].ExePath != "/usr/local/bin/codex-acp" {
		t.Errorf("Adapters[codex].ExePath = %q, want /usr/local/bin/codex-acp",
			state.Adapters["codex"].ExePath)
	}
}

// --- Tests: switch session persistence ---

// minimalMockAdapter is an adapter.Adapter that connects to the mock ACP server
// embedded in the test binary itself (activated via GO_CLIENT_ACP_MOCK=1).
type minimalMockAdapter struct{}

func (a *minimalMockAdapter) Name() string { return "mock" }
func (a *minimalMockAdapter) Connect(_ context.Context) (*acp.Conn, error) {
	conn := acp.New(os.Args[0], []string{"GO_CLIENT_ACP_MOCK=1"})
	if err := conn.Start(); err != nil {
		return nil, err
	}
	return conn, nil
}
func (a *minimalMockAdapter) Close() error { return nil }

var _ adapter.Adapter = (*minimalMockAdapter)(nil)

// TestSwitchAdapter_PersistsOutgoingSessionID verifies that the outgoing adapter's
// session ID is saved to state before the switch completes.
func TestSwitchAdapter_PersistsOutgoingSessionID(t *testing.T) {
	outgoingSession := &mockSession{adapterN: "codex", sessionN: "outgoing-sess-123"}
	store := &mockStore{state: &State{
		ActiveAdapter: "codex",
		SessionIDs:    map[string]string{"codex": "outgoing-sess-123"},
		Adapters:      map[string]AdapterConfig{},
	}}
	c := New(store, nil)
	c.state = &State{
		ActiveAdapter: "codex",
		SessionIDs:    map[string]string{"codex": "outgoing-sess-123"},
		Adapters:      map[string]AdapterConfig{},
	}
	c.session = outgoingSession

	// Register the "new-adapter" factory using the embedded mock ACP server.
	c.RegisterAdapter("new-adapter", func(exePath string, env map[string]string) adapter.Adapter {
		return &minimalMockAdapter{}
	})

	msgs := captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use new-adapter"})

	// Verify state was saved.
	if len(store.saved) == 0 {
		t.Fatal("state was not saved after switch")
	}
	lastState := store.saved[len(store.saved)-1]

	// The outgoing "codex" session ID must be persisted.
	if got := lastState.SessionIDs["codex"]; got != "outgoing-sess-123" {
		t.Errorf("SessionIDs[codex] = %q, want outgoing-sess-123", got)
	}
	// The active adapter must be updated.
	if lastState.ActiveAdapter != "new-adapter" {
		t.Errorf("ActiveAdapter = %q, want new-adapter", lastState.ActiveAdapter)
	}

	// Switch success reply expected.
	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "new-adapter") {
		t.Errorf("reply = %v, want switch confirmation", *msgs)
	}
}

// TestRegisterAdapter_FactoryCalledWithStateConfig verifies that RegisterAdapter
// factories receive ExePath and Env from persisted State.Adapters at connect time.
func TestRegisterAdapter_FactoryCalledWithStateConfig(t *testing.T) {
	store := &mockStore{state: &State{
		ActiveAdapter: "codex",
		Adapters: map[string]AdapterConfig{
			"codex": {ExePath: "/custom/codex", Env: map[string]string{"API_KEY": "test-key"}},
		},
		SessionIDs: map[string]string{},
	}}

	var gotExePath string
	var gotEnv map[string]string
	c := New(store, nil)
	c.RegisterAdapter("codex", func(exePath string, env map[string]string) adapter.Adapter {
		gotExePath = exePath
		gotEnv = env
		return &minimalMockAdapter{}
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	if gotExePath != "/custom/codex" {
		t.Errorf("factory exePath = %q, want /custom/codex", gotExePath)
	}
	if gotEnv["API_KEY"] != "test-key" {
		t.Errorf("factory env[API_KEY] = %q, want test-key", gotEnv["API_KEY"])
	}
}

// --- Minimal ACP mock server for client tests ---

// runClientMockAgent is a minimal ACP server that handles initialize and session/new.
// Activated when GO_CLIENT_ACP_MOCK=1 is set (used by minimalMockAdapter).
func runClientMockAgent() {
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(line, &raw); err != nil || raw.ID == nil {
			continue
		}
		id := *raw.ID

		switch raw.Method {
		case "initialize":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion":   "0.1",
					"agentCapabilities": map[string]any{"loadSession": false},
					"agentInfo":         map[string]any{"name": "client-mock-agent"},
				},
			})
		case "session/new":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  map[string]any{"sessionId": fmt.Sprintf("client-mock-sess-%d", id)},
			})
		default:
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  map[string]any{},
			})
		}
	}
}
