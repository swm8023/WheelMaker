package client

// client_internal_test.go consolidates all white-box (package client) tests and
// the export helpers that expose internals to the external test package.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/hub/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

// ---------------------------------------------------------------------------
// Export helpers (used by package client_test via export_test.go convention)
// ---------------------------------------------------------------------------

// InjectForwarder injects a ready test instance with prompt/cancel callbacks.
func (c *Client) InjectForwarder(agentName, sessionID string, promptFn func(context.Context, string) (<-chan acp.Update, error), cancelFn func() error) {
	sess := c.activeSession
	c.mu.Lock()
	name := strings.TrimSpace(agentName)
	if name == "" {
		name = defaultAgentName
	}
	if c.state != nil && strings.TrimSpace(c.state.ActiveAgent) != "" && strings.TrimSpace(agentName) == "" {
		name = c.state.ActiveAgent
	}
	if c.state == nil {
		c.state = defaultProjectState()
		sess.state = c.state
	}
	c.mu.Unlock()

	runtime := &testInjectedInstance{
		name:      name,
		sessionID: sessionID,
		callbacks: sess,
		promptFn:  promptFn,
		cancelFn:  cancelFn,
	}
	sess.mu.Lock()
	sess.instance = runtime
	sess.acpSessionID = sessionID
	sess.ready = true
	sess.mu.Unlock()
}

type testInjectedInstance struct {
	name      string
	sessionID string
	callbacks SessionCallbacks
	promptFn  func(context.Context, string) (<-chan acp.Update, error)
	cancelFn  func() error
}

var _ agent.Instance = (*testInjectedInstance)(nil)

func (i *testInjectedInstance) Name() string { return i.name }

func (i *testInjectedInstance) HandleACPRequest(context.Context, string, json.RawMessage) (any, error) {
	return nil, errors.New("not implemented in test injected instance")
}

func (i *testInjectedInstance) HandleACPResponse(context.Context, string, json.RawMessage) {}

func (i *testInjectedInstance) Initialize(context.Context, acp.InitializeParams) (acp.InitializeResult, error) {
	return acp.InitializeResult{
		ProtocolVersion: "0.1",
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: false,
		},
		AgentInfo: &acp.AgentInfo{Name: "test-injected-agent"},
	}, nil
}

func (i *testInjectedInstance) SessionNew(context.Context, acp.SessionNewParams) (acp.SessionNewResult, error) {
	sid := strings.TrimSpace(i.sessionID)
	if sid == "" {
		sid = "sess-1"
	}
	return acp.SessionNewResult{SessionID: sid}, nil
}

func (i *testInjectedInstance) SessionLoad(context.Context, acp.SessionLoadParams) (acp.SessionLoadResult, error) {
	return acp.SessionLoadResult{}, nil
}

func (i *testInjectedInstance) SessionList(context.Context, acp.SessionListParams) (acp.SessionListResult, error) {
	return acp.SessionListResult{}, nil
}

func (i *testInjectedInstance) SessionPrompt(ctx context.Context, p acp.SessionPromptParams) (acp.SessionPromptResult, error) {
	if i.promptFn == nil {
		return acp.SessionPromptResult{StopReason: "end_turn"}, nil
	}
	text := ""
	for _, b := range p.Prompt {
		if b.Type == "text" {
			text = b.Text
			break
		}
	}
	updates, err := i.promptFn(ctx, text)
	if err != nil {
		return acp.SessionPromptResult{}, err
	}
	stopReason := "end_turn"
	for u := range updates {
		if u.Err != nil {
			return acp.SessionPromptResult{}, u.Err
		}
		if u.Done {
			if strings.TrimSpace(u.Content) != "" {
				stopReason = strings.TrimSpace(u.Content)
			}
			break
		}
		i.emitUpdate(p.SessionID, u)
	}
	return acp.SessionPromptResult{StopReason: stopReason}, nil
}

func (i *testInjectedInstance) SessionCancel(_ string) error {
	if i.cancelFn != nil {
		return i.cancelFn()
	}
	return nil
}

func (i *testInjectedInstance) SessionSetConfigOption(_ context.Context, p acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
	return []acp.ConfigOption{
		{
			ID:           p.ConfigID,
			CurrentValue: p.Value,
		},
	}, nil
}

func (i *testInjectedInstance) Close() error { return nil }

