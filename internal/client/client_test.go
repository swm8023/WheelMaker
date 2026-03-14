package client_test

// client_test.go: black-box unit tests for client.Client.
// Uses only exported API plus the helpers in export_test.go.

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
	"time"

	"github.com/swm8023/wheelmaker/internal/adapter"
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/agent/acp"
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
	// Default: return single done update.
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

func (m *mockSession) SetMode(_ context.Context, _ string) error { return nil }

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

var _ agent.Session = (*mockSession)(nil)

// --- mock Store ---

type mockStore struct {
	mu    sync.Mutex
	state *client.State
	saved []*client.State
}

func (s *mockStore) Load() (*client.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return client.DefaultState(), nil
	}
	return s.state, nil
}

func (s *mockStore) Save(st *client.State) error {
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
	c := client.New(store, nil)
	c.InjectSession(mock)
	return c
}

// captureReplies redirects Client replies to a string slice for inspection.
func captureReplies(c *client.Client) *[]string {
	messages := &[]string{}
	c.InjectIMAdapter(&captureAdapter{messages: messages})
	return messages
}

type captureAdapter struct {
	messages *[]string
}

func (a *captureAdapter) OnMessage(_ im.MessageHandler)        {}
func (a *captureAdapter) SendText(_ string, text string) error {
	*a.messages = append(*a.messages, text)
	return nil
}
func (a *captureAdapter) SendCard(_ string, _ im.Card) error          { return nil }
func (a *captureAdapter) SendReaction(_, _ string) error              { return nil }
func (a *captureAdapter) Run(_ context.Context) error                 { return nil }

