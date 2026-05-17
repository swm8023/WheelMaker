package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
	logger "github.com/swm8023/wheelmaker/internal/shared"
	_ "modernc.org/sqlite"
)

const testRouteKey = "test:local"

type Message struct {
	ChannelID string
	ChatID    string
	Text      string
	SessionID string
}

type testInjectedInstance struct {
	name        string
	sessionID   string
	alive       bool
	callbacks   agent.Callbacks
	promptFn    func(context.Context, string) (<-chan acp.SessionUpdateParams, acp.SessionPromptResult, error)
	lastPrompt  []acp.ContentBlock
	cancelFn    func() error
	initResult  acp.InitializeResult
	loadResult  acp.SessionLoadResult
	loadUpdates []acp.SessionUpdateParams
	loadErr     error
	newResult   *acp.SessionNewResult
	listResult  acp.SessionListResult
	listErr     error
	setConfigFn func(context.Context, acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error)
	setCalls    []acp.SessionSetConfigOptionParams
	skills      []agent.SkillDescriptor
	skillsErr   error
}

type fakeIMRouter struct {
	binds   []fakeIMBind
	updates []fakeIMUpdate
	systems []fakeIMSystem
}

type fakeIMBind struct {
	chat      im.ChatRef
	sessionID string
	opts      im.BindOptions
}

type fakeIMUpdate struct {
	target im.SendTarget
	params acp.SessionUpdateParams
}

type fakeIMSystem struct {
	target  im.SendTarget
	payload im.SystemPayload
}

type TestCaptureRouter struct {
	mu          sync.Mutex
	Messages    []string
	ChatIDs     []string
	CardCount   int
	textBuffers map[string]string
}

func (c *Client) InjectForwarder(agentName, sessionID string, promptFn func(context.Context, string) (<-chan acp.SessionUpdateParams, acp.SessionPromptResult, error), cancelFn func() error) {
	name := strings.TrimSpace(agentName)
	if name == "" {
		name = string(acp.ACPProviderClaude)
	}
	c.mu.Lock()
	sess := c.sessions[c.routeMap[testRouteKey]]
	if sess == nil {
		var err error
		sess, err = c.newWiredSession(sessionID, name)
		if err != nil {
			panic(err)
		}
		c.sessions[sessionID] = sess
		c.routeMap[testRouteKey] = sessionID
	}
	c.mu.Unlock()

	runtime := &testInjectedInstance{
		name:      name,
		sessionID: sessionID,
		alive:     true,
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
	routeKey := testRouteKey

	if cmd, args, ok := parseCommand(text); ok {
		switch cmd {
		case "/new":
			agentType := strings.TrimSpace(args)
			if agentType == "" {
				return
			}
			sess, err := c.ClientNewSession(routeKey, agentType)
			if err != nil {
				return
			}
			if source.ChatID != "" {
				sess.setIMSource(source)
			}
			sess.reply("Created new session: " + sess.acpSessionID)
			return
		case "/load":
			idx, err := parsePositiveIndex(args)
			if err != nil {
				if sess, resolveErr := c.resolveSession(routeKey); resolveErr == nil {
					sess.reply("Load error: " + err.Error())
				}
				return
			}
			loaded, err := c.ClientLoadSession(routeKey, idx)
			if err != nil {
				if sess, resolveErr := c.resolveSession(routeKey); resolveErr == nil {
					sess.reply("Load error: " + err.Error())
				}
				return
			}
			if source.ChatID != "" {
				loaded.setIMSource(source)
			}
			loaded.reply("Loaded session: " + loaded.acpSessionID)
			return
		}
	}

	sess, err := c.resolveSession(routeKey)
	if err != nil {
		return
	}
	if source.ChatID != "" {
		sess.setIMSource(source)
	}
	if cmd, args, ok := parseCommand(text); ok {
		c.handleCommand(sess, routeKey, cmd, args)
		return
	}
	sess.handlePrompt(text)
}

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
	registry.Register(provider, creator)
}

func (c *Client) RouteSessionIDForTest(routeKey string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.routeMap[routeKey]
}

func (c *Client) HasSessionInMemoryForTest(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.sessions[sessionID]
	return ok
}

func (c *Client) ResolveSessionForTest(routeKey string) (*Session, error) {
	return c.resolveSession(routeKey)
}

func (f *fakeIMRouter) Bind(_ context.Context, chat im.ChatRef, sessionID string, opts im.BindOptions) error {
	f.binds = append(f.binds, fakeIMBind{chat: chat, sessionID: sessionID, opts: opts})
	return nil
}

func (f *fakeIMRouter) PublishSessionUpdate(_ context.Context, target im.SendTarget, params acp.SessionUpdateParams) error {
	f.updates = append(f.updates, fakeIMUpdate{target: target, params: params})
	return nil
}

func (f *fakeIMRouter) PublishPromptResult(context.Context, im.SendTarget, acp.SessionPromptResult) error {
	return nil
}

func (f *fakeIMRouter) SystemNotify(_ context.Context, target im.SendTarget, payload im.SystemPayload) error {
	f.systems = append(f.systems, fakeIMSystem{target: target, payload: payload})
	return nil
}

func (f *fakeIMRouter) Run(context.Context) error { return nil }

func NewTestCaptureRouter() *TestCaptureRouter {
	return &TestCaptureRouter{textBuffers: map[string]string{}}
}

func (r *TestCaptureRouter) Bind(context.Context, im.ChatRef, string, im.BindOptions) error {
	return nil
}

func (r *TestCaptureRouter) PublishSessionUpdate(_ context.Context, target im.SendTarget, params acp.SessionUpdateParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	chatID := target.ChatID
	if target.Source != nil && strings.TrimSpace(target.Source.ChatID) != "" {
		chatID = strings.TrimSpace(target.Source.ChatID)
	}
	switch params.Update.SessionUpdate {
	case acp.SessionUpdateAgentMessageChunk:
		var content acp.ContentBlock
		if len(params.Update.Content) > 0 && json.Unmarshal(params.Update.Content, &content) == nil {
			key := strings.TrimSpace(target.SessionID)
			if key == "" {
				key = chatID
			}
			r.textBuffers[key] += content.Text
		}
	case acp.SessionUpdateConfigOptionUpdate:
		r.Messages = append(r.Messages, formatConfigOptionUpdateMessage(mustJSON(params.Update)))
		r.ChatIDs = append(r.ChatIDs, chatID)
	default:
		if strings.HasPrefix(params.Update.SessionUpdate, acp.SessionUpdateToolCall) {
			r.CardCount++
		}
	}
	return nil
}

func (r *TestCaptureRouter) PublishPromptResult(_ context.Context, target im.SendTarget, _ acp.SessionPromptResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	chatID := target.ChatID
	if target.Source != nil && strings.TrimSpace(target.Source.ChatID) != "" {
		chatID = strings.TrimSpace(target.Source.ChatID)
	}
	key := strings.TrimSpace(target.SessionID)
	if key == "" {
		key = chatID
	}
	if text := r.textBuffers[key]; text != "" {
		r.Messages = append(r.Messages, text)
		r.ChatIDs = append(r.ChatIDs, chatID)
		delete(r.textBuffers, key)
	}
	return nil
}
func (r *TestCaptureRouter) SystemNotify(_ context.Context, target im.SendTarget, payload im.SystemPayload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	chatID := target.ChatID
	if target.Source != nil && strings.TrimSpace(target.Source.ChatID) != "" {
		chatID = strings.TrimSpace(target.Source.ChatID)
	}
	r.Messages = append(r.Messages, payload.Body)
	r.ChatIDs = append(r.ChatIDs, chatID)
	return nil
}

func (r *TestCaptureRouter) Run(context.Context) error { return nil }

func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

func mustNewSession(t *testing.T, id, cwd, agentType string) *Session {
	t.Helper()
	sess, err := newSession(id, cwd, agentType)
	if err != nil {
		t.Fatalf("newSession(%q): %v", id, err)
	}
	return sess
}

func mustNewWiredSession(t *testing.T, c *Client, id, agentType string) *Session {
	t.Helper()
	sess, err := c.newWiredSession(id, agentType)
	if err != nil {
		t.Fatalf("newWiredSession(%q): %v", id, err)
	}
	return sess
}

func (i *testInjectedInstance) Name() string { return i.name }
func (i *testInjectedInstance) Alive() bool {
	return i.alive
}
func (i *testInjectedInstance) SetCallbacks(callbacks agent.Callbacks) {
	i.callbacks = callbacks
}
func (i *testInjectedInstance) HandleACPRequest(context.Context, int64, string, json.RawMessage) (any, error) {
	return nil, errors.New("not implemented in test injected instance")
}
func (i *testInjectedInstance) HandleACPResponse(context.Context, string, json.RawMessage) {}
func (i *testInjectedInstance) Initialize(context.Context, acp.InitializeParams) (acp.InitializeResult, error) {
	if i.initResult.ProtocolVersion != "" || i.initResult.AgentInfo != nil || i.initResult.AgentCapabilities.LoadSession {
		return i.initResult, nil
	}
	return acp.InitializeResult{
		ProtocolVersion: "0.1",
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: false,
		},
		AgentInfo: &acp.AgentInfo{Name: "test-injected-agent"},
	}, nil
}
func (i *testInjectedInstance) SessionNew(context.Context, acp.SessionNewParams) (acp.SessionNewResult, error) {
	if i.newResult != nil {
		return *i.newResult, nil
	}
	sid := strings.TrimSpace(i.sessionID)
	if sid == "" {
		sid = "sess-1"
	}
	return acp.SessionNewResult{SessionID: sid}, nil
}
func (i *testInjectedInstance) SessionLoad(context.Context, acp.SessionLoadParams) (acp.SessionLoadResult, error) {
	for _, params := range i.loadUpdates {
		if strings.TrimSpace(params.SessionID) == "" {
			params.SessionID = i.sessionID
		}
		if i.callbacks != nil {
			i.callbacks.SessionUpdate(params)
		}
	}
	return i.loadResult, i.loadErr
}
func (i *testInjectedInstance) SessionList(context.Context, acp.SessionListParams) (acp.SessionListResult, error) {
	return i.listResult, i.listErr
}
func (i *testInjectedInstance) SessionPrompt(ctx context.Context, p acp.SessionPromptParams) (acp.SessionPromptResult, error) {
	i.lastPrompt = append([]acp.ContentBlock(nil), p.Prompt...)
	if i.promptFn == nil {
		return acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn}, nil
	}
	text := ""
	for _, b := range p.Prompt {
		if b.Type == acp.ContentBlockTypeText {
			text = b.Text
			break
		}
	}
	updates, result, err := i.promptFn(ctx, text)
	if err != nil {
		return acp.SessionPromptResult{}, err
	}
	for params := range updates {
		if strings.TrimSpace(params.SessionID) == "" {
			params.SessionID = p.SessionID
		}
		if i.callbacks != nil {
			i.callbacks.SessionUpdate(params)
		}
	}
	if strings.TrimSpace(result.StopReason) == "" {
		result.StopReason = acp.StopReasonEndTurn
	}
	return result, nil
}
func (i *testInjectedInstance) SessionCancel(_ string) error {
	if i.cancelFn != nil {
		return i.cancelFn()
	}
	return nil
}
func (i *testInjectedInstance) SessionSetConfigOption(_ context.Context, p acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
	i.setCalls = append(i.setCalls, p)
	if i.setConfigFn != nil {
		return i.setConfigFn(context.Background(), p)
	}
	return []acp.ConfigOption{{ID: p.ConfigID, CurrentValue: p.Value}}, nil
}
func (i *testInjectedInstance) ListSkills(context.Context, string) ([]agent.SkillDescriptor, error) {
	if i.skillsErr != nil {
		return nil, i.skillsErr
	}
	return append([]agent.SkillDescriptor(nil), i.skills...), nil
}

func (i *testInjectedInstance) Close() error { return nil }

var _ agent.Instance = (*testInjectedInstance)(nil)
var _ IMRouter = (*TestCaptureRouter)(nil)

type noopStore struct{}

