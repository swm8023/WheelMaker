package client_test

// client_test.go: black-box unit tests for client.Client.
// Uses only exported API plus the helpers in export_test.go.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/acp"
	agent "github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/client"
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
	modeCalls    []string
	agentN       string
	sessionN     string
	promptResult func(text string) (<-chan acp.Update, error)
}

func (m *mockSession) Prompt(ctx context.Context, text string) (<-chan acp.Update, error) {
	m.mu.Lock()
	m.promptCalls = append(m.promptCalls, text)
	fn := m.promptResult
	m.mu.Unlock()
	if fn != nil {
		return fn(text)
	}
	// Default: return single done update.
	ch := make(chan acp.Update, 1)
	ch <- acp.Update{Type: acp.UpdateDone, Content: "end_turn", Done: true}
	close(ch)
	return ch, nil
}

func (m *mockSession) Cancel() error {
	m.mu.Lock()
	m.cancelCalls++
	m.mu.Unlock()
	return nil
}

func (m *mockSession) SetMode(_ context.Context, modeID string) error {
	m.mu.Lock()
	m.modeCalls = append(m.modeCalls, modeID)
	m.mu.Unlock()
	return nil
}

func (m *mockSession) AgentName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.agentN
}

func (m *mockSession) SessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionN
}

func (m *mockSession) Close() error { return nil }

var _ acp.Session = (*mockSession)(nil)

// --- mock Store ---

type mockStore struct {
	mu    sync.Mutex
	state *client.ProjectState
	saved []*client.ProjectState
}

func (s *mockStore) Load() (*client.ProjectState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return client.DefaultState(), nil
	}
	return s.state, nil
}

func (s *mockStore) Save(st *client.ProjectState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, st)
	return nil
}

var _ client.Store = (*mockStore)(nil)

// --- helpers ---

// newTestClient creates a Client for testing with an injected mock session.
func newTestClient(mock *mockSession) *client.Client {
	store := &mockStore{}
	c := client.New(store, nil, "test", "/tmp")
	c.InjectSession(mock)
	return c
}

// captureReplies redirects Client replies to a string slice for inspection.
func captureReplies(c *client.Client) *[]string {
	messages := &[]string{}
	c.InjectIMProvider(&captureProvider{messages: messages})
	return messages
}

type captureProvider struct {
	messages *[]string
}

func (a *captureProvider) OnMessage(_ im.MessageHandler) {}
func (a *captureProvider) SendText(_ string, text string) error {
	*a.messages = append(*a.messages, text)
	return nil
}
func (a *captureProvider) SendCard(_ string, _ im.Card) error { return nil }
func (a *captureProvider) SendReaction(_, _ string) error     { return nil }
func (a *captureProvider) Run(_ context.Context) error        { return nil }

var _ im.Provider = (*captureProvider)(nil)

// testLogWriter is a goroutine-safe log output writer for capturing log output in tests.
type testLogWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *testLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *testLogWriter) Contains(s string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Contains(w.buf.String(), s)
}

func (w *testLogWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// --- Tests: command routing ---

func TestHandleMessage_Cancel(t *testing.T) {
	mock := &mockSession{agentN: "codex", sessionN: "sess-1"}
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
	store := &mockStore{}
	c := client.New(store, nil, "test", "/tmp")
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/cancel"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "No active session") {
		t.Errorf("reply = %v, want no active session message", *msgs)
	}
}

func TestHandleMessage_Status(t *testing.T) {
	mock := &mockSession{agentN: "codex", sessionN: "sess-abc"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/status"})

	if len(*msgs) == 0 {
		t.Fatal("no reply received")
	}
	reply := (*msgs)[0]
	if !strings.Contains(reply, "codex") {
		t.Errorf("status reply %q does not contain agent name", reply)
	}
	if !strings.Contains(reply, "sess-abc") {
		t.Errorf("status reply %q does not contain session ID", reply)
	}
}

func TestHandleMessage_Status_NoSession(t *testing.T) {
	store := &mockStore{}
	c := client.New(store, nil, "test", "/tmp")
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/status"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "No active session") {
		t.Errorf("reply = %v, want no active session message", *msgs)
	}
}