var _ im.Adapter = (*captureAdapter)(nil)

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
	store := &mockStore{}
	c := client.New(store, nil)
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
	store := &mockStore{}
	c := client.New(store, nil)
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
		promptResult: func(_ string) (<-chan agent.Update, error) {
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

// TestHandleMessage_Prompt_AllowsSubsequentSwitch verifies that after a prompt
// completes, a subsequent /use can proceed immediately (promptMu is released).
func TestHandleMessage_Prompt_AllowsSubsequentSwitch(t *testing.T) {
	mock := &mockSession{adapterN: "codex"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	// Complete a prompt synchronously.
	c.HandleMessage(im.Message{ChatID: "chat1", Text: "hello"})

	// Register a new adapter and switch to it after the prompt completes.
	c.RegisterAdapter("other", func(_ string, _ map[string]string) adapter.Adapter {
		return &minimalMockAdapter{}
	})
	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use other"})

	found := false
	for _, m := range *msgs {
		if strings.Contains(m, "Switched to adapter") {
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
	c := client.New(store, nil)
	msgs := captureReplies(c)

	c.HandleMessage(im.Message{ChatID: "chat1", Text: "hello"})

	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "No active session") {
		t.Errorf("reply = %v, want no active session", *msgs)
	}
}

// TestHandlePrompt_ConcurrentSwitch is a regression test for the prompt/switch race:
// a slow prompt must complete correctly even when /use is issued concurrently.
// promptMu ensures switchAdapter waits for handlePrompt before calling ag.Switch.
func TestHandlePrompt_ConcurrentSwitch(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})

	slow := &mockSession{
		adapterN: "slow",
		sessionN: "sess-slow",
		promptResult: func(_ string) (<-chan agent.Update, error) {
			close(started) // signal that Prompt() was entered
			<-done         // block until test unblocks us
			ch := make(chan agent.Update, 1)
			ch <- agent.Update{Type: agent.UpdateDone, Done: true}
			close(ch)
			return ch, nil
		},
	}
	c := newTestClient(slow)
	c.RegisterAdapter("fast", func(_ string, _ map[string]string) adapter.Adapter {
		return &minimalMockAdapter{}
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
		if strings.Contains(m, "Switched to adapter") {
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
	store := client.NewJSONStore(filepath.Join(dir, "state.json"))

	original := &client.State{
		ActiveAdapter: "codex",
		Adapters: map[string]client.AdapterConfig{
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

	store := client.NewJSONStore(path)
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state.ActiveAdapter != "codex" {
		t.Errorf("ActiveAdapter = %q, want codex (migrated from active_agent)", state.ActiveAdapter)
	}
	if state.SessionIDs["codex"] != "legacy-session-id" {
		t.Errorf("SessionIDs[codex] = %q, want legacy-session-id (migrated from acp_session_ids)",
			state.SessionIDs["codex"])
	}
}

func TestJSONStore_SaveWritesNewKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store := client.NewJSONStore(path)

	state := &client.State{
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

	if _, ok := raw["activeAdapter"]; !ok {
		t.Error("saved file missing 'activeAdapter' key")
	}
	if _, ok := raw["session_ids"]; !ok {
		t.Error("saved file missing 'session_ids' key")
	}
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

	store := client.NewJSONStore(path)
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

	store := client.NewJSONStore(path)
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
// embedded in the test binary (activated via GO_CLIENT_ACP_MOCK=1).
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

// TestSwitchAdapter_PersistsOutgoingSessionID verifies that the outgoing
// adapter's session ID is saved to state before the switch completes.
func TestSwitchAdapter_PersistsOutgoingSessionID(t *testing.T) {
	outgoing := &mockSession{adapterN: "codex", sessionN: "outgoing-sess-123"}
	st := &client.State{
		ActiveAdapter: "codex",
		SessionIDs:    map[string]string{"codex": "outgoing-sess-123"},
		Adapters:      map[string]client.AdapterConfig{},
	}
	store := &mockStore{state: st}
	c := client.New(store, nil)
	c.InjectSession(outgoing)
	c.InjectState(&client.State{
		ActiveAdapter: "codex",
		SessionIDs:    map[string]string{"codex": "outgoing-sess-123"},
		Adapters:      map[string]client.AdapterConfig{},
	})
	c.RegisterAdapter("new-adapter", func(_ string, _ map[string]string) adapter.Adapter {
		return &minimalMockAdapter{}
	})

	msgs := captureReplies(c)
	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use new-adapter"})

	if len(store.saved) == 0 {
		t.Fatal("state was not saved after switch")
	}
	last := store.saved[len(store.saved)-1]

	if got := last.SessionIDs["codex"]; got != "outgoing-sess-123" {
		t.Errorf("SessionIDs[codex] = %q, want outgoing-sess-123", got)
	}
	if last.ActiveAdapter != "new-adapter" {
		t.Errorf("ActiveAdapter = %q, want new-adapter", last.ActiveAdapter)
	}
	if len(*msgs) == 0 || !strings.Contains((*msgs)[0], "new-adapter") {
		t.Errorf("reply = %v, want switch confirmation", *msgs)
	}
}

// TestRegisterAdapter_FactoryCalledWithStateConfig verifies that RegisterAdapter
// factories receive ExePath and Env from persisted State.Adapters at connect time.
func TestRegisterAdapter_FactoryCalledWithStateConfig(t *testing.T) {
	store := &mockStore{state: &client.State{
		ActiveAdapter: "codex",
		Adapters: map[string]client.AdapterConfig{
			"codex": {ExePath: "/custom/codex", Env: map[string]string{"API_KEY": "test-key"}},
		},
		SessionIDs: map[string]string{},
	}}

	var gotExePath string
	var gotEnv map[string]string
	c := client.New(store, nil)
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

// TestHandleMessage_Use_Continue verifies that /use <name> --continue is parsed
// correctly and performs a successful adapter switch (SwitchWithContext mode).
func TestHandleMessage_Use_Continue(t *testing.T) {
	mock := &mockSession{adapterN: "codex"}
	c := newTestClient(mock)
	msgs := captureReplies(c)

	c.RegisterAdapter("other", func(_ string, _ map[string]string) adapter.Adapter {
		return &minimalMockAdapter{}
	})
	c.HandleMessage(im.Message{ChatID: "chat1", Text: "/use other --continue"})

	found := false
	for _, m := range *msgs {
		if strings.Contains(m, "Switched to adapter") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("messages = %v, missing switch confirmation for /use --continue", *msgs)
	}
}

// TestClient_Close_PersistsSessionID verifies that Close() saves the current
// agent session ID to the store, satisfying AC-5.
// Uses Start() to create a real agent.Agent so c.ag is non-nil.
func TestClient_Close_PersistsSessionID(t *testing.T) {
	store := &mockStore{state: &client.State{
		ActiveAdapter: "codex",
		Adapters:      map[string]client.AdapterConfig{"codex": {}},
		SessionIDs:    map[string]string{},
	}}
	c := client.New(store, nil)
	c.RegisterAdapter("codex", func(_ string, _ map[string]string) adapter.Adapter {
		return &minimalMockAdapter{}
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
	if last.SessionIDs["codex"] == "" {
		t.Errorf("Close did not persist codex session ID; SessionIDs = %v", last.SessionIDs)
	}
}

// TestClient_Run_StdinLoop verifies that Run() drives the stdin read loop when no
// IM adapter is configured, forwarding each line as an im.Message to HandleMessage.
func TestClient_Run_StdinLoop(t *testing.T) {
	mock := &mockSession{adapterN: "codex"}
	c := newTestClient(mock)

	// Replace os.Stdin with a pipe to simulate typed input.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		r.Close()
	}()

	// Write one message then close stdin to trigger EOF and exit the loop.
	go func() {
		defer w.Close()
		fmt.Fprintln(w, "hello from stdin")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mock.mu.Lock()
	calls := mock.promptCalls
	mock.mu.Unlock()
	if len(calls) == 0 || calls[0] != "hello from stdin" {
		t.Errorf("Run did not forward stdin message; promptCalls = %v", calls)
	}
}

// runClientMockAgent is a minimal ACP server for client-level tests.
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
