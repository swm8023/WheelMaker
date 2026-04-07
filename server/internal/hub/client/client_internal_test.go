package client

// client_internal_test.go consolidates all white-box (package client) tests and
// the export helpers that expose internals to the external test package.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/im"
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
	callbacks agent.Callbacks
	promptFn  func(context.Context, string) (<-chan acp.Update, error)
	cancelFn  func() error
}

type Message struct {
	ChannelID string
	ChatID    string
	Text      string
	SessionID string
}

type TestCaptureRouter struct {
	mu          sync.Mutex
	Messages    []string
	ChatIDs     []string
	CardCount   int
	Decisions   []fakeIMDecision
	textBuffers map[string]string
}

type fakeIMRouter struct {
	binds     []fakeIMBind
	sends     []fakeIMSend
	decisions []fakeIMDecision
}

type fakeIMBind struct {
	chat      im.ChatRef
	sessionID string
	opts      im.BindOptions
}

type fakeIMSend struct {
	target im.SendTarget
	event  im.OutboundEvent
}

type fakeIMDecision struct {
	target im.SendTarget
	req    im.DecisionRequest
}

func (f *fakeIMRouter) Bind(_ context.Context, chat im.ChatRef, sessionID string, opts im.BindOptions) error {
	f.binds = append(f.binds, fakeIMBind{chat: chat, sessionID: sessionID, opts: opts})
	return nil
}

func (f *fakeIMRouter) Send(_ context.Context, target im.SendTarget, event im.OutboundEvent) error {
	f.sends = append(f.sends, fakeIMSend{target: target, event: event})
	return nil
}

func (f *fakeIMRouter) RequestDecision(_ context.Context, target im.SendTarget, req im.DecisionRequest) (im.DecisionResult, error) {
	f.decisions = append(f.decisions, fakeIMDecision{target: target, req: req})
	return im.DecisionResult{Outcome: "selected", OptionID: "allow", Value: "allow_once", Source: "card_action"}, nil
}

func (f *fakeIMRouter) Run(context.Context) error { return nil }

func NewTestCaptureRouter() *TestCaptureRouter {
	return &TestCaptureRouter{textBuffers: map[string]string{}}
}

func (r *TestCaptureRouter) Bind(context.Context, im.ChatRef, string, im.BindOptions) error {
	return nil
}

func (r *TestCaptureRouter) Send(_ context.Context, target im.SendTarget, event im.OutboundEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	chatID := target.ChatID
	if target.Source != nil && strings.TrimSpace(target.Source.ChatID) != "" {
		chatID = strings.TrimSpace(target.Source.ChatID)
	}

	switch event.Kind {
	case im.OutboundSystem:
		payload, ok := event.Payload.(im.TextPayload)
		if !ok {
			return nil
		}
		r.Messages = append(r.Messages, payload.Text)
		r.ChatIDs = append(r.ChatIDs, chatID)
	case im.OutboundACP:
		payload, ok := event.Payload.(im.ACPPayload)
		if !ok {
			return nil
		}
		key := strings.TrimSpace(target.SessionID)
		if key == "" {
			key = chatID
		}
		switch payload.UpdateType {
		case string(acp.UpdateText):
			r.textBuffers[key] += payload.Text
		case string(acp.UpdateDone):
			if text := r.textBuffers[key]; text != "" {
				r.Messages = append(r.Messages, text)
				r.ChatIDs = append(r.ChatIDs, chatID)
				delete(r.textBuffers, key)
			}
		default:
			if strings.HasPrefix(payload.UpdateType, "tool_call") {
				r.CardCount++
			}
		}
	}
	return nil
}

func (r *TestCaptureRouter) RequestDecision(_ context.Context, target im.SendTarget, req im.DecisionRequest) (im.DecisionResult, error) {
	r.mu.Lock()
	r.Decisions = append(r.Decisions, fakeIMDecision{target: target, req: req})
	r.mu.Unlock()
	return im.DecisionResult{Outcome: "cancelled", Source: "test"}, nil
}

func (r *TestCaptureRouter) Run(context.Context) error { return nil }