func TestHandleMessage_Use_UnknownAgent(t *testing.T) {
	mock := &mockSession{agentN: "codex"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use nonexistent"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "Switch error") {
		t.Errorf("reply = %v, want Switch error message", *msgs)
	}
}

func TestHandleMessage_Use_MissingName(t *testing.T) {
	mock := &mockSession{agentN: "codex"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "Usage:") {
		t.Errorf("reply = %v, want Usage: message", *msgs)
	}
}

func TestHandleMessage_Mode_SetsMode(t *testing.T) {
	store := &mockStore{state: &client.ProjectState{
		ActiveAgent: "codex",
	}}
	c := client.New(store, nil, "test", "/tmp")
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/mode code"})

	found := false
	for _, m := range *msgs {
		if strings.Contains(m, "Mode set to: code") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("reply = %v, want mode confirmation", *msgs)
	}
}

func TestHandleMessage_Mode_MissingName(t *testing.T) {
	mock := &mockSession{agentN: "codex", sessionN: "sess-1"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/mode"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "Usage: /mode") {
		t.Fatalf("reply = %v, want /mode usage", *msgs)
	}
}

func TestHandleMessage_Model_SetsConfigOption(t *testing.T) {
	store := &mockStore{state: &client.ProjectState{
		ActiveAgent: "codex",
	}}
	c := client.New(store, nil, "test", "/tmp")
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	msgs := captureReplies(c)
	// Prime the session so model updates can be applied on an active ACP session.
	c.HandleMessage(im.Message{ChatID: "c1", Text: "hello"})
	c.HandleMessage(im.Message{ChatID: "c1", Text: "/model gpt-4.1-mini"})

	found := false
	for _, m := range *msgs {
		if strings.Contains(m, "Model set to: gpt-4.1-mini") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("messages = %v, want model confirmation", *msgs)
	}
}

func TestHandleMessage_EmptyMessage(t *testing.T) {
	mock := &mockSession{agentN: "codex"}
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
		agentN: "codex",
		promptResult: func(_ string) (<-chan acp.Update, error) {
			ch := make(chan acp.Update, 4)
			ch <- acp.Update{Type: acp.UpdateText, Content: "hello "}
			ch <- acp.Update{Type: acp.UpdateText, Content: "world"}
			ch <- acp.Update{Type: acp.UpdateDone, Content: "end_turn", Done: true}
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

func TestHandleMessage_Prompt_ConfigOptionUpdate_NotifiesIM(t *testing.T) {
	store := &mockStore{state: &client.ProjectState{
		ActiveAgent: "codex",
	}}
	c := client.New(store, nil, "test", "/tmp")
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "emit-config-update"})

	found := false
	for _, m := range *msgs {
		if strings.Contains(m, "Config options updated:") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("messages = %v, want config update notification", *msgs)
	}
}

// TestHandleMessage_Prompt_AllowsSubsequentSwitch verifies that after a prompt
// completes, a subsequent /use can proceed immediately (promptMu is released).
func TestHandleMessage_Prompt_AllowsSubsequentSwitch(t *testing.T) {
	mock := &mockSession{agentN: "codex"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	// Complete a prompt synchronously.
	c.HandleMessage(im.Message{ChatID: "chat1", Text: "hello"})

	// Register a new agent and switch to it after the prompt completes.
	c.RegisterAgent("other", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})
	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use other"})

	found := false
	for _, m := range *msgs {
		if strings.Contains(m, "Switched to agent") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("messages = %v, missing switch confirmation", *msgs)
	}
}