func (s *noopStore) LoadRouteBindings(context.Context, string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (s *noopStore) SaveRouteBinding(context.Context, string, string, string) error { return nil }
func (s *noopStore) DeleteRouteBinding(context.Context, string, string) error       { return nil }
func (s *noopStore) LoadProjectDefaultAgent(context.Context, string) (string, error) {
	return "", nil
}
func (s *noopStore) SaveProjectDefaultAgent(context.Context, string, string) error { return nil }
func (s *noopStore) LoadSession(context.Context, string, string) (*SessionRecord, error) {
	return nil, nil
}
func (s *noopStore) SaveSession(context.Context, *SessionRecord) error { return nil }
func (s *noopStore) ListSessions(context.Context, string) ([]SessionRecord, error) {
	return nil, nil
}
func (s *noopStore) LoadAgentPreference(context.Context, string, string) (*AgentPreferenceRecord, error) {
	return nil, nil
}
func (s *noopStore) SaveAgentPreference(context.Context, AgentPreferenceRecord) error { return nil }
func (s *noopStore) DeleteSession(context.Context, string, string) error              { return nil }
func (s *noopStore) Close() error                                                     { return nil }

func TestIsAgentExitError(t *testing.T) {
	cases := []string{
		"acp rpc error -1: agent process exited",
		"io: broken pipe",
		"read tcp ... connection reset by peer",
		"EOF",
	}
	for _, c := range cases {
		if !isAgentExitError(errors.New(c)) {
			t.Fatalf("expected agent-exit match for %q", c)
		}
	}
}

func TestIsAgentExitError_TLSHandshakeEOFFalse(t *testing.T) {
	err := errors.New("failed to connect to websocket: IO error: tls handshake eof")
	if isAgentExitError(err) {
		t.Fatal("tls handshake eof should not be treated as local agent process exit")
	}
}

func TestResolveConfigArg_ValidatesOptionValue(t *testing.T) {
	st := &SessionAgentState{
		ConfigOptions: []acp.ConfigOption{{
			ID: "theme",
			Options: []acp.ConfigOptionValue{
				{Name: "Dark", Value: "dark"},
				{Name: "Light", Value: "light"},
			},
		}},
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

func TestResolveConfigArg_ResolvesCategoryAlias(t *testing.T) {
	st := &SessionAgentState{
		ConfigOptions: []acp.ConfigOption{{
			ID:       acp.ConfigOptionIDReasoningEffort,
			Category: acp.ConfigOptionCategoryThoughtLv,
			Options: []acp.ConfigOptionValue{
				{Name: "High", Value: "high"},
				{Name: "Medium", Value: "medium"},
			},
		}},
	}
	id, value, err := resolveConfigArg("thought_level High", st)
	if err != nil {
		t.Fatalf("resolveConfigArg returned error: %v", err)
	}
	if id != acp.ConfigOptionIDReasoningEffort || value != "high" {
		t.Fatalf("resolveConfigArg = (%q,%q), want (%q,%q)", id, value, acp.ConfigOptionIDReasoningEffort, "high")
	}
}

func TestSessionInfoLine_UsesPrimaryAgentStateWithoutLegacyAgentMap(t *testing.T) {
	s := mustNewSession(t, "sess-1", "/tmp", "claude")
	s.mu.Lock()
	s.agentState.ConfigOptions = []acp.ConfigOption{
		{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
		{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
	}
	s.mu.Unlock()

	got := s.sessionInfoLine()
	if !strings.Contains(got, "agent: claude") {
		t.Fatalf("sessionInfoLine() = %q, want agent name", got)
	}
	if !strings.Contains(got, "mode: code") {
		t.Fatalf("sessionInfoLine() = %q, want mode from primary agent state", got)
	}
	if !strings.Contains(got, "model: gpt-5") {
		t.Fatalf("sessionInfoLine() = %q, want model from primary agent state", got)
	}
}

func TestHandleIMInbound_ListDirectDoesNotBind(t *testing.T) {
	c := New(&noopStore{}, "test", "/tmp")
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
	if len(fake.systems) != 1 {
		t.Fatalf("systems=%+v, want direct /list response", fake.systems)
	}
}

func TestHandleIMInbound_NewWithoutAgentOpensHelpCardAndDoesNotBind(t *testing.T) {
	c := New(&noopStore{}, "test", "/tmp")
	fake := &fakeIMRouter{}
	c.SetIMRouter(fake)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := c.HandleIMInbound(context.Background(), im.InboundEvent{ChannelID: "feishu", ChatID: "chat-a", Text: "/new"}); err != nil {
		t.Fatalf("HandleIMInbound: %v", err)
	}

	if len(fake.binds) != 0 {
		t.Fatalf("binds=%+v, want none", fake.binds)
	}
	if got := c.RouteSessionIDForTest("im:feishu:chat-a"); got != "" {
		t.Fatalf("route session = %q, want empty", got)
	}
	if len(fake.systems) != 1 {
		t.Fatalf("systems=%+v, want one help card", fake.systems)
	}
	system := fake.systems[0]
	if system.payload.Kind != "help_card" || system.payload.HelpCard == nil {
		t.Fatalf("system payload = %+v, want help card", system.payload)
	}
	if system.payload.HelpCard.MenuID != "menu:new" {
		t.Fatalf("help menu id = %q, want menu:new", system.payload.HelpCard.MenuID)
	}
}

func TestHandleIMInbound_UnboundPromptBindsAndEmitsACP(t *testing.T) {
	c := New(&noopStore{}, "test", "/tmp")
	fake := &fakeIMRouter{}
	c.SetIMRouter(fake)
	factory := func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{
			name:      "claude",
			sessionID: "acp-1",
			promptFn: func(context.Context, string) (<-chan acp.SessionUpdateParams, acp.SessionPromptResult, error) {
				ch := make(chan acp.SessionUpdateParams, 1)
				content, _ := json.Marshal(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello back"})
				ch <- acp.SessionUpdateParams{Update: acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: content}}
				close(ch)
				return ch, acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn}, nil
			},
		}, nil
	}
	c.InjectAgentFactory(acp.ACPProviderClaude, factory)
	c.InjectAgentFactory(acp.ACPProviderCodex, factory)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.HandleIMInbound(context.Background(), im.InboundEvent{ChannelID: "feishu", ChatID: "chat-a", Text: "hello"}); err != nil {
		t.Fatalf("HandleIMInbound: %v", err)
	}
	if len(fake.binds) != 1 {
		t.Fatalf("binds=%+v, want one bind", fake.binds)
	}
	foundACP := false
	for _, update := range fake.updates {
		if update.params.Update.SessionUpdate == acp.SessionUpdateAgentMessageChunk {
			foundACP = true
			break
		}
	}
	if !foundACP {
		t.Fatalf("updates=%+v, want ACP session/update emission", fake.updates)
	}
}

func TestResolveOrCreateIMSessionUsesStoredDefaultAndFallbackDoesNotRewrite(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.SaveProjectDefaultAgent(context.Background(), "proj1", "claude"); err != nil {
		t.Fatalf("SaveProjectDefaultAgent: %v", err)
	}

	c := New(store, "proj1", "/tmp")
	c.registry = &agent.ACPFactory{}
	codexInst := &testInjectedInstance{name: "codex", initResult: acp.InitializeResult{ProtocolVersion: "0.1"}, newResult: &acp.SessionNewResult{SessionID: "sess-codex"}}
	c.registry.Register(acp.ACPProviderCodex, func(context.Context, string) (agent.Instance, error) { return codexInst, nil })

	sess := c.resolveOrCreateIMSession(context.Background(), im.ChatRef{ChannelID: "feishu", ChatID: "chat-a"}, "im:feishu:chat-a")
	if sess == nil {
		t.Fatal("resolveOrCreateIMSession = nil")
	}
	if sess.agentType != "codex" {
		t.Fatalf("agentType = %q, want codex fallback", sess.agentType)
	}
	still, err := store.LoadProjectDefaultAgent(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if still != "claude" {
		t.Fatalf("stored default rewritten to %q, want claude", still)
	}
}

func TestHandleIMInbound_ViewSinkFailureDoesNotBlockIMUpdates(t *testing.T) {
	c := New(&noopStore{}, "test", "/tmp")
	fake := &fakeIMRouter{}
	c.SetIMRouter(fake)
	c.SetSessionViewSink(&failingSessionViewSink{})
	factory := func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{
			name:      "claude",
			sessionID: "acp-1",
			promptFn: func(context.Context, string) (<-chan acp.SessionUpdateParams, acp.SessionPromptResult, error) {
				ch := make(chan acp.SessionUpdateParams, 1)
				content, _ := json.Marshal(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello back"})
				ch <- acp.SessionUpdateParams{Update: acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: content}}
				close(ch)
				return ch, acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn}, nil
			},
		}, nil
	}
	c.InjectAgentFactory(acp.ACPProviderClaude, factory)
	c.InjectAgentFactory(acp.ACPProviderCodex, factory)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.HandleIMInbound(context.Background(), im.InboundEvent{ChannelID: "feishu", ChatID: "chat-a", Text: "hello"}); err != nil {
		t.Fatalf("HandleIMInbound: %v", err)
	}

	foundACP := false
	for _, update := range fake.updates {
		if update.params.Update.SessionUpdate == acp.SessionUpdateAgentMessageChunk {
			foundACP = true
			break
		}
	}
	if !foundACP {
		t.Fatalf("updates=%+v, want ACP session/update emission even when view sink fails", fake.updates)
	}
}

type failingPermissionIMRouter struct{}

func (f *failingPermissionIMRouter) Bind(context.Context, im.ChatRef, string, im.BindOptions) error {
	return nil
}
func (f *failingPermissionIMRouter) PublishSessionUpdate(context.Context, im.SendTarget, acp.SessionUpdateParams) error {
	return nil
}
func (f *failingPermissionIMRouter) PublishPromptResult(context.Context, im.SendTarget, acp.SessionPromptResult) error {
	return nil
}
func (f *failingPermissionIMRouter) SystemNotify(context.Context, im.SendTarget, im.SystemPayload) error {
	return nil
}
func (f *failingPermissionIMRouter) Run(context.Context) error { return nil }

type failingSessionViewSink struct{}

func (f *failingSessionViewSink) RecordEvent(context.Context, SessionViewEvent) error {
	return errors.New("session view sink failed")
}

type recordingSessionViewSink struct {
	events []SessionViewEvent
}

func (s *recordingSessionViewSink) RecordEvent(_ context.Context, event SessionViewEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestSessionRequestPermissionAutoAllowsWithoutIMRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := logger.Setup(logger.LoggerConfig{Level: logger.LevelWarn}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer logger.Close()
	logger.SetOutput(&buf)
	defer logger.SetOutput(os.Stderr)

	s := mustNewSession(t, "sess-1", "/tmp", "claude")
	s.imRouter = &failingPermissionIMRouter{}
	s.setIMSource(im.ChatRef{ChannelID: "app", ChatID: "chat-1"})
	result, err := s.SessionRequestPermission(context.Background(), 1, acp.PermissionRequestParams{
		Options: []acp.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: "allow_once"}},
	})
	if err != nil {
		t.Fatalf("SessionRequestPermission: %v", err)
	}
	if result.Outcome != "selected" || result.OptionID != "allow" {
		t.Fatalf("permission result = %+v, want selected allow", result)
	}
	if got := buf.String(); strings.Contains(got, "permission publish failed") {
		t.Fatalf("unexpected permission publish failure log: %q", got)
	}
}

func TestSessionRequestPermissionRecognizesLegacyOnceKind(t *testing.T) {
	s := mustNewSession(t, "sess-1", "/tmp", "claude")
	result, err := s.SessionRequestPermission(context.Background(), 1, acp.PermissionRequestParams{
		Options: []acp.PermissionOption{{
			OptionID: "allow",
			Name:     "Allow",
			Kind:     "once",
		}},
	})
	if err != nil {
		t.Fatalf("SessionRequestPermission: %v", err)
	}
	if result.Outcome != "selected" || result.OptionID != "allow" {
		t.Fatalf("permission result = %+v, want selected allow", result)
	}
}

func TestReplyWithTitleRecordsLegacySystemEvent(t *testing.T) {
	router := &fakeIMRouter{}
	sink := &recordingSessionViewSink{}
	s := mustNewSession(t, "sess-1", "/tmp", "claude")
	s.imRouter = router
	s.viewSink = sink
	s.setIMSource(im.ChatRef{ChannelID: "app", ChatID: "chat-1"})

	s.replyWithTitle("Switched", "session: sess-1")

	if len(sink.events) != 1 {
		t.Fatalf("session view events len = %d, want 1", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != SessionViewEventTypeSystem {
		t.Fatalf("event.Type = %q, want %q", event.Type, SessionViewEventTypeSystem)
	}
	if event.SessionID != "sess-1" {
		t.Fatalf("event.SessionID = %q, want %q", event.SessionID, "sess-1")
	}
	if strings.TrimSpace(event.Content) != "Switched\nsession: sess-1" {
		t.Fatalf("event.Content = %q, want %q", event.Content, "Switched\nsession: sess-1")
	}
	if strings.TrimSpace(event.SourceChannel) != "app" || strings.TrimSpace(event.SourceChatID) != "chat-1" {
		t.Fatalf("event source = (%q, %q), want (%q, %q)", event.SourceChannel, event.SourceChatID, "app", "chat-1")
	}

	if len(router.systems) != 1 {
		t.Fatalf("router system notifications len = %d, want 1", len(router.systems))
	}
	if strings.TrimSpace(router.systems[0].payload.Title) != "Switched" {
		t.Fatalf("system payload title = %q, want %q", router.systems[0].payload.Title, "Switched")
	}
	if strings.TrimSpace(router.systems[0].payload.Body) != "session: sess-1" {
		t.Fatalf("system payload body = %q, want %q", router.systems[0].payload.Body, "session: sess-1")
	}
}

func TestReportTimeoutError_RecordsSystemEvent(t *testing.T) {
	router := &fakeIMRouter{}
	sink := &recordingSessionViewSink{}
	s := mustNewSession(t, "sess-1", "/tmp", "claude")
	s.imRouter = router
	s.viewSink = sink
	s.setIMSource(im.ChatRef{ChannelID: "app", ChatID: "chat-1"})

	s.reportTimeoutError("stream", "silence")

	if len(sink.events) != 1 {
		t.Fatalf("session view events len = %d, want 1", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != SessionViewEventTypeSystem {
		t.Fatalf("event.Type = %q, want %q", event.Type, SessionViewEventTypeSystem)
	}
	if !strings.Contains(event.Content, "category=timeout stage=stream") {
		t.Fatalf("event.Content = %q, want timeout payload", event.Content)
	}

	if len(router.systems) != 1 {
		t.Fatalf("router system notifications len = %d, want 1", len(router.systems))
	}
	if got := strings.TrimSpace(router.systems[0].payload.Body); !strings.Contains(got, "category=timeout stage=stream") {
		t.Fatalf("system payload body = %q, want timeout payload", got)
	}
}

func TestCurrentAgentNameLocked_PrefersSessionAgentType(t *testing.T) {
	s := mustNewSession(t, "sess-1", "/tmp", "claude")
	s.mu.Lock()
	s.instance = &testInjectedInstance{name: "codex"}
	got := s.agentType
	s.mu.Unlock()

	if got != "claude" {
		t.Fatalf("agentType = %q, want %q", got, "claude")
	}
}

func TestClientLoadSession_RestoresFromStore(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(store, "proj1", "/tmp")
	ctx := context.Background()
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:           "restore-me",
		ProjectName:  "proj1",
		AgentType:    "claude",
		AgentJSON:    `{"title":"Persisted"}`,
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActiveAt: time.Now().Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := store.SaveRouteBinding(ctx, "proj1", "route-1", "restore-me"); err != nil {
		t.Fatalf("save route binding: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	sess, err := c.resolveSession("route-1")
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	if sess.acpSessionID != "restore-me" {
		t.Fatalf("resolved session ID = %q, want restore-me", sess.acpSessionID)
	}

}

func TestListSessions_DiskOnlySessionsAreMarkedPersisted(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	createdAt := time.Now().Add(-2 * time.Hour).UTC()
	lastMessageAt := time.Now().Add(-5 * time.Minute).UTC()
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:          "persisted-only",
		ProjectName: "proj1",
		Status:      SessionSuspended,
		AgentType:   "claude",
		AgentJSON:   `{"title":"Persisted Title"}`,

		CreatedAt:    createdAt,
		LastActiveAt: lastMessageAt,
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	c := New(store, "proj1", "/tmp")
	entries, err := c.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Status != SessionPersisted {
		t.Fatalf("entries[0].Status = %v, want %v", entries[0].Status, SessionPersisted)
	}
	if entries[0].InMemory {
		t.Fatal("entries[0].InMemory = true, want false")
	}
}

func TestListSessions_InMemorySessionKeepsStoredProjectionMetadata(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	createdAt := time.Now().Add(-2 * time.Hour).UTC()
	lastMessageAt := time.Now().Add(-3 * time.Minute).UTC()
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:          "sess-1",
		ProjectName: "proj1",
		Status:      SessionSuspended,
		AgentType:   "claude",
		AgentJSON:   `{"title":"Persisted Title"}`,
		Title:       "Persisted Title",

		CreatedAt:    createdAt,
		LastActiveAt: lastMessageAt,
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	c := New(store, "proj1", "/tmp")
	c.mu.Lock()
	sess := mustNewWiredSession(t, c, "sess-1", "claude")
	sess.createdAt = createdAt
	sess.lastActiveAt = time.Now().UTC()
	sess.Status = SessionActive
	sess.agentState.Title = "Runtime Title"
	c.sessions[sess.acpSessionID] = sess
	c.mu.Unlock()

	entries, err := c.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Title != "Persisted Title" {
		t.Fatalf("entries[0].Title = %q, want %q", entries[0].Title, "Persisted Title")
	}
	if !entries[0].LastActiveAt.Equal(lastMessageAt) {
		t.Fatalf("entries[0].LastActiveAt = %q, want %q", entries[0].LastActiveAt.Format(time.RFC3339), lastMessageAt.Format(time.RFC3339))
	}
	if entries[0].Status != SessionActive {
		t.Fatalf("entries[0].Status = %v, want %v", entries[0].Status, SessionActive)
	}
	if !entries[0].InMemory {
		t.Fatal("entries[0].InMemory = false, want true")
	}

}

func TestEnsureReady_SessionLoadKeepsPersistedConfigWhenLoadResultEmpty(t *testing.T) {
	s := mustNewSession(t, "acp-keep", "/tmp", "claude")
	s.projectName = "proj1"
	s.agentState = SessionAgentState{
		ConfigOptions: []acp.ConfigOption{
			{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
			{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
		},
	}
	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{
			name:      "claude",
			sessionID: "acp-keep",
			initResult: acp.InitializeResult{
				ProtocolVersion: "0.1",
				AgentCapabilities: acp.AgentCapabilities{
					LoadSession: true,
				},
				AgentInfo: &acp.AgentInfo{Name: "test-injected-agent"},
			},
			loadResult: acp.SessionLoadResult{},
		}, nil
	})

	if err := s.ensureInstance(context.Background()); err != nil {
		t.Fatalf("ensureInstance: %v", err)
	}
	if err := s.ensureReady(context.Background()); err != nil {
		t.Fatalf("ensureReady: %v", err)
	}

	s.mu.Lock()
	opts := append([]acp.ConfigOption(nil), s.agentState.ConfigOptions...)
	s.mu.Unlock()
	if got := findCurrentValue(opts, acp.ConfigOptionIDMode); got != "code" {
		t.Fatalf("mode = %q, want %q", got, "code")
	}
	if got := findCurrentValue(opts, acp.ConfigOptionIDModel); got != "gpt-5" {
		t.Fatalf("model = %q, want %q", got, "gpt-5")
	}
}

func TestEnsureReady_SessionLoadFailure_ReturnsErrorWithoutAllocatingNewSessionID(t *testing.T) {
	s := mustNewSession(t, "acp-old", "/tmp", "claude")
	s.projectName = "proj1"
	s.agentState = SessionAgentState{
		ConfigOptions: []acp.ConfigOption{
			{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
			{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
		},
	}

	inst := &testInjectedInstance{
		name:      "claude",
		sessionID: "acp-new",
		initResult: acp.InitializeResult{
			ProtocolVersion: "0.1",
			AgentCapabilities: acp.AgentCapabilities{
				LoadSession: true,
			},
			AgentInfo: &acp.AgentInfo{Name: "test-injected-agent"},
		},
		loadErr: errors.New("session not found"),
	}

	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return inst, nil
	})

	if err := s.ensureInstance(context.Background()); err != nil {
		t.Fatalf("ensureInstance: %v", err)
	}
	err := s.ensureReady(context.Background())
	if err == nil {
		t.Fatal("ensureReady error = nil, want error")
	}
	if s.acpSessionID != "acp-old" {
		t.Fatalf("session ID = %q, want acp-old", s.acpSessionID)
	}
}

func TestEnsureReady_FailsWhenAgentDoesNotSupportLoadSession(t *testing.T) {
	s := mustNewSession(t, "acp-existing", "/tmp", "claude")
	s.projectName = "proj1"
	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{
			name:      "claude",
			sessionID: "acp-existing",
			initResult: acp.InitializeResult{
				ProtocolVersion:   "0.1",
				AgentCapabilities: acp.AgentCapabilities{LoadSession: false},
			},
		}, nil
	})

	if err := s.ensureInstance(context.Background()); err != nil {
		t.Fatalf("ensureInstance: %v", err)
	}
	err := s.ensureReady(context.Background())
	if err == nil {
		t.Fatal("ensureReady error = nil, want error")
	}
	if !strings.Contains(err.Error(), "does not support session/load") {
		t.Fatalf("ensureReady error = %v, want unsupported load-session error", err)
	}
}

func TestEnsureReadyAndNotify_DoesNotEmitReadySystemPrompt(t *testing.T) {
	s := mustNewSession(t, "acp-1", "/tmp", "claude")
	s.projectName = "proj1"
	router := &fakeIMRouter{}
	s.imRouter = router
	s.imSource = &im.ChatRef{ChannelID: "feishu", ChatID: "chat-1"}
	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{
			name:      "claude",
			sessionID: "acp-1",
			initResult: acp.InitializeResult{
				ProtocolVersion:   "0.1",
				AgentCapabilities: acp.AgentCapabilities{LoadSession: true},
			},
			loadResult: acp.SessionLoadResult{},
		}, nil
	})

	if err := s.ensureInstance(context.Background()); err != nil {
		t.Fatalf("ensureInstance: %v", err)
	}
	if err := s.ensureReadyAndNotify(context.Background()); err != nil {
		t.Fatalf("ensureReadyAndNotify(first): %v", err)
	}
	if got := len(router.systems); got != 0 {
		t.Fatalf("system notify count after first ensureReadyAndNotify = %d, want 0", got)
	}

	if err := s.ensureReadyAndNotify(context.Background()); err != nil {
		t.Fatalf("ensureReadyAndNotify(second): %v", err)
	}
	if got := len(router.systems); got != 0 {
		t.Fatalf("system notify count after second ensureReadyAndNotify = %d, want 0", got)
	}
}

func TestEvictSuspendedSessions(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(store, "proj1", "/tmp")
	c.suspendTimeout = 0

	c.mu.Lock()
	sess := mustNewWiredSession(t, c, "evict-me", "claude")
	sess.Status = SessionSuspended
	sess.lastActiveAt = time.Now().Add(-time.Minute)
	c.sessions["evict-me"] = sess
	c.mu.Unlock()

	c.evictSuspendedSessions()

	if c.HasSessionInMemoryForTest("evict-me") {
		t.Fatal("evicted session should not remain in memory")
	}
	rec, err := store.LoadSession(context.Background(), "proj1", "evict-me")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if rec == nil {
		t.Fatal("evicted session not persisted")
	}
}

func TestSessionUpdate_NoPromptContext_DoesNotBlockWhenChannelFull(t *testing.T) {
	s := mustNewSession(t, "sess", "/tmp", "claude")
	content, _ := json.Marshal(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "chunk"})

	ch := make(chan acp.SessionUpdateParams, 1)
	ch <- acp.SessionUpdateParams{SessionID: "acp-1", Update: acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk}}

	s.mu.Lock()
	s.acpSessionID = "acp-1"
	s.prompt.updatesCh = ch
	s.prompt.ctx = nil
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.SessionUpdate(acp.SessionUpdateParams{
			SessionID: "acp-1",
			Update: acp.SessionUpdate{
				SessionUpdate: acp.SessionUpdateAgentMessageChunk,
				Content:       content,
			},
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("SessionUpdate blocked when prompt channel was full and prompt context was nil")
	}
}

func TestPromptObserve_FirstWaitTransitions(t *testing.T) {
	st := newPromptObserveState(time.Unix(0, 0))
	e := st.Eval(time.Unix(60, 0), false)
	if !e.WarnFirstWait || e.ErrorFirstWait {
		t.Fatalf("unexpected first wait events: %+v", e)
	}
	e = st.Eval(time.Unix(180, 0), false)
	if !e.ErrorFirstWait {
		t.Fatalf("expected first wait error at 180s: %+v", e)
	}
}

func TestPromptObserve_SilenceTransitions(t *testing.T) {
	st := newPromptObserveState(time.Unix(0, 0))
	st.MarkActivity(time.Unix(5, 0), true)
	e := st.Eval(time.Unix(65, 0), true)
	if !e.WarnSilence || e.ErrorSilence {
		t.Fatalf("unexpected silence events: %+v", e)
	}
	e = st.Eval(time.Unix(185, 0), true)
	if !e.ErrorSilence {
		t.Fatalf("expected silence error at 180s: %+v", e)
	}
}

func TestTimeoutNotifyLimiter_Cooldown(t *testing.T) {
	n := newTimeoutNotifyLimiter(60 * time.Second)
	now := time.Unix(100, 0)
	if !n.Allow("sess-1:first-wait", now) {
		t.Fatal("first report should be allowed")
	}
	if n.Allow("sess-1:first-wait", now.Add(30*time.Second)) {
		t.Fatal("report inside cooldown should be blocked")
	}
	if !n.Allow("sess-1:first-wait", now.Add(61*time.Second)) {
		t.Fatal("report after cooldown should be allowed")
	}
}

func findCurrentValue(options []acp.ConfigOption, id string) string {
	for _, opt := range options {
		if strings.EqualFold(opt.ID, id) {
			return strings.TrimSpace(opt.CurrentValue)
		}
	}
	return ""
}

func TestSessionFromRecord_RestoresSingleAgentState(t *testing.T) {
	rec := &SessionRecord{
		ID:          "sess-restored",
		ProjectName: "proj1",
		Status:      SessionPersisted,
		AgentType:   "claude",
		AgentJSON:   `{"title":"Persisted","commands":[{"name":"/status"}]}`,
		Title:       "Persisted",
	}

	sess, err := sessionFromRecord(rec, "/tmp")
	if err != nil {
		t.Fatalf("sessionFromRecord: %v", err)
	}
	if sess.acpSessionID != "sess-restored" {
		t.Fatalf("ID = %q, want sess-restored", sess.acpSessionID)
	}
	if got := sess.agentType; got != "claude" {
		t.Fatalf("agentType = %q, want claude", got)
	}
	if got := sess.agentState.Title; got != "Persisted" {
		t.Fatalf("agentState.Title = %q, want Persisted", got)
	}
}

func TestCreateSessionWithAgent_UsesACPResultAsUnifiedSessionID(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	inst := &testInjectedInstance{
		name: "claude",
		initResult: acp.InitializeResult{
			ProtocolVersion:   "0.1",
			AgentCapabilities: acp.AgentCapabilities{},
		},
		newResult: &acp.SessionNewResult{SessionID: "sess-from-agent"},
	}

	c := New(store, "proj1", "/tmp")
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register("claude", func(context.Context, string) (agent.Instance, error) { return inst, nil })

	sess, err := c.CreateSession(context.Background(), "claude", "hello")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.acpSessionID != "sess-from-agent" {
		t.Fatalf("session ID = %q, want sess-from-agent", sess.acpSessionID)
	}
	if got := sess.agentType; got != "claude" {
		t.Fatalf("agentType = %q, want claude", got)
	}

	loaded, err := store.LoadSession(context.Background(), "proj1", "sess-from-agent")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil || loaded.AgentType != "claude" {
		t.Fatalf("LoadSession = %+v, want agentType claude", loaded)
	}
}

func TestCreateSessionWithAgent_FailsWhenACPReturnsEmptySessionID(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	inst := &testInjectedInstance{
		name: "claude",
		initResult: acp.InitializeResult{
			ProtocolVersion:   "0.1",
			AgentCapabilities: acp.AgentCapabilities{},
		},
		newResult: &acp.SessionNewResult{SessionID: ""},
	}

	c := New(store, "proj1", "/tmp")
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register("claude", func(context.Context, string) (agent.Instance, error) { return inst, nil })

	sess, err := c.CreateSession(context.Background(), "claude", "hello")
	if err == nil {
		t.Fatalf("CreateSession error = nil, want error")
	}
	if sess != nil {
		t.Fatalf("CreateSession session = %#v, want nil", sess)
	}

	entries, err := store.ListSessions(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("ListSessions count = %d, want 0", len(entries))
	}
}

func TestNewSession_RequiresNonEmptyACPID(t *testing.T) {
	sess, err := newSession("   ", "/tmp", "claude")
	if err == nil {
		t.Fatalf("newSession error = nil, want error")
	}
	if sess != nil {
		t.Fatalf("newSession session = %#v, want nil", sess)
	}
}

func TestClientNewSession_ReappliesProjectAgentBaseline(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.SaveAgentPreference(context.Background(), AgentPreferenceRecord{
		ProjectName: "proj1",
		AgentType:   "claude",
		PreferenceJSON: string(mustJSON(PreferenceState{ConfigOptions: []PreferenceConfigOption{
			{ID: acp.ConfigOptionIDMode, CurrentValue: "code"},
			{ID: acp.ConfigOptionIDModel, CurrentValue: "gpt-5"},
			{ID: acp.ConfigOptionIDThoughtLevel, CurrentValue: "high"},
		}})),
	}); err != nil {
		t.Fatalf("SaveAgentPreference: %v", err)
	}

	c := New(store, "proj1", "/tmp")
	inst := &testInjectedInstance{
		name: "claude",
		initResult: acp.InitializeResult{
			ProtocolVersion:   "0.1",
			AgentCapabilities: acp.AgentCapabilities{},
		},
		newResult: &acp.SessionNewResult{SessionID: "acp-new", ConfigOptions: []acp.ConfigOption{
			{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "ask"},
			{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-4o-mini"},
			{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "low"},
		}},
	}
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) { return inst, nil })
	c.mu.Lock()
	oldSess := mustNewWiredSession(t, c, "sess-old", "claude")
	c.sessions[oldSess.acpSessionID] = oldSess
	c.routeMap["route-1"] = oldSess.acpSessionID
	c.mu.Unlock()

	sess, err := c.ClientNewSession("route-1", "claude")
	if err != nil {
		t.Fatalf("ClientNewSession: %v", err)
	}
	if err := sess.ensureInstance(context.Background()); err != nil {
		t.Fatalf("ensureInstance: %v", err)
	}
	if err := sess.ensureReady(context.Background()); err != nil {
		t.Fatalf("ensureReady: %v", err)
	}

	if got := len(inst.setCalls); got != 3 {
		t.Fatalf("set calls = %d, want 3", got)
	}

}

func TestClientNewSessionPersistsProjectDefaultAgent(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	c := New(store, "proj1", "/tmp")
	inst := &testInjectedInstance{name: "claude", initResult: acp.InitializeResult{ProtocolVersion: "0.1"}, newResult: &acp.SessionNewResult{SessionID: "sess-new"}}
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) { return inst, nil })

	if _, err := c.ClientNewSession("route-1", "claude"); err != nil {
		t.Fatalf("ClientNewSession: %v", err)
	}
	got, err := store.LoadProjectDefaultAgent(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if got != "claude" {
		t.Fatalf("default agent = %q, want claude", got)
	}
}