var _ agent.Instance = (*testInjectedInstance)(nil)
var _ IMRouter = (*TestCaptureRouter)(nil)

func (i *testInjectedInstance) Name() string { return i.name }

func (i *testInjectedInstance) SetCallbacks(callbacks agent.Callbacks) {
	i.callbacks = callbacks
}

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

// HandleMessage preserves the old test entrypoint shape while routing through IM.
func (c *Client) HandleMessage(msg Message) {
	channelID := strings.TrimSpace(msg.ChannelID)
	if channelID == "" {
		channelID = "test"
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	source := im.ChatRef{ChannelID: channelID, ChatID: strings.TrimSpace(msg.ChatID)}
	routeKey := normalizeRouteKey(imRouteKey(source))
	if source.ChatID == "" {
		routeKey = "default"
	}

	if cmd, args, ok := parseCommand(text); ok {
		switch cmd {
		case "/new":
			sess := c.ClientNewSession(routeKey)
			if source.ChatID != "" {
				sess.setIMSource(source)
			}
			sess.reply("Created new session: " + sess.ID)
			return
		case "/load":
			idx, err := parsePositiveIndex(args)
			if err != nil {
				sess := c.resolveSession(routeKey)
				if source.ChatID != "" {
					sess.setIMSource(source)
				}
				sess.reply("Load error: " + err.Error())
				return
			}
			loaded, err := c.ClientLoadSession(routeKey, idx)
			if err != nil {
				sess := c.resolveSession(routeKey)
				if source.ChatID != "" {
					sess.setIMSource(source)
				}
				sess.reply("Load error: " + err.Error())
				return
			}
			if source.ChatID != "" {
				loaded.setIMSource(source)
			}
			loaded.reply("Loaded session: " + loaded.ID)
			return
		}
	}

	sess := c.resolveSession(routeKey)
	if source.ChatID != "" {
		sess.setIMSource(source)
	}

	if cmd, args, ok := parseCommand(text); ok {
		c.handleCommand(sess, routeKey, cmd, args)
		return
	}
	sess.handlePrompt(text)
}

// InjectAgentFactory overrides one built-in provider creator for tests.
func (c *Client) InjectAgentFactory(provider acp.ACPProvider, creator agent.InstanceCreator) {
	if c == nil || creator == nil {
		return
	}

	c.mu.Lock()
	if c.registry == nil {
		c.registry = agent.DefaultACPFactory().Clone()
	} else if c.registry == agent.DefaultACPFactory() {
		c.registry = c.registry.Clone()
	}
	registry := c.registry
	for _, sess := range c.sessions {
		sess.registry = registry
	}
	c.mu.Unlock()

	if registry != nil {
		registry.Register(provider, creator)
	}
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
	c.InjectAgentFactory(acp.ACPProviderCodex, nopCreator)
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
	c.InjectAgentFactory(acp.ACPProviderCodex, nopCreator)
	c.InjectAgentFactory(acp.ACPProviderClaude, nopCreator)
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

func nopCreator(context.Context) (agent.Instance, error) {
	return nil, errors.New("test-only factory")
}

// noopStore is a Store that always returns a default state and discards saves.
type noopStore struct{}

func (s *noopStore) Load() (*ProjectState, error) { return defaultProjectState(), nil }
func (s *noopStore) Save(_ *ProjectState) error   { return nil }

func TestHandleIMInbound_ListDirectDoesNotBind(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	fake := &fakeIMRouter{}
	c.SetIMRouter(fake)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := c.HandleIMInbound(context.Background(), im.InboundEvent{ChannelID: "feishu", ChatID: "chat-a", Text: "/list"}); err != nil {
		t.Fatalf("HandleIMInbound: %v", err)
	}

	if len(fake.binds) != 0 {
		t.Fatalf("binds=%+v, want none", fake.binds)
	}
	if len(fake.sends) != 1 {
		t.Fatalf("sends=%+v, want direct /list response", fake.sends)
	}
	if fake.sends[0].target.SessionID != "" || fake.sends[0].target.ChannelID != "feishu" || fake.sends[0].target.ChatID != "chat-a" {
		t.Fatalf("target=%+v, want direct chat target", fake.sends[0].target)
	}
}

func TestHandleIMInbound_UnboundPromptBindsAndEmitsACP(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	fake := &fakeIMRouter{}
	c.SetIMRouter(fake)
	c.InjectAgentFactory(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) {
		return &testInjectedInstance{
			name:      "claude",
			sessionID: "acp-1",
			promptFn: func(context.Context, string) (<-chan acp.Update, error) {
				ch := make(chan acp.Update, 2)
				ch <- acp.Update{Type: acp.UpdateText, Content: "hello back"}
				ch <- acp.Update{Type: acp.UpdateDone, Content: "end_turn", Done: true}
				close(ch)
				return ch, nil
			},
		}, nil
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := c.HandleIMInbound(context.Background(), im.InboundEvent{ChannelID: "feishu", ChatID: "chat-a", Text: "hello"}); err != nil {
		t.Fatalf("HandleIMInbound: %v", err)
	}

	if len(fake.binds) != 1 {
		t.Fatalf("binds=%+v, want one bind", fake.binds)
	}
	if fake.binds[0].chat != (im.ChatRef{ChannelID: "feishu", ChatID: "chat-a"}) {
		t.Fatalf("bind=%+v", fake.binds[0])
	}
	foundACP := false
	for _, send := range fake.sends {
		if send.event.Kind == im.OutboundACP {
			payload, ok := send.event.Payload.(im.ACPPayload)
			if ok && payload.UpdateType == string(acp.UpdateText) {
				foundACP = true
				if payload.Text != "hello back" {
					t.Fatalf("payload=%+v", payload)
				}
			}
		}
	}
	if !foundACP {
		t.Fatalf("sends=%+v, want OutboundACP", fake.sends)
	}
}

func TestHandleIMInbound_ReusesBoundRouteWithoutSessionID(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	fake := &fakeIMRouter{}
	c.SetIMRouter(fake)

	routeKey := imRouteKey(im.ChatRef{ChannelID: "feishu", ChatID: "chat-a"})
	existing := c.ClientNewSession(routeKey)

	if err := c.HandleIMInbound(context.Background(), im.InboundEvent{ChannelID: "feishu", ChatID: "chat-a", Text: "/status"}); err != nil {
		t.Fatalf("HandleIMInbound: %v", err)
	}

	c.mu.Lock()
	activeID := c.activeSession.ID
	routedID := c.routeMap[routeKey]
	sessionCount := len(c.sessions)
	c.mu.Unlock()

	if activeID != existing.ID {
		t.Fatalf("active session = %q, want %q", activeID, existing.ID)
	}
	if routedID != existing.ID {
		t.Fatalf("routeMap[%q] = %q, want %q", routeKey, routedID, existing.ID)
	}
	if sessionCount != 2 {
		t.Fatalf("session count = %d, want 2", sessionCount)
	}
}

func TestSessionRequestPermission_UsesIMDecision(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	fake := &fakeIMRouter{}
	c.SetIMRouter(fake)
	sess := c.activeSession
	sess.setIMSource(im.ChatRef{ChannelID: "feishu", ChatID: "chat-a"})

	res, err := sess.SessionRequestPermission(context.Background(), acp.PermissionRequestParams{
		SessionID: "acp-1",
		ToolCall:  acp.ToolCallRef{ToolCallID: "tc-1", Title: "Read file", Kind: "read"},
		Options:   []acp.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: "allow_once"}},
	})
	if err != nil {
		t.Fatalf("SessionRequestPermission: %v", err)
	}
	if res.OptionID != "allow" || res.Outcome != "selected" {
		t.Fatalf("result=%+v", res)
	}
	if len(fake.decisions) != 1 {
		t.Fatalf("decisions=%+v, want one", fake.decisions)
	}
	if fake.decisions[0].target.Source == nil || fake.decisions[0].target.Source.ChatID != "chat-a" {
		t.Fatalf("target=%+v", fake.decisions[0].target)
	}
	if fake.decisions[0].req.Options[0].ID != "allow" {
		t.Fatalf("request=%+v", fake.decisions[0].req)
	}
}

// --- merged from server/internal/hub/client/multi_session_test.go ---

// ---------------------------------------------------------------------------
// Client-level session management: /new, /load, /list
// ---------------------------------------------------------------------------

func TestClientNewSession_CreatesAndBindsRoute(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	oldSess := c.activeSession
	if oldSess == nil {
		t.Fatal("expected default session")
	}

	newSess := c.ClientNewSession("route-1")
	if newSess == nil {
		t.Fatal("ClientNewSession returned nil")
	}
	if newSess.ID == oldSess.ID {
		t.Fatal("new session should have different ID from old")
	}
	if c.activeSession != newSess {
		t.Fatal("activeSession should be the new session")
	}

	c.mu.Lock()
	sessID := c.routeMap["route-1"]
	c.mu.Unlock()
	if sessID != newSess.ID {
		t.Fatalf("routeMap[route-1] = %q, want %q", sessID, newSess.ID)
	}

	// Old session should still exist in the sessions map.
	c.mu.Lock()
	_, oldExists := c.sessions[oldSess.ID]
	c.mu.Unlock()
	if !oldExists {
		t.Fatal("old session should still be in sessions map")
	}
}

func TestClientNewSession_SuspendsOldSession(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	// Map "route-1" to the default session first.
	c.mu.Lock()
	c.routeMap["route-1"] = "default"
	c.mu.Unlock()

	oldSess := c.activeSession

	_ = c.ClientNewSession("route-1")

	oldSess.mu.Lock()
	status := oldSess.Status
	oldSess.mu.Unlock()
	if status != SessionSuspended {
		t.Fatalf("old session status = %d, want SessionSuspended(%d)", status, SessionSuspended)
	}
}

func TestClientListSessions_MergesInMemory(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	// Create a second session.
	_ = c.ClientNewSession("route-1")

	entries, err := c.clientListSessions()
	if err != nil {
		t.Fatalf("clientListSessions: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}

	// All should be in-memory.
	for _, e := range entries {
		if !e.InMemory {
			t.Fatalf("entry %q should be in-memory", e.ID)
		}
	}
}

func TestClientListSessions_MergesPersistedSessions(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)

	// Persist a session.
	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "persisted-1",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		LastReply:    "hello",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActiveAt: time.Now().Add(-30 * time.Minute),
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save snap: %v", err)
	}

	entries, err := c.clientListSessions()
	if err != nil {
		t.Fatalf("clientListSessions: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.ID == "persisted-1" {
			found = true
			if e.InMemory {
				t.Fatal("persisted session should not be marked in-memory")
			}
			if e.Status != SessionPersisted {
				t.Fatalf("persisted session status = %d, want SessionPersisted", e.Status)
			}
		}
	}
	if !found {
		t.Fatal("persisted session not in list")
	}
}

func TestClientLoadSession_RestoresFromStore(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)

	// Persist a session.
	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "restore-me",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		LastReply:    "previous reply",
		ACPSessionID: "acp-999",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActiveAt: time.Now().Add(-10 * time.Minute),
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save snap: %v", err)
	}

	// List to get index.
	entries, err := c.clientListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	// Find the index of "restore-me".
	idx := -1
	for i, e := range entries {
		if e.ID == "restore-me" {
			idx = i + 1
			break
		}
	}
	if idx == -1 {
		t.Fatal("restore-me not in list")
	}

	loaded, err := c.ClientLoadSession("route-1", idx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ID != "restore-me" {
		t.Fatalf("loaded session ID = %q, want %q", loaded.ID, "restore-me")
	}
	if loaded.lastReply != "previous reply" {
		t.Fatalf("loaded lastReply = %q, want %q", loaded.lastReply, "previous reply")
	}
	if loaded.Status != SessionActive {
		t.Fatalf("loaded status = %d, want SessionActive", loaded.Status)
	}

	// Route should point to the loaded session.
	c.mu.Lock()
	routedID := c.routeMap["route-1"]
	c.mu.Unlock()
	if routedID != "restore-me" {
		t.Fatalf("route-1 -> %q, want %q", routedID, "restore-me")
	}
}