func TestHandleMessage_Prompt_NoSession(t *testing.T) {
	store := &mockStore{}
	c := client.New(store, nil, "test", "/tmp")
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "hello"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "No active session") {
		t.Errorf("reply = %v, want no active session", *msgs)
	}
}

// TestHandlePrompt_ConcurrentSwitch is a regression test for the prompt/switch race:
// a slow prompt must complete correctly even when /use is issued concurrently.
// promptMu ensures switchAgent waits for handlePrompt before calling ag.Switch.
func TestHandlePrompt_ConcurrentSwitch(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})

	slow := &mockSession{
		agentN:   "slow",
		sessionN: "sess-slow",
		promptResult: func(_ string) (<-chan acp.Update, error) {
			close(started) // signal that Prompt() was entered
			<-done         // block until test unblocks us
			ch := make(chan acp.Update, 1)
			ch <- acp.Update{Type: acp.UpdateDone, Done: true}
			close(ch)
			return ch, nil
		},
	}
	c := newTestClient(slow)
	c.RegisterAgent("fast", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})
	msgs := captureReplies(c)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.HandleMessage(im.Message{ChatID: "c1", Text: "slow prompt"})
	}()

	// Wait for the prompt goroutine to be inside Prompt().
	<-started

	// Issue /use while prompt is active; promptMu forces it to wait.
	switchDone := make(chan struct{})
	go func() {
		c.HandleMessage(im.Message{ChatID: "c1", Text: "/use fast"})
		close(switchDone)
	}()

	// Brief pause, then unblock the slow prompt.
	time.Sleep(20 * time.Millisecond)
	close(done)

	wg.Wait()
	<-switchDone

	found := false
	for _, m := range *msgs {
		if strings.Contains(m, "Switched to agent") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("messages = %v, missing switch confirmation after concurrent prompt", *msgs)
	}
}

// --- Tests: state persistence (JSONStore + migration) ---

func TestJSONStore_DefaultState(t *testing.T) {
	dir := t.TempDir()
	store := client.NewJSONStore(filepath.Join(dir, "state.json"))

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.ActiveAgent != "claude" {
		t.Errorf("ActiveAgent = %q, want claude", state.ActiveAgent)
	}
	if state.Agents == nil {
		t.Error("Agents should not be nil")
	}
}

func TestJSONStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	store := client.NewJSONStore(filepath.Join(dir, "state.json"))

	original := &client.ProjectState{
		ActiveAgent: "codex",
		Agents: map[string]*client.AgentState{
			"codex": {LastSessionID: "session-xyz-123"},
		},
	}

	if err := store.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ActiveAgent != "codex" {
		t.Errorf("ActiveAgent = %q, want codex", loaded.ActiveAgent)
	}
	if loaded.Agents["codex"] == nil || loaded.Agents["codex"].LastSessionID != "session-xyz-123" {
		t.Errorf("Agents[codex].LastSessionID = %q, want session-xyz-123", loaded.Agents["codex"].LastSessionID)
	}
}

func TestJSONStore_IgnoresFlatStateWithoutProjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	flat := map[string]any{
		"activeAgent": "new-codex",
		"agents": map[string]any{
			"new-codex": map[string]any{"lastSessionId": "new-sess"},
		},
	}
	data, _ := json.MarshalIndent(flat, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write flat file: %v", err)
	}

	store := client.NewJSONStore(path)
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state.ActiveAgent != "claude" {
		t.Errorf("ActiveAgent = %q, want claude", state.ActiveAgent)
	}
	if len(state.Agents) != 0 {
		t.Errorf("Agents = %v, want empty default map", state.Agents)
	}
}

func TestJSONStore_SaveWritesNewKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := client.NewJSONStore(path)

	state := &client.ProjectState{
		ActiveAgent: "myagent",
		Agents:      map[string]*client.AgentState{"myagent": {LastSessionID: "sess-123"}},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(path)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal saved file: %v", err)
	}

	if _, ok := raw["projects"]; !ok {
		t.Error("saved file missing top-level 'projects' key")
	}
	if _, ok := raw["activeAgent"]; ok {
		t.Error("saved file should not have flat 'activeAgent' key (must be nested under 'projects')")
	}
}