func TestEnsureReady_SessionLoadSuccess_ReplaysStoredConfigValuesByID(t *testing.T) {
	s := mustNewSession(t, "acp-old", "/tmp", "claude")
	s.projectName = "proj1"
	s.agentState = SessionAgentState{
		ConfigOptions: []acp.ConfigOption{
			{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
			{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
			{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
			{ID: "custom_toggle", CurrentValue: "persisted-custom"},
		},
	}

	inst := &testInjectedInstance{
		name:      "claude",
		sessionID: "acp-old",
		initResult: acp.InitializeResult{
			ProtocolVersion:   "0.1",
			AgentCapabilities: acp.AgentCapabilities{LoadSession: true},
		},
		loadResult: acp.SessionLoadResult{ConfigOptions: []acp.ConfigOption{
			{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "ask"},
			{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-4o-mini"},
			{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "low"},
			{ID: "custom_toggle", CurrentValue: "agent-custom"},
		}},
	}
	inst.setConfigFn = func(_ context.Context, p acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
		switch p.ConfigID {
		case acp.ConfigOptionIDMode:
			return []acp.ConfigOption{
				{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: p.Value},
				{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-4o-mini"},
				{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "low"},
				{ID: "custom_toggle", CurrentValue: "agent-custom"},
			}, nil
		case acp.ConfigOptionIDModel:
			return []acp.ConfigOption{
				{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
				{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: p.Value},
				{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "low"},
				{ID: "custom_toggle", CurrentValue: "agent-custom"},
			}, nil
		case acp.ConfigOptionIDThoughtLevel:
			return []acp.ConfigOption{
				{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
				{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
				{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: p.Value},
				{ID: "custom_toggle", CurrentValue: "agent-custom"},
			}, nil
		case "custom_toggle":
			return []acp.ConfigOption{
				{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
				{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
				{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
				{ID: "custom_toggle", CurrentValue: p.Value},
			}, nil
		default:
			return nil, errors.New("unexpected config id")
		}
	}

	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return inst, nil
	})

	if err := s.ensureInstance(context.Background()); err != nil {
		t.Fatalf("ensureInstance: %v", err)
	}
	if err := s.ensureReady(context.Background()); err != nil {
		t.Fatalf("ensureReady: %v", err)
	}

	s.mu.Lock()
	state := cloneSessionAgentState(&s.agentState)
	s.mu.Unlock()
	if state == nil {
		t.Fatal("currentAgentStateSnapshot returned nil state")
	}
	if findCurrentValue(state.ConfigOptions, "custom_toggle") != "persisted-custom" {
		t.Fatalf("custom_toggle should restore persisted value")
	}
	if got := len(inst.setCalls); got != 4 {
		t.Fatalf("set calls = %d, want 4", got)
	}
}
func TestCancelPrompt_DoesNotClearSessionConfig(t *testing.T) {
	s := mustNewSession(t, "cancel-keep-config", "/tmp", "claude")
	s.ready = true
	s.agentState = SessionAgentState{
		ConfigOptions: []acp.ConfigOption{
			{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
			{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
			{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
		},
	}
	s.instance = &testInjectedInstance{name: "claude"}

	if err := s.cancelPrompt(); err != nil {
		t.Fatalf("cancelPrompt: %v", err)
	}

	s.mu.Lock()
	opts := append([]acp.ConfigOption(nil), s.agentState.ConfigOptions...)
	s.mu.Unlock()
	if findCurrentValue(opts, acp.ConfigOptionIDMode) != "code" ||
		findCurrentValue(opts, acp.ConfigOptionIDModel) != "gpt-5" ||
		findCurrentValue(opts, acp.ConfigOptionIDThoughtLevel) != "high" {
		t.Fatalf("config after cancel = %+v", opts)
	}
}

func TestCancelPromptSendsSessionCancelBeforeCancellingPromptContext(t *testing.T) {
	s := mustNewSession(t, "cancel-order", "/tmp", "claude")
	s.ready = true

	var mu sync.Mutex
	order := []string{}
	record := func(label string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, label)
	}

	s.prompt.cancel = func() { record("context") }
	s.instance = &testInjectedInstance{
		name: "claude",
		cancelFn: func() error {
			record("session")
			return nil
		},
	}

	if err := s.cancelPrompt(); err != nil {
		t.Fatalf("cancelPrompt: %v", err)
	}
	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	want := []string{"session", "context"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cancel order=%v, want %v", got, want)
	}
}

func TestEnsureReady_SessionLoadSuccess_AgentCommandsOverrideCachedCommands(t *testing.T) {
	s := mustNewSession(t, "acp-1", "/tmp", "claude")
	s.projectName = "proj1"
	s.agentState = SessionAgentState{
		Commands: []acp.AvailableCommand{{Name: "/cached"}},
	}

	inst := &testInjectedInstance{
		name:      "claude",
		sessionID: "acp-1",
		initResult: acp.InitializeResult{
			ProtocolVersion:   "0.1",
			AgentCapabilities: acp.AgentCapabilities{LoadSession: true},
		},
		loadResult: acp.SessionLoadResult{ConfigOptions: []acp.ConfigOption{{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"}}},
	}

	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return inst, nil
	})

	if err := s.ensureInstance(context.Background()); err != nil {
		t.Fatalf("ensureInstance: %v", err)
	}

	s.SessionUpdate(acp.SessionUpdateParams{
		SessionID: "acp-1",
		Update: acp.SessionUpdate{
			SessionUpdate:     acp.SessionUpdateAvailableCommandsUpdate,
			AvailableCommands: []acp.AvailableCommand{{Name: "/agent"}},
		},
	})

	s.mu.Lock()
	state := cloneSessionAgentState(&s.agentState)
	s.mu.Unlock()
	if state == nil {
		t.Fatal("currentAgentStateSnapshot returned nil state")
	}
	if got := len(state.Commands); got != 1 {
		t.Fatalf("commands = %d, want 1", got)
	}
	if got := state.Commands[0].Name; got != "/agent" {
		t.Fatalf("command = %q, want /agent", got)
	}
}

func TestNormalizeIMPromptBlocks_PreservesImageAndText(t *testing.T) {
	blocks := normalizeIMPromptBlocks([]acp.ContentBlock{
		{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "aGVsbG8="},
		{Type: acp.ContentBlockTypeText, Text: "  hello  "},
	})
	if len(blocks) != 2 {
		t.Fatalf("blocks=%+v, want 2", blocks)
	}
	if blocks[0].Type != acp.ContentBlockTypeImage {
		t.Fatalf("first block type=%q, want image", blocks[0].Type)
	}
	if blocks[1].Type != acp.ContentBlockTypeText || blocks[1].Text != "hello" {
		t.Fatalf("second block=%+v", blocks[1])
	}
}

func TestNormalizeChatRef_TrimsFields(t *testing.T) {
	got := normalizeChatRef(im.ChatRef{ChannelID: " feishu ", ChatID: " chat-1 "})
	if got.ChannelID != "feishu" || got.ChatID != "chat-1" {
		t.Fatalf("normalizeChatRef() = %#v", got)
	}
}

func TestPromptToSession_TrimsSourceBeforeRouting(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	c := New(store, "proj1", "/tmp")
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{name: "claude", sessionID: "acp-route-test", alive: true}, nil
	})
	sess, err := c.ClientNewSession("route:test", "claude")
	if err != nil {
		t.Fatalf("ClientNewSession: %v", err)
	}

	err = c.PromptToSession(context.Background(), sess.acpSessionID, im.ChatRef{ChannelID: " feishu ", ChatID: " chat-1 "}, []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: "hello"}})
	if err != nil {
		t.Fatalf("PromptToSession: %v", err)
	}

	bindings, err := store.LoadRouteBindings(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadRouteBindings: %v", err)
	}
	if got := bindings["im:feishu:chat-1"]; got != sess.acpSessionID {
		t.Fatalf("route binding = %q, want %q", got, sess.acpSessionID)
	}
}

func TestResolveHelpModelRefreshesSessionMenuFromRuntimeList(t *testing.T) {
	s := mustNewSession(t, "sess-local", ".", "claude")
	inst := &testInjectedInstance{
		name:      string(acp.ACPProviderClaude),
		sessionID: "sess-current",
		alive:     true,
		listResult: acp.SessionListResult{
			Sessions: []acp.SessionInfo{
				{SessionID: "sess-older", Title: "Older Session"},
				{SessionID: "sess-current", Title: "Current Session"},
			},
		},
	}

	s.mu.Lock()
	s.instance = inst
	s.agentType = inst.name
	s.ready = true
	state := &s.agentState
	state.AgentCapabilities = acp.AgentCapabilities{
		LoadSession: true,
		SessionCapabilities: &acp.SessionCapabilities{
			List: &acp.SessionListCapability{},
		},
	}
	s.mu.Unlock()

	model, err := s.resolveHelpModel(context.Background(), "")
	if err != nil {
		t.Fatalf("resolveHelpModel() err = %v", err)
	}

	sessionMenu, ok := model.Menus["menu:sessions"]
	if !ok {
		t.Fatalf("session menu not found")
	}
	if len(sessionMenu.Options) != 1 {
		t.Fatalf("session menu options len = %d, want 1", len(sessionMenu.Options))
	}
	if sessionMenu.Options[0].Command != "/list" {
		t.Fatalf("session menu option[0] = %#v, want /list", sessionMenu.Options[0])
	}
	if !strings.Contains(sessionMenu.Body, "/list") {
		t.Fatalf("session menu body = %q, want usage hint about /list", sessionMenu.Body)
	}
}

func TestResolveHelpModel_RootStartsWithNewConversationMenu(t *testing.T) {
	s := mustNewSession(t, "sess-help", "/tmp", "claude")
	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{name: "claude", alive: true}, nil
	})
	s.registry.Register(acp.ACPProviderCodex, func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{name: "codex", alive: true}, nil
	})
	s.mu.Lock()
	s.instance = &testInjectedInstance{name: "claude", alive: true}
	s.ready = true
	s.mu.Unlock()

	model, err := s.resolveHelpModel(context.Background(), "")
	if err != nil {
		t.Fatalf("resolveHelpModel: %v", err)
	}
	if len(model.Options) == 0 || model.Options[0].Label != "New Conversation" {
		t.Fatalf("root option[0] = %+v, want New Conversation", model.Options)
	}
	newMenu, ok := model.Menus["menu:new"]
	if !ok {
		t.Fatalf("menus = %+v, want menu:new", model.Menus)
	}
	for _, opt := range model.Options {
		if strings.Contains(opt.Label, "Switch Agent") || opt.Command == "/use" {
			t.Fatalf("unexpected switch-agent option: %+v", opt)
		}
	}
	seenAgents := map[string]bool{}
	for _, opt := range newMenu.Options {
		if opt.Command != "/new" {
			t.Fatalf("new menu option = %+v, want /new", opt)
		}
		seenAgents[strings.TrimPrefix(opt.Label, "Agent: ")] = true
	}
	if !seenAgents["claude"] || !seenAgents["codex"] {
		t.Fatalf("new menu options = %+v, want claude and codex", newMenu.Options)
	}
}

func TestStoreAgentPreferenceRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	pref := PreferenceState{
		ConfigOptions: []PreferenceConfigOption{
			{ID: acp.ConfigOptionIDMode, CurrentValue: "code"},
			{ID: acp.ConfigOptionIDModel, CurrentValue: "gpt-5"},
			{ID: acp.ConfigOptionIDThoughtLevel, CurrentValue: "high"},
		},
		UpdatedAt: "2026-04-11T00:00:00Z",
	}
	raw, err := json.Marshal(pref)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := store.SaveAgentPreference(context.Background(), AgentPreferenceRecord{
		ProjectName:    "proj1",
		AgentType:      "codex",
		PreferenceJSON: string(raw),
	}); err != nil {
		t.Fatalf("SaveAgentPreference: %v", err)
	}

	loaded, err := store.LoadAgentPreference(context.Background(), "proj1", "codex")
	if err != nil {
		t.Fatalf("LoadAgentPreference: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadAgentPreference: nil, want preference")
	}
	var decoded PreferenceState
	if err := json.Unmarshal([]byte(loaded.PreferenceJSON), &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got := len(decoded.ConfigOptions); got != 3 {
		t.Fatalf("decoded config option count = %d, want 3", got)
	}
	if decoded.ConfigOptions[0].ID != acp.ConfigOptionIDMode || decoded.ConfigOptions[0].CurrentValue != "code" {
		t.Fatalf("decoded config[0] = %+v", decoded.ConfigOptions[0])
	}
	if decoded.ConfigOptions[1].ID != acp.ConfigOptionIDModel || decoded.ConfigOptions[1].CurrentValue != "gpt-5" {
		t.Fatalf("decoded config[1] = %+v", decoded.ConfigOptions[1])
	}
	if decoded.ConfigOptions[2].ID != acp.ConfigOptionIDThoughtLevel || decoded.ConfigOptions[2].CurrentValue != "high" {
		t.Fatalf("decoded config[2] = %+v", decoded.ConfigOptions[2])
	}
}

func TestStoreProjectDefaultAgentRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.SaveProjectDefaultAgent(context.Background(), "proj1", "claude"); err != nil {
		t.Fatalf("SaveProjectDefaultAgent: %v", err)
	}
	got, err := store.LoadProjectDefaultAgent(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if got != "claude" {
		t.Fatalf("default agent = %q, want claude", got)
	}
}

func TestApplyStoredConfigOptions_ReplaysByExactConfigID(t *testing.T) {
	inst := &testInjectedInstance{name: "agent"}
	inst.setConfigFn = func(_ context.Context, p acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
		return []acp.ConfigOption{
			{ID: p.ConfigID, CurrentValue: p.Value},
		}, nil
	}

	current := []acp.ConfigOption{
		{ID: "mode", CurrentValue: "ask"},
		{ID: "custom_toggle", CurrentValue: "off"},
	}
	target := []PreferenceConfigOption{
		{ID: "mode", CurrentValue: "code"},
		{ID: "custom_toggle", CurrentValue: "on"},
	}
	updated := applyStoredConfigOptions(context.Background(), "proj1", inst, "sess-1", current, target)

	if got := len(inst.setCalls); got != 2 {
		t.Fatalf("set calls = %d, want 2", got)
	}
	if inst.setCalls[0].ConfigID != "mode" || inst.setCalls[0].Value != "code" {
		t.Fatalf("set call[0] = %+v", inst.setCalls[0])
	}
	if inst.setCalls[1].ConfigID != "custom_toggle" || inst.setCalls[1].Value != "on" {
		t.Fatalf("set call[1] = %+v", inst.setCalls[1])
	}
	if got := findCurrentValue(updated, "mode"); got != "code" {
		t.Fatalf("updated mode = %q, want %q", got, "code")
	}
	if got := findCurrentValue(updated, "custom_toggle"); got != "on" {
		t.Fatalf("updated custom_toggle = %q, want %q", got, "on")
	}
}

func TestSessionSetConfigOption_ResolvesCategoryAliasToOptionID(t *testing.T) {
	inst := &testInjectedInstance{name: "codexapp", alive: true}
	inst.setConfigFn = func(_ context.Context, p acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
		return []acp.ConfigOption{
			{ID: acp.ConfigOptionIDReasoningEffort, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: p.Value},
		}, nil
	}
	s := mustNewSession(t, "sess-config-alias", t.TempDir(), string(acp.ACPProviderCodexApp))
	s.mu.Lock()
	s.instance = inst
	s.ready = true
	s.acpSessionID = "thread-1"
	s.agentState.ConfigOptions = []acp.ConfigOption{
		{ID: acp.ConfigOptionIDReasoningEffort, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "medium"},
	}
	s.mu.Unlock()

	opts, err := s.SetConfigOption(context.Background(), acp.ConfigOptionIDThoughtLevel, "high")
	if err != nil {
		t.Fatalf("SetConfigOption: %v", err)
	}
	if len(inst.setCalls) != 1 {
		t.Fatalf("set calls=%d, want 1", len(inst.setCalls))
	}
	if inst.setCalls[0].ConfigID != acp.ConfigOptionIDReasoningEffort {
		t.Fatalf("config id sent=%q, want %q", inst.setCalls[0].ConfigID, acp.ConfigOptionIDReasoningEffort)
	}
	if got := findCurrentValue(opts, acp.ConfigOptionIDReasoningEffort); got != "high" {
		t.Fatalf("reasoning_effort=%q, want high", got)
	}
}

func TestApplyStoredConfigOptions_ReplaysByCategoryAlias(t *testing.T) {
	inst := &testInjectedInstance{name: "codexapp"}
	inst.setConfigFn = func(_ context.Context, p acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
		return []acp.ConfigOption{
			{ID: p.ConfigID, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: p.Value},
		}, nil
	}

	current := []acp.ConfigOption{
		{ID: acp.ConfigOptionIDReasoningEffort, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "medium"},
	}
	target := []PreferenceConfigOption{
		{ID: acp.ConfigOptionIDThoughtLevel, CurrentValue: "high"},
	}
	updated := applyStoredConfigOptions(context.Background(), "proj1", inst, "thread-1", current, target)

	if got := len(inst.setCalls); got != 1 {
		t.Fatalf("set calls = %d, want 1", got)
	}
	if inst.setCalls[0].ConfigID != acp.ConfigOptionIDReasoningEffort {
		t.Fatalf("set call = %+v, want reasoning_effort", inst.setCalls[0])
	}
	if got := findCurrentValue(updated, acp.ConfigOptionIDReasoningEffort); got != "high" {
		t.Fatalf("updated reasoning_effort = %q, want high", got)
	}
}

func TestResolveHelpModel_UsesRawConfigOptionName(t *testing.T) {
	s := mustNewSession(t, "sess-help-reasoning", "/tmp", "copilot")
	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register(acp.ACPProviderCopilot, func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{name: "copilot", alive: true}, nil
	})
	s.mu.Lock()
	s.instance = &testInjectedInstance{name: "copilot", alive: true}
	s.ready = true
	s.agentState.ConfigOptions = []acp.ConfigOption{
		{
			ID:           "reasoning_effort",
			Name:         "Reasoning Effort",
			CurrentValue: "medium",
			Options: []acp.ConfigOptionValue{
				{Name: "Low", Value: "low"},
				{Name: "Medium", Value: "medium"},
				{Name: "High", Value: "high"},
			},
		},
	}
	s.mu.Unlock()

	model, err := s.resolveHelpModel(context.Background(), "")
	if err != nil {
		t.Fatalf("resolveHelpModel: %v", err)
	}
	found := false
	for _, opt := range model.Options {
		if opt.MenuID == "menu:config:reasoning_effort" {
			if !strings.HasPrefix(opt.Label, "Config: Reasoning Effort") {
				t.Fatalf("config option label = %q, want Reasoning Effort", opt.Label)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("did not find config option menu for reasoning_effort")
	}
	menu, ok := model.Menus["menu:config:reasoning_effort"]
	if !ok {
		t.Fatal("missing config menu for reasoning_effort")
	}
	if menu.Title != "Config: Reasoning Effort" {
		t.Fatalf("config menu title = %q, want %q", menu.Title, "Config: Reasoning Effort")
	}
}

func TestStoreProjectDefaultAgentMissingReturnsEmpty(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	got, err := store.LoadProjectDefaultAgent(context.Background(), "proj-missing")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if got != "" {
		t.Fatalf("default agent = %q, want empty", got)
	}
}

func TestCheckStoreSchemaRejectsLegacyProjectsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacyDB.Exec(sqliteSchema); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("init schema: %v", err)
	}
	if _, err := legacyDB.Exec(`
		DROP TABLE sessions;
		DROP TABLE IF EXISTS projects;
		CREATE TABLE projects (
			project_name TEXT PRIMARY KEY,
			yolo INTEGER NOT NULL DEFAULT 0,
			agent_state_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			project_name TEXT NOT NULL,
			status INTEGER NOT NULL,
			acp_session_id TEXT NOT NULL DEFAULT '',
			agents_json TEXT NOT NULL DEFAULT '{}',
			title TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			last_active_at TEXT NOT NULL
		);
	`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("create legacy projects table: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	err = CheckStoreSchema(dbPath)
	if err == nil {
		t.Fatal("CheckStoreSchema() error = nil, want mismatch for legacy projects/sessions columns")
	}
	if !IsStoreSchemaMismatch(err) {
		t.Fatalf("IsStoreSchemaMismatch(err) = false, err=%v", err)
	}
	if !strings.Contains(err.Error(), `table "projects" columns mismatch`) && !strings.Contains(err.Error(), `table "sessions" columns mismatch`) {
		t.Fatalf("CheckStoreSchema() err = %v, want projects/session schema mismatch", err)
	}
}
func TestCheckStoreSchemaRejectsUnexpectedLegacyTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacyDB.Exec(sqliteSchema); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("init schema: %v", err)
	}
	if _, err := legacyDB.Exec(`
		CREATE TABLE session_messages (
			message_id TEXT PRIMARY KEY,
			project_name TEXT NOT NULL,
			session_id TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("create legacy session_messages table: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	err = CheckStoreSchema(dbPath)
	if err == nil {
		t.Fatal("CheckStoreSchema() error = nil, want mismatch")
	}
	if !IsStoreSchemaMismatch(err) {
		t.Fatalf("IsStoreSchemaMismatch(err) = false, err=%v", err)
	}
	if !strings.Contains(err.Error(), `unexpected table "session_messages"`) {
		t.Fatalf("CheckStoreSchema() err = %v, want unexpected session_messages", err)
	}
}

func TestNewStoreRejectsExistingPartialSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacyDB.Exec(`
		CREATE TABLE route_bindings (
			project_name TEXT NOT NULL,
			route_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (project_name, route_key)
		)
	`); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("create partial schema: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := NewStore(dbPath)
	if err == nil {
		if store != nil {
			_ = store.Close()
		}
		t.Fatal("NewStore() error = nil, want schema mismatch")
	}
	if !IsStoreSchemaMismatch(err) {
		t.Fatalf("IsStoreSchemaMismatch(err) = false, err=%v", err)
	}
	if !strings.Contains(err.Error(), `missing table "sessions"`) {
		t.Fatalf("NewStore() err = %v, want missing sessions table", err)
	}
}

func TestStoreSessionProjectionRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	rec := &SessionRecord{
		ID:          "sess-1",
		ProjectName: "proj1",
		Status:      SessionActive,
		AgentType:   "claude",
		AgentJSON:   `{"title":"Fix app sessions"}`,

		CreatedAt:    time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 4, 12, 10, 5, 0, 0, time.UTC),
		Title:        "Fix app sessions",
	}

	if err := store.SaveSession(context.Background(), rec); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	loaded, err := store.LoadSession(context.Background(), "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadSession returned nil record")
	}
	if loaded.Title != "Fix app sessions" {
		t.Fatalf("LoadSession().Title = %q, want %q", loaded.Title, "Fix app sessions")
	}

	entries, err := store.ListSessions(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListSessions() len = %d, want 1", len(entries))
	}
	if entries[0].Title != "Fix app sessions" {
		t.Fatalf("ListSessions()[0].Title = %q, want %q", entries[0].Title, "Fix app sessions")
	}
}

func TestStoreSessionSyncJSONRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	want := `{"latestPersistedTurnIndex":4}`
	rec := &SessionRecord{
		ID:              "sess-sync",
		ProjectName:     "proj1",
		Status:          SessionActive,
		AgentType:       "claude",
		AgentJSON:       `{}`,
		SessionSyncJSON: want,
		CreatedAt:       time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt:    time.Date(2026, 4, 12, 10, 5, 0, 0, time.UTC),
	}

	if err := store.SaveSession(context.Background(), rec); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	loaded, err := store.LoadSession(context.Background(), "proj1", "sess-sync")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadSession returned nil record")
	}
	if loaded.SessionSyncJSON != want {
		t.Fatalf("LoadSession().SessionSyncJSON = %q, want %q", loaded.SessionSyncJSON, want)
	}

	entries, err := store.ListSessions(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListSessions() len = %d, want 1", len(entries))
	}
	if entries[0].SessionSyncJSON != want {
		t.Fatalf("ListSessions()[0].SessionSyncJSON = %q, want %q", entries[0].SessionSyncJSON, want)
	}
}

func TestStoreSessionSyncJSONSurvivesMetadataSave(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	want := `{"latestPersistedTurnIndex":4}`
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:              "sess-sync",
		ProjectName:     "proj1",
		Status:          SessionActive,
		AgentType:       "claude",
		AgentJSON:       `{}`,
		SessionSyncJSON: want,
		CreatedAt:       time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt:    time.Date(2026, 4, 12, 10, 5, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession initial: %v", err)
	}

	if err := store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-sync",
		ProjectName:  "proj1",
		Status:       SessionActive,
		AgentType:    "claude",
		AgentJSON:    `{}`,
		Title:        "Updated Title",
		LastActiveAt: time.Date(2026, 4, 12, 10, 6, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession metadata: %v", err)
	}

	loaded, err := store.LoadSession(ctx, "proj1", "sess-sync")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.SessionSyncJSON != want {
		t.Fatalf("SessionSyncJSON = %q, want %q", loaded.SessionSyncJSON, want)
	}
}

func TestSessionReadUsesFinishedTurnCursor(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	seedPromptWithTurns(t, c, ctx, "sess-1", "hello", []acp.SessionUpdate{
		{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "answer"})},
	})

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 1)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns len = %d, want 2", len(turns))
	}
	for _, turn := range turns {
		if turn.TurnIndex <= 1 {
			t.Fatalf("returned turn before cursor: %#v", turn)
		}
	}
}

func TestStartingNextPromptSynthesizesInterruptedPromptDone(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Interrupted Prompt")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "first", nil)); err != nil {
		t.Fatalf("RecordEvent first prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "second", nil)); err != nil {
		t.Fatalf("RecordEvent second prompt: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if !hasPromptDoneTurnWithStopReason(t, turns, "interrupted") {
		t.Fatalf("turns missing interrupted prompt_done: %#v", turns)
	}
}

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
	sess, err := c.newWiredSession(sessionID, agent)
	if err != nil {
		panic(err)
	}
	sess.mu.Lock()
	sess.acpSessionID = sessionID
	sess.agentState.Title = title
	sess.Status = SessionActive
	sess.createdAt = createdAt
	sess.lastActiveAt = lastActiveAt
	sess.mu.Unlock()

	c.mu.Lock()
	c.sessions[sess.acpSessionID] = sess
	c.mu.Unlock()
}

func sessionViewCreatedEvent(sessionID, title string) SessionViewEvent {
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: acp.BuildACPContentJSON(acp.MethodSessionNew, map[string]any{
			"params": map[string]any{
				"sessionId": sessionID,
				"agentType": "claude",
				"title":     title,
			},
		}),
	}
}

func sessionViewPromptEvent(sessionID, text string, blocks []acp.ContentBlock) SessionViewEvent {
	params := acp.SessionPromptParams{SessionID: sessionID}
	if len(blocks) > 0 {
		params.Prompt = cloneSessionContentBlocks(blocks)
	} else {
		params.Prompt = []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}}
	}
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
			"params": params,
		}),
	}
}

func sessionViewUpdateEvent(sessionID string, update acp.SessionUpdate) SessionViewEvent {
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: acp.BuildACPContentJSON(acp.MethodSessionUpdate, map[string]any{
			"params": acp.SessionUpdateParams{
				SessionID: sessionID,
				Update:    update,
			},
		}),
	}
}

func sessionViewAssistantChunkTextEvent(sessionID, text, status string) SessionViewEvent {
	update := acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Status:        strings.TrimSpace(status),
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: text}),
	}
	return sessionViewUpdateEvent(sessionID, update)
}

func sessionViewToolUpdatedTextEvent(sessionID, title string) SessionViewEvent {
	update := acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateToolCallUpdate,
		ToolCallID:    "call-shared",
		Title:         title,
	}
	return sessionViewUpdateEvent(sessionID, update)
}
func sessionViewPromptFinishedEvent(sessionID, stopReason string) SessionViewEvent {
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
			"result": acp.SessionPromptResult{
				StopReason: stopReason,
			},
		}),
	}
}

type publishedSessionEvent struct {
	method  string
	payload map[string]any
}

func captureSessionMessageEvents(t *testing.T, c *Client) *[]publishedSessionEvent {
	t.Helper()
	published := []publishedSessionEvent{}
	c.sessionRecorder.SetEventPublisher(func(method string, payload any) error {
		if method != "registry.session.message" {
			return nil
		}
		body, ok := payload.(map[string]any)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]any", payload)
		}
		published = append(published, publishedSessionEvent{method: method, payload: body})
		return nil
	})
	return &published
}

func lastPublishedEvent(t *testing.T, published []publishedSessionEvent, method string) map[string]any {
	t.Helper()
	for i := len(published) - 1; i >= 0; i-- {
		if published[i].method == method {
			return published[i].payload
		}
	}
	t.Fatalf("no published event for %s", method)
	return nil
}

func sessionSummaryMap(t *testing.T, summary any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal session summary: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal session summary: %v", err)
	}
	return out
}

func writeClaudeSessionFixture(t *testing.T, homeDir, projectDirName, sessionID, cwd, title, assistant string) {
	t.Helper()
	projectDir := filepath.Join(homeDir, ".claude", "projects", projectDirName)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", projectDir, err)
	}
	lines := []string{
		fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"%s"},"cwd":%q}`, title, cwd),
		fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":"%s"}}`, assistant),
	}
	sessionPath := filepath.Join(projectDir, sessionID+".jsonl")
	if err := os.WriteFile(sessionPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", sessionPath, err)
	}
}

func writeCodexSessionFixture(t *testing.T, homeDir, sessionID, cwd, title, assistant string) {
	t.Helper()
	codexDir := filepath.Join(homeDir, ".codex")
	sessionDir := filepath.Join(codexDir, "sessions", "2026", "05", "12")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", sessionDir, err)
	}
	indexLine, err := json.Marshal(map[string]any{
		"id":          sessionID,
		"thread_name": title,
		"updated_at":  "2026-05-12T08:00:00Z",
	})
	if err != nil {
		t.Fatalf("marshal codex index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "session_index.jsonl"), append(indexLine, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile codex index: %v", err)
	}

	metaLine, err := json.Marshal(map[string]any{
		"type": "session_meta",
		"payload": map[string]any{
			"id":  sessionID,
			"cwd": cwd,
		},
	})
	if err != nil {
		t.Fatalf("marshal codex meta: %v", err)
	}
	doneLine, err := json.Marshal(map[string]any{
		"type": "event_msg",
		"payload": map[string]any{
			"type":               "task_complete",
			"last_agent_message": assistant,
		},
	})
	if err != nil {
		t.Fatalf("marshal codex event: %v", err)
	}
	sessionPath := filepath.Join(sessionDir, "rollout-2026-05-12T08-00-00-"+sessionID+".jsonl")
	content := string(metaLine) + "\n" + string(doneLine) + "\n"
	if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", sessionPath, err)
	}
}

func sessionViewPermissionRequestedEvent(sessionID, text string, requestID int64, options []acp.PermissionOption) SessionViewEvent {
	params := acp.PermissionRequestParams{
		SessionID: sessionID,
		ToolCall: acp.ToolCallRef{
			ToolCallID: fmt.Sprintf("call-%d", requestID),
			Title:      text,
		},
		Options: cloneSessionPermissionOptions(options),
	}
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: acp.BuildACPContentJSON(acp.MethodRequestPermission, map[string]any{
			"id":     requestID,
			"params": params,
		}),
	}
}