func TestClientLoadSession_IndexOutOfRange(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	_, err := c.ClientLoadSession("route-1", 999)
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestClientLoadSession_InMemoryRebind(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	// Create 2 sessions.
	s1 := c.ClientNewSession("route-1")
	_ = c.ClientNewSession("route-1") // s2 now active, s1 suspended

	// List and find s1.
	entries, err := c.clientListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	idx := -1
	for i, e := range entries {
		if e.ID == s1.ID {
			idx = i + 1
			break
		}
	}
	if idx == -1 {
		t.Fatalf("s1 not in list")
	}

	loaded, err := c.ClientLoadSession("route-1", idx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ID != s1.ID {
		t.Fatalf("loaded ID = %q, want %q", loaded.ID, s1.ID)
	}
}

// ---------------------------------------------------------------------------
// Timer-driven eviction
// ---------------------------------------------------------------------------

func TestEvictSuspendedSessions(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)
	c.suspendTimeout = 0 // instant eviction

	// Create and suspend a session.
	c.mu.Lock()
	sess := c.newWiredSession("evict-me")
	sess.Status = SessionSuspended
	sess.lastActiveAt = time.Now().Add(-time.Minute)
	c.sessions["evict-me"] = sess
	c.mu.Unlock()

	c.evictSuspendedSessions()

	// Session should be removed from memory.
	c.mu.Lock()
	_, exists := c.sessions["evict-me"]
	c.mu.Unlock()
	if exists {
		t.Fatal("evicted session should not be in sessions map")
	}

	// Should be in SQLite.
	ctx := context.Background()
	snap, err := store.Load(ctx, "evict-me")
	if err != nil {
		t.Fatalf("load from store: %v", err)
	}
	if snap == nil {
		t.Fatal("evicted session not found in store")
	}
	if snap.ID != "evict-me" {
		t.Fatalf("snap ID = %q, want %q", snap.ID, "evict-me")
	}
}

func TestEvictSuspendedSessions_ActiveNotEvicted(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)
	c.suspendTimeout = 0

	// Default session is active, should not be evicted.
	c.evictSuspendedSessions()

	c.mu.Lock()
	_, exists := c.sessions["default"]
	c.mu.Unlock()
	if !exists {
		t.Fatal("active session should not be evicted")
	}
}