// --- Tests: switch session persistence ---

// minimalMockAgent is an agent.Agent that connects to the mock ACP server
// embedded in the test binary (activated via GO_CLIENT_ACP_MOCK=1).
type minimalMockAgent struct{}

func (a *minimalMockAgent) Name() string { return "mock" }
func (a *minimalMockAgent) Connect(_ context.Context) (*agent.Conn, error) {
	conn := agent.New(os.Args[0], []string{"GO_CLIENT_ACP_MOCK=1"})
	if err := conn.Start(); err != nil {
		return nil, err
	}
	return conn, nil
}
func (a *minimalMockAgent) Close() error                    { return nil }
func (a *minimalMockAgent) Plugin() acp.AgentPlugin         { return acp.DefaultPlugin{} }

var _ agent.Agent = (*minimalMockAgent)(nil)

// contextRejectMockAgent spawns the mock ACP server with GO_CLIENT_ACP_MOCK_REJECT_CONTEXT=1,
// causing it to reject [context] bootstrap prompts with a JSON-RPC error.
// This makes SwitchWithContext observable: the agent's drain goroutine logs a warning.
type contextRejectMockAgent struct{}

func (a *contextRejectMockAgent) Name() string { return "mock-reject" }
func (a *contextRejectMockAgent) Connect(_ context.Context) (*agent.Conn, error) {
	conn := agent.New(os.Args[0], []string{"GO_CLIENT_ACP_MOCK=1", "GO_CLIENT_ACP_MOCK_REJECT_CONTEXT=1"})
	if err := conn.Start(); err != nil {
		return nil, err
	}
	return conn, nil
}
func (a *contextRejectMockAgent) Close() error                    { return nil }
func (a *contextRejectMockAgent) Plugin() acp.AgentPlugin         { return acp.DefaultPlugin{} }

var _ agent.Agent = (*contextRejectMockAgent)(nil)

// failConnectAgent is an agent.Agent whose Connect always returns an error.
// Used to test that Start() is non-fatal when the Active agent cannot connect.
type failConnectAgent struct{}

func (a *failConnectAgent) Name() string { return "fail" }
func (a *failConnectAgent) Connect(_ context.Context) (*agent.Conn, error) {
	return nil, fmt.Errorf("mock: binary not found")
}
func (a *failConnectAgent) Close() error                    { return nil }
func (a *failConnectAgent) Plugin() acp.AgentPlugin         { return acp.DefaultPlugin{} }

var _ agent.Agent = (*failConnectAgent)(nil)

// TestSwitchAgent_PersistsOutgoingSessionID verifies that the outgoing
// agent's session ID is saved to state before the switch completes.
func TestSwitchAgent_PersistsOutgoingSessionID(t *testing.T) {
	outgoing := &mockSession{agentN: "codex", sessionN: "outgoing-sess-123"}
	store := &mockStore{}
	c := client.New(store, nil, "test", "/tmp")
	c.InjectSession(outgoing)
	c.InjectState(&client.ProjectState{
		ActiveAgent: "codex",
		Agents: map[string]*client.AgentState{
			"codex": {LastSessionID: "outgoing-sess-123"},
		},
	})
	c.RegisterAgent("new-agent", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})

	msgs := captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use new-agent"})

	if len(store.saved) == 0 {
		t.Fatal("state was not saved after switch")
	}
	last := store.saved[len(store.saved)-1]

	if last.Agents["codex"] == nil || last.Agents["codex"].LastSessionID != "outgoing-sess-123" {
		t.Errorf("Agents[codex].LastSessionID = %q, want outgoing-sess-123", last.Agents["codex"].LastSessionID)
	}
	if last.ActiveAgent != "new-agent" {
		t.Errorf("ActiveAgent = %q, want new-agent", last.ActiveAgent)
	}
	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "new-agent") {
		t.Errorf("reply = %v, want switch confirmation", *msgs)
	}
}