func (i *testInjectedInstance) emitUpdate(sessionID string, u acp.Update) {
	if i.callbacks == nil {
		return
	}
	update := acp.SessionUpdate{}
	switch u.Type {
	case acp.UpdateText:
		content, _ := json.Marshal(acp.ContentBlock{Type: "text", Text: u.Content})
		update = acp.SessionUpdate{SessionUpdate: "agent_message_chunk", Content: content}
	case acp.UpdateThought:
		content, _ := json.Marshal(acp.ContentBlock{Type: "text", Text: u.Content})
		update = acp.SessionUpdate{SessionUpdate: "agent_thought_chunk", Content: content}
	case acp.UpdateToolCall, acp.UpdateConfigOption, acp.UpdateAvailableCommands, acp.UpdateSessionInfo, acp.UpdatePlan, acp.UpdateModeChange:
		if len(u.Raw) == 0 || json.Unmarshal(u.Raw, &update) != nil {
			return
		}
	default:
		return
	}
	i.callbacks.SessionUpdate(acp.SessionUpdateParams{SessionID: sessionID, Update: update})
}

// InjectState replaces the persisted state.
func (c *Client) InjectState(st *ProjectState) {
	c.mu.Lock()
	c.state = st
	c.mu.Unlock()
	c.activeSession.mu.Lock()
	c.activeSession.state = st
	c.activeSession.mu.Unlock()
}

// InjectIMChannel sets the IM bridge over the provided IM channel.
func (c *Client) InjectIMChannel(p im.Channel) {
	c.imBridge = im.NewBridge(p)
	c.activeSession.mu.Lock()
	c.activeSession.imBridge = c.imBridge
	c.activeSession.mu.Unlock()
}

// DefaultState returns a freshly initialised default state.
func DefaultState() *ProjectState {
	return defaultProjectState()
}

// ---------------------------------------------------------------------------
// Keepalive / error-detection helpers
// ---------------------------------------------------------------------------

func TestIsAgentExitError(t *testing.T) {
	cases := []string{
		"acp rpc error -1: agent process exited",
		"io: broken pipe",
		"read tcp ... connection reset by peer",
		"EOF",
	}
	for _, c := range cases {
		if !isAgentExitError(internalTestErr(c)) {
			t.Fatalf("expected agent-exit match for %q", c)
		}
	}
}

func TestIsAgentExitErrorFalse(t *testing.T) {
	if isAgentExitError(internalTestErr("Selected model is at capacity")) {
		t.Fatalf("capacity error must not be treated as process exit")
	}
}

func TestHasSandboxRefreshError(t *testing.T) {
	u := acp.Update{
		Type:    acp.UpdateToolCall,
		Content: "tool failed: windows sandbox: spawn setup refresh",
	}
	if !hasSandboxRefreshError(u) {
		t.Fatal("expected sandbox refresh detection")
	}
}

type internalTestErr string

func (e internalTestErr) Error() string { return string(e) }

// ---------------------------------------------------------------------------
// Permission router
// ---------------------------------------------------------------------------

func TestPermissionRouterDecisionSlotSerializes(t *testing.T) {
	r := newPermissionRouter(&Session{})

	if !r.acquireDecisionSlot(context.Background()) {
		t.Fatal("first acquire failed")
	}

	acquiredSecond := make(chan struct{})
	releaseSecond := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if !r.acquireDecisionSlot(context.Background()) {
			return
		}
		close(acquiredSecond)
		<-releaseSecond
		r.releaseDecisionSlot()
	}()

	select {
	case <-acquiredSecond:
		t.Fatal("second acquire should block until first release")
	case <-time.After(120 * time.Millisecond):
	}

	r.releaseDecisionSlot()

	select {
	case <-acquiredSecond:
	case <-time.After(2 * time.Second):
		t.Fatal("second acquire did not proceed after first release")
	}

	close(releaseSecond)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second goroutine did not finish")
	}
}

func TestPermissionRouterDecisionSlotRespectsContextCancel(t *testing.T) {
	r := newPermissionRouter(&Session{})

	if !r.acquireDecisionSlot(context.Background()) {
		t.Fatal("first acquire failed")
	}
	defer r.releaseDecisionSlot()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if r.acquireDecisionSlot(ctx) {
		t.Fatal("acquire should fail when context is cancelled")
	}
	if time.Since(start) < 80*time.Millisecond {
		t.Fatal("acquire returned too early; expected to wait for context cancellation")
	}
}

func TestChooseAutoAllowOption(t *testing.T) {
	opts := []acp.PermissionOption{
		{OptionID: "reject", Kind: "reject_once"},
		{OptionID: "allow", Kind: "allow_once"},
	}
	if got := chooseAutoAllowOption(opts); got != "allow" {
		t.Fatalf("chooseAutoAllowOption()=%q, want allow", got)
	}
}