func sessionViewPermissionResolvedEvent(sessionID string, requestID int64, status string, updatedAt time.Time) SessionViewEvent {
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: acp.BuildACPContentJSON(acp.MethodRequestPermission, map[string]any{
			"id": requestID,
			"result": acp.PermissionResponse{
				Outcome: acp.PermissionResult{Outcome: status},
			},
		}),
		UpdatedAt: updatedAt,
	}
}

func sessionViewSystemEvent(sessionID, text string) SessionViewEvent {
	return SessionViewEvent{
		Type:      SessionViewEventTypeSystem,
		SessionID: sessionID,
		Content:   text,
	}
}

func sessionViewACPSystemEvent(sessionID, text string) SessionViewEvent {
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: acp.BuildACPContentJSON(acp.IMMethodSystem, map[string]any{
			"result": text,
		}),
	}
}

func TestParseSessionViewEventSessionUpdateReturnsToolTurnKey(t *testing.T) {
	event := sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateToolCallUpdate,
		ToolCallID:    "call-1",
		Title:         "build",
		Status:        "completed",
	})
	parsed, err := parseSessionViewEvent(event)
	if err != nil {
		t.Fatalf("parseSessionViewEvent: %v", err)
	}
	if !parsed.bMessage {
		t.Fatal("parsed.bMessage = false, want true")
	}
	if parsed.method != acp.IMMethodToolCall {
		t.Fatalf("parsed.method = %q, want %q", parsed.method, acp.IMMethodToolCall)
	}
	if parsed.turnKey != "call-1" {
		t.Fatalf("parsed.turnKey = %q, want %q", parsed.turnKey, "call-1")
	}
}

func TestBuildACPContentJSONIncludesMethodAndFields(t *testing.T) {
	raw := acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
		"params": acp.SessionPromptParams{
			SessionID: "sess-1",
			Prompt:    []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: "hello"}},
		},
	})

	var doc struct {
		Method string                  `json:"method"`
		Params acp.SessionPromptParams `json:"params"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("json.Unmarshal(buildACPContentJSON): %v", err)
	}
	if doc.Method != acp.MethodSessionPrompt {
		t.Fatalf("method = %q, want %q", doc.Method, acp.MethodSessionPrompt)
	}
	if doc.Params.SessionID != "sess-1" {
		t.Fatalf("params.sessionId = %q, want %q", doc.Params.SessionID, "sess-1")
	}
	if len(doc.Params.Prompt) != 1 || strings.TrimSpace(doc.Params.Prompt[0].Text) != "hello" {
		t.Fatalf("params.prompt = %#v, want single text block", doc.Params.Prompt)
	}
}

func TestMergeTurnMessageMergesTypedTextPayload(t *testing.T) {
	merged := mergeTurnMessage(
		sessionTurnMessage{
			sessionID: "sess-1",
			method:    acp.IMMethodAgentMessage,
			payload:   acp.IMTextResult{Text: "hello"},
			turnIndex: 2,
		},
		sessionTurnMessage{
			sessionID: "sess-1",
			method:    acp.IMMethodAgentMessage,
			payload:   acp.IMTextResult{Text: " world"},
			turnIndex: 2,
		},
		2,
	)
	result, ok := merged.payload.(acp.IMTextResult)
	if !ok {
		t.Fatalf("merged.payload type = %T, want %T", merged.payload, acp.IMTextResult{})
	}
	if result.Text != "hello world" {
		t.Fatalf("merged text = %q, want %q", result.Text, "hello world")
	}
}

func TestBuildIMContentJSONDoesNotTrimMethod(t *testing.T) {
	raw := buildIMContentJSON("  method.with.space  ", map[string]any{"k": "v"})
	msg := acp.IMTurnMessage{}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if msg.Method != "  method.with.space  " {
		t.Fatalf("msg.Method = %q, want %q", msg.Method, "  method.with.space  ")
	}
}

func TestSessionPromptStateUpdateTurnDoesNotTrimFields(t *testing.T) {
	state := newSessionPromptState(1)
	state.updateTurn(sessionTurnMessage{sessionID: "  sid  ", method: "  method  "}, "  key  ")
	if len(state.turns) != 1 {
		t.Fatalf("turns len = %d, want 1", len(state.turns))
	}
	if state.turns[0].sessionID != "  sid  " {
		t.Fatalf("turn.SessionID = %q, want %q", state.turns[0].sessionID, "  sid  ")
	}
	if state.turns[0].method != "  method  " {
		t.Fatalf("turn.method = %q, want %q", state.turns[0].method, "  method  ")
	}
	if _, ok := state.turnIndexByKey["  key  "]; !ok {
		t.Fatalf("turnIndexByKey missing exact key %q", "  key  ")
	}
}

func TestCurrentPromptStateLockedReturnsNilWhenMissing(t *testing.T) {
	c := newSessionViewTestClient(t)

	state, err := c.sessionRecorder.currentPromptStateLocked(context.Background(), "sess-missing")
	if err != nil {
		t.Fatalf("currentPromptStateLocked: %v", err)
	}
	if state != nil {
		t.Fatalf("state = %#v, want nil", *state)
	}
}

func TestCurrentPromptStateLockedIgnoresBlankSessionID(t *testing.T) {
	c := newSessionViewTestClient(t)

	state, err := c.sessionRecorder.currentPromptStateLocked(context.Background(), "   ")
	if err != nil {
		t.Fatalf("currentPromptStateLocked blank sessionID: %v", err)
	}
	if state != nil {
		t.Fatalf("state = %#v, want nil", *state)
	}
}

func TestCurrentPromptStateLockedReturnsLiveCachedState(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	c.sessionRecorder.writeMu.Lock()
	cached := newSessionPromptState(1)
	c.sessionRecorder.promptState["sess-live"] = &cached
	c.sessionRecorder.writeMu.Unlock()

	state, err := c.sessionRecorder.currentPromptStateLocked(ctx, "sess-live")
	if err != nil {
		t.Fatalf("currentPromptStateLocked first read: %v", err)
	}
	if state == nil {
		t.Fatal("currentPromptStateLocked first read = nil, want state")
	}
	state.nextTurnIndex = 7

	reloaded, err := c.sessionRecorder.currentPromptStateLocked(ctx, "sess-live")
	if err != nil {
		t.Fatalf("currentPromptStateLocked second read: %v", err)
	}
	if reloaded == nil {
		t.Fatal("currentPromptStateLocked second read = nil, want state")
	}
	if reloaded.nextTurnIndex != 7 {
		t.Fatalf("reloaded.nextTurnIndex = %d, want 7", reloaded.nextTurnIndex)
	}
}

func TestCurrentPromptStateLockedDoesNotRestorePersistedTurns(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)

	if err := c.store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-no-restore",
		ProjectName:  "proj1",
		Status:       SessionActive,
		AgentType:    "claude",
		AgentJSON:    `{}`,
		CreatedAt:    now,
		LastActiveAt: now,
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	state, err := c.sessionRecorder.currentPromptStateLocked(ctx, "sess-no-restore")
	if err != nil {
		t.Fatalf("currentPromptStateLocked: %v", err)
	}
	if state != nil {
		t.Fatalf("state = %#v, want nil", *state)
	}
}

func TestGetTurnIndexUsesGenericTurnKeyIndex(t *testing.T) {
	state := sessionPromptState{
		nextTurnIndex: 3,
		turns: []sessionTurnMessage{
			{turnIndex: 1, method: acp.IMMethodSystem},
			{turnIndex: 2, method: acp.IMMethodToolCall},
		},
		turnIndexByKey: map[string]int64{
			"merge-key": 2,
		},
	}

	turnIndex := state.turnIndexByKey["merge-key"]
	if turnIndex != 2 {
		t.Fatalf("turnIndex = %d, want 2", turnIndex)
	}
}

func TestAddMessageTurnMutatesStateInPlace(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}

	parsed, err := parseSessionViewEvent(sessionViewPromptEvent("sess-1", "say hi", nil))
	if err != nil {
		t.Fatalf("parseSessionViewEvent: %v", err)
	}

	state := newSessionPromptState(1)
	if err := c.sessionRecorder.addMessageTurn(&state, parsed); err != nil {
		t.Fatalf("addMessageTurn: %v", err)
	}

	if state.nextTurnIndex != 2 {
		t.Fatalf("state.nextTurnIndex = %d, want 2", state.nextTurnIndex)
	}
	if len(state.turns) != 1 {
		t.Fatalf("len(state.turns) = %d, want 1", len(state.turns))
	}
	turn := state.turns[0]
	if turn.method != acp.IMMethodPromptRequest {
		t.Fatalf("turn method = %q, want %q", turn.method, acp.IMMethodPromptRequest)
	}
}

func TestParseSessionViewEventSeparatesControlAndMessageEvents(t *testing.T) {
	tests := []struct {
		name          string
		event         SessionViewEvent
		wantMessage   bool
		wantACPMethod string
		wantMethod    string
		wantTurnKey   string
		check         func(*testing.T, parsedSessionViewEvent)
	}{
		{
			name:          "session new stays control event",
			event:         sessionViewCreatedEvent("sess-1", "Task"),
			wantMessage:   false,
			wantACPMethod: acp.MethodSessionNew,
		},
		{
			name:          "prompt params becomes prompt message",
			event:         sessionViewPromptEvent("sess-1", "say hi", nil),
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionPrompt,
			wantMethod:    acp.IMMethodPromptRequest,
			check: func(t *testing.T, parsed parsedSessionViewEvent) {
				t.Helper()
				requestPayload, ok := parsed.payload.(acp.IMPromptRequest)
				if !ok {
					t.Fatalf("parsed.payload type = %T, want %T", parsed.payload, acp.IMPromptRequest{})
				}
				if len(requestPayload.ContentBlocks) != 1 || strings.TrimSpace(requestPayload.ContentBlocks[0].Text) != "say hi" {
					t.Fatalf("payload.ContentBlocks = %#v, want single text block", requestPayload.ContentBlocks)
				}
			},
		},
		{
			name:          "prompt result becomes prompt message",
			event:         sessionViewPromptFinishedEvent("sess-1", acp.StopReasonEndTurn),
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionPrompt,
			wantMethod:    acp.IMMethodPromptDone,
			check: func(t *testing.T, parsed parsedSessionViewEvent) {
				t.Helper()
				resultPayload, ok := parsed.payload.(acp.IMPromptResult)
				if !ok {
					t.Fatalf("parsed.payload type = %T, want %T", parsed.payload, acp.IMPromptResult{})
				}
				if resultPayload.StopReason != acp.StopReasonEndTurn {
					t.Fatalf("payload.StopReason = %q, want %q", resultPayload.StopReason, acp.StopReasonEndTurn)
				}
			},
		},
		{
			name: "session update becomes turn message",
			event: sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
				SessionUpdate: acp.SessionUpdateToolCallUpdate,
				ToolCallID:    "call-1",
				Title:         "build",
				Status:        acp.ToolCallStatusCompleted,
			}),
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionUpdate,
			wantMethod:    acp.IMMethodToolCall,
			wantTurnKey:   "call-1",
		},
		{
			name:          "permission stays ignored control event",
			event:         sessionViewPermissionRequestedEvent("sess-1", "allow?", 7, nil),
			wantMessage:   false,
			wantACPMethod: acp.MethodRequestPermission,
		},
		{
			name: "ACP event type matching is case-insensitive",
			event: SessionViewEvent{
				Type:      SessionViewEventType("ACP"),
				SessionID: "sess-1",
				Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
					"params": acp.SessionPromptParams{},
				}),
			},
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionPrompt,
			wantMethod:    acp.IMMethodPromptRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseSessionViewEvent(tt.event)
			if err != nil {
				t.Fatalf("parseSessionViewEvent: %v", err)
			}
			if parsed.bMessage != tt.wantMessage {
				t.Fatalf("parsed.bMessage = %v, want %v", parsed.bMessage, tt.wantMessage)
			}
			if parsed.acpMethod != tt.wantACPMethod {
				t.Fatalf("parsed.acpMethod = %q, want %q", parsed.acpMethod, tt.wantACPMethod)
			}
			if parsed.method != tt.wantMethod {
				t.Fatalf("parsed.method = %q, want %q", parsed.method, tt.wantMethod)
			}
			if parsed.turnKey != tt.wantTurnKey {
				t.Fatalf("parsed.turnKey = %q, want %q", parsed.turnKey, tt.wantTurnKey)
			}
			if tt.check != nil {
				tt.check(t, parsed)
			}
		})
	}
}

func TestParseSessionViewEventSilentlyHandlesMissingParams(t *testing.T) {
	tests := []struct {
		name          string
		event         SessionViewEvent
		wantMessage   bool
		wantACPMethod string
		wantMethod    string
		check         func(*testing.T, parsedSessionViewEvent)
	}{
		{
			name: "prompt without params becomes empty prompt message",
			event: SessionViewEvent{
				Type:      SessionViewEventTypeACP,
				SessionID: "sess-1",
				Content:   acp.BuildACPContentJSON(acp.MethodSessionPrompt, nil),
			},
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionPrompt,
			wantMethod:    acp.IMMethodPromptRequest,
			check: func(t *testing.T, parsed parsedSessionViewEvent) {
				t.Helper()
				request, ok := parsed.payload.(acp.IMPromptRequest)
				if !ok {
					t.Fatalf("parsed.payload type = %T, want acp.IMPromptRequest", parsed.payload)
				}
				if len(request.ContentBlocks) != 0 {
					t.Fatalf("request.ContentBlocks len = %d, want 0", len(request.ContentBlocks))
				}
			},
		},
		{
			name: "prompt with malformed params becomes empty prompt message",
			event: SessionViewEvent{
				Type:      SessionViewEventTypeACP,
				SessionID: "sess-1",
				Content:   acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{"params": "oops"}),
			},
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionPrompt,
			wantMethod:    acp.IMMethodPromptRequest,
			check: func(t *testing.T, parsed parsedSessionViewEvent) {
				t.Helper()
				request, ok := parsed.payload.(acp.IMPromptRequest)
				if !ok {
					t.Fatalf("parsed.payload type = %T, want acp.IMPromptRequest", parsed.payload)
				}
				if len(request.ContentBlocks) != 0 {
					t.Fatalf("request.ContentBlocks len = %d, want 0", len(request.ContentBlocks))
				}
			},
		},
		{
			name: "session update without params is ignored without error",
			event: SessionViewEvent{
				Type:      SessionViewEventTypeACP,
				SessionID: "sess-1",
				Content:   acp.BuildACPContentJSON(acp.MethodSessionUpdate, nil),
			},
			wantMessage:   false,
			wantACPMethod: acp.MethodSessionUpdate,
			wantMethod:    "",
		},
		{
			name: "session update with malformed params is ignored without error",
			event: SessionViewEvent{
				Type:      SessionViewEventTypeACP,
				SessionID: "sess-1",
				Content:   acp.BuildACPContentJSON(acp.MethodSessionUpdate, map[string]any{"params": "oops"}),
			},
			wantMessage:   false,
			wantACPMethod: acp.MethodSessionUpdate,
			wantMethod:    "",
		},
		{
			name: "ACP system without result is ignored without error",
			event: SessionViewEvent{
				Type:      SessionViewEventTypeACP,
				SessionID: "sess-1",
				Content:   acp.BuildACPContentJSON(acp.IMMethodSystem, nil),
			},
			wantMessage:   false,
			wantACPMethod: acp.IMMethodSystem,
			wantMethod:    "",
		},
		{
			name: "ACP system with result is ignored without error",
			event: SessionViewEvent{
				Type:      SessionViewEventTypeACP,
				SessionID: "sess-1",
				Content: acp.BuildACPContentJSON(acp.IMMethodSystem, map[string]any{
					"result": "ignored",
				}),
			},
			wantMessage:   false,
			wantACPMethod: acp.IMMethodSystem,
			wantMethod:    "",
		},
		{
			name: "legacy system event is ignored without error",
			event: SessionViewEvent{
				Type:      SessionViewEventTypeSystem,
				SessionID: "sess-1",
				Content:   "legacy system",
			},
			wantMessage:   true,
			wantACPMethod: "",
			wantMethod:    acp.IMMethodSystem,
			check: func(t *testing.T, parsed parsedSessionViewEvent) {
				t.Helper()
				result, ok := parsed.payload.(acp.IMTextResult)
				if !ok {
					t.Fatalf("parsed.payload type = %T, want acp.IMTextResult", parsed.payload)
				}
				if strings.TrimSpace(result.Text) != "legacy system" {
					t.Fatalf("result.Text = %q, want %q", result.Text, "legacy system")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseSessionViewEvent(tt.event)
			if err != nil {
				t.Fatalf("parseSessionViewEvent: %v", err)
			}
			if parsed.bMessage != tt.wantMessage {
				t.Fatalf("parsed.bMessage = %v, want %v", parsed.bMessage, tt.wantMessage)
			}
			if parsed.acpMethod != tt.wantACPMethod {
				t.Fatalf("parsed.acpMethod = %q, want %q", parsed.acpMethod, tt.wantACPMethod)
			}
			if parsed.method != tt.wantMethod {
				t.Fatalf("parsed.method = %q, want %q", parsed.method, tt.wantMethod)
			}
			if tt.check != nil {
				tt.check(t, parsed)
			}
		})
	}
}

func TestParseSessionViewEventSessionUpdateUnknownTypeReturnsError(t *testing.T) {
	event := sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: "session/update.unknown",
		Content:       mustJSON(map[string]any{"text": "ignored"}),
	})

	_, err := parseSessionViewEvent(event)
	if err == nil {
		t.Fatal("parseSessionViewEvent error = nil, want unsupported session update type error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unsupported session update type") {
		t.Fatalf("parseSessionViewEvent error = %v, want unsupported session update type", err)
	}
}

func TestJSONDecodeAtSupportsTopLevelAndSingleNestedField(t *testing.T) {
	raw := mustJSON(map[string]any{
		"method": "session.update",
		"params": map[string]any{
			"title": "hello",
			"meta":  map[string]any{"title": "too-deep"},
		},
	})

	var method string
	if !jsonDecodeAt(raw, "method", &method) {
		t.Fatal("jsonDecodeAt(method) = false, want true")
	}
	if method != "session.update" {
		t.Fatalf("method = %q, want %q", method, "session.update")
	}

	var title string
	if !jsonDecodeAt(raw, "params.title", &title) {
		t.Fatal("jsonDecodeAt(params.title) = false, want true")
	}
	if title != "hello" {
		t.Fatalf("title = %q, want %q", title, "hello")
	}

	if jsonDecodeAt(raw, "params.meta.title", &title) {
		t.Fatal("jsonDecodeAt(params.meta.title) = true, want false")
	}
}

func TestExtractUpdateTextSupportsExpectedShapes(t *testing.T) {
	if got := extractUpdateText(mustJSON("hello")); got != "hello" {
		t.Fatalf("extractUpdateText(string) = %q, want %q", got, "hello")
	}
	if got := extractUpdateText(mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "world"})); got != "world" {
		t.Fatalf("extractUpdateText(content block) = %q, want %q", got, "world")
	}
	if got := extractUpdateText(mustJSON(map[string]any{"text": "compat"})); got != "compat" {
		t.Fatalf("extractUpdateText(text map) = %q, want %q", got, "compat")
	}
}

type archiveManifestForTest struct {
	Version  int                                    `json:"version"`
	Sessions map[string]archiveManifestEntryForTest `json:"sessions"`
}

type archiveManifestEntryForTest struct {
	SessionID          string `json:"sessionId"`
	ProjectName        string `json:"projectName"`
	Title              string `json:"title"`
	AgentType          string `json:"agentType"`
	Storage            string `json:"storage"`
	File               string `json:"file"`
	Offset             int64  `json:"offset"`
	Length             int64  `json:"length"`
	UncompressedLength int64  `json:"uncompressedLength"`
	Codec              string `json:"codec"`
	SHA256             string `json:"sha256"`
	UncompressedSHA256 string `json:"uncompressedSha256"`
	TurnCount          int    `json:"turnCount"`
	GapCount           int    `json:"gapCount"`
	WMT2Version        int    `json:"wmt2Version"`
	ChunkSizeCode      int    `json:"chunkSizeCode"`
	ArchivedAt         string `json:"archivedAt"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

func readArchiveManifestForTest(t *testing.T, historyRoot, projectName string) archiveManifestForTest {
	t.Helper()
	path := filepath.Join(filepath.Dir(historyRoot), "session-archive", safeHistoryPathPart(projectName), "manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile archive manifest: %v", err)
	}
	var manifest archiveManifestForTest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("Unmarshal archive manifest: %v", err)
	}
	if manifest.Version != 1 {
		t.Fatalf("manifest version = %d, want 1", manifest.Version)
	}
	if manifest.Sessions == nil {
		t.Fatal("manifest sessions map is nil")
	}
	return manifest
}