// TestSwitchAgent_PreservesIncomingSessionIDOnCleanSwitch verifies that a plain
// /use does NOT clear state.SessionIDs for the incoming acp. The saved ID is
// threaded through to ag.Switch() so ensureReady can attempt session/load on
// backends that support it.
func TestSwitchAgent_PreservesIncomingSessionIDOnCleanSwitch(t *testing.T) {
	outgoing := &mockSession{agentN: "codex", sessionN: "codex-sess-1"}
	store := &mockStore{}
	c := client.New(store, nil, "test", "/tmp")
	c.InjectSession(outgoing)
	c.InjectState(&client.ProjectState{
		ActiveAgent: "codex",
		Agents: map[string]*client.AgentState{
			"codex":     {LastSessionID: "codex-sess-1"},
			"new-agent": {LastSessionID: "old-stale-sess"}, // pre-existing saved session for target
		},
	})
	c.RegisterAgent("new-agent", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})

	captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use new-agent"})

	if len(store.saved) == 0 {
		t.Fatal("state was not saved after switch")
	}
	last := store.saved[len(store.saved)-1]

	// The incoming agent's saved session ID must be preserved (not overwritten by "")
	// so that a subsequent switch back can pass it to ag.Switch for session/load.
	if last.Agents["new-agent"] == nil || last.Agents["new-agent"].LastSessionID != "old-stale-sess" {
		t.Errorf("Agents[new-agent].LastSessionID = %q, want old-stale-sess (preserved for session/load)",
			last.Agents["new-agent"].LastSessionID)
	}
}

// TestSwitchAgent_PersistsTargetSessionIDOnContinue verifies that after
// /use <agent> --continue, the new session ID for the target agent is saved
// to state immediately (not deferred to Close()), so a crash before shutdown
// does not lose the bootstrapped session.
func TestSwitchAgent_PersistsTargetSessionIDOnContinue(t *testing.T) {
	store := &mockStore{state: &client.ProjectState{
		ActiveAgent: "codex",
	}}
	c := client.New(store, nil, "test", "/tmp")
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})
	c.RegisterAgent("other", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	// Prime lastReply so SwitchWithContext has something to bootstrap with.
	msgs := captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "c1", Text: "hello"})
	_ = msgs

	// Reset saves so we only inspect the switch-triggered save.
	store.mu.Lock()
	store.saved = nil
	store.mu.Unlock()

	c.HandleMessage(im.Message{ChatID: "c1", Text: "/use other --continue"})

	store.mu.Lock()
	saved := store.saved
	store.mu.Unlock()

	if len(saved) == 0 {
		t.Fatal("state was not saved after /use --continue")
	}
	last := saved[len(saved)-1]
	if last.Agents["other"] == nil || last.Agents["other"].LastSessionID == "" {
		t.Errorf("Agents[other].LastSessionID is empty after /use --continue; want bootstrapped session ID")
	}
}

// TestStart_UnregisteredAgent_NonFatal verifies that Start() succeeds when the
// persisted activeAgent was not registered in this build, leaving session nil.
// The user can issue /use <agent> to connect a registered agent and recover.
func TestStart_UnregisteredAgent_NonFatal(t *testing.T) {
	store := &mockStore{state: &client.ProjectState{
		ActiveAgent: "unknown-agent",
	}}
	c := client.New(store, nil, "test", "/tmp")
	// Register "codex" but NOT "unknown-agent" (simulating a removed agent).
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() returned error %v, want nil (unknown agent should be non-fatal)", err)
	}

	// With no active session, messages should get "No active session".
	msgs := captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "c1", Text: "hello"})
	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "No active session") {
		t.Errorf("reply = %v, want 'No active session'", *msgs)
	}

	// /use with a registered agent should recover.
	c.HandleMessage(im.Message{ChatID: "c1", Text: "/use codex"})
	found := false
	for _, m := range *msgs {
		if strings.Contains(m, "Switched to agent") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("messages = %v, expected switch confirmation after /use codex", *msgs)
	}
}