func TestEvictSuspendedSessions_RespectsTimeout(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)
	c.suspendTimeout = 1 * time.Hour // won't expire

	c.mu.Lock()
	sess := c.newWiredSession("not-yet")
	sess.Status = SessionSuspended
	sess.lastActiveAt = time.Now() // just suspended
	c.sessions["not-yet"] = sess
	c.mu.Unlock()

	c.evictSuspendedSessions()

	c.mu.Lock()
	_, exists := c.sessions["not-yet"]
	c.mu.Unlock()
	if !exists {
		t.Fatal("recently suspended session should not be evicted yet")
	}
}

// ---------------------------------------------------------------------------
// resolveSession: restore evicted sessions
// ---------------------------------------------------------------------------

func TestResolveSession_RestoresEvictedSession(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(&noopStore{}, nil, "proj1", "/tmp")
	c.SetSessionStore(store)

	// Persist a session to SQLite.
	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "evicted-sess",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		LastReply:    "restored content",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActiveAt: time.Now().Add(-5 * time.Minute),
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Set up routeMap pointing to the evicted session ID.
	c.mu.Lock()
	c.routeMap["route-x"] = "evicted-sess"
	c.mu.Unlock()

	// resolveSession should restore from SQLite.
	sess := c.resolveSession("route-x")

	if sess.ID != "evicted-sess" {
		t.Fatalf("resolved session ID = %q, want %q", sess.ID, "evicted-sess")
	}
	if sess.lastReply != "restored content" {
		t.Fatalf("restored lastReply = %q, want %q", sess.lastReply, "restored content")
	}

	// Should now be in memory.
	c.mu.Lock()
	_, exists := c.sessions["evicted-sess"]
	c.mu.Unlock()
	if !exists {
		t.Fatal("restored session should be in sessions map")
	}
}