func readArchivedSessionPayloadForTest(t *testing.T, historyRoot, projectName string, entry archiveManifestEntryForTest) ([]byte, []byte) {
	t.Helper()
	path := filepath.Join(filepath.Dir(historyRoot), "session-archive", safeHistoryPathPart(projectName), entry.File)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile archive pack: %v", err)
	}
	if entry.Offset < 0 || entry.Length <= 0 || entry.Offset+entry.Length > int64(len(raw)) {
		t.Fatalf("archive segment offset/length outside pack: offset=%d length=%d pack=%d", entry.Offset, entry.Length, len(raw))
	}
	segment := raw[entry.Offset : entry.Offset+entry.Length]
	if len(segment) < 26 {
		t.Fatalf("archive segment too short: %d", len(segment))
	}
	if string(segment[0:4]) != "WMSA" {
		t.Fatalf("archive segment magic = %q, want WMSA", string(segment[0:4]))
	}
	if version := binary.LittleEndian.Uint16(segment[4:6]); version != 1 {
		t.Fatalf("archive segment version = %d, want 1", version)
	}
	if codec := segment[6]; codec != 1 {
		t.Fatalf("archive segment codec = %d, want gzip codec 1", codec)
	}
	sessionIDLen := int(binary.LittleEndian.Uint16(segment[8:10]))
	payloadLen := int64(binary.LittleEndian.Uint64(segment[10:18]))
	uncompressedLen := int64(binary.LittleEndian.Uint64(segment[18:26]))
	payloadStart := 26 + sessionIDLen
	payloadEnd := payloadStart + int(payloadLen)
	if payloadStart > len(segment) || payloadEnd != len(segment) {
		t.Fatalf("archive segment payload bounds invalid: start=%d end=%d len=%d", payloadStart, payloadEnd, len(segment))
	}
	if gotSessionID := string(segment[26:payloadStart]); gotSessionID != entry.SessionID {
		t.Fatalf("archive segment sessionID = %q, want %q", gotSessionID, entry.SessionID)
	}
	if uncompressedLen != entry.UncompressedLength {
		t.Fatalf("archive segment uncompressedLen = %d, want %d", uncompressedLen, entry.UncompressedLength)
	}
	compressedPayload := segment[payloadStart:payloadEnd]
	reader, err := gzip.NewReader(bytes.NewReader(compressedPayload))
	if err != nil {
		t.Fatalf("NewReader gzip payload: %v", err)
	}
	defer reader.Close()
	uncompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll gzip payload: %v", err)
	}
	if int64(len(uncompressed)) != uncompressedLen {
		t.Fatalf("uncompressed payload len = %d, want %d", len(uncompressed), uncompressedLen)
	}
	return uncompressed, segment
}

func decodeWMT2ContentsForTest(t *testing.T, raw []byte, turnCount int) []string {
	t.Helper()
	if len(raw) < 8 {
		t.Fatalf("WMT2 payload too short: %d", len(raw))
	}
	if string(raw[0:4]) != sessionTurnFileMagic {
		t.Fatalf("WMT2 magic = %q, want %s", string(raw[0:4]), sessionTurnFileMagic)
	}
	if version := binary.LittleEndian.Uint16(raw[4:6]); version != sessionTurnFileVersion {
		t.Fatalf("WMT2 version = %d, want %d", version, sessionTurnFileVersion)
	}
	code := raw[6]
	capacity := 256 << code
	if turnCount > capacity {
		t.Fatalf("turnCount = %d exceeds WMT2 capacity %d", turnCount, capacity)
	}
	headerSize := sessionTurnFilePreambleSize + capacity*sessionTurnFileMetaSize
	if len(raw) < headerSize {
		t.Fatalf("WMT2 payload len = %d, want at least header %d", len(raw), headerSize)
	}
	contents := make([]string, 0, turnCount)
	for slot := 0; slot < turnCount; slot++ {
		pos := sessionTurnFilePreambleSize + slot*sessionTurnFileMetaSize
		offset := binary.LittleEndian.Uint32(raw[pos : pos+4])
		length := binary.LittleEndian.Uint32(raw[pos+4 : pos+8])
		if offset == 0 || length == 0 {
			t.Fatalf("WMT2 slot %d is empty", slot)
		}
		end := int(offset) + int(length)
		if int(offset) < headerSize || end > len(raw) {
			t.Fatalf("WMT2 slot %d points outside payload", slot)
		}
		contents = append(contents, string(raw[int(offset):end]))
	}
	return contents
}

func TestSessionViewCreatedEventSilentlyHandlesMalformedTitle(t *testing.T) {
	c := newSessionViewTestClient(t)
	event := SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: "sess-1",
		Content:   acp.BuildACPContentJSON(acp.MethodSessionNew, map[string]any{"params": map[string]any{"agentType": "claude", "title": 123}}),
	}

	if err := c.RecordEvent(context.Background(), event); err != nil {
		t.Fatalf("RecordEvent malformed session.new: %v", err)
	}

	sessions, err := c.listSessionViews(context.Background())
	if err != nil {
		t.Fatalf("listSessionViews: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if sessions[0].Title != "" {
		t.Fatalf("sessions[0].Title = %q, want empty", sessions[0].Title)
	}
}

func TestSessionViewAssistantChunksReusePreviousTurnByUpdateType(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "New Session")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "say hi", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewAssistantChunkTextEvent("sess-1", "hello", "")); err != nil {
		t.Fatalf("RecordEvent chunk1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewAssistantChunkTextEvent("sess-1", " world", "")); err != nil {
		t.Fatalf("RecordEvent chunk2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3", len(turns))
	}
	update2 := decodeTurnSessionUpdate(t, turns[1].Content)
	if text := extractTextChunk(update2.Content); text != "hello world" {
		t.Fatalf("turns[1] text = %q, want %q", text, "hello world")
	}
}

func TestSessionViewListIncludesProjectionFields(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent first prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "latest prompt", nil)); err != nil {
		t.Fatalf("RecordEvent second prompt: %v", err)
	}

	sessions, err := c.listSessionViews(ctx)
	if err != nil {
		t.Fatalf("listSessionViews: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if sessions[0].Title != "latest prompt" {
		t.Fatalf("sessions[0].Title = %q, want %q", sessions[0].Title, "latest prompt")
	}
}

func TestSessionViewPersistSessionDoesNotOverrideLatestPromptTitle(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	now := time.Now().UTC()
	addRuntimeSession(c, "sess-1", "Runtime Title", "claude", now.Add(-2*time.Minute), now)

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Created Title")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "latest prompt title", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	sessionsBefore, err := c.listSessionViews(ctx)
	if err != nil {
		t.Fatalf("listSessionViews before persist: %v", err)
	}
	if len(sessionsBefore) != 1 {
		t.Fatalf("sessionsBefore len = %d, want 1", len(sessionsBefore))
	}
	if sessionsBefore[0].Title != "latest prompt title" {
		t.Fatalf("sessionsBefore[0].Title = %q, want %q", sessionsBefore[0].Title, "latest prompt title")
	}

	sess, err := c.SessionByID(ctx, "sess-1")
	if err != nil {
		t.Fatalf("SessionByID: %v", err)
	}
	sess.mu.Lock()
	sess.agentState.Title = "Stale Runtime Title"
	sess.mu.Unlock()
	if err := sess.persistSession(ctx); err != nil {
		t.Fatalf("persistSession: %v", err)
	}

	sessionsAfter, err := c.listSessionViews(ctx)
	if err != nil {
		t.Fatalf("listSessionViews after persist: %v", err)
	}
	if len(sessionsAfter) != 1 {
		t.Fatalf("sessionsAfter len = %d, want 1", len(sessionsAfter))
	}
	if sessionsAfter[0].Title != "latest prompt title" {
		t.Fatalf("sessionsAfter[0].Title = %q, want %q", sessionsAfter[0].Title, "latest prompt title")
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
	if sessions[0].AgentType != "claude" {
		t.Fatalf("sessions[0].AgentType = %q, want %q", sessions[0].AgentType, "claude")
	}
}

func TestSessionReadOmitsTurnIDAndSummaryExtras(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	if _, ok := body["session"]; ok {
		t.Fatalf("session.read unexpectedly returned session: %+v", body["session"])
	}
	if _, ok := body["prompts"]; ok {
		t.Fatalf("session.read unexpectedly returned prompts: %+v", body["prompts"])
	}
	if _, ok := body["messages"]; ok {
		t.Fatalf("session.read unexpectedly returned messages: %+v", body["messages"])
	}
	turns := body["turns"].([]sessionViewTurn)
	if len(turns) != 1 {
		t.Fatalf("turns len = %d, want 1", len(turns))
	}
	if turns[0].TurnIndex != 1 {
		t.Fatalf("turns[0].TurnIndex = %d, want 1", turns[0].TurnIndex)
	}
}

func TestSessionReadReturnsTurnsOnlyWithLatestTurnIndex(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "answer"}),
	})); err != nil {
		t.Fatalf("RecordEvent answer: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "end_turn")); err != nil {
		t.Fatalf("RecordEvent finished: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterTurnIndex": int64(1)})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	if _, ok := body["session"]; ok {
		t.Fatalf("session.read unexpectedly returned session: %+v", body["session"])
	}
	if _, ok := body["prompts"]; ok {
		t.Fatalf("session.read unexpectedly returned prompts: %+v", body["prompts"])
	}
	if _, ok := body["messages"]; ok {
		t.Fatalf("session.read unexpectedly returned messages: %+v", body["messages"])
	}
	if got := body["latestTurnIndex"]; got != int64(3) {
		t.Fatalf("latestTurnIndex = %v, want 3", got)
	}
	turns, ok := body["turns"].([]sessionViewTurn)
	if !ok {
		t.Fatalf("turns type = %T, want []sessionViewTurn", body["turns"])
	}
	if len(turns) != 2 {
		t.Fatalf("turns len = %d, want 2", len(turns))
	}
	if turns[0].TurnIndex != 2 || turns[1].TurnIndex != 3 {
		t.Fatalf("turns = %#v, want indexes 2 and 3", turns)
	}
}

func TestSessionMessageOmitsPromptIndex(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	published := captureSessionMessageEvents(t, c)

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	last := lastPublishedEvent(t, *published, "registry.session.message")
	if _, ok := last["promptIndex"]; ok {
		t.Fatalf("published payload unexpectedly contains promptIndex: %+v", last)
	}
}

func TestSessionTurnsUseSessionGlobalTurnIndexAcrossPrompts(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	seedPromptWithTurns(t, c, ctx, "sess-1", "first", []acp.SessionUpdate{{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "one"}),
	}})
	seedPromptWithTurns(t, c, ctx, "sess-1", "second", []acp.SessionUpdate{{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "two"}),
	}})

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterTurnIndex": int64(2)})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	if got := body["latestTurnIndex"]; got != int64(6) {
		t.Fatalf("latestTurnIndex = %v, want 6", got)
	}
	turns := body["turns"].([]sessionViewTurn)
	if len(turns) != 4 {
		t.Fatalf("turns len = %d, want 4", len(turns))
	}
	for i, turn := range turns {
		want := int64(i + 3)
		if turn.TurnIndex != want {
			t.Fatalf("turns[%d].TurnIndex = %d, want %d", i, turn.TurnIndex, want)
		}
	}
}

func TestPromptBoundaryTurnsCarryModelAndTimes(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	published := captureSessionMessageEvents(t, c)
	c.sessionRecorder.modelLookup = func(sessionID string) string {
		if sessionID == "sess-1" {
			return "gpt-5.3-codex"
		}
		return ""
	}

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	startedAt := mustRFC3339Time(t, "2026-05-14T01:02:03Z")
	prompt := sessionViewPromptEvent("sess-1", "run", nil)
	prompt.UpdatedAt = startedAt
	if err := c.RecordEvent(ctx, prompt); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	finishedAt := mustRFC3339Time(t, "2026-05-14T01:02:09Z")
	done := sessionViewPromptFinishedEvent("sess-1", "end_turn")
	done.UpdatedAt = finishedAt
	if err := c.RecordEvent(ctx, done); err != nil {
		t.Fatalf("RecordEvent finished: %v", err)
	}

	var requestContent string
	var doneContent string
	for _, event := range *published {
		content := event.payload["content"].(string)
		var msg acp.IMTurnMessage
		if err := json.Unmarshal([]byte(content), &msg); err != nil {
			t.Fatalf("unmarshal content: %v", err)
		}
		switch msg.Method {
		case acp.IMMethodPromptRequest:
			requestContent = content
		case acp.IMMethodPromptDone:
			doneContent = content
		}
	}
	if requestContent == "" || doneContent == "" {
		t.Fatalf("missing prompt boundary turns request=%q done=%q", requestContent, doneContent)
	}
	requestParam := decodeTurnParamMap(t, requestContent)
	if got := requestParam["modelName"]; got != "gpt-5.3-codex" {
		t.Fatalf("prompt_request modelName = %v, want gpt-5.3-codex", got)
	}
	if got := requestParam["createdAt"]; got != "2026-05-14T01:02:03Z" {
		t.Fatalf("prompt_request createdAt = %v, want %s", got, "2026-05-14T01:02:03Z")
	}
	doneParam := decodeTurnParamMap(t, doneContent)
	if got := doneParam["completedAt"]; got != "2026-05-14T01:02:09Z" {
		t.Fatalf("prompt_done completedAt = %v, want %s", got, "2026-05-14T01:02:09Z")
	}
}

func TestSessionViewPublishMessageOmitsTurnID(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	var published map[string]any
	c.sessionRecorder.SetEventPublisher(func(method string, payload any) error {
		if method != "registry.session.message" {
			return nil
		}
		var ok bool
		published, ok = payload.(map[string]any)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]any", payload)
		}
		return nil
	})

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	if published == nil {
		t.Fatalf("expected session message event to be published")
	}
	if _, ok := published["turnId"]; ok {
		t.Fatalf("published payload unexpectedly contains turnId: %+v", published)
	}
}

func TestSessionMessagePublishesFinishedFieldInsteadOfDone(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	published := captureSessionMessageEvents(t, c)

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Finished Field")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	last := lastPublishedEvent(t, *published, "registry.session.message")
	if _, ok := last["done"]; ok {
		t.Fatalf("payload contains legacy done field: %#v", last)
	}
	if got := last["finished"]; got != true {
		t.Fatalf("finished = %v, want true", got)
	}
}

func TestPromptDoneIsPublishedAsFinishedRealTurn(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	published := captureSessionMessageEvents(t, c)

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Prompt Done Turn")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "answer"}),
	})); err != nil {
		t.Fatalf("RecordEvent answer: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "end_turn")); err != nil {
		t.Fatalf("RecordEvent done: %v", err)
	}

	last := lastPublishedEvent(t, *published, "registry.session.message")
	content := last["content"].(string)
	var msg acp.IMTurnMessage
	if err := json.Unmarshal([]byte(content), &msg); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if msg.Method != acp.IMMethodPromptDone {
		t.Fatalf("method = %q, want %q", msg.Method, acp.IMMethodPromptDone)
	}
	if got := last["finished"]; got != true {
		t.Fatalf("finished = %v, want true", got)
	}
	if got := last["turnIndex"].(int64); got != 3 {
		t.Fatalf("turnIndex = %d, want 3", got)
	}
}

func TestSessionReadReturnsPromptDoneAsFinishedTurn(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Read Prompt Done Turn")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "answer"}),
	})); err != nil {
		t.Fatalf("RecordEvent answer: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "end_turn")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	turns := body["turns"].([]sessionViewTurn)
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3", len(turns))
	}
	last := turns[2]
	if last.TurnIndex != 3 {
		t.Fatalf("prompt_done turnIndex = %d, want 3", last.TurnIndex)
	}
	if last.Finished != true {
		t.Fatalf("prompt_done finished = %v, want true", last.Finished)
	}
	var msg acp.IMTurnMessage
	if err := json.Unmarshal([]byte(last.Content), &msg); err != nil {
		t.Fatalf("unmarshal prompt_done content: %v", err)
	}
	if msg.Method != acp.IMMethodPromptDone {
		t.Fatalf("last method = %q, want %q", msg.Method, acp.IMMethodPromptDone)
	}
}

func TestDuplicatePromptDoneDoesNotOverwriteStoredTurns(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Duplicate Prompt Done")); err != nil {
		t.Fatalf("RecordEvent created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "answer"}),
	})); err != nil {
		t.Fatalf("RecordEvent answer: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "end_turn")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "end_turn")); err != nil {
		t.Fatalf("RecordEvent duplicate prompt finished: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3; turns=%+v", len(turns), turns)
	}
	if text := strings.TrimSpace(extractTextChunk(decodeTurnSessionUpdate(t, turns[1].Content).Content)); text != "answer" {
		t.Fatalf("agent text = %q, want answer", text)
	}
	if turns[2].TurnIndex != 3 || turns[2].Finished != true {
		t.Fatalf("prompt_done turn = %#v, want turnIndex=3 finished=true", turns[2])
	}
}

func TestSessionViewPublishMessageOmitsUpdateIndexAndPublishesMergedTurn(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	var published []map[string]any
	c.sessionRecorder.SetEventPublisher(func(method string, payload any) error {
		if method != "registry.session.message" {
			return nil
		}
		body, ok := payload.(map[string]any)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]any", payload)
		}
		published = append(published, body)
		return nil
	})

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Merge Publish")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "say hi", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello "}),
		Status:        "streaming",
	})); err != nil {
		t.Fatalf("RecordEvent update1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "world"}),
		Status:        "done",
	})); err != nil {
		t.Fatalf("RecordEvent update2: %v", err)
	}

	if len(published) < 3 {
		t.Fatalf("published len = %d, want at least 3", len(published))
	}
	last := published[len(published)-1]
	if _, ok := last["updateIndex"]; ok {
		t.Fatalf("published payload unexpectedly contains updateIndex: %+v", last)
	}
	if got := last["turnIndex"].(int64); got != 2 {
		t.Fatalf("published turnIndex = %d, want 2", got)
	}
	content, _ := last["content"].(string)
	if text := extractTextChunk(decodeTurnSessionUpdate(t, content).Content); text != "hello world" {
		t.Fatalf("published content text = %q, want %q", text, "hello world")
	}
}