// TestStart_ConnectError_NonFatal verifies that Start() succeeds when the active
// agent fails to connect, leaving session nil. Subsequent messages get
// "No active session" rather than causing a hard startup failure.
// Also verifies that /use <agent> can recover by connecting a working acp.
func TestStart_ConnectError_NonFatal(t *testing.T) {
	store := &mockStore{state: &client.ProjectState{
		ActiveAgent: "codex",
	}}
	c := client.New(store, nil, "test", "/tmp")
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return &failConnectAgent{}
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() returned error %v, want nil (connect failure should be non-fatal)", err)
	}

	// With no active session, messages should get "No active session".
	msgs := captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "c1", Text: "hello"})
	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "No active session") {
		t.Errorf("reply = %v, want 'No active session'", *msgs)
	}

	// /use with a working agent should recover and allow prompts.
	c.RegisterAgent("other", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})
	c.HandleMessage(im.Message{ChatID: "c1", Text: "/use other"})
	found := false
	for _, m := range *msgs {
		if strings.Contains(m, "Switched to agent") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("messages = %v, expected switch confirmation after /use other", *msgs)
	}

	// After recovery, a prompt should not return "No active session".
	prevLen := len(*msgs)
	c.HandleMessage(im.Message{ChatID: "c1", Text: "prompt after recovery"})
	if len(*msgs) > prevLen {
		for _, m := range (*msgs)[prevLen:] {
			if strings.Contains(m, "No active session") {
				t.Errorf("prompt after recovery got 'No active session'; /use did not restore session")
			}
		}
	}
}

// --- Minimal ACP mock server for client tests ---

// TestHandleMessage_Use_Continue_BootstrapsContext verifies that /use <name> --continue
// calls ag.Switch with SwitchWithContext, which sends a [context] bootstrap prompt to the
// new acp. Uses a real Client.Start() so c.ag is non-nil and switchAgent executes
// ag.Switch(..., mode).
func TestHandleMessage_Use_Continue_BootstrapsContext(t *testing.T) {
	store := &mockStore{state: &client.ProjectState{
		ActiveAgent: "codex",
	}}
	c := client.New(store, nil, "test", "/tmp")
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	// Redirect the default logger so we can observe the bootstrap warning.
	lw := &testLogWriter{}
	origOutput := log.Writer()
	log.SetOutput(lw)
	defer log.SetOutput(origOutput)

	// Prime lastReply: the mock sends "client-mock-reply" text notification for session/prompt.
	msgs := captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "c1", Text: "hello"})
	_ = msgs

	// Register "other" with a backend that rejects [context] bootstrap prompts.
	c.RegisterAgent("other", func(_ string, _ map[string]string) agent.Agent {
		return &contextRejectMockAgent{}
	})

	// /use other --continue: ag.Switch sends "[context] client-mock-reply" to the new acp.
	c.HandleMessage(im.Message{ChatID: "c1", Text: "/use other --continue"})

	// Poll until the async drain goroutine logs the rejection warning (or timeout).
	const want = "SwitchWithContext bootstrap prompt failed"
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if lw.Contains(want) {
			return // success
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("expected log to contain %q after /use --continue; log was: %s", want, lw.String())
}