// ---------------------------------------------------------------------------
// Session ID generation
// ---------------------------------------------------------------------------

func TestNextSessionID_Unique(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		c.mu.Lock()
		id := c.nextSessionID()
		c.mu.Unlock()
		if ids[id] {
			t.Fatalf("duplicate session ID: %s", id)
		}
		ids[id] = true
	}
}

// --- merged from server/internal/hub/client/sqlite_store_test.go ---

func tempDBPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

func TestSQLiteSessionStore_SaveLoad(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	snap := &SessionSnapshot{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       SessionSuspended,
		ActiveAgent:  "claude",
		LastReply:    "hello world",
		ACPSessionID: "acp-123",
		CreatedAt:    now.Add(-time.Hour),
		LastActiveAt: now,
		Agents: map[string]*SessionAgentState{
			"claude": {
				ACPSessionID: "acp-123",
				ConfigOptions: []acp.ConfigOption{
					{ID: "mode", CurrentValue: "code"},
				},
				Title:     "Test Session",
				UpdatedAt: "2025-01-01T00:00:00Z",
			},
		},
	}

	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load(ctx, "sess-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded is nil")
	}
	if loaded.ID != "sess-1" {
		t.Errorf("id=%q, want sess-1", loaded.ID)
	}
	if loaded.Status != SessionSuspended {
		t.Errorf("status=%d, want %d", loaded.Status, SessionSuspended)
	}
	if loaded.ActiveAgent != "claude" {
		t.Errorf("activeAgent=%q, want claude", loaded.ActiveAgent)
	}
	if loaded.LastReply != "hello world" {
		t.Errorf("lastReply=%q, want hello world", loaded.LastReply)
	}
	if loaded.ACPSessionID != "acp-123" {
		t.Errorf("acpSessionID=%q, want acp-123", loaded.ACPSessionID)
	}
	if as := loaded.Agents["claude"]; as == nil {
		t.Error("missing agent claude")
	} else {
		if as.Title != "Test Session" {
			t.Errorf("agent title=%q, want 'Test Session'", as.Title)
		}
		if len(as.ConfigOptions) != 1 || as.ConfigOptions[0].ID != "mode" {
			t.Errorf("agent configOptions=%v, want [{ID:mode}]", as.ConfigOptions)
		}
	}
}