func TestSessionReadWithoutCheckpointReturnsAllTurns(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "first", nil)); err != nil {
		t.Fatalf("RecordEvent prompt #1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "one"}),
	})); err != nil {
		t.Fatalf("RecordEvent update #1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished #1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "second", nil)); err != nil {
		t.Fatalf("RecordEvent prompt #2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "two"}),
	})); err != nil {
		t.Fatalf("RecordEvent update #2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished #2: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	rawMessages, err := json.Marshal(body["turns"])
	if err != nil {
		t.Fatalf("json.Marshal(turns): %v", err)
	}
	var messages []struct {
		SessionID string `json:"sessionId"`
		TurnIndex int64  `json:"turnIndex"`
		Content   string `json:"content"`
		Finished  bool   `json:"finished"`
	}
	if err := json.Unmarshal(rawMessages, &messages); err != nil {
		t.Fatalf("json.Unmarshal(turns): %v", err)
	}
	if len(messages) != 6 {
		t.Fatalf("turns len = %d, want 6", len(messages))
	}
	for i, message := range messages {
		if message.TurnIndex != int64(i+1) {
			t.Fatalf("turns[%d].TurnIndex = %d, want %d", i, message.TurnIndex, i+1)
		}
	}
}

func TestSessionReadAfterTurnIndexReturnsLaterTurns(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "first", nil)); err != nil {
		t.Fatalf("RecordEvent prompt #1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "one"}),
	})); err != nil {
		t.Fatalf("RecordEvent update #1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished #1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "second", nil)); err != nil {
		t.Fatalf("RecordEvent prompt #2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "two"}),
	})); err != nil {
		t.Fatalf("RecordEvent update #2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished #2: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterTurnIndex": 2})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	rawMessages, err := json.Marshal(body["turns"])
	if err != nil {
		t.Fatalf("json.Marshal(turns): %v", err)
	}
	var messages []struct {
		TurnIndex int64  `json:"turnIndex"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(rawMessages, &messages); err != nil {
		t.Fatalf("json.Unmarshal(messages): %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("turns len = %d, want 4", len(messages))
	}
	for i, message := range messages {
		want := int64(i + 3)
		if message.TurnIndex != want {
			t.Fatalf("turns[%d].TurnIndex = %d, want %d", i, message.TurnIndex, want)
		}
	}
}

func TestSessionRecorderResetPromptStateRestartsTurnIndexWhenNothingPersisted(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "first", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	c.sessionRecorder.ResetPromptState()

	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "second", nil)); err != nil {
		t.Fatalf("RecordEvent prompt after reset: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns len = %d, want 1", len(turns))
	}
	if turns[0].TurnIndex != 1 {
		t.Fatalf("turns[0].TurnIndex = %d, want 1", turns[0].TurnIndex)
	}
}

func TestSessionViewListPreservesStoredProjectionMetadataForRuntimeSessions(t *testing.T) {
	c := newSessionViewTestClient(t)

	ctx := context.Background()
	lastActiveAt := mustRFC3339Time(t, "2026-04-12T10:05:00Z")
	if err := c.store.SaveSession(ctx, &SessionRecord{
		ID:          "sess-runtime-1",
		ProjectName: "proj1",
		Status:      SessionSuspended,
		AgentType:   "claude",
		AgentJSON:   `{"title":"Persisted Title"}`,
		Title:       "Persisted Title",

		CreatedAt:    mustRFC3339Time(t, "2026-04-12T10:00:00Z"),
		LastActiveAt: lastActiveAt,
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	runtimeLastActiveAt := time.Now().UTC()
	addRuntimeSession(
		c,
		"sess-runtime-1",
		"Runtime Session",
		"claude",
		mustRFC3339Time(t, "2026-04-12T10:00:00Z"),
		runtimeLastActiveAt,
	)

	sessions, err := c.listSessionViews(ctx)
	if err != nil {
		t.Fatalf("listSessionViews: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if sessions[0].Title != "Persisted Title" {
		t.Fatalf("sessions[0].Title = %q, want %q", sessions[0].Title, "Persisted Title")
	}
	if sessions[0].UpdatedAt != lastActiveAt.Format(time.RFC3339) {
		t.Fatalf("sessions[0].UpdatedAt = %q, want %q", sessions[0].UpdatedAt, lastActiveAt.Format(time.RFC3339))
	}
}

func TestSessionViewPreservesUserImageBlocks(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), sessionViewCreatedEvent("sess-1", "Images")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewPromptEvent("sess-1", "Sent an image", []acp.ContentBlock{{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "abc123"}})); err != nil {
		t.Fatalf("RecordEvent user image message: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(context.Background(), "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns len = %d, want 1", len(turns))
	}

	promptMessage := acp.IMTurnMessage{}
	if err := json.Unmarshal([]byte(turns[0].Content), &promptMessage); err != nil {
		t.Fatalf("unmarshal prompt message: %v", err)
	}
	var promptDoc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(turns[0].Content), &promptDoc); err != nil {
		t.Fatalf("unmarshal prompt message doc: %v", err)
	}
	if strings.TrimSpace(promptMessage.Method) != acp.IMMethodPromptRequest {
		t.Fatalf("messages[0].method = %q, want %q", promptMessage.Method, acp.IMMethodPromptRequest)
	}
	if _, ok := promptDoc["session"]; ok {
		t.Fatalf("messages[0].content unexpectedly contains session field")
	}
	if _, ok := promptDoc["index"]; ok {
		t.Fatalf("messages[0].content unexpectedly contains index field")
	}
	promptRequest := acp.IMPromptRequest{}
	if err := json.Unmarshal(promptMessage.Param, &promptRequest); err != nil {
		t.Fatalf("unmarshal prompt request: %v", err)
	}
	if len(promptRequest.ContentBlocks) != 1 || promptRequest.ContentBlocks[0].Type != acp.ContentBlockTypeImage {
		t.Fatalf("messages[0].request.contentBlocks = %#v, want image block", promptRequest.ContentBlocks)
	}
}

func TestSessionRecorderPromptTimingUpdatesSessionSummaryAndDuration(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	createdAt := mustRFC3339Time(t, "2026-05-06T12:00:00Z")
	startedAt := mustRFC3339Time(t, "2026-05-06T12:01:02Z")
	finishedAt := mustRFC3339Time(t, "2026-05-06T12:01:27Z")

	created := sessionViewCreatedEvent("sess-1", "Timing")
	created.UpdatedAt = createdAt
	if err := c.RecordEvent(ctx, created); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}

	started := sessionViewPromptEvent("sess-1", "measure", nil)
	started.UpdatedAt = startedAt
	if err := c.RecordEvent(ctx, started); err != nil {
		t.Fatalf("RecordEvent prompt started: %v", err)
	}

	finished := sessionViewPromptFinishedEvent("sess-1", "end_turn")
	finished.UpdatedAt = finishedAt
	if err := c.RecordEvent(ctx, finished); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	rec, err := c.store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if rec == nil {
		t.Fatal("LoadSession returned nil record")
	}
	if !rec.LastActiveAt.Equal(finishedAt) {
		t.Fatalf("session LastActiveAt = %q, want %q", rec.LastActiveAt.Format(time.RFC3339Nano), finishedAt.Format(time.RFC3339Nano))
	}

	sessions, err := c.listSessionViews(ctx)
	if err != nil {
		t.Fatalf("listSessionViews: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if sessions[0].UpdatedAt != finishedAt.Format(time.RFC3339) {
		t.Fatalf("summary UpdatedAt = %q, want %q", sessions[0].UpdatedAt, finishedAt.Format(time.RFC3339))
	}

	turns := listRecordedPromptTurns(ctx, t, c, "sess-1", 1)
	if len(turns) != 2 {
		t.Fatalf("turns len = %d, want 2", len(turns))
	}
	startParam := decodeTurnParamMap(t, turns[0])
	if got := startParam["createdAt"]; got != startedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("prompt_request createdAt = %v, want %s", got, startedAt.UTC().Format(time.RFC3339Nano))
	}
	doneParam := decodeTurnParamMap(t, turns[1])
	if got := doneParam["completedAt"]; got != finishedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("prompt_done completedAt = %v, want %s", got, finishedAt.UTC().Format(time.RFC3339Nano))
	}
}

func TestSessionSummaryExposesRunningDoneAndReadState(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Status")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt started: %v", err)
	}

	runningSessions, err := c.listSessionViews(ctx)
	if err != nil {
		t.Fatalf("list running sessions: %v", err)
	}
	if len(runningSessions) != 1 {
		t.Fatalf("running sessions len = %d, want 1", len(runningSessions))
	}
	runningSummary := sessionSummaryMap(t, runningSessions[0])
	if got := runningSummary["running"]; got != true {
		t.Fatalf("running summary running = %#v, want true", got)
	}
	if got := runningSummary["lastDoneTurnIndex"]; got != float64(0) {
		t.Fatalf("running summary lastDoneTurnIndex = %#v, want 0", got)
	}
	if got := runningSummary["lastReadTurnIndex"]; got != float64(0) {
		t.Fatalf("running summary lastReadTurnIndex = %#v, want 0", got)
	}

	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "failed")); err != nil {
		t.Fatalf("RecordEvent prompt failed: %v", err)
	}

	doneSessions, err := c.listSessionViews(ctx)
	if err != nil {
		t.Fatalf("list done sessions: %v", err)
	}
	doneSummary := sessionSummaryMap(t, doneSessions[0])
	if got := doneSummary["running"]; got != false {
		t.Fatalf("done summary running = %#v, want false", got)
	}
	if got := doneSummary["lastDoneTurnIndex"]; got != float64(2) {
		t.Fatalf("done summary lastDoneTurnIndex = %#v, want 2", got)
	}
	if got := doneSummary["lastDoneSuccess"]; got != false {
		t.Fatalf("done summary lastDoneSuccess = %#v, want false", got)
	}
	if got := doneSummary["lastReadTurnIndex"]; got != float64(0) {
		t.Fatalf("done summary lastReadTurnIndex = %#v, want 0", got)
	}
}

func TestHandleSessionRequestMarkReadPersistsReadCursor(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Read Cursor")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "end_turn")); err != nil {
		t.Fatalf("RecordEvent finished: %v", err)
	}

	resp, err := c.HandleSessionRequest(ctx, "session.markRead", "proj1", []byte(`{"sessionId":"sess-1","lastReadTurnIndex":2}`))
	if err != nil {
		t.Fatalf("session.markRead: %v", err)
	}
	body := resp.(map[string]any)
	if got := body["ok"]; got != true {
		t.Fatalf("session.markRead ok = %#v, want true", got)
	}
	session := sessionSummaryMap(t, body["session"])
	if got := session["lastReadTurnIndex"]; got != float64(2) {
		t.Fatalf("marked summary lastReadTurnIndex = %#v, want 2", got)
	}

	if _, err := c.HandleSessionRequest(ctx, "session.markRead", "proj1", []byte(`{"sessionId":"sess-1","lastReadTurnIndex":1}`)); err != nil {
		t.Fatalf("session.markRead lower cursor: %v", err)
	}
	sessions, err := c.listSessionViews(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	summary := sessionSummaryMap(t, sessions[0])
	if got := summary["lastReadTurnIndex"]; got != float64(2) {
		t.Fatalf("summary lastReadTurnIndex after lower mark = %#v, want 2", got)
	}

	rec, err := c.store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	var projection map[string]any
	if err := json.Unmarshal([]byte(rec.SessionSyncJSON), &projection); err != nil {
		t.Fatalf("unmarshal projection: %v", err)
	}
	if got := projection["lastReadTurnIndex"]; got != float64(2) {
		t.Fatalf("projection lastReadTurnIndex = %#v, want 2", got)
	}
}

func TestSessionRecorderPromptFinishWithoutLiveStateSeedsPromptTimes(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	createdAt := mustRFC3339Time(t, "2026-05-06T13:00:00Z")
	finishedAt := mustRFC3339Time(t, "2026-05-06T13:00:12Z")

	created := sessionViewCreatedEvent("sess-1", "Timing Fallback")
	created.UpdatedAt = createdAt
	if err := c.RecordEvent(ctx, created); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}

	finished := sessionViewPromptFinishedEvent("sess-1", "end_turn")
	finished.UpdatedAt = finishedAt
	if err := c.RecordEvent(ctx, finished); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns len = %d, want 1", len(turns))
	}
	doneParam := decodeTurnParamMap(t, turns[0].Content)
	if got := doneParam["completedAt"]; got != finishedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("prompt_done completedAt = %v, want %s", got, finishedAt.UTC().Format(time.RFC3339Nano))
	}
}

func TestSessionPersistKeepsRecorderLastActiveAtAndStoresLocalOffset(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.FixedZone("UTC+8", 8*60*60)
	defer func() {
		time.Local = originalLocal
	}()

	c := newSessionViewTestClient(t)
	ctx := context.Background()

	createdAt := time.Date(2026, 5, 6, 22, 10, 33, 582878300, time.Local)
	startedAt := time.Date(2026, 5, 6, 22, 13, 35, 544978800, time.Local)
	finishedAt := time.Date(2026, 5, 6, 22, 31, 35, 451831900, time.Local)

	created := sessionViewCreatedEvent("sess-1", "Timing")
	created.UpdatedAt = createdAt
	if err := c.RecordEvent(ctx, created); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}

	sess, err := c.SessionByID(ctx, "sess-1")
	if err != nil {
		t.Fatalf("SessionByID: %v", err)
	}

	started := sessionViewPromptEvent("sess-1", "measure", nil)
	started.UpdatedAt = startedAt
	if err := c.RecordEvent(ctx, started); err != nil {
		t.Fatalf("RecordEvent prompt started: %v", err)
	}

	finished := sessionViewPromptFinishedEvent("sess-1", "end_turn")
	finished.UpdatedAt = finishedAt
	if err := c.RecordEvent(ctx, finished); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	sess.mu.Lock()
	sess.lastActiveAt = createdAt
	sess.mu.Unlock()
	if err := sess.persistSession(ctx); err != nil {
		t.Fatalf("persistSession: %v", err)
	}

	rec, err := c.store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if rec == nil {
		t.Fatal("LoadSession returned nil record")
	}
	if !rec.LastActiveAt.Equal(finishedAt) {
		t.Fatalf("session LastActiveAt = %q, want %q", rec.LastActiveAt.Format(time.RFC3339Nano), finishedAt.Format(time.RFC3339Nano))
	}

	sqliteStore, ok := c.store.(*sqliteStore)
	if !ok {
		t.Fatalf("store type = %T, want *sqliteStore", c.store)
	}
	var rawLastActiveAt string
	if err := sqliteStore.db.QueryRowContext(ctx, `SELECT updated_at FROM sessions WHERE id = ?`, "sess-1").Scan(&rawLastActiveAt); err != nil {
		t.Fatalf("QueryRowContext session updated_at: %v", err)
	}
	if !strings.HasSuffix(rawLastActiveAt, "+08:00") {
		t.Fatalf("raw session last_active_at = %q, want +08:00 offset", rawLastActiveAt)
	}
}

func TestSessionViewPersistsLegacySystemEventsButIgnoresACPSystemEvents(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "System Events")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "start", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewACPSystemEvent("sess-1", "from acp")); err != nil {
		t.Fatalf("RecordEvent acp system: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewSystemEvent("sess-1", "from legacy")); err != nil {
		t.Fatalf("RecordEvent legacy system: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	turns := listRecordedPromptTurns(ctx, t, c, "sess-1", 1)
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3 (prompt + legacy system + prompt_done)", len(turns))
	}

	msg := acp.IMTurnMessage{}
	if err := json.Unmarshal([]byte(turns[1]), &msg); err != nil {
		t.Fatalf("unmarshal legacy system turn: %v", err)
	}
	if strings.TrimSpace(msg.Method) != acp.IMMethodSystem {
		t.Fatalf("legacy system turn method = %q, want %q", msg.Method, acp.IMMethodSystem)
	}
	result := acp.IMTextResult{}
	if err := json.Unmarshal(msg.Param, &result); err != nil {
		t.Fatalf("unmarshal legacy system result: %v", err)
	}
	if strings.TrimSpace(result.Text) != "from legacy" {
		t.Fatalf("legacy system text = %q, want %q", result.Text, "from legacy")
	}
}

func TestPromptTitleFromBlocks(t *testing.T) {
	if got := promptTitleFromBlocks([]acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: " hello "}}); got != "hello" {
		t.Fatalf("promptTitleFromBlocks(text) = %q, want %q", got, "hello")
	}
	if got := promptTitleFromBlocks([]acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: " first "}, {Type: acp.ContentBlockTypeText, Text: "second"}}); got != "first\nsecond" {
		t.Fatalf("promptTitleFromBlocks(multi-text) = %q, want %q", got, "first\nsecond")
	}
	if got := promptTitleFromBlocks([]acp.ContentBlock{{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "abc123"}}); got != "Sent an image" {
		t.Fatalf("promptTitleFromBlocks(image) = %q, want %q", got, "Sent an image")
	}
	if got := promptTitleFromBlocks(nil); got != "" {
		t.Fatalf("promptTitleFromBlocks(nil) = %q, want empty", got)
	}
}

func TestSessionRecorderUsesClientSessionIDWhenACPEventCarriesDifferentSessionID(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("client-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}

	promptEvent := SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: "client-1",
		Content: acp.BuildACPContentJSON(acp.MethodSessionPrompt, map[string]any{
			"params": acp.SessionPromptParams{
				SessionID: "acp-1",
				Prompt:    []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: "run"}},
			},
		}),
	}
	if err := c.RecordEvent(ctx, promptEvent); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	updateEvent := SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: "client-1",
		Content: acp.BuildACPContentJSON(acp.MethodSessionUpdate, map[string]any{
			"params": acp.SessionUpdateParams{
				SessionID: "acp-1",
				Update: acp.SessionUpdate{
					SessionUpdate: acp.SessionUpdateAgentMessageChunk,
					Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"}),
				},
			},
		}),
	}
	if err := c.RecordEvent(ctx, updateEvent); err != nil {
		t.Fatalf("RecordEvent update: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "client-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns(client-1): %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("client turns len = %d, want 2", len(turns))
	}

	if _, _, err := c.sessionRecorder.ReadSessionTurns(ctx, "acp-1", 0); err == nil {
		t.Fatalf("ReadSessionTurns(acp-1) unexpectedly succeeded")
	}
}

func TestSessionViewToolUpdatesReuseSingleMessage(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Tools")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run build", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewToolUpdatedTextEvent("sess-1", "Running build")); err != nil {
		t.Fatalf("RecordEvent tool updated #1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewToolUpdatedTextEvent("sess-1", "Build finished")); err != nil {
		t.Fatalf("RecordEvent tool updated #2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	turns := listRecordedPromptTurns(ctx, t, c, "sess-1", 1)
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3", len(turns))
	}
	if got := decodeTurnSessionUpdate(t, turns[1]).Title; strings.TrimSpace(got) != "Build finished" {
		t.Fatalf("turns[1].title = %q, want Build finished", got)
	}
}

func TestSessionViewPersistsSessionUpdateParamsPayload(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Raw Update")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "say hi", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	updateChunk1 := acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"}),
	}
	updateChunk2 := acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(map[string]any{"text": " world"}),
	}

	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", updateChunk1)); err != nil {
		t.Fatalf("RecordEvent chunk #1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", updateChunk2)); err != nil {
		t.Fatalf("RecordEvent chunk #2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	stored := listRecordedPromptTurns(ctx, t, c, "sess-1", 1)
	if len(stored) != 3 {
		t.Fatalf("stored len = %d, want 3", len(stored))
	}
	updateStored := decodeTurnSessionUpdate(t, stored[1])
	if strings.TrimSpace(updateStored.SessionUpdate) != acp.SessionUpdateAgentMessageChunk {
		t.Fatalf("stored assistant update kind = %q, want %q", updateStored.SessionUpdate, acp.SessionUpdateAgentMessageChunk)
	}
	if text := strings.TrimSpace(extractTextChunk(updateStored.Content)); text != "hello world" {
		t.Fatalf("stored assistant text = %q, want hello world", text)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	rawMessages, err := json.Marshal(body["turns"])
	if err != nil {
		t.Fatalf("json.Marshal(turns): %v", err)
	}
	var messages []struct {
		TurnIndex int64  `json:"turnIndex"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(rawMessages, &messages); err != nil {
		t.Fatalf("json.Unmarshal(turns): %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("turns len = %d, want 3", len(messages))
	}
	if messages[1].TurnIndex != 2 {
		t.Fatalf("messages[1].TurnIndex = %d, want 2", messages[1].TurnIndex)
	}
	update2 := decodeTurnSessionUpdate(t, messages[1].Content)
	if strings.TrimSpace(extractTextChunk(update2.Content)) != "hello world" {
		t.Fatalf("messages[1] text = %q, want hello world", extractTextChunk(update2.Content))
	}
}

func TestSessionViewSessionUpdateMergeUsesACPUpdateType(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Merge")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	userChunk := acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateUserMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "user says hi"}),
	}
	agentChunk := acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "assistant says hi"}),
	}

	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", userChunk)); err != nil {
		t.Fatalf("RecordEvent user chunk: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", agentChunk)); err != nil {
		t.Fatalf("RecordEvent agent chunk: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 4 {
		t.Fatalf("turns len = %d, want 4; turns=%+v", len(turns), turns)
	}
	seen := map[string]string{}
	for _, turn := range turns {
		if decodeTurnMethod(t, turn.Content) == acp.MethodSessionPrompt {
			continue
		}
		update := decodeTurnSessionUpdate(t, turn.Content)
		seen[update.SessionUpdate] = extractTextChunk(update.Content)
	}
	if got := seen[acp.SessionUpdateUserMessageChunk]; got != "user says hi" {
		t.Fatalf("user chunk text = %q, want %q", got, "user says hi")
	}
	if got := seen[acp.SessionUpdateAgentMessageChunk]; got != "assistant says hi" {
		t.Fatalf("assistant chunk text = %q, want %q", got, "assistant says hi")
	}
}

func TestSessionViewKeepsUserMessageChunkTurn(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "User Chunk")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateUserMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "user says hi"}),
	})); err != nil {
		t.Fatalf("RecordEvent user chunk: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3; turns=%+v", len(turns), turns)
	}
	if got := decodeTurnMethod(t, turns[1].Content); got != acp.SessionUpdateUserMessageChunk {
		t.Fatalf("turns[1] method = %q, want %q", got, acp.SessionUpdateUserMessageChunk)
	}
}

func TestParseSessionViewEventUserMessageChunkAsMessage(t *testing.T) {
	event := sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateUserMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "user says hi"}),
	})
	parsed, err := parseSessionViewEvent(event)
	if err != nil {
		t.Fatalf("parseSessionViewEvent: %v", err)
	}
	if !parsed.bMessage {
		t.Fatal("parsed.bMessage = false, want true")
	}
	if parsed.method != acp.SessionUpdateUserMessageChunk {
		t.Fatalf("parsed.method = %q, want %q", parsed.method, acp.SessionUpdateUserMessageChunk)
	}
}

func TestSessionViewSystemMessageIsNotPersisted(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "No System")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewSystemEvent("sess-1", "max_output_tokens")); err != nil {
		t.Fatalf("RecordEvent system message: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 0 {
		t.Fatalf("turns len = %d, want 0", len(turns))
	}
}

func TestSessionViewUpdateWithoutPromptIsDropped(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "No Prompt")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"}),
	})); err != nil {
		t.Fatalf("RecordEvent update: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 0 {
		t.Fatalf("turns len = %d, want 0", len(turns))
	}
}

func TestSessionViewMergedTurnPublishesIncomingContentWithMergedIndices(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	var published []map[string]any
	c.sessionRecorder.SetEventPublisher(func(method string, payload any) error {
		if method != "registry.session.message" {
			return nil
		}
		body, ok := payload.(map[string]any)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]any", payload)
		}
		published = append(published, body)
		return nil
	})

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Merge Publish")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "say hi", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"}),
		Status:        "streaming",
	})); err != nil {
		t.Fatalf("RecordEvent update1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "world"}),
		Status:        "done",
	})); err != nil {
		t.Fatalf("RecordEvent update2: %v", err)
	}

	if len(published) < 3 {
		t.Fatalf("published len = %d, want at least 3", len(published))
	}
	last := published[len(published)-1]
	if got := last["turnIndex"].(int64); got != 2 {
		t.Fatalf("published turnIndex = %d, want 2", got)
	}
	if _, ok := last["updateIndex"]; ok {
		t.Fatalf("published payload unexpectedly contains updateIndex: %+v", last)
	}
	content, _ := last["content"].(string)
	if text := extractTextChunk(decodeTurnSessionUpdate(t, content).Content); text != "helloworld" {
		t.Fatalf("published content text = %q, want helloworld", text)
	}
}

func TestSessionViewPromptFinishedPublishesPromptDoneMessage(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	var published []map[string]any
	c.sessionRecorder.SetEventPublisher(func(method string, payload any) error {
		if method != "registry.session.message" {
			return nil
		}
		body, ok := payload.(map[string]any)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]any", payload)
		}
		published = append(published, body)
		return nil
	})

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Prompt Done Publish")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewAssistantChunkTextEvent("sess-1", "hello", "streaming")); err != nil {
		t.Fatalf("RecordEvent update: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", acp.StopReasonEndTurn)); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	if len(published) < 3 {
		t.Fatalf("published len = %d, want at least 3", len(published))
	}
	last := published[len(published)-1]
	if _, ok := last["promptIndex"]; ok {
		t.Fatalf("published payload unexpectedly contains promptIndex: %+v", last)
	}
	if got := last["turnIndex"].(int64); got != 3 {
		t.Fatalf("published turnIndex = %d, want 3", got)
	}
	content, _ := last["content"].(string)
	msg := acp.IMTurnMessage{}
	if err := json.Unmarshal([]byte(content), &msg); err != nil {
		t.Fatalf("unmarshal prompt_done content: %v", err)
	}
	if strings.TrimSpace(msg.Method) != acp.IMMethodPromptDone {
		t.Fatalf("published method = %q, want %q", msg.Method, acp.IMMethodPromptDone)
	}
	result := acp.IMPromptResult{}
	if err := json.Unmarshal(msg.Param, &result); err != nil {
		t.Fatalf("unmarshal prompt_done param: %v", err)
	}
	if strings.TrimSpace(result.StopReason) != acp.StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want %q", result.StopReason, acp.StopReasonEndTurn)
	}
}