// TestHandleMessage_Use_Clean_NoBootstrap verifies that plain /use (SwitchClean) does NOT
// send a [context] bootstrap prompt to the new agent, even when lastReply is non-empty.
func TestHandleMessage_Use_Clean_NoBootstrap(t *testing.T) {
	store := &mockStore{state: &client.ProjectState{
		ActiveAgent: "codex",
	}}
	c := client.New(store, nil, "test", "/tmp")
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	// Redirect the default logger; no bootstrap warning should appear.
	lw := &testLogWriter{}
	origOutput := log.Writer()
	log.SetOutput(lw)
	defer log.SetOutput(origOutput)

	// Prime lastReply so that if SwitchWithContext were used, it would have content.
	msgs := captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "c1", Text: "hello"})
	_ = msgs

	// Register "other" using a context-rejecting backend.
	c.RegisterAgent("other", func(_ string, _ map[string]string) agent.Agent {
		return &contextRejectMockAgent{}
	})

	// Plain /use (SwitchClean): should NOT send a bootstrap prompt.
	c.HandleMessage(im.Message{ChatID: "c1", Text: "/use other"})

	// Wait long enough for any async operations before asserting.
	time.Sleep(200 * time.Millisecond)

	const forbidden = "SwitchWithContext bootstrap prompt failed"
	if lw.Contains(forbidden) {
		t.Errorf("plain /use triggered unexpected SwitchWithContext bootstrap; log: %s", lw.String())
	}
}

// TestClient_Close_PersistsSessionID verifies that Close() saves the current
// agent session ID to the store, satisfying AC-5.
// Uses Start() to create a real agent.Agent so c.ag is non-nil.
func TestClient_Close_PersistsSessionID(t *testing.T) {
	store := &mockStore{state: &client.ProjectState{
		ActiveAgent: "codex",
	}}
	c := client.New(store, nil, "test", "/tmp")
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return &minimalMockAgent{}
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Trigger ensureReady to establish a session ID via session/new.
	msgs := captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "c1", Text: "hello"})
	_ = msgs

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(store.saved) == 0 {
		t.Fatal("state not saved after Close")
	}
	last := store.saved[len(store.saved)-1]
	if last.Agents["codex"] == nil || last.Agents["codex"].LastSessionID == "" {
		t.Errorf("Close did not persist codex session ID; Agents = %v", last.Agents)
	}
}

// runClientMockAgent is a minimal ACP server for client-level tests.
// Activated when GO_CLIENT_ACP_MOCK=1 is set (used by minimalMockAgent).
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
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params,omitempty"`
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
		case "session/prompt":
			rejectCtx := os.Getenv("GO_CLIENT_ACP_MOCK_REJECT_CONTEXT") == "1"
			var params struct {
				SessionID string          `json:"sessionId"`
				Prompt    json.RawMessage `json:"prompt"`
			}
			_ = json.Unmarshal(raw.Params, &params)
			// Prompt is now []ContentBlock; extract text from first block.
			var blocks []struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(params.Prompt, &blocks)
			promptText := ""
			if len(blocks) > 0 {
				promptText = blocks[0].Text
			}
			if rejectCtx && strings.HasPrefix(promptText, "[context]") {
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"error":   map[string]any{"code": -32603, "message": "mock: context bootstrap rejected"},
				})
			} else {
				if promptText == "emit-config-update" {
					_ = enc.Encode(map[string]any{
						"jsonrpc": "2.0",
						"method":  "session/update",
						"params": map[string]any{
							"sessionId": params.SessionID,
							"update": map[string]any{
								"sessionUpdate": "config_option_update",
								"configOptions": []map[string]any{
									{"id": "mode", "category": "mode", "currentValue": "code"},
									{"id": "model", "category": "model", "currentValue": "gpt-4.1-mini"},
								},
							},
						},
					})
				}
				// Send a text notification so lastReply is populated for SwitchWithContext.
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": params.SessionID,
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content":       map[string]any{"type": "text", "text": "client-mock-reply"},
						},
					},
				})
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result":  map[string]any{"stopReason": "end_turn"},
				})
			}
		default:
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  map[string]any{},
			})
		}
	}
}