func TestChooseAutoAllowOptionFallbackFirst(t *testing.T) {
	opts := []acp.PermissionOption{
		{OptionID: "abort", Kind: "reject_once"},
		{OptionID: "deny", Kind: "reject_always"},
	}
	if got := chooseAutoAllowOption(opts); got != "" {
		t.Fatalf("chooseAutoAllowOption()=%q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Help model / config arg resolution
// ---------------------------------------------------------------------------

func TestResolveHelpModel_ExcludesDebugStatusAction(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	c.RegisterAgent("codex", nopFactory{name: "codex"})
	c.activeSession.ready = true

	model, err := c.activeSession.resolveHelpModel(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("resolveHelpModel error: %v", err)
	}

	hasDebugStatus := false
	for _, opt := range model.Options {
		if opt.Label == "Project Debug Status" && opt.Command == "/debug" && opt.Value == "" {
			hasDebugStatus = true
		}
	}
	if hasDebugStatus {
		t.Fatalf("help options should not include debug status action: %+v", model.Options)
	}
}

func TestResolveHelpModel_RootHasConfigEntriesAndAgentSubmenu(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	c.RegisterAgent("codex", nopFactory{name: "codex"})
	c.RegisterAgent("claude", nopFactory{name: "claude"})
	c.activeSession.ready = true
	c.activeSession.sessionMeta.ConfigOptions = []acp.ConfigOption{
		{
			ID:           "mode",
			CurrentValue: "plan",
			Options: []acp.ConfigOptionValue{
				{Name: "Plan", Value: "plan"},
				{Name: "Run", Value: "run"},
			},
		},
		{
			ID:           "theme",
			CurrentValue: "dark",
			Options: []acp.ConfigOptionValue{
				{Name: "Dark", Value: "dark"},
				{Name: "Light", Value: "light"},
			},
		},
	}

	model, err := c.activeSession.resolveHelpModel(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("resolveHelpModel error: %v", err)
	}

	hasAgentSwitch := false
	hasModeAtRoot := false
	hasThemeAtRoot := false
	for _, opt := range model.Options {
		switch {
		case opt.Label == "Agent Switch" && strings.TrimSpace(opt.MenuID) != "":
			hasAgentSwitch = true
		case strings.HasPrefix(opt.Label, "Config: mode"):
			hasModeAtRoot = true
		case strings.HasPrefix(opt.Label, "Config: theme"):
			hasThemeAtRoot = true
		}
	}
	if !hasAgentSwitch {
		t.Fatalf("help root menu missing agent switch entry: %+v", model.Options)
	}
	if !hasModeAtRoot || !hasThemeAtRoot {
		t.Fatalf("help root menu missing config entries: %+v", model.Options)
	}
}

func TestResolveConfigArg_ValidatesOptionValue(t *testing.T) {
	st := &SessionState{
		ConfigOptions: []acp.ConfigOption{
			{
				ID: "theme",
				Options: []acp.ConfigOptionValue{
					{Name: "Dark", Value: "dark"},
					{Name: "Light", Value: "light"},
				},
			},
		},
	}

	id, value, err := resolveConfigArg("theme Dark", st)
	if err != nil {
		t.Fatalf("resolveConfigArg returned error: %v", err)
	}
	if id != "theme" || value != "dark" {
		t.Fatalf("resolveConfigArg = (%q,%q), want (%q,%q)", id, value, "theme", "dark")
	}

	if _, _, err := resolveConfigArg("theme blue", st); err == nil {
		t.Fatalf("expected unknown config value error")
	}
}

func TestCanonicalIMBlockType(t *testing.T) {
	cases := map[string]string{
		"tool":                "tool_call",
		"tool_call":           "tool_call",
		"tool_call_update":    "tool_call",
		"tool_call_cancelled": "tool_call",
		"system":              "error",
		"thought":             "thought",
		"  TEXT  ":            "text",
		"":                    "",
	}
	for in, want := range cases {
		if got := canonicalIMBlockType(in); got != want {
			t.Fatalf("canonicalIMBlockType(%q)=%q, want %q", in, got, want)
		}
	}
}

type nopFactory struct {
	name string
}

func (f nopFactory) Name() string { return f.name }

func (f nopFactory) SupportsSharedConn() bool { return false }

func (f nopFactory) CreateInstance(context.Context, SessionCallbacks, io.Writer) (agent.Instance, error) {
	return nil, errors.New("test-only factory")
}

// noopStore is a Store that always returns a default state and discards saves.
type noopStore struct{}

func (s *noopStore) Load() (*ProjectState, error) { return defaultProjectState(), nil }
func (s *noopStore) Save(_ *ProjectState) error   { return nil }