func TestSessionViewPromptFinishedMarksOpenTextTurnDone(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	var published []map[string]any
	c.sessionRecorder.SetEventPublisher(func(method string, payload any) error {
		if method != "registry.session.message" {
			return nil
		}
		body, ok := payload.(map[string]any)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]any", payload)
		}
		published = append(published, body)
		return nil
	})

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Prompt Done Turn")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewAssistantChunkTextEvent("sess-1", "hello", "streaming")); err != nil {
		t.Fatalf("RecordEvent update: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", acp.StopReasonEndTurn)); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	if len(published) < 4 {
		t.Fatalf("published len = %d, want at least 4", len(published))
	}
	doneTurn := published[len(published)-2]
	if got := doneTurn["finished"]; got != true {
		t.Fatalf("finished turn marker = %v, want true", got)
	}
	if got := doneTurn["turnIndex"].(int64); got != 2 {
		t.Fatalf("finished turnIndex = %d, want 2", got)
	}
	content, _ := doneTurn["content"].(string)
	update := decodeTurnSessionUpdate(t, content)
	if strings.TrimSpace(update.SessionUpdate) != acp.SessionUpdateAgentMessageChunk {
		t.Fatalf("finished method = %q, want %q", update.SessionUpdate, acp.SessionUpdateAgentMessageChunk)
	}
	if text := extractTextChunk(update.Content); text != "hello" {
		t.Fatalf("finished text = %q, want hello", text)
	}
}

func TestSessionViewReadSkipsPermissionRequestTurns(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewPromptEvent("sess-1", "run protected", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"}),
		Status:        "done",
	})); err != nil {
		t.Fatalf("RecordEvent update: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewPermissionRequestedEvent("sess-1", "Run tool?", 42, nil)); err != nil {
		t.Fatalf("RecordEvent permission requested: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(context.Background(), "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	messages := body["turns"].([]sessionViewTurn)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[0].TurnIndex != 1 {
		t.Fatalf("messages[0] = %#v, want turnIndex=1", messages[0])
	}
	if method := decodeTurnMethod(t, messages[1].Content); method != acp.SessionUpdateAgentMessageChunk {
		t.Fatalf("messages[1] method = %q, want %q", method, acp.SessionUpdateAgentMessageChunk)
	}
}

func TestSessionViewReadReturnsMergedStreamingTurn(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), sessionViewCreatedEvent("sess-1", "Stream")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewAssistantChunkTextEvent("sess-1", "hello", "streaming")); err != nil {
		t.Fatalf("RecordEvent chunk: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(context.Background(), "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	messages := body["turns"].([]sessionViewTurn)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[1].TurnIndex != 2 {
		t.Fatalf("messages[1].TurnIndex = %d, want 2", messages[1].TurnIndex)
	}
	update := decodeTurnSessionUpdate(t, messages[1].Content)
	if text := strings.TrimSpace(extractTextChunk(update.Content)); text != "hello" {
		t.Fatalf("message text = %q, want hello", text)
	}
}

func TestSessionViewBufferedUpdatesReusePreviousTurnByUpdateType(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Merge Cursor")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "say hi", nil)); err != nil {
		t.Fatalf("RecordEvent user message: %v", err)
	}
	chunk1 := acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello "}),
		Status:        "streaming",
	}
	chunk2 := acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "world"}),
		Status:        "done",
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", chunk1)); err != nil {
		t.Fatalf("RecordEvent chunk1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", chunk2)); err != nil {
		t.Fatalf("RecordEvent chunk2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	turns := listRecordedPromptTurns(ctx, t, c, "sess-1", 1)
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3 (prompt + merged assistant turn + prompt_done)", len(turns))
	}
	update2 := decodeTurnSessionUpdate(t, turns[1])
	if text := strings.TrimSpace(extractTextChunk(update2.Content)); text != "hello world" {
		t.Fatalf("assistant text = %q, want hello world", text)
	}
}

func TestSessionReadMarksCompletedTextTurnsDone(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Read Done")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent user message: %v", err)
	}
	msg := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "answer"}), Status: "streaming"}
	thought := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentThoughtChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "reason"}), Status: "streaming"}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", msg)); err != nil {
		t.Fatalf("RecordEvent message update: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", thought)); err != nil {
		t.Fatalf("RecordEvent thought update: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	messages := body["turns"].([]sessionViewTurn)
	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	if messages[1].Finished != true {
		t.Fatalf("assistant finished = %v, want true", messages[1].Finished)
	}
	if messages[2].Finished != true {
		t.Fatalf("thought finished = %v, want true", messages[2].Finished)
	}
}

func TestSessionReadDerivesRoleAndKindFromACPUpdateTypes(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Role Kind")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run", nil)); err != nil {
		t.Fatalf("RecordEvent user message: %v", err)
	}
	msg := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "answer"}), Status: "streaming"}
	thought := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentThoughtChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "reason"}), Status: "streaming"}
	tool := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateToolCall, ToolCallID: "call-1", Status: "in_progress", Title: "build"}

	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", msg)); err != nil {
		t.Fatalf("RecordEvent message update: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", thought)); err != nil {
		t.Fatalf("RecordEvent thought update: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", tool)); err != nil {
		t.Fatalf("RecordEvent tool update: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	messages := body["turns"].([]sessionViewTurn)
	if len(messages) != 5 {
		t.Fatalf("messages len = %d, want 5", len(messages))
	}

	seen := map[string]bool{}
	for _, msg := range messages {
		if decodeTurnMethod(t, msg.Content) == acp.MethodSessionPrompt {
			seen["prompt"] = true
			continue
		}
		update := decodeTurnSessionUpdate(t, msg.Content)
		if strings.TrimSpace(update.SessionUpdate) != "" {
			seen[update.SessionUpdate] = true
		}
	}
	if !seen["prompt"] {
		t.Fatalf("missing prompt message, messages=%+v", messages)
	}
	if !seen[acp.SessionUpdateAgentMessageChunk] {
		t.Fatalf("missing agent message chunk, messages=%+v", messages)
	}
	if !seen[acp.SessionUpdateAgentThoughtChunk] {
		t.Fatalf("missing agent thought chunk, messages=%+v", messages)
	}
	if !seen[acp.SessionUpdateToolCallUpdate] {
		t.Fatalf("missing tool call update, messages=%+v", messages)
	}
}
func TestHandleSessionRequestMarkReadRequiresSessionID(t *testing.T) {
	c := newSessionViewTestClient(t)
	_, err := c.HandleSessionRequest(context.Background(), "session.markRead", "proj1", []byte(`{"lastReadTurnIndex":1}`))
	if err == nil {
		t.Fatalf("expected session.markRead to require sessionId")
	}
}

func TestHandleSessionRequestSessionDeleteRemovesSessionAndPrompts(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	now := time.Now().UTC()
	addRuntimeSession(c, "sess-1", "Delete Target", "claude", now, now)
	c.mu.Lock()
	c.routeMap["im:app:chat-1"] = "sess-1"
	c.mu.Unlock()

	c.sessionRecorder.writeMu.Lock()
	c.sessionRecorder.promptState["sess-1"] = &sessionPromptState{nextTurnIndex: 2}
	c.sessionRecorder.writeMu.Unlock()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Delete Target")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "world"}),
	})); err != nil {
		t.Fatalf("RecordEvent update: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	_, storedTurns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns before delete: %v", err)
	}
	if len(storedTurns) == 0 {
		t.Fatal("expected stored turns before delete")
	}

	resp, err := c.HandleSessionRequest(ctx, "session.delete", "proj1", json.RawMessage(`{"sessionId":"  sess-1  "}`))
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.delete): %v", err)
	}
	body, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("session.delete response type = %T, want map[string]any", resp)
	}
	if body["ok"] != true || body["sessionId"] != "sess-1" {
		t.Fatalf("unexpected session.delete response body: %#v", body)
	}

	c.mu.Lock()
	_, inMemory := c.sessions["sess-1"]
	_, routeMapped := c.routeMap["im:app:chat-1"]
	c.mu.Unlock()
	if inMemory {
		t.Fatal("session still present in memory after delete")
	}
	if routeMapped {
		t.Fatal("route binding still present after delete")
	}

	c.sessionRecorder.writeMu.Lock()
	_, hasPromptState := c.sessionRecorder.promptState["sess-1"]
	c.sessionRecorder.writeMu.Unlock()
	if hasPromptState {
		t.Fatal("prompt state still present after delete")
	}

	storedSession, err := c.store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession after delete: %v", err)
	}
	if storedSession != nil {
		t.Fatalf("stored session still exists after delete: %+v", storedSession)
	}

	if _, _, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0); err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("ReadSessionTurns err = %v, want session not found", err)
	}
}

func TestHandleSessionRequestSessionDeleteCleansAgentArtifacts(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	oldCleanup := cleanupSessionArtifacts
	var cleanupCalls []struct {
		projectName string
		agentType   string
		sessionID   string
	}
	cleanupSessionArtifacts = func(projectName, agentType, sessionID string) error {
		cleanupCalls = append(cleanupCalls, struct {
			projectName string
			agentType   string
			sessionID   string
		}{projectName: projectName, agentType: agentType, sessionID: sessionID})
		return nil
	}
	t.Cleanup(func() { cleanupSessionArtifacts = oldCleanup })

	now := time.Now().UTC()
	addRuntimeSession(c, "sess-codexapp-memory", "Delete CodexApp", "codexapp", now, now)
	if err := c.store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-codexapp-stored",
		ProjectName:  "proj1",
		Status:       SessionPersisted,
		AgentType:    "codexapp",
		AgentJSON:    `{}`,
		CreatedAt:    now,
		LastActiveAt: now,
		Title:        "Stored CodexApp",
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	for _, sessionID := range []string{"sess-codexapp-memory", "sess-codexapp-stored"} {
		if _, err := c.HandleSessionRequest(ctx, "session.delete", "proj1", json.RawMessage(fmt.Sprintf(`{"sessionId":%q}`, sessionID))); err != nil {
			t.Fatalf("HandleSessionRequest(session.delete %s): %v", sessionID, err)
		}
	}

	if len(cleanupCalls) != 2 {
		t.Fatalf("cleanup calls=%#v, want 2", cleanupCalls)
	}
	for _, call := range cleanupCalls {
		if call.projectName != "proj1" || call.agentType != "codexapp" || !strings.HasPrefix(call.sessionID, "sess-codexapp-") {
			t.Fatalf("cleanup call=%#v, want codexapp project/session", call)
		}
	}
}

func TestHandleSessionRequestSessionArchiveWritesPackAndDeletesActiveSession(t *testing.T) {
	c := newSessionViewTestClient(t)
	historyRoot := filepath.Join(t.TempDir(), "db", "session")
	c.SetSessionHistoryRoot(historyRoot)
	ctx := context.Background()

	now := time.Date(2026, 5, 17, 10, 30, 0, 0, time.UTC)
	addRuntimeSession(c, "sess-archive", "Archive Target", "claude", now, now)
	c.mu.Lock()
	c.routeMap["im:app:archive"] = "sess-archive"
	c.mu.Unlock()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-archive", "Archive Target")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-archive", "hello", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-archive", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "world"}),
	})); err != nil {
		t.Fatalf("RecordEvent update: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-archive", acp.StopReasonEndTurn)); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	sessionDir := filepath.Join(historyRoot, safeHistoryPathPart("proj1"), safeHistoryPathPart("sess-archive"))
	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("expected source session dir before archive: %v", err)
	}

	resp, err := c.HandleSessionRequest(ctx, "session.archive", "proj1", json.RawMessage(`{"sessionId":"sess-archive"}`))
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.archive): %v", err)
	}
	body, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("session.archive response type = %T, want map[string]any", resp)
	}
	if body["ok"] != true || body["sessionId"] != "sess-archive" {
		t.Fatalf("unexpected session.archive response body: %#v", body)
	}

	storedSession, err := c.store.LoadSession(ctx, "proj1", "sess-archive")
	if err != nil {
		t.Fatalf("LoadSession after archive: %v", err)
	}
	if storedSession != nil {
		t.Fatalf("stored session still exists after archive: %+v", storedSession)
	}
	c.mu.Lock()
	_, inMemory := c.sessions["sess-archive"]
	_, routeMapped := c.routeMap["im:app:archive"]
	c.mu.Unlock()
	if inMemory {
		t.Fatal("session still present in memory after archive")
	}
	if routeMapped {
		t.Fatal("route binding still present after archive")
	}
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatalf("source session dir stat err = %v, want not exist", err)
	}

	manifest := readArchiveManifestForTest(t, historyRoot, "proj1")
	entry := manifest.Sessions["sess-archive"]
	if entry.SessionID != "sess-archive" || entry.ProjectName != "proj1" || entry.Title != "hello" {
		t.Fatalf("unexpected manifest entry identity: %#v", entry)
	}
	if entry.Storage != "pack" || entry.File != "archive.pack" || entry.Codec != "gzip" {
		t.Fatalf("unexpected manifest storage fields: %#v", entry)
	}
	if entry.TurnCount != 3 || entry.GapCount != 0 || entry.WMT2Version != 2 || entry.ChunkSizeCode != 0 {
		t.Fatalf("unexpected manifest counters: %#v", entry)
	}

	rawWMT2, compressedSegment := readArchivedSessionPayloadForTest(t, historyRoot, "proj1", entry)
	if got := fmt.Sprintf("%x", sha256.Sum256(compressedSegment)); got != entry.SHA256 {
		t.Fatalf("compressed sha256 = %s, want %s", got, entry.SHA256)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(rawWMT2)); got != entry.UncompressedSHA256 {
		t.Fatalf("uncompressed sha256 = %s, want %s", got, entry.UncompressedSHA256)
	}
	contents := decodeWMT2ContentsForTest(t, rawWMT2, entry.TurnCount)
	if len(contents) != 3 {
		t.Fatalf("archive turn contents len = %d, want 3", len(contents))
	}
	if !strings.Contains(contents[0], acp.IMMethodPromptRequest) {
		t.Fatalf("first archived turn = %s, want prompt request", contents[0])
	}
	if !strings.Contains(contents[1], "world") {
		t.Fatalf("second archived turn = %s, want agent message", contents[1])
	}
	if !strings.Contains(contents[2], acp.IMMethodPromptDone) {
		t.Fatalf("third archived turn = %s, want prompt done", contents[2])
	}
}

func TestHandleSessionRequestSessionArchiveFillsMissingTurnsWithGap(t *testing.T) {
	c := newSessionViewTestClient(t)
	historyRoot := filepath.Join(t.TempDir(), "db", "session")
	c.SetSessionHistoryRoot(historyRoot)
	ctx := context.Background()

	now := time.Date(2026, 5, 17, 11, 0, 0, 0, time.UTC)
	if err := c.store.SaveSession(ctx, &SessionRecord{
		ID:              "sess-gap",
		ProjectName:     "proj1",
		Status:          SessionPersisted,
		AgentType:       "claude",
		Title:           "Gap Target",
		SessionSyncJSON: sessionSyncJSON(2),
		CreatedAt:       now,
		LastActiveAt:    now,
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	if _, err := c.HandleSessionRequest(ctx, "session.archive", "proj1", json.RawMessage(`{"sessionId":"sess-gap"}`)); err != nil {
		t.Fatalf("HandleSessionRequest(session.archive): %v", err)
	}

	manifest := readArchiveManifestForTest(t, historyRoot, "proj1")
	entry := manifest.Sessions["sess-gap"]
	if entry.TurnCount != 2 || entry.GapCount != 2 {
		t.Fatalf("manifest counters = turnCount:%d gapCount:%d, want 2/2", entry.TurnCount, entry.GapCount)
	}
	rawWMT2, _ := readArchivedSessionPayloadForTest(t, historyRoot, "proj1", entry)
	contents := decodeWMT2ContentsForTest(t, rawWMT2, entry.TurnCount)
	for i, content := range contents {
		if !strings.Contains(content, "session/archive_gap") || !strings.Contains(content, "missing_turn") {
			t.Fatalf("content[%d] = %s, want archive gap turn", i, content)
		}
	}
}

func TestHandleSessionRequestSessionMutationsRejectRunningSession(t *testing.T) {
	for _, method := range []string{"session.archive", "session.delete", "session.reload"} {
		t.Run(method, func(t *testing.T) {
			c := newSessionViewTestClient(t)
			c.SetSessionHistoryRoot(filepath.Join(t.TempDir(), "db", "session"))
			ctx := context.Background()
			now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
			if err := c.store.SaveSession(ctx, &SessionRecord{
				ID:              "sess-running",
				ProjectName:     "proj1",
				Status:          SessionPersisted,
				AgentType:       "claude",
				Title:           "Running Target",
				SessionSyncJSON: sessionSyncJSON(0),
				CreatedAt:       now,
				LastActiveAt:    now,
			}); err != nil {
				t.Fatalf("SaveSession: %v", err)
			}
			c.sessionRecorder.writeMu.Lock()
			state := newSessionPromptState(1)
			state.updateTurn(sessionTurnMessage{
				sessionID: "sess-running",
				method:    acp.IMMethodPromptRequest,
				payload:   acp.IMPromptRequest{ContentBlocks: []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: "still running"}}},
				turnIndex: 1,
				finished:  true,
			}, "")
			c.sessionRecorder.promptState["sess-running"] = &state
			c.sessionRecorder.writeMu.Unlock()

			_, err := c.HandleSessionRequest(ctx, method, "proj1", json.RawMessage(`{"sessionId":"sess-running"}`))
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "running") {
				t.Fatalf("HandleSessionRequest(%s) err = %v, want running rejection", method, err)
			}
			storedSession, loadErr := c.store.LoadSession(ctx, "proj1", "sess-running")
			if loadErr != nil {
				t.Fatalf("LoadSession after rejected mutation: %v", loadErr)
			}
			if storedSession == nil {
				t.Fatal("running session was deleted after rejected mutation")
			}
		})
	}
}

func TestHandleSessionRequestSessionReloadClearsPromptStateBeforeReplay(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 5, 9, 0, 0, 0, time.UTC)
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{
			name:      "claude",
			sessionID: "sess-reload",
			initResult: acp.InitializeResult{
				ProtocolVersion:   "0.1",
				AgentCapabilities: acp.AgentCapabilities{LoadSession: true},
			},
			loadErr: errors.New("resource not found"),
		}, nil
	})

	if err := c.store.SaveSession(ctx, &SessionRecord{
		ID:              "sess-reload",
		ProjectName:     "proj1",
		Status:          SessionPersisted,
		AgentType:       "claude",
		AgentJSON:       `{}`,
		SessionSyncJSON: sessionSyncJSON(4),
		CreatedAt:       now,
		LastActiveAt:    now,
		Title:           "Reload target",
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	c.sessionRecorder.writeMu.Lock()
	cached := newSessionPromptState(9)
	c.sessionRecorder.promptState["sess-reload"] = &cached
	c.sessionRecorder.writeMu.Unlock()

	_, err := c.HandleSessionRequest(ctx, "session.reload", "proj1", json.RawMessage(`{"sessionId":"sess-reload"}`))
	if err == nil {
		t.Fatal("expected reload to fail when replay load fails")
	}

	c.sessionRecorder.writeMu.Lock()
	_, hasPromptState := c.sessionRecorder.promptState["sess-reload"]
	c.sessionRecorder.writeMu.Unlock()
	if hasPromptState {
		t.Fatal("prompt state still present after reload failure")
	}
}

func TestHandleSessionRequestSessionReloadRecordsAgentOnlyReplay(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC)
	inst := &testInjectedInstance{
		name:      "codexapp",
		sessionID: "sess-codexapp",
		initResult: acp.InitializeResult{
			ProtocolVersion:   "0.1",
			AgentCapabilities: acp.AgentCapabilities{LoadSession: true},
		},
		loadUpdates: []acp.SessionUpdateParams{{
			SessionID: "sess-codexapp",
			Update: acp.SessionUpdate{
				SessionUpdate: acp.SessionUpdateAgentMessageChunk,
				Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "restored without user chunk"}),
			},
		}},
	}
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register(acp.ACPProviderCodexApp, func(context.Context, string) (agent.Instance, error) {
		return inst, nil
	})
	if err := c.store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-codexapp",
		ProjectName:  "proj1",
		Status:       SessionPersisted,
		AgentType:    "codexapp",
		AgentJSON:    `{}`,
		CreatedAt:    now,
		LastActiveAt: now,
		Title:        "CodexApp reload",
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	resp, err := c.HandleSessionRequest(ctx, "session.reload", "proj1", json.RawMessage(`{"sessionId":"sess-codexapp"}`))
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.reload): %v", err)
	}
	body := resp.(map[string]any)
	if body["ok"] != true {
		t.Fatalf("reload response = %#v, want ok", body)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-codexapp", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns len = %d, want restored agent message plus prompt_done, turns=%+v", len(turns), turns)
	}
	if !strings.Contains(turns[0].Content, "restored without user chunk") {
		t.Fatalf("turn content = %s, want restored agent text", turns[0].Content)
	}
}

func TestHandleSessionRequestSessionResumeListSupportsCodexApp(t *testing.T) {
	cwd := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	writeCodexSessionFixture(t, homeDir, "sess-codexapp-resume", cwd, "Resume CodexApp", "assistant preview")

	c := newSessionViewTestClient(t)
	c.cwd = cwd

	resp, err := c.HandleSessionRequest(context.Background(), "session.resume.list", "proj1", json.RawMessage(`{"agentType":"codexapp"}`))
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.resume.list): %v", err)
	}
	body, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("response type = %T, want map", resp)
	}
	sessions, ok := body["sessions"].([]recoverySession)
	if !ok {
		t.Fatalf("sessions type = %T, want []recoverySession", body["sessions"])
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if sessions[0].SessionID != "sess-codexapp-resume" || sessions[0].AgentType != "codexapp" {
		t.Fatalf("session = %+v, want codexapp resumable session", sessions[0])
	}

	importResp, err := c.HandleSessionRequest(context.Background(), "session.resume.import", "proj1", json.RawMessage(`{"agentType":"codexapp","sessionId":"sess-codexapp-resume"}`))
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.resume.import): %v", err)
	}
	importBody := importResp.(map[string]any)
	if importBody["ok"] != true {
		t.Fatalf("import response = %#v, want ok", importBody)
	}
	rec, err := c.store.LoadSession(context.Background(), "proj1", "sess-codexapp-resume")
	if err != nil {
		t.Fatalf("LoadSession imported: %v", err)
	}
	if rec == nil || rec.AgentType != "codexapp" {
		t.Fatalf("imported record = %+v, want codexapp agent", rec)
	}
}

func TestHandleSessionRequestSessionResumeImportRejectsAlreadyManagedClaudeSession(t *testing.T) {
	cwd := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	writeClaudeSessionFixture(t, homeDir, "wheelmaker", "sess-dup", cwd, "Resume me", "assistant preview")

	c := newSessionViewTestClient(t)
	c.cwd = cwd
	ctx := context.Background()
	now := time.Date(2026, 5, 5, 9, 5, 0, 0, time.UTC)

	if err := c.store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-dup",
		ProjectName:  "proj1",
		Status:       SessionPersisted,
		AgentType:    "claude",
		AgentJSON:    `{}`,
		CreatedAt:    now,
		LastActiveAt: now,
		Title:        "Already managed",
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	_, err := c.HandleSessionRequest(ctx, "session.resume.import", "proj1", json.RawMessage(`{"agentType":"claude","sessionId":"sess-dup"}`))
	if err == nil {
		t.Fatal("expected duplicate managed session import to fail")
	}
	if !strings.Contains(err.Error(), "already managed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleSessionRequest_SessionNewRequiresAgentType(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	c := New(store, "proj1", "/tmp")
	_, err = c.HandleSessionRequest(context.Background(), "session.new", "proj1", json.RawMessage(`{"title":"hello"}`))
	if err == nil || !strings.Contains(err.Error(), "agentType is required") {
		t.Fatalf("HandleSessionRequest() err = %v, want agentType is required", err)
	}
}

func TestHandleSessionRequest_SessionNewPersistsProjectDefaultAgent(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	c := New(store, "proj1", "/tmp")
	inst := &testInjectedInstance{name: "claude", initResult: acp.InitializeResult{ProtocolVersion: "0.1"}, newResult: &acp.SessionNewResult{SessionID: "sess-1"}}
	c.registry = agent.DefaultACPFactory().Clone()
	c.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) { return inst, nil })

	_, err = c.HandleSessionRequest(context.Background(), "session.new", "proj1", json.RawMessage(`{"agentType":"claude","title":"hello"}`))
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.new): %v", err)
	}

	got, err := store.LoadProjectDefaultAgent(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if got != "claude" {
		t.Fatalf("default agent = %q, want claude", got)
	}
}