func TestSQLiteSessionStore_LoadNonExistent(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	loaded, err := store.Load(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil for nonexistent, got %+v", loaded)
	}
}

func TestSQLiteSessionStore_List(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	for i, id := range []string{"a", "b", "c"} {
		snap := &SessionSnapshot{
			ID:           id,
			ProjectName:  "proj1",
			ActiveAgent:  "claude",
			CreatedAt:    now.Add(time.Duration(-3+i) * time.Hour),
			LastActiveAt: now.Add(time.Duration(-3+i) * time.Hour),
		}
		if err := store.Save(ctx, snap); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	entries, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("list len=%d, want 3", len(entries))
	}
	// Should be ordered by last_active DESC.
	if entries[0].ID != "c" || entries[1].ID != "b" || entries[2].ID != "a" {
		t.Errorf("order: %s, %s, %s — want c, b, a", entries[0].ID, entries[1].ID, entries[2].ID)
	}
}

func TestSQLiteSessionStore_Delete(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "del-me",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
		Agents: map[string]*SessionAgentState{
			"claude": {ACPSessionID: "acp-1"},
		},
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := store.Delete(ctx, "del-me"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	loaded, err := store.Load(ctx, "del-me")
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if loaded != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestSQLiteSessionStore_Upsert(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	snap := &SessionSnapshot{
		ID:           "upsert-me",
		ProjectName:  "proj1",
		ActiveAgent:  "claude",
		LastReply:    "first",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save 1: %v", err)
	}

	snap.LastReply = "second"
	snap.ActiveAgent = "copilot"
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save 2: %v", err)
	}

	loaded, err := store.Load(ctx, "upsert-me")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.LastReply != "second" {
		t.Errorf("lastReply=%q, want second", loaded.LastReply)
	}
	if loaded.ActiveAgent != "copilot" {
		t.Errorf("activeAgent=%q, want copilot", loaded.ActiveAgent)
	}
}

func TestSQLiteSessionStore_ProjectIsolation(t *testing.T) {
	dbPath := tempDBPath(t)

	store1, err := NewSQLiteSessionStore(dbPath, "proj1")
	if err != nil {
		t.Fatalf("new store1: %v", err)
	}
	defer store1.Close()

	store2, err := NewSQLiteSessionStore(dbPath, "proj2")
	if err != nil {
		t.Fatalf("new store2: %v", err)
	}
	defer store2.Close()

	ctx := context.Background()
	_ = store1.Save(ctx, &SessionSnapshot{ID: "s1", ProjectName: "proj1", CreatedAt: time.Now(), LastActiveAt: time.Now()})
	_ = store2.Save(ctx, &SessionSnapshot{ID: "s2", ProjectName: "proj2", CreatedAt: time.Now(), LastActiveAt: time.Now()})

	// proj1 should only see s1.
	list1, _ := store1.List(ctx)
	if len(list1) != 1 || list1[0].ID != "s1" {
		t.Errorf("proj1 list: %v", list1)
	}

	// proj2 should only see s2.
	list2, _ := store2.List(ctx)
	if len(list2) != 1 || list2[0].ID != "s2" {
		t.Errorf("proj2 list: %v", list2)
	}

	// proj1 cannot load s2.
	loaded, _ := store1.Load(ctx, "s2")
	if loaded != nil {
		t.Error("proj1 should not load proj2's session")
	}
}

func TestSQLiteSessionStore_FileCreation(t *testing.T) {
	dbPath := tempDBPath(t)
	store, err := NewSQLiteSessionStore(dbPath, "test")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

// ---------------------------------------------------------------------------
// Additional regressions for session robustness
// ---------------------------------------------------------------------------

type nilLoadSessionStore struct {
	entries []SessionSummaryEntry
}

func (s *nilLoadSessionStore) Save(context.Context, *SessionSnapshot) error { return nil }
func (s *nilLoadSessionStore) Load(context.Context, string) (*SessionSnapshot, error) {
	return nil, nil
}
func (s *nilLoadSessionStore) List(context.Context) ([]SessionSummaryEntry, error) {
	return append([]SessionSummaryEntry(nil), s.entries...), nil
}
func (s *nilLoadSessionStore) Delete(context.Context, string) error { return nil }
func (s *nilLoadSessionStore) Close() error                         { return nil }

func TestClientLoadSession_MissingStoredSnapshotReturnsError(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	c.SetSessionStore(&nilLoadSessionStore{entries: []SessionSummaryEntry{
		{ID: "ghost", ActiveAgent: "claude", CreatedAt: time.Now().Add(-time.Hour), LastActiveAt: time.Now().Add(time.Hour)},
	}})

	entries, err := c.clientListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	idx := -1
	for i, e := range entries {
		if e.ID == "ghost" {
			idx = i + 1
			break
		}
	}
	if idx < 0 {
		t.Fatalf("ghost entry not found: %+v", entries)
	}

	_, err = c.ClientLoadSession("route-x", idx)
	if err == nil || !strings.Contains(err.Error(), "not found in session store") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

func TestSessionUpdate_NoPromptContext_DoesNotBlockWhenChannelFull(t *testing.T) {
	s := newSession("sess", "/tmp")
	content, _ := json.Marshal(acp.ContentBlock{Type: "text", Text: "chunk"})

	ch := make(chan acp.Update, 1)
	ch <- acp.Update{Type: acp.UpdateText, Content: "prefill"}

	s.mu.Lock()
	s.acpSessionID = "acp-1"
	s.prompt.updatesCh = ch
	s.prompt.ctx = nil
	s.mu.Unlock()

	params := acp.SessionUpdateParams{
		SessionID: "acp-1",
		Update: acp.SessionUpdate{
			SessionUpdate: "agent_message_chunk",
			Content:       content,
		},
	}

	done := make(chan struct{})
	go func() {
		s.SessionUpdate(params)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("SessionUpdate blocked when prompt channel was full and prompt context was nil")
	}
}

func TestSessionSnapshot_DeepCopiesSlices(t *testing.T) {
	s := newSession("sess", "/tmp")
	s.mu.Lock()
	s.sessionMeta = clientSessionMeta{
		ConfigOptions:     []acp.ConfigOption{{ID: "mode", CurrentValue: "code"}},
		AvailableCommands: []acp.AvailableCommand{{Name: "build"}},
	}
	s.agents["claude"] = &SessionAgentState{
		ConfigOptions: []acp.ConfigOption{{ID: "model", CurrentValue: "gpt-4"}},
		Commands:      []acp.AvailableCommand{{Name: "run"}},
	}
	s.mu.Unlock()

	snap := s.Snapshot("proj")

	s.mu.Lock()
	s.sessionMeta.ConfigOptions[0].CurrentValue = "plan"
	s.sessionMeta.AvailableCommands[0].Name = "changed"
	s.agents["claude"].ConfigOptions[0].CurrentValue = "gpt-5"
	s.agents["claude"].Commands[0].Name = "changed-run"
	s.mu.Unlock()

	if got := snap.SessionMeta.ConfigOptions[0].CurrentValue; got != "code" {
		t.Fatalf("snapshot sessionMeta config mutated: got=%q want=code", got)
	}
	if got := snap.SessionMeta.AvailableCommands[0].Name; got != "build" {
		t.Fatalf("snapshot sessionMeta command mutated: got=%q want=build", got)
	}
	if got := snap.Agents["claude"].ConfigOptions[0].CurrentValue; got != "gpt-4" {
		t.Fatalf("snapshot agent config mutated: got=%q want=gpt-4", got)
	}
	if got := snap.Agents["claude"].Commands[0].Name; got != "run" {
		t.Fatalf("snapshot agent command mutated: got=%q want=run", got)
	}
}

func TestRestoreFromSnapshot_DeepCopiesSlices(t *testing.T) {
	snap := &SessionSnapshot{
		ID:          "sess",
		ProjectName: "proj",
		SessionMeta: clientSessionMeta{
			ConfigOptions:     []acp.ConfigOption{{ID: "mode", CurrentValue: "code"}},
			AvailableCommands: []acp.AvailableCommand{{Name: "build"}},
		},
		Agents: map[string]*SessionAgentState{
			"claude": {
				ConfigOptions: []acp.ConfigOption{{ID: "model", CurrentValue: "gpt-4"}},
				Commands:      []acp.AvailableCommand{{Name: "run"}},
			},
		},
	}

	restored := RestoreFromSnapshot(snap, "/tmp")

	snap.SessionMeta.ConfigOptions[0].CurrentValue = "plan"
	snap.SessionMeta.AvailableCommands[0].Name = "changed"
	snap.Agents["claude"].ConfigOptions[0].CurrentValue = "gpt-5"
	snap.Agents["claude"].Commands[0].Name = "changed-run"

	restored.mu.Lock()
	defer restored.mu.Unlock()
	if got := restored.sessionMeta.ConfigOptions[0].CurrentValue; got != "code" {
		t.Fatalf("restored sessionMeta config mutated: got=%q want=code", got)
	}
	if got := restored.sessionMeta.AvailableCommands[0].Name; got != "build" {
		t.Fatalf("restored sessionMeta command mutated: got=%q want=build", got)
	}
	if got := restored.agents["claude"].ConfigOptions[0].CurrentValue; got != "gpt-4" {
		t.Fatalf("restored agent config mutated: got=%q want=gpt-4", got)
	}
	if got := restored.agents["claude"].Commands[0].Name; got != "run" {
		t.Fatalf("restored agent command mutated: got=%q want=run", got)
	}
}

func TestSQLiteSessionStore_SaveNilSnapshot(t *testing.T) {
	store, err := NewSQLiteSessionStore(tempDBPath(t), "proj1")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	if err := store.Save(context.Background(), nil); err == nil {
		t.Fatal("expected error when saving nil snapshot")
	}
}