func TestHandleSessionRequest_SessionListIncludesConfigOptions(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 10, 5, 0, 0, 0, time.UTC)

	if err := c.store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       SessionPersisted,
		AgentType:    "claude",
		AgentJSON:    `{}`,
		Title:        "Session 1",
		CreatedAt:    now,
		LastActiveAt: now,
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := c.store.SaveAgentPreference(ctx, AgentPreferenceRecord{
		ProjectName:    "proj1",
		AgentType:      "claude",
		PreferenceJSON: `{"configOptions":[{"id":"mode","currentValue":"code"}]}`,
	}); err != nil {
		t.Fatalf("SaveAgentPreference: %v", err)
	}

	resp, err := c.HandleSessionRequest(ctx, "session.list", "proj1", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.list): %v", err)
	}
	body, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("response type = %T, want map[string]any", resp)
	}
	sessions, ok := body["sessions"].([]sessionViewSummary)
	if !ok {
		t.Fatalf("sessions type = %T, want []sessionViewSummary", body["sessions"])
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if got := len(sessions[0].ConfigOptions); got != 1 {
		t.Fatalf("configOptions len = %d, want 1", got)
	}
	if sessions[0].ConfigOptions[0].ID != "mode" || sessions[0].ConfigOptions[0].CurrentValue != "code" {
		t.Fatalf("config option = %+v", sessions[0].ConfigOptions[0])
	}
}

func TestSessionViewReadRepairsSameTurnOverwriteFromCheckpoint(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewPromptEvent("sess-1", "run protected", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"}),
		Status:        "streaming",
	})); err != nil {
		t.Fatalf("RecordEvent update #1: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewUpdateEvent("sess-1", acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateAgentMessageChunk,
		Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: " world"}),
		Status:        "done",
	})); err != nil {
		t.Fatalf("RecordEvent update #2: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(context.Background(), "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	messages := body["turns"].([]sessionViewTurn)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[1].TurnIndex != 2 {
		t.Fatalf("messages[1].TurnIndex = %d, want 2", messages[1].TurnIndex)
	}
	if method := decodeTurnMethod(t, messages[1].Content); method != acp.SessionUpdateAgentMessageChunk {
		t.Fatalf("messages[1] method = %q, want %q", method, acp.SessionUpdateAgentMessageChunk)
	}
	if text := strings.TrimSpace(extractTextChunk(decodeTurnSessionUpdate(t, messages[1].Content).Content)); text != "hello world" {
		t.Fatalf("messages[1] text = %q, want hello world", text)
	}
}

func decodeTurnSessionUpdate(t *testing.T, raw string) acp.SessionUpdate {
	t.Helper()

	var legacy struct {
		Method string `json:"method"`
		Params struct {
			Update acp.SessionUpdate `json:"update"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(raw), &legacy); err == nil && strings.TrimSpace(legacy.Params.Update.SessionUpdate) != "" {
		return legacy.Params.Update
	}

	msg := acp.IMTurnMessage{}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("unmarshal turn update_json: %v", err)
	}
	switch strings.TrimSpace(msg.Method) {
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought, acp.SessionUpdateUserMessageChunk:
		result := acp.IMTextResult{}
		if err := json.Unmarshal(msg.Param, &result); err != nil {
			t.Fatalf("unmarshal text result: %v", err)
		}
		return acp.SessionUpdate{SessionUpdate: strings.TrimSpace(msg.Method), Content: mustJSON(map[string]any{"text": result.Text})}
	case acp.IMMethodToolCall:
		result := acp.IMToolResult{}
		if err := json.Unmarshal(msg.Param, &result); err != nil {
			t.Fatalf("unmarshal tool result: %v", err)
		}
		return acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateToolCallUpdate,
			Title:         strings.TrimSpace(result.Cmd),
			Kind:          strings.TrimSpace(result.Kind),
			Status:        strings.TrimSpace(result.Status),
		}
	case acp.IMMethodAgentPlan:
		plan := []acp.IMPlanResult{}
		if err := json.Unmarshal(msg.Param, &plan); err != nil {
			t.Fatalf("unmarshal plan result: %v", err)
		}
		entries := make([]acp.PlanEntry, 0, len(plan))
		for _, entry := range plan {
			entries = append(entries, acp.PlanEntry{Content: entry.Content, Status: entry.Status})
		}
		return acp.SessionUpdate{SessionUpdate: acp.SessionUpdatePlan, Entries: entries}
	default:
		t.Fatalf("turn update_json has unsupported method for update decode: %s", raw)
	}
	return acp.SessionUpdate{}
}

func decodeTurnMethod(t *testing.T, raw string) string {
	t.Helper()
	msg := acp.IMTurnMessage{}
	if err := json.Unmarshal([]byte(raw), &msg); err == nil {
		switch strings.TrimSpace(msg.Method) {
		case acp.IMMethodPromptRequest, acp.IMMethodPromptDone:
			return acp.MethodSessionPrompt
		default:
			return strings.TrimSpace(msg.Method)
		}
	}
	var doc struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("unmarshal turn method: %v", err)
	}
	return strings.TrimSpace(doc.Method)
}
func TestSessionViewToolCallAndUpdateMergeByToolCallID(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Tool Merge")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run build", nil)); err != nil {
		t.Fatalf("RecordEvent user message: %v", err)
	}

	toolStart := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateToolCall, ToolCallID: "call-1", Status: "in_progress", Title: "build"}
	toolDone := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateToolCallUpdate, ToolCallID: "call-1", Status: "completed", Title: "build"}

	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", toolStart)); err != nil {
		t.Fatalf("RecordEvent tool start: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", toolDone)); err != nil {
		t.Fatalf("RecordEvent tool done: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	turns := listRecordedPromptTurns(ctx, t, c, "sess-1", 1)
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3 (prompt + merged tool turn + prompt_done)", len(turns))
	}
	toolTurn := turns[1]
	update := decodeTurnSessionUpdate(t, toolTurn)
	if update.SessionUpdate != acp.SessionUpdateToolCallUpdate {
		t.Fatalf("tool turn sessionUpdate = %q, want %q", update.SessionUpdate, acp.SessionUpdateToolCallUpdate)
	}
	if update.Status != "completed" {
		t.Fatalf("tool turn status = %q, want %q", update.Status, "completed")
	}
}

func TestSessionViewBufferedUpdatesDoNotLeakAcrossPrompts(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Prompt Isolation")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}

	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "p1", nil)); err != nil {
		t.Fatalf("RecordEvent user prompt #1: %v", err)
	}
	chunk1 := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"})}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", chunk1)); err != nil {
		t.Fatalf("RecordEvent chunk #1: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished #1: %v", err)
	}

	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "p2", nil)); err != nil {
		t.Fatalf("RecordEvent user prompt #2: %v", err)
	}
	chunk2 := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "world"})}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", chunk2)); err != nil {
		t.Fatalf("RecordEvent chunk #2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished #2: %v", err)
	}

	turnsPrompt1 := listRecordedPromptTurns(ctx, t, c, "sess-1", 1)
	turnsPrompt2 := listRecordedPromptTurns(ctx, t, c, "sess-1", 2)
	if len(turnsPrompt1) != 3 || len(turnsPrompt2) != 3 {
		t.Fatalf("turn counts = (%d,%d), want (3,3)", len(turnsPrompt1), len(turnsPrompt2))
	}

	update1 := decodeTurnSessionUpdate(t, turnsPrompt1[1])
	update2 := decodeTurnSessionUpdate(t, turnsPrompt2[1])
	if text := extractTextChunk(update1.Content); text != "hello" {
		t.Fatalf("prompt1 assistant text = %q, want %q", text, "hello")
	}
	if text := extractTextChunk(update2.Content); text != "world" {
		t.Fatalf("prompt2 assistant text = %q, want %q", text, "world")
	}
}
func TestExtractTextChunkSupportsLooseShapes(t *testing.T) {
	if got := extractTextChunk(mustJSON(map[string]any{"text": "hello"})); got != "hello" {
		t.Fatalf("extractTextChunk(map text) = %q, want %q", got, "hello")
	}
	if got := extractTextChunk(mustJSON([]any{
		map[string]any{"type": "text", "text": "hello"},
		map[string]any{"text": " world"},
	})); got != "hello world" {
		t.Fatalf("extractTextChunk(array) = %q, want %q", got, "hello world")
	}
	if got := extractTextChunk(mustJSON("!")); got != "!" {
		t.Fatalf("extractTextChunk(string) = %q, want %q", got, "!")
	}
}
func TestSessionViewToolCallTerminalUpdatesRemainSingleTurn(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Tool Terminal Merge")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run task", nil)); err != nil {
		t.Fatalf("RecordEvent user message: %v", err)
	}

	updates := []acp.SessionUpdate{
		{SessionUpdate: acp.SessionUpdateToolCall, ToolCallID: "call-terminal", Status: acp.ToolCallStatusInProgress, Title: "task"},
		{SessionUpdate: acp.SessionUpdateToolCallUpdate, ToolCallID: "call-terminal", Status: acp.ToolCallStatusFailed, Title: "task"},
		{SessionUpdate: acp.SessionUpdateToolCallUpdate, ToolCallID: "call-terminal", Status: acp.ToolCallStatusCancelled, Title: "task"},
	}
	for i := range updates {
		u := updates[i]
		if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", u)); err != nil {
			t.Fatalf("RecordEvent tool update #%d: %v", i+1, err)
		}
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}

	turns := listRecordedPromptTurns(ctx, t, c, "sess-1", 1)
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3 (prompt + merged tool turn + prompt_done)", len(turns))
	}
	toolTurn := turns[1]
	update := decodeTurnSessionUpdate(t, toolTurn)
	if update.Status != acp.ToolCallStatusCancelled {
		t.Fatalf("tool turn status = %q, want %q", update.Status, acp.ToolCallStatusCancelled)
	}
}

func TestSessionViewPermissionEventsAreIgnored(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Permission Ignored")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run protected", nil)); err != nil {
		t.Fatalf("RecordEvent user message: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPermissionRequestedEvent("sess-1", "allow?", 7, nil)); err != nil {
		t.Fatalf("RecordEvent permission requested: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPermissionResolvedEvent("sess-1", 7, "done", mustRFC3339Time(t, "2026-04-12T10:02:00Z"))); err != nil {
		t.Fatalf("RecordEvent permission resolved: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns len = %d, want 1 (prompt only)", len(turns))
	}
}
func TestSessionViewDropsOrphanPermissionResult(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Permission Orphan")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "run protected", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPermissionResolvedEvent("sess-1", 7, "done", mustRFC3339Time(t, "2026-04-12T10:02:00Z"))); err != nil {
		t.Fatalf("RecordEvent orphan permission resolved: %v", err)
	}

	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, "sess-1", 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns len = %d, want 1 (prompt only)", len(turns))
	}
}
func TestSessionViewNextPromptFlushesPreviousWithoutPromptFinished(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Prompt Carry")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "first", nil)); err != nil {
		t.Fatalf("RecordEvent user prompt #1: %v", err)
	}
	chunk1 := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"})}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", chunk1)); err != nil {
		t.Fatalf("RecordEvent chunk #1: %v", err)
	}

	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "second", nil)); err != nil {
		t.Fatalf("RecordEvent user prompt #2: %v", err)
	}
	chunk2 := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "world"})}
	if err := c.RecordEvent(ctx, sessionViewUpdateEvent("sess-1", chunk2)); err != nil {
		t.Fatalf("RecordEvent chunk #2: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent("sess-1", "")); err != nil {
		t.Fatalf("RecordEvent prompt finished #2: %v", err)
	}

	turnsPrompt1 := listRecordedPromptTurns(ctx, t, c, "sess-1", 1)
	turnsPrompt2 := listRecordedPromptTurns(ctx, t, c, "sess-1", 2)
	if len(turnsPrompt1) != 3 {
		t.Fatalf("prompt1 stored turns = %d, want 3 (prompt + chunk + interrupted prompt_done)", len(turnsPrompt1))
	}
	if len(turnsPrompt2) != 3 {
		t.Fatalf("prompt2 stored turns = %d, want 3", len(turnsPrompt2))
	}
	update1 := decodeTurnSessionUpdate(t, turnsPrompt1[1])
	if text := extractTextChunk(update1.Content); text != "hello" {
		t.Fatalf("prompt1 assistant text = %q, want %q", text, "hello")
	}
	if stopReason := decodePromptDoneStopReason(t, turnsPrompt1[2]); stopReason != "interrupted" {
		t.Fatalf("prompt1 prompt_done stopReason = %q, want interrupted", stopReason)
	}
	update2 := decodeTurnSessionUpdate(t, turnsPrompt2[1])
	if text := extractTextChunk(update2.Content); text != "world" {
		t.Fatalf("prompt2 assistant text = %q, want %q", text, "world")
	}
}

func seedPromptWithTurns(t *testing.T, c *Client, ctx context.Context, sessionID, promptText string, updates []acp.SessionUpdate) {
	t.Helper()
	if err := c.RecordEvent(ctx, sessionViewCreatedEvent(sessionID, promptText)); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent(sessionID, promptText, nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}
	for i, update := range updates {
		if err := c.RecordEvent(ctx, sessionViewUpdateEvent(sessionID, update)); err != nil {
			t.Fatalf("RecordEvent update #%d: %v", i+1, err)
		}
	}
	if err := c.RecordEvent(ctx, sessionViewPromptFinishedEvent(sessionID, acp.StopReasonEndTurn)); err != nil {
		t.Fatalf("RecordEvent prompt finished: %v", err)
	}
}

func hasPromptDoneTurnWithStopReason(t *testing.T, turns []sessionViewTurn, stopReason string) bool {
	t.Helper()
	for _, message := range turns {
		var turn acp.IMTurnMessage
		if err := json.Unmarshal([]byte(message.Content), &turn); err != nil {
			t.Fatalf("unmarshal turn content: %v", err)
		}
		if turn.Method != acp.IMMethodPromptDone {
			continue
		}
		var result acp.IMPromptResult
		raw, err := json.Marshal(turn.Param)
		if err != nil {
			t.Fatalf("marshal prompt_done param: %v", err)
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatalf("unmarshal prompt_done param: %v", err)
		}
		return result.StopReason == stopReason
	}
	return false
}

func decodePromptDoneStopReason(t *testing.T, raw string) string {
	t.Helper()
	var turn acp.IMTurnMessage
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		t.Fatalf("unmarshal prompt_done turn: %v", err)
	}
	if turn.Method != acp.IMMethodPromptDone {
		t.Fatalf("turn method = %q, want %q", turn.Method, acp.IMMethodPromptDone)
	}
	var result acp.IMPromptResult
	if err := json.Unmarshal(turn.Param, &result); err != nil {
		t.Fatalf("unmarshal prompt_done param: %v", err)
	}
	return result.StopReason
}

func decodeTurnParamMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var turn acp.IMTurnMessage
	if err := json.Unmarshal([]byte(raw), &turn); err != nil {
		t.Fatalf("unmarshal turn: %v", err)
	}
	out := map[string]any{}
	if len(turn.Param) == 0 {
		return out
	}
	if err := json.Unmarshal(turn.Param, &out); err != nil {
		t.Fatalf("unmarshal turn param: %v", err)
	}
	return out
}

func listRecordedPromptTurns(ctx context.Context, t *testing.T, c *Client, sessionID string, promptOrdinal int64) []string {
	t.Helper()
	_, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, sessionID, 0)
	if err != nil {
		t.Fatalf("ReadSessionTurns: %v", err)
	}
	currentPrompt := int64(0)
	out := []string{}
	for _, turn := range turns {
		var msg acp.IMTurnMessage
		if err := json.Unmarshal([]byte(turn.Content), &msg); err != nil {
			t.Fatalf("unmarshal turn content: %v", err)
		}
		if strings.TrimSpace(msg.Method) == acp.IMMethodPromptRequest || currentPrompt == 0 {
			currentPrompt++
		}
		if currentPrompt == promptOrdinal {
			out = append(out, turn.Content)
		}
	}
	return out
}

func mustRFC3339Time(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q): %v", value, err)
	}
	return parsed
}

type mockSession struct {
	promptCalls []string
	cancelCalls int
	agentName   string
	sessionID   string
	promptFn    func(string) (<-chan acp.SessionUpdateParams, acp.SessionPromptResult, error)
}

func newTestClient(t *testing.T, mock *mockSession) *Client {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	c := New(store, "test", t.TempDir())
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

func captureReplies(c *Client) *[]string {
	router := NewTestCaptureRouter()
	c.SetIMRouter(router)
	return &router.Messages
}

func TestStart_LoadsRouteBindingsWithoutRestoringSessions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db", "client.sqlite3")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveRouteBinding(ctx, "proj-a", "im:feishu:chat-1", "sess-1"); err != nil {
		t.Fatalf("SaveRouteBinding() error = %v", err)
	}
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:          "sess-1",
		ProjectName: "proj-a",
		Status:      SessionPersisted,
		AgentType:   "claude",
		AgentJSON:   `{"title":"Persisted"}`,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	c := New(store, "proj-a", dir)
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

func TestStart_CreatesProjectRowWhenMissing(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	c := New(store, "proj-a", t.TempDir())
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	sqliteStore, ok := store.(*sqliteStore)
	if !ok {
		t.Fatal("store type mismatch")
	}

	var rows int
	if err := sqliteStore.db.QueryRow(`SELECT COUNT(1) FROM projects WHERE project_name = ?`, "proj-a").Scan(&rows); err != nil {
		t.Fatalf("query projects row count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("projects rows = %d, want 1", rows)
	}

	got, err := store.LoadProjectDefaultAgent(context.Background(), "proj-a")
	if err != nil {
		t.Fatalf("LoadProjectDefaultAgent: %v", err)
	}
	if got != "" {
		t.Fatalf("default agent = %q, want empty", got)
	}
}

func TestResolveSession_RejectsEmptyRouteKey(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	c := New(store, "proj-a", t.TempDir())
	if _, err := c.ResolveSessionForTest(""); err == nil {
		t.Fatal(`ResolveSessionForTest("") should fail`)
	}
}

func TestHandleMessage_Cancel(t *testing.T) {
	mock := &mockSession{agentName: "claude", sessionID: "sess-1"}
	c := newTestClient(t, mock)
	msgs := captureReplies(c)

	c.HandleMessage(Message{ChatID: "chat1", Text: "/cancel"})

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

	c.HandleMessage(Message{ChatID: "chat1", Text: "/status"})

	if len(*msgs) == 0 {
		t.Fatal("no reply received")
	}
	reply := (*msgs)[0]
	if !strings.Contains(reply, "codex") {
		t.Fatalf("status reply %q missing agent name", reply)
	}
	if !strings.Contains(reply, "session:") {
		t.Fatalf("status reply %q missing session field", reply)
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

	c.HandleMessage(Message{ChatID: "chat1", Text: "hi there"})

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

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveRouteBinding(ctx, "proj-a", "im:feishu:chat-1", "sess-1"); err != nil {
		t.Fatalf("SaveRouteBinding() error = %v", err)
	}
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj-a",
		Status:       SessionSuspended,
		AgentType:    "claude",
		AgentJSON:    `{"title":"Persisted","commands":[{"name":"/status"}]}`,
		Title:        "Persisted",
		CreatedAt:    time.Unix(10, 0).UTC(),
		LastActiveAt: time.Unix(20, 0).UTC(),
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := store.SaveAgentPreference(ctx, AgentPreferenceRecord{
		ProjectName:    "proj-a",
		AgentType:      "claude",
		PreferenceJSON: `{"configOptions":[{"id":"mode","currentValue":"code"}]}`,
	}); err != nil {
		t.Fatalf("SaveAgentPreference() error = %v", err)
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
	if rec.AgentType != "claude" {
		t.Fatalf("LoadSession().AgentType = %q, want claude", rec.AgentType)
	}
	if strings.Contains(rec.AgentJSON, "acpSessionId") {
		t.Fatalf("LoadSession().AgentJSON = %q, should not contain acpSessionId", rec.AgentJSON)
	}

	pref, err := store.LoadAgentPreference(ctx, "proj-a", "claude")
	if err != nil {
		t.Fatalf("LoadAgentPreference() error = %v", err)
	}
	if pref == nil || !strings.Contains(pref.PreferenceJSON, `"mode"`) {
		t.Fatalf("LoadAgentPreference() = %+v, want mode config", pref)
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

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	err = store.SaveRouteBinding(context.Background(), "proj-a", "", "sess-1")
	if err == nil {
		t.Fatal("SaveRouteBinding() should reject empty route keys")
	}
}
func TestHandleMessage_Skills(t *testing.T) {
	mock := &mockSession{agentName: "codex", sessionID: "sess-skills"}
	c := newTestClient(t, mock)
	msgs := captureReplies(c)

	sess, err := c.resolveSession(testRouteKey)
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	inst, ok := sess.instance.(*testInjectedInstance)
	if !ok {
		t.Fatalf("instance type = %T", sess.instance)
	}
	inst.skills = []agent.SkillDescriptor{{Name: "frontend-design", Path: "D:/repo/.agents/skills/frontend-design/SKILL.md"}}

	c.HandleMessage(Message{ChatID: "chat1", Text: "/skills"})

	if len(*msgs) == 0 {
		t.Fatal("no reply received")
	}
	reply := strings.Join(*msgs, "\n")
	if !strings.Contains(reply, "Skills (1):") {
		t.Fatalf("skills reply missing header: %q", reply)
	}
	if !strings.Contains(reply, "frontend-design") {
		t.Fatalf("skills reply missing skill name: %q", reply)
	}
}
func TestHandleSessionRequest_SessionSendSlashSkills(t *testing.T) {
	mock := &mockSession{agentName: "codex", sessionID: "sess-send-skills"}
	c := newTestClient(t, mock)
	msgs := captureReplies(c)

	sess, err := c.resolveSession(testRouteKey)
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	inst, ok := sess.instance.(*testInjectedInstance)
	if !ok {
		t.Fatalf("instance type = %T", sess.instance)
	}
	inst.skills = []agent.SkillDescriptor{{Name: "diagnose", Path: "D:/repo/.agents/skills/diagnose/SKILL.md"}}

	payload := json.RawMessage(`{"sessionId":"sess-send-skills","text":"/skills"}`)
	resp, err := c.HandleSessionRequest(context.Background(), "session.send", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest(session.send): %v", err)
	}
	body, ok := resp.(map[string]any)
	if !ok || body["ok"] != true {
		t.Fatalf("response = %#v, want ok=true", resp)
	}

	if len(*msgs) == 0 {
		t.Fatal("no reply received")
	}
	reply := strings.Join(*msgs, "\n")
	if !strings.Contains(reply, "Skills (1):") {
		t.Fatalf("skills reply missing header: %q", reply)
	}
	if !strings.Contains(reply, "diagnose") {
		t.Fatalf("skills reply missing skill name: %q", reply)
	}
}
