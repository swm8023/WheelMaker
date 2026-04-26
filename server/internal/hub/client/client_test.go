package client

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	loadErr     error
	newResult   *acp.SessionNewResult
	listResult  acp.SessionListResult
	listErr     error
	setConfigFn func(context.Context, acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error)
	setCalls    []acp.SessionSetConfigOptionParams
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
	c.mu.Lock()
	sess := c.sessions[c.routeMap[testRouteKey]]
	if sess == nil {
		sess = c.newWiredSession("")
		c.sessions[sess.ID] = sess
		c.routeMap[testRouteKey] = sess.ID
	}
	c.mu.Unlock()

	name := strings.TrimSpace(agentName)
	if name == "" {
		name = string(acp.ACPProviderClaude)
	}
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
	sess.activeAgent = name
	sess.acpSessionID = sessionID
	sess.ready = true
	state := sess.agentStateLocked(name)
	state.ACPSessionID = sessionID
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
			sess, err := c.ClientNewSession(routeKey)
			if err != nil {
				return
			}
			if source.ChatID != "" {
				sess.setIMSource(source)
			}
			sess.reply("Created new session: " + sess.ID)
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
			loaded.reply("Loaded session: " + loaded.ID)
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
func (i *testInjectedInstance) Close() error { return nil }

var _ agent.Instance = (*testInjectedInstance)(nil)
var _ IMRouter = (*TestCaptureRouter)(nil)

type noopStore struct{}

func (s *noopStore) LoadProject(context.Context, string) (*ProjectConfig, error) {
	return &ProjectConfig{}, nil
}
func (s *noopStore) SaveProject(context.Context, string, ProjectConfig) error { return nil }
func (s *noopStore) LoadRouteBindings(context.Context, string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (s *noopStore) SaveRouteBinding(context.Context, string, string, string) error { return nil }
func (s *noopStore) DeleteRouteBinding(context.Context, string, string) error       { return nil }
func (s *noopStore) LoadSession(context.Context, string, string) (*SessionRecord, error) {
	return nil, nil
}
func (s *noopStore) SaveSession(context.Context, *SessionRecord) error { return nil }
func (s *noopStore) ListSessions(context.Context, string) ([]SessionRecord, error) {
	return nil, nil
}
func (s *noopStore) DeleteSession(context.Context, string, string) error { return nil }
func (s *noopStore) UpsertSessionPrompt(context.Context, SessionPromptRecord) error {
	return nil
}
func (s *noopStore) LoadSessionPrompt(context.Context, string, string, int64) (*SessionPromptRecord, error) {
	return nil, nil
}
func (s *noopStore) ListSessionPrompts(context.Context, string, string) ([]SessionPromptRecord, error) {
	return nil, nil
}
func (s *noopStore) ListSessionPromptsAfterIndex(context.Context, string, string, int64) ([]SessionPromptRecord, error) {
	return nil, nil
}
func (s *noopStore) UpsertSessionTurn(context.Context, SessionTurnRecord) error {
	return nil
}
func (s *noopStore) LoadSessionTurn(context.Context, string, string, int64, int64) (*SessionTurnRecord, error) {
	return nil, nil
}
func (s *noopStore) ListSessionTurns(context.Context, string, string, int64) ([]SessionTurnRecord, error) {
	return nil, nil
}
func (s *noopStore) Close() error { return nil }

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

func TestIsAgentRecoverableRuntimeErr(t *testing.T) {
	if !isAgentRecoverableRuntimeErr(errors.New("agent owned conn: process exited")) {
		t.Fatal("expected process-exit error to be recoverable")
	}
	if !isAgentRecoverableRuntimeErr(errors.New("windows sandbox: spawn setup refresh")) {
		t.Fatal("expected sandbox refresh error to be recoverable")
	}
	if isAgentRecoverableRuntimeErr(errors.New("selected model is at capacity")) {
		t.Fatal("capacity error should not be treated as recoverable runtime error")
	}
}

func TestSessionShouldReconnectOnRecoverableErr_RequiresDeadProcess(t *testing.T) {
	s := newSession("sess-1", "/tmp")

	s.mu.Lock()
	s.instance = &testInjectedInstance{name: "codex", alive: true}
	s.mu.Unlock()
	if s.shouldReconnectOnRecoverableErr(errors.New("windows sandbox: spawn setup refresh")) {
		t.Fatal("alive process should not trigger reconnect")
	}

	s.mu.Lock()
	s.instance = &testInjectedInstance{name: "codex", alive: false}
	s.mu.Unlock()
	if !s.shouldReconnectOnRecoverableErr(errors.New("windows sandbox: spawn setup refresh")) {
		t.Fatal("dead process should trigger reconnect for recoverable runtime error")
	}
}

func TestHasSandboxRefreshUpdate(t *testing.T) {
	u := acp.SessionUpdate{SessionUpdate: acp.SessionUpdateToolCallUpdate, Content: json.RawMessage(`"tool failed: windows sandbox: spawn setup refresh"`)}
	if !hasSandboxRefreshUpdate(u) {
		t.Fatal("expected sandbox refresh detection")
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

func TestSessionRequestPermissionAutoAllowsWithoutIMRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := logger.Setup(logger.LoggerConfig{Level: logger.LevelWarn}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer logger.Close()
	logger.SetOutput(&buf)
	defer logger.SetOutput(os.Stderr)

	s := newSession("sess-1", "/tmp")
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
	s := newSession("sess-1", "/tmp")
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

func TestClientLoadSession_RestoresFromStore(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	c := New(store, "proj1", "/tmp")
	ctx := context.Background()
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:          "restore-me",
		ProjectName: "proj1",

		ACPSessionID: "acp-999",
		AgentsJSON:   `{"claude":{"acpSessionId":"acp-999","title":"Persisted"}}`,
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
	if sess.ID != "restore-me" {
		t.Fatalf("resolved session ID = %q, want restore-me", sess.ID)
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
		AgentsJSON:  `{"claude":{"title":"Persisted Title"}}`,

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
		Title:       "Persisted Title",

		CreatedAt:    createdAt,
		LastActiveAt: lastMessageAt,
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	c := New(store, "proj1", "/tmp")
	c.mu.Lock()
	sess := c.newWiredSession("sess-1")
	sess.createdAt = createdAt
	sess.lastActiveAt = time.Now().UTC()
	sess.Status = SessionActive
	sess.activeAgent = "claude"
	sess.agents = map[string]*SessionAgentState{
		"claude": {Title: "Runtime Title"},
	}
	c.sessions[sess.ID] = sess
	c.mu.Unlock()

	entries, err := c.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Title != "Runtime Title" {
		t.Fatalf("entries[0].Title = %q, want %q", entries[0].Title, "Runtime Title")
	}
	if entries[0].Status != SessionActive {
		t.Fatalf("entries[0].Status = %v, want %v", entries[0].Status, SessionActive)
	}
	if !entries[0].InMemory {
		t.Fatal("entries[0].InMemory = false, want true")
	}

}

func TestEnsureReady_SessionLoadKeepsPersistedConfigWhenLoadResultEmpty(t *testing.T) {
	s := newSession("restore-mode", "/tmp")
	s.projectName = "proj1"
	s.activeAgent = "claude"
	s.acpSessionID = "acp-keep"
	s.agents = map[string]*SessionAgentState{
		"claude": {
			ACPSessionID: "acp-keep",
			ConfigOptions: []acp.ConfigOption{
				{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
				{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
			},
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

	snap := s.sessionConfigSnapshot()
	if snap.Mode != "code" {
		t.Fatalf("mode = %q, want %q", snap.Mode, "code")
	}
	if snap.Model != "gpt-5" {
		t.Fatalf("model = %q, want %q", snap.Model, "gpt-5")
	}
}

func TestEnsureReady_SessionLoadFailure_ReappliesPersistedModeModel(t *testing.T) {
	s := newSession("restore-fallback", "/tmp")
	s.projectName = "proj1"
	s.activeAgent = "claude"
	s.acpSessionID = "acp-old"
	s.agents = map[string]*SessionAgentState{
		"claude": {
			ACPSessionID: "acp-old",
			ConfigOptions: []acp.ConfigOption{
				{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
				{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
			},
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
		newResult: &acp.SessionNewResult{
			SessionID: "acp-new",
			ConfigOptions: []acp.ConfigOption{
				{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "ask"},
				{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-4o-mini"},
			},
		},
		setConfigFn: func(_ context.Context, p acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
			switch p.ConfigID {
			case acp.ConfigOptionIDMode:
				return []acp.ConfigOption{
					{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: p.Value},
					{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-4o-mini"},
				}, nil
			case acp.ConfigOptionIDModel:
				return []acp.ConfigOption{
					{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
					{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: p.Value},
				}, nil
			default:
				return nil, errors.New("unexpected config id")
			}
		},
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

	snap := s.sessionConfigSnapshot()
	if snap.Mode != "code" {
		t.Fatalf("mode = %q, want %q", snap.Mode, "code")
	}
	if snap.Model != "gpt-5" {
		t.Fatalf("model = %q, want %q", snap.Model, "gpt-5")
	}
	if len(inst.setCalls) < 2 {
		t.Fatalf("set config calls = %d, want at least 2 (mode+model)", len(inst.setCalls))
	}
}

func TestEnsureReadyAndNotify_DoesNotSendSessionInfoAfterReady(t *testing.T) {
	s := newSession("no-session-info", "/tmp")
	s.projectName = "proj1"
	s.activeAgent = "claude"
	s.agents = map[string]*SessionAgentState{
		"claude": {},
	}
	router := &fakeIMRouter{}
	s.imRouter = router
	s.imSource = &im.ChatRef{ChannelID: "feishu", ChatID: "chat-1"}
	s.registry = agent.DefaultACPFactory().Clone()
	s.registry.Register(acp.ACPProviderClaude, func(context.Context, string) (agent.Instance, error) {
		return &testInjectedInstance{
			name:      "claude",
			sessionID: "acp-1",
		}, nil
	})

	if err := s.ensureInstance(context.Background()); err != nil {
		t.Fatalf("ensureInstance: %v", err)
	}
	if err := s.ensureReadyAndNotify(context.Background()); err != nil {
		t.Fatalf("ensureReadyAndNotify(first): %v", err)
	}
	if got := len(router.systems); got != 1 {
		t.Fatalf("system notify count after first ensureReadyAndNotify = %d, want 1", got)
	}
	if got := router.systems[0].payload.Title; got != "Session ready" {
		t.Fatalf("first title = %q, want %q", got, "Session ready")
	}

	if err := s.ensureReadyAndNotify(context.Background()); err != nil {
		t.Fatalf("ensureReadyAndNotify(second): %v", err)
	}
	if got := len(router.systems); got != 1 {
		t.Fatalf("system notify count after second ensureReadyAndNotify = %d, want 1", got)
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
	sess := c.newWiredSession("evict-me")
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
	s := newSession("sess", "/tmp")
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

func TestClientNewSession_ReappliesProjectAgentBaseline(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.SaveProject(context.Background(), "proj1", ProjectConfig{
		AgentState: map[string]ProjectAgentState{
			"claude": {
				ConfigOptions: []acp.ConfigOption{
					{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
					{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
					{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("SaveProject: %v", err)
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
	oldSess := c.newWiredSession("sess-old")
	oldSess.activeAgent = "claude"
	c.sessions[oldSess.ID] = oldSess
	c.routeMap["route-1"] = oldSess.ID
	c.mu.Unlock()

	sess, err := c.ClientNewSession("route-1")
	if err != nil {
		t.Fatalf("ClientNewSession: %v", err)
	}
	if err := sess.ensureInstance(context.Background()); err != nil {
		t.Fatalf("ensureInstance: %v", err)
	}
	if err := sess.ensureReady(context.Background()); err != nil {
		t.Fatalf("ensureReady: %v", err)
	}

	if got := sess.preferredAgentName(); got != "claude" {
		t.Fatalf("preferred agent = %q, want claude", got)
	}

	if got := len(inst.setCalls); got != 3 {
		t.Fatalf("set calls = %d, want 3", got)
	}

}

func TestEnsureReady_SessionLoadSuccess_ReplaysOnlyReplayableSessionValues(t *testing.T) {
	s := newSession("restore-load-success", "/tmp")
	s.projectName = "proj1"
	s.activeAgent = "claude"
	s.acpSessionID = "acp-old"
	s.agents = map[string]*SessionAgentState{
		"claude": {
			ACPSessionID: "acp-old",
			ConfigOptions: []acp.ConfigOption{
				{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
				{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
				{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
				{ID: "custom_toggle", CurrentValue: "persisted-custom"},
			},
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

	state, _ := s.currentAgentStateSnapshot()
	if state == nil {
		t.Fatal("currentAgentStateSnapshot returned nil state")
	}
	if findCurrentValue(state.ConfigOptions, "custom_toggle") != "agent-custom" {
		t.Fatalf("custom_toggle should stay agent-owned")
	}
	if got := len(inst.setCalls); got != 3 {
		t.Fatalf("set calls = %d, want 3", got)
	}
}
func TestCancelPrompt_DoesNotClearSessionConfig(t *testing.T) {
	s := newSession("cancel-keep-config", "/tmp")
	s.ready = true
	s.acpSessionID = "acp-1"
	s.activeAgent = "claude"
	s.agents = map[string]*SessionAgentState{
		"claude": {
			ConfigOptions: []acp.ConfigOption{
				{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
				{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
				{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
			},
		},
	}
	s.instance = &testInjectedInstance{name: "claude"}

	if err := s.cancelPrompt(); err != nil {
		t.Fatalf("cancelPrompt: %v", err)
	}

	snap := s.sessionConfigSnapshot()
	if snap.Mode != "code" || snap.Model != "gpt-5" || snap.ThoughtLevel != "high" {
		t.Fatalf("snapshot after cancel = %+v", snap)
	}
}

func TestEnsureReady_SessionLoadSuccess_AgentCommandsOverrideCachedCommands(t *testing.T) {
	s := newSession("commands-load", "/tmp")
	s.projectName = "proj1"
	s.activeAgent = "claude"
	s.acpSessionID = "acp-1"
	s.agents = map[string]*SessionAgentState{
		"claude": {
			ACPSessionID: "acp-1",
			Commands:     []acp.AvailableCommand{{Name: "/cached"}},
		},
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

	state, _ := s.currentAgentStateSnapshot()
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
	sess, err := c.ClientNewSession("route:test")
	if err != nil {
		t.Fatalf("ClientNewSession: %v", err)
	}

	err = c.PromptToSession(context.Background(), sess.ID, im.ChatRef{ChannelID: " feishu ", ChatID: " chat-1 "}, []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: "hello"}})
	if err != nil {
		t.Fatalf("PromptToSession: %v", err)
	}

	bindings, err := store.LoadRouteBindings(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadRouteBindings: %v", err)
	}
	if got := bindings["im:feishu:chat-1"]; got != sess.ID {
		t.Fatalf("route binding = %q, want %q", got, sess.ID)
	}
}

func TestResolveHelpModelRefreshesSessionMenuFromRuntimeList(t *testing.T) {
	s := newSession("sess-local", ".")
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
	s.activeAgent = inst.name
	s.acpSessionID = "sess-current"
	s.ready = true
	state := s.agentStateLocked(inst.name)
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
	if len(sessionMenu.Options) != 2 {
		t.Fatalf("session menu options len = %d, want 2", len(sessionMenu.Options))
	}
	if sessionMenu.Options[0].Command != "/load" || sessionMenu.Options[0].Value != "1" {
		t.Fatalf("session menu option[0] = %#v, want /load 1", sessionMenu.Options[0])
	}
	if sessionMenu.Options[1].Command != "/load" || sessionMenu.Options[1].Value != "2" {
		t.Fatalf("session menu option[1] = %#v, want /load 2", sessionMenu.Options[1])
	}
	if strings.Contains(sessionMenu.Body, "No cached sessions") {
		t.Fatalf("session menu body should not show cached-session fallback: %q", sessionMenu.Body)
	}
}

func TestStoreProjectAgentStateRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	cfg := ProjectConfig{
		AgentState: map[string]ProjectAgentState{
			"codex": {
				ConfigOptions: []acp.ConfigOption{
					{ID: acp.ConfigOptionIDMode, Category: acp.ConfigOptionCategoryMode, CurrentValue: "code"},
					{ID: acp.ConfigOptionIDModel, Category: acp.ConfigOptionCategoryModel, CurrentValue: "gpt-5"},
					{ID: acp.ConfigOptionIDThoughtLevel, Category: acp.ConfigOptionCategoryThoughtLv, CurrentValue: "high"},
				},
				AvailableCommands: []acp.AvailableCommand{{Name: "/status"}},
				UpdatedAt:         "2026-04-11T00:00:00Z",
			},
		},
	}
	if err := store.SaveProject(context.Background(), "proj1", cfg); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	loaded, err := store.LoadProject(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	codex := loaded.AgentState["codex"]
	if got := len(codex.ConfigOptions); got != 3 {
		t.Fatalf("config options = %d, want 3", got)
	}
	if got := len(codex.AvailableCommands); got != 1 {
		t.Fatalf("commands = %d, want 1", got)
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
		DROP TABLE projects;
		CREATE TABLE projects (
			project_name TEXT PRIMARY KEY,
			yolo INTEGER NOT NULL DEFAULT 0,
			agent_state_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
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
		t.Fatal("CheckStoreSchema() error = nil, want mismatch for projects.yolo column")
	}
	if !IsStoreSchemaMismatch(err) {
		t.Fatalf("IsStoreSchemaMismatch(err) = false, err=%v", err)
	}
	if !strings.Contains(err.Error(), `table "projects" columns mismatch`) {
		t.Fatalf("CheckStoreSchema() err = %v, want projects columns mismatch", err)
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

func TestNewStoreSessionTurnsSchemaOmitsExtraJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "client.sqlite3")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`PRAGMA table_info(session_turns)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(session_turns): %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table_info(session_turns): %v", err)
		}
		if strings.EqualFold(strings.TrimSpace(name), "extra_json") {
			t.Fatal("session_turns unexpectedly contains extra_json column")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info(session_turns): %v", err)
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

func TestStoreSessionTurnRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.SaveSession(context.Background(), &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       SessionActive,
		CreatedAt:    time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.UpsertSessionPrompt(context.Background(), SessionPromptRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		UpdatedAt:   time.Date(2026, 4, 12, 10, 6, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertSessionPrompt: %v", err)
	}

	turnJSON := `{"method":"session/prompt","params":{"prompt":[{"type":"image","mimeType":"image/png","data":"abc123"}]}}`
	if err := store.UpsertSessionTurn(context.Background(), SessionTurnRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		TurnIndex:   1,
		UpdateIndex: 1,
		UpdateJSON:  turnJSON,
	}); err != nil {
		t.Fatalf("UpsertSessionTurn: %v", err)
	}

	turns, err := store.ListSessionTurns(context.Background(), "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("ListSessionTurns() len = %d, want 1", len(turns))
	}
	var doc struct {
		Method string `json:"method"`
		Params struct {
			Prompt []acp.ContentBlock `json:"prompt"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(turns[0].UpdateJSON), &doc); err != nil {
		t.Fatalf("unmarshal turn update json: %v", err)
	}
	if strings.TrimSpace(doc.Method) != acp.MethodSessionPrompt {
		t.Fatalf("turn method = %q, want %q", doc.Method, acp.MethodSessionPrompt)
	}
	if len(doc.Params.Prompt) != 1 || doc.Params.Prompt[0].Type != acp.ContentBlockTypeImage {
		t.Fatalf("turn params.prompt = %#v, want image block", doc.Params.Prompt)
	}
}

func TestStoreSessionTurnUpsertRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "client.sqlite3"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj1",
		Status:       SessionActive,
		CreatedAt:    time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		LastActiveAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := store.UpsertSessionPrompt(ctx, SessionPromptRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		UpdatedAt:   time.Date(2026, 4, 12, 10, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertSessionPrompt: %v", err)
	}
	if err := store.UpsertSessionTurn(ctx, SessionTurnRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		TurnIndex:   1,
		UpdateIndex: 1,
		UpdateJSON:  `{"method":"session/request_permission","id":42,"params":{"toolCall":{"title":"Run tool?"}}}`,
	}); err != nil {
		t.Fatalf("UpsertSessionTurn initial: %v", err)
	}
	if err := store.UpsertSessionTurn(ctx, SessionTurnRecord{
		SessionID:   "sess-1",
		PromptIndex: 1,
		TurnIndex:   1,
		UpdateIndex: 2,
		UpdateJSON:  `{"method":"session/request_permission","id":42,"result":{"outcome":{"outcome":"done"}}}`,
	}); err != nil {
		t.Fatalf("UpsertSessionTurn merged: %v", err)
	}

	turns, err := store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("ListSessionTurns() len = %d, want 1", len(turns))
	}
	if turns[0].UpdateIndex != 2 {
		t.Fatalf("turn updateIndex = %d, want 2", turns[0].UpdateIndex)
	}
	var resultDoc struct {
		Result struct {
			Outcome struct {
				Outcome string `json:"outcome"`
			} `json:"outcome"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(turns[0].UpdateJSON), &resultDoc); err != nil {
		t.Fatalf("unmarshal turn update json: %v", err)
	}
	if strings.TrimSpace(resultDoc.Result.Outcome.Outcome) != "done" {
		t.Fatalf("turn result outcome = %q, want done", resultDoc.Result.Outcome.Outcome)
	}

	rec, err := store.LoadSession(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if rec == nil {
		t.Fatalf("LoadSession() returned nil")
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

func sessionViewCreatedEvent(sessionID, title string) SessionViewEvent {
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: buildACPMethodParamsContent(acp.MethodSessionNew, map[string]any{
			"sessionId": sessionID,
			"title":     title,
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
		Content:   buildACPMethodParamsContent(acp.MethodSessionPrompt, params),
	}
}

func sessionViewUpdateEvent(sessionID string, update acp.SessionUpdate) SessionViewEvent {
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: buildACPMethodParamsContent(acp.MethodSessionUpdate, acp.SessionUpdateParams{
			SessionID: sessionID,
			Update:    update,
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
		Content: buildACPMethodResultContent(acp.MethodSessionPrompt, acp.SessionPromptResult{
			StopReason: stopReason,
		}),
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
		Content:   buildACPMethodRequestContent(requestID, acp.MethodRequestPermission, params),
	}
}

func sessionViewPermissionResolvedEvent(sessionID string, requestID int64, status string, updatedAt time.Time) SessionViewEvent {
	return SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: sessionID,
		Content: buildACPMethodResponseContent(requestID, acp.MethodRequestPermission, acp.PermissionResponse{
			Outcome: acp.PermissionResult{Outcome: status},
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
		Content:   buildACPMethodResultContent(acp.IMMethodSystem, text),
	}
}

func TestBuildConvertedMessageFromSessionUpdateIncludesToolMergeKey(t *testing.T) {
	converted, ok, err := buildTurnMessageFromSessionUpdate(acp.SessionUpdate{
		SessionUpdate: acp.SessionUpdateToolCallUpdate,
		ToolCallID:    "call-1",
		Title:         "build",
		Status:        "completed",
	})
	if err != nil {
		t.Fatalf("buildConvertedMessageFromSessionUpdate: %v", err)
	}
	if !ok {
		t.Fatalf("buildConvertedMessageFromSessionUpdate ok = false, want true")
	}
	if strings.TrimSpace(converted.IMMessage.Method) != acp.IMMethodToolCall {
		t.Fatalf("converted method = %q, want %q", converted.IMMessage.Method, acp.IMMethodToolCall)
	}
	if converted.MergeKey.ToolCallID != "call-1" {
		t.Fatalf("mergeKey.toolCallId = %q, want %q", converted.MergeKey.ToolCallID, "call-1")
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
			wantMethod:    acp.IMMethodPrompt,
			check: func(t *testing.T, parsed parsedSessionViewEvent) {
				t.Helper()
				request := acp.IMPromptRequest{}
				if err := json.Unmarshal(parsed.message.Request, &request); err != nil {
					t.Fatalf("json.Unmarshal(prompt request): %v", err)
				}
				if len(request.ContentBlocks) != 1 || strings.TrimSpace(request.ContentBlocks[0].Text) != "say hi" {
					t.Fatalf("request.ContentBlocks = %#v, want single text block", request.ContentBlocks)
				}
			},
		},
		{
			name:          "prompt result becomes prompt message",
			event:         sessionViewPromptFinishedEvent("sess-1", acp.StopReasonEndTurn),
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionPrompt,
			wantMethod:    acp.IMMethodPrompt,
			check: func(t *testing.T, parsed parsedSessionViewEvent) {
				t.Helper()
				result := acp.IMPromptResult{}
				if err := json.Unmarshal(parsed.message.Result, &result); err != nil {
					t.Fatalf("json.Unmarshal(prompt result): %v", err)
				}
				if result.StopReason != acp.StopReasonEndTurn {
					t.Fatalf("result.StopReason = %q, want %q", result.StopReason, acp.StopReasonEndTurn)
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
				Content:   buildACPMethodParamsContent(acp.MethodSessionPrompt, acp.SessionPromptParams{}),
			},
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionPrompt,
			wantMethod:    acp.IMMethodPrompt,
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
			if strings.TrimSpace(parsed.message.Method) != tt.wantMethod {
				t.Fatalf("parsed.message.Method = %q, want %q", parsed.message.Method, tt.wantMethod)
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
				Content:   buildACPMethodContentJSON(acp.MethodSessionPrompt, nil),
			},
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionPrompt,
			wantMethod:    acp.IMMethodPrompt,
			check: func(t *testing.T, parsed parsedSessionViewEvent) {
				t.Helper()
				request := acp.IMPromptRequest{}
				if err := json.Unmarshal(parsed.message.Request, &request); err != nil {
					t.Fatalf("json.Unmarshal(prompt request): %v", err)
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
				Content:   buildACPMethodContentJSON(acp.MethodSessionPrompt, map[string]any{"params": "oops"}),
			},
			wantMessage:   true,
			wantACPMethod: acp.MethodSessionPrompt,
			wantMethod:    acp.IMMethodPrompt,
			check: func(t *testing.T, parsed parsedSessionViewEvent) {
				t.Helper()
				request := acp.IMPromptRequest{}
				if err := json.Unmarshal(parsed.message.Request, &request); err != nil {
					t.Fatalf("json.Unmarshal(prompt request): %v", err)
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
				Content:   buildACPMethodContentJSON(acp.MethodSessionUpdate, nil),
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
				Content:   buildACPMethodContentJSON(acp.MethodSessionUpdate, map[string]any{"params": "oops"}),
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
				Content:   buildACPMethodContentJSON(acp.IMMethodSystem, nil),
			},
			wantMessage:   false,
			wantACPMethod: acp.IMMethodSystem,
			wantMethod:    "",
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
			if strings.TrimSpace(parsed.message.Method) != tt.wantMethod {
				t.Fatalf("parsed.message.Method = %q, want %q", parsed.message.Method, tt.wantMethod)
			}
			if tt.check != nil {
				tt.check(t, parsed)
			}
		})
	}
}

func TestSessionViewCreatedEventSilentlyHandlesMalformedTitle(t *testing.T) {
	c := newSessionViewTestClient(t)
	event := SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: "sess-1",
		Content:   buildACPMethodContentJSON(acp.MethodSessionNew, map[string]any{"params": map[string]any{"title": 123}}),
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
	if sessions[0].Title != "sess-1" {
		t.Fatalf("sessions[0].Title = %q, want %q", sessions[0].Title, "sess-1")
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

	_, messages, _, _, err := c.sessionRecorder.ReadSessionMessages(ctx, "sess-1", 0, 0)
	if err != nil {
		t.Fatalf("ReadSessionMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[1].UpdateIndex != 2 {
		t.Fatalf("messages[1].UpdateIndex = %d, want 2", messages[1].UpdateIndex)
	}
	update2 := decodeTurnSessionUpdate(t, messages[1].Content)
	if text := extractTextChunk(update2.Content); text != "hello world" {
		t.Fatalf("messages[1] text = %q, want %q", text, "hello world")
	}
}

func TestSessionViewListIncludesProjectionFields(t *testing.T) {
	c := newSessionViewTestClient(t)

	if err := c.RecordEvent(context.Background(), sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(context.Background(), sessionViewPromptEvent("sess-1", "hello", nil)); err != nil {
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
	if sessions[0].Agent != "claude" {
		t.Fatalf("sessions[0].Agent = %q, want %q", sessions[0].Agent, "claude")
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
	summary := body["session"].(sessionViewSummary)
	summaryType := reflect.TypeOf(summary)
	if _, ok := summaryType.FieldByName("Status"); ok {
		t.Fatalf("sessionViewSummary unexpectedly still contains Status field")
	}
	if _, ok := summaryType.FieldByName("ProjectName"); ok {
		t.Fatalf("sessionViewSummary unexpectedly still contains ProjectName field")
	}
	messages := body["messages"].([]sessionViewMessage)
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
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

func TestSessionRecorderResetPromptStateRestartsIndexes(t *testing.T) {
	c := newSessionViewTestClient(t)
	ctx := context.Background()

	if err := c.RecordEvent(ctx, sessionViewCreatedEvent("sess-1", "Task")); err != nil {
		t.Fatalf("RecordEvent session created: %v", err)
	}
	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "first", nil)); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	sqliteStore, ok := c.store.(*sqliteStore)
	if !ok {
		t.Fatalf("store type = %T, want *sqliteStore", c.store)
	}
	if _, err := sqliteStore.db.ExecContext(ctx, `DELETE FROM session_turns`); err != nil {
		t.Fatalf("DELETE session_turns: %v", err)
	}
	if _, err := sqliteStore.db.ExecContext(ctx, `DELETE FROM session_prompts`); err != nil {
		t.Fatalf("DELETE session_prompts: %v", err)
	}

	c.sessionRecorder.ResetPromptState()

	if err := c.RecordEvent(ctx, sessionViewPromptEvent("sess-1", "second", nil)); err != nil {
		t.Fatalf("RecordEvent prompt after reset: %v", err)
	}

	_, messages, _, _, err := c.sessionRecorder.ReadSessionMessages(ctx, "sess-1", 0, 0)
	if err != nil {
		t.Fatalf("ReadSessionMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].PromptIndex != 1 {
		t.Fatalf("messages[0].PromptIndex = %d, want 1", messages[0].PromptIndex)
	}
}

func TestSessionMessagePageTracksLatestCursorBeyondFilteredTurns(t *testing.T) {
	page := newSessionMessagePage(2, 2)
	page.append(SessionTurnRecord{SessionID: "sess-1", PromptIndex: 1, TurnIndex: 1, UpdateIndex: 1, UpdateJSON: `{}`})
	page.append(SessionTurnRecord{SessionID: "sess-1", PromptIndex: 2, TurnIndex: 1, UpdateIndex: 1, UpdateJSON: `{}`})
	page.append(SessionTurnRecord{SessionID: "sess-1", PromptIndex: 2, TurnIndex: 2, UpdateIndex: 1, UpdateJSON: `{}`})

	if len(page.messages) != 0 {
		t.Fatalf("page.messages len = %d, want 0", len(page.messages))
	}
	if page.lastPromptIndex != 2 {
		t.Fatalf("page.lastPromptIndex = %d, want 2", page.lastPromptIndex)
	}
	if page.lastTurnIndex != 2 {
		t.Fatalf("page.lastTurnIndex = %d, want 2", page.lastTurnIndex)
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
		Title:       "Persisted Title",

		CreatedAt:    mustRFC3339Time(t, "2026-04-12T10:00:00Z"),
		LastActiveAt: lastActiveAt,
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

	_, messages, _, _, err := c.sessionRecorder.ReadSessionMessages(context.Background(), "sess-1", 0, 0)
	if err != nil {
		t.Fatalf("ReadSessionMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}

	promptMessage := acp.IMMessage{}
	if err := json.Unmarshal([]byte(messages[0].Content), &promptMessage); err != nil {
		t.Fatalf("unmarshal prompt message: %v", err)
	}
	if strings.TrimSpace(promptMessage.Method) != acp.IMMethodPrompt {
		t.Fatalf("messages[0].method = %q, want %q", promptMessage.Method, acp.IMMethodPrompt)
	}
	promptRequest := acp.IMPromptRequest{}
	if err := json.Unmarshal(promptMessage.Request, &promptRequest); err != nil {
		t.Fatalf("unmarshal prompt request: %v", err)
	}
	if len(promptRequest.ContentBlocks) != 1 || promptRequest.ContentBlocks[0].Type != acp.ContentBlockTypeImage {
		t.Fatalf("messages[0].request.contentBlocks = %#v, want image block", promptRequest.ContentBlocks)
	}
}

func TestSessionViewStoresSystemMethodFromACPAndLegacyEvents(t *testing.T) {
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

	turns, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("turns len = %d, want 3 (prompt + two system turns)", len(turns))
	}

	for i, want := range []string{"from acp", "from legacy"} {
		msg := acp.IMMessage{}
		if err := json.Unmarshal([]byte(turns[i+1].UpdateJSON), &msg); err != nil {
			t.Fatalf("unmarshal system turn #%d: %v", i+1, err)
		}
		if strings.TrimSpace(msg.Method) != acp.IMMethodSystem {
			t.Fatalf("system turn #%d method = %q, want %q", i+1, msg.Method, acp.IMMethodSystem)
		}
		result := acp.IMTextResult{}
		if err := json.Unmarshal(msg.Result, &result); err != nil {
			t.Fatalf("unmarshal system turn #%d result: %v", i+1, err)
		}
		if strings.TrimSpace(result.Text) != want {
			t.Fatalf("system turn #%d text = %q, want %q", i+1, result.Text, want)
		}
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
		Content: buildACPMethodParamsContent(acp.MethodSessionPrompt, acp.SessionPromptParams{
			SessionID: "acp-1",
			Prompt:    []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: "run"}},
		}),
	}
	if err := c.RecordEvent(ctx, promptEvent); err != nil {
		t.Fatalf("RecordEvent prompt: %v", err)
	}

	updateEvent := SessionViewEvent{
		Type:      SessionViewEventTypeACP,
		SessionID: "client-1",
		Content: buildACPMethodParamsContent(acp.MethodSessionUpdate, acp.SessionUpdateParams{
			SessionID: "acp-1",
			Update: acp.SessionUpdate{
				SessionUpdate: acp.SessionUpdateAgentMessageChunk,
				Content:       mustJSON(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: "hello"}),
			},
		}),
	}
	if err := c.RecordEvent(ctx, updateEvent); err != nil {
		t.Fatalf("RecordEvent update: %v", err)
	}

	_, messages, _, _, err := c.sessionRecorder.ReadSessionMessages(ctx, "client-1", 0, 0)
	if err != nil {
		t.Fatalf("ReadSessionMessages(client-1): %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("client messages len = %d, want 2", len(messages))
	}

	if _, _, _, _, err := c.sessionRecorder.ReadSessionMessages(ctx, "acp-1", 0, 0); err == nil {
		t.Fatalf("ReadSessionMessages(acp-1) unexpectedly succeeded")
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

	turns, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns len = %d, want 2", len(turns))
	}
	if turns[1].UpdateIndex != 2 {
		t.Fatalf("tool turn updateIndex = %d, want 2", turns[1].UpdateIndex)
	}
	if got := decodeTurnSessionUpdate(t, turns[1].UpdateJSON).Title; strings.TrimSpace(got) != "Build finished" {
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

	stored, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored len = %d, want 2", len(stored))
	}
	updateStored := decodeTurnSessionUpdate(t, stored[1].UpdateJSON)
	if strings.TrimSpace(updateStored.SessionUpdate) != acp.SessionUpdateAgentMessageChunk {
		t.Fatalf("stored assistant update kind = %q, want %q", updateStored.SessionUpdate, acp.SessionUpdateAgentMessageChunk)
	}
	if stored[1].UpdateIndex != 2 {
		t.Fatalf("stored assistant UpdateIndex = %d, want 2", stored[1].UpdateIndex)
	}
	if text := strings.TrimSpace(extractTextChunk(updateStored.Content)); text != "hello world" {
		t.Fatalf("stored assistant text = %q, want hello world", text)
	}

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterIndex": 0})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	messages := body["messages"].([]sessionViewMessage)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[1].UpdateIndex != 2 {
		t.Fatalf("messages[1].UpdateIndex = %d, want 2", messages[1].UpdateIndex)
	}
	update2 := decodeTurnSessionUpdate(t, messages[1].Content)
	if strings.TrimSpace(extractTextChunk(update2.Content)) != "hello world" {
		t.Fatalf("message[1] text = %q, want hello world", extractTextChunk(update2.Content))
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

	_, messages, _, _, err := c.sessionRecorder.ReadSessionMessages(ctx, "sess-1", 0, 0)
	if err != nil {
		t.Fatalf("ReadSessionMessages: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(messages))
	}
	seen := map[string]string{}
	for _, message := range messages {
		if decodeTurnMethod(t, message.Content) == acp.MethodSessionPrompt {
			continue
		}
		update := decodeTurnSessionUpdate(t, message.Content)
		seen[update.SessionUpdate] = extractTextChunk(update.Content)
	}
	if got := seen[acp.SessionUpdateUserMessageChunk]; got != "user says hi" {
		t.Fatalf("user chunk text = %q, want %q", got, "user says hi")
	}
	if got := seen[acp.SessionUpdateAgentMessageChunk]; got != "assistant says hi" {
		t.Fatalf("assistant chunk text = %q, want %q", got, "assistant says hi")
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

	messages, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages len = %d, want 0", len(messages))
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

	messages, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages len = %d, want 0", len(messages))
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
	if got := last["updateIndex"].(int64); got != 2 {
		t.Fatalf("published updateIndex = %d, want 2", got)
	}
	content, _ := last["content"].(string)
	if text := extractTextChunk(decodeTurnSessionUpdate(t, content).Content); text != "world" {
		t.Fatalf("published content text = %q, want world", text)
	}
}

func TestSessionViewReadAfterIndexReturnsIncrementalMessages(t *testing.T) {
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

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterIndex": 1, "afterSubIndex": 1})
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
	if method := decodeTurnMethod(t, messages[0].Content); method != acp.SessionUpdateAgentMessageChunk {
		t.Fatalf("messages[0] method = %q, want %q", method, acp.SessionUpdateAgentMessageChunk)
	}
	if got := body["lastIndex"].(int64); got != 1 {
		t.Fatalf("lastIndex = %d, want 1", got)
	}
	if got := body["lastSubIndex"].(int64); got != 2 {
		t.Fatalf("lastSubIndex = %d, want 2", got)
	}
}

func TestSessionViewStreamingChunksAdvanceSyncIndexBeforeFlush(t *testing.T) {
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

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterIndex": 1, "afterSubIndex": 1})
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
	update := decodeTurnSessionUpdate(t, messages[0].Content)
	if text := strings.TrimSpace(extractTextChunk(update.Content)); text != "hello" {
		t.Fatalf("message text = %q, want hello", text)
	}
	if got := body["lastIndex"].(int64); got != 1 {
		t.Fatalf("lastIndex = %d, want 1", got)
	}
	if got := body["lastSubIndex"].(int64); got != 2 {
		t.Fatalf("lastSubIndex = %d, want 2", got)
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

	turns, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns len = %d, want 2 (prompt + merged assistant turn)", len(turns))
	}
	if turns[1].UpdateIndex != 2 {
		t.Fatalf("assistant turn updateIndex = %d, want 2", turns[1].UpdateIndex)
	}
	update2 := decodeTurnSessionUpdate(t, turns[1].UpdateJSON)
	if text := strings.TrimSpace(extractTextChunk(update2.Content)); text != "hello world" {
		t.Fatalf("assistant text = %q, want hello world", text)
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

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterIndex": 0})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := c.HandleSessionRequest(ctx, "session.read", "proj1", payload)
	if err != nil {
		t.Fatalf("HandleSessionRequest: %v", err)
	}
	body := resp.(map[string]any)
	messages := body["messages"].([]sessionViewMessage)
	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4 (prompt + assistant + thought + tool)", len(messages))
	}

	seen := map[string]bool{}
	for _, message := range messages {
		if decodeTurnMethod(t, message.Content) == acp.MethodSessionPrompt {
			seen["prompt"] = true
			continue
		}
		update := decodeTurnSessionUpdate(t, message.Content)
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
func TestHandleSessionRequestMarkReadIsUnsupported(t *testing.T) {
	c := newSessionViewTestClient(t)
	_, err := c.HandleSessionRequest(context.Background(), "session.markRead", "proj1", []byte(`{"sessionId":"sess-1"}`))
	if err == nil {
		t.Fatalf("expected session.markRead to be unsupported")
	}
}

func TestSessionViewReadAfterSubIndexReturnsUpdatedMessage(t *testing.T) {
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

	payload, err := json.Marshal(map[string]any{"sessionId": "sess-1", "afterIndex": 1, "afterSubIndex": 1})
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
	if method := decodeTurnMethod(t, messages[0].Content); method != acp.SessionUpdateAgentMessageChunk {
		t.Fatalf("messages[0] method = %q, want %q", method, acp.SessionUpdateAgentMessageChunk)
	}
	if got := body["lastIndex"].(int64); got != 1 {
		t.Fatalf("lastIndex = %d, want 1", got)
	}
	if got := body["lastSubIndex"].(int64); got != 2 {
		t.Fatalf("lastSubIndex = %d, want 2", got)
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

	msg := acp.IMMessage{}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("unmarshal turn update_json: %v", err)
	}
	switch strings.TrimSpace(msg.Method) {
	case acp.IMMethodAgentMessage, acp.IMMethodAgentThought, acp.SessionUpdateUserMessageChunk:
		result := acp.IMTextResult{}
		if err := json.Unmarshal(msg.Result, &result); err != nil {
			t.Fatalf("unmarshal text result: %v", err)
		}
		return acp.SessionUpdate{SessionUpdate: strings.TrimSpace(msg.Method), Content: mustJSON(map[string]any{"text": result.Text})}
	case acp.IMMethodToolCall:
		result := acp.IMToolResult{}
		if err := json.Unmarshal(msg.Result, &result); err != nil {
			t.Fatalf("unmarshal tool result: %v", err)
		}
		return acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateToolCallUpdate,
			Title:         strings.TrimSpace(result.Cmd),
			Kind:          strings.TrimSpace(result.Kind),
			Status:        strings.TrimSpace(result.Status),
			Content:       mustJSON(map[string]any{"text": result.Output}),
		}
	case acp.IMMethodAgentPlan:
		plan := []acp.IMPlanResult{}
		if err := json.Unmarshal(msg.Result, &plan); err != nil {
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
	msg := acp.IMMessage{}
	if err := json.Unmarshal([]byte(raw), &msg); err == nil {
		switch strings.TrimSpace(msg.Method) {
		case acp.IMMethodPrompt:
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

	turns, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns len = %d, want 2 (prompt + merged tool turn)", len(turns))
	}
	toolTurn := turns[1]
	if toolTurn.UpdateIndex != 2 {
		t.Fatalf("tool turn updateIndex = %d, want 2", toolTurn.UpdateIndex)
	}
	update := decodeTurnSessionUpdate(t, toolTurn.UpdateJSON)
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

	turnsPrompt1, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns prompt1: %v", err)
	}
	turnsPrompt2, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 2)
	if err != nil {
		t.Fatalf("ListSessionTurns prompt2: %v", err)
	}
	if len(turnsPrompt1) != 2 || len(turnsPrompt2) != 2 {
		t.Fatalf("turn counts = (%d,%d), want (2,2)", len(turnsPrompt1), len(turnsPrompt2))
	}

	update1 := decodeTurnSessionUpdate(t, turnsPrompt1[1].UpdateJSON)
	update2 := decodeTurnSessionUpdate(t, turnsPrompt2[1].UpdateJSON)
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

	turns, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns len = %d, want 2 (prompt + merged tool turn)", len(turns))
	}
	toolTurn := turns[1]
	if toolTurn.UpdateIndex != 3 {
		t.Fatalf("tool turn updateIndex = %d, want 3", toolTurn.UpdateIndex)
	}
	update := decodeTurnSessionUpdate(t, toolTurn.UpdateJSON)
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

	turns, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
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

	turns, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
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

	prompts, err := c.store.ListSessionPrompts(ctx, "proj1", "sess-1")
	if err != nil {
		t.Fatalf("ListSessionPrompts: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("prompts len = %d, want 2", len(prompts))
	}

	turnsPrompt1, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns prompt1: %v", err)
	}
	turnsPrompt2, err := c.store.ListSessionTurns(ctx, "proj1", "sess-1", 2)
	if err != nil {
		t.Fatalf("ListSessionTurns prompt2: %v", err)
	}
	if len(turnsPrompt1) != 2 || len(turnsPrompt2) != 2 {
		t.Fatalf("turn counts = (%d,%d), want (2,2)", len(turnsPrompt1), len(turnsPrompt2))
	}
	update1 := decodeTurnSessionUpdate(t, turnsPrompt1[1].UpdateJSON)
	update2 := decodeTurnSessionUpdate(t, turnsPrompt2[1].UpdateJSON)
	if text := extractTextChunk(update1.Content); text != "hello" {
		t.Fatalf("prompt1 assistant text = %q, want %q", text, "hello")
	}
	if text := extractTextChunk(update2.Content); text != "world" {
		t.Fatalf("prompt2 assistant text = %q, want %q", text, "world")
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
	if err := store.SaveProject(ctx, "proj-a", ProjectConfig{}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := store.SaveRouteBinding(ctx, "proj-a", "im:feishu:chat-1", "sess-1"); err != nil {
		t.Fatalf("SaveRouteBinding() error = %v", err)
	}
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj-a",
		Status:       SessionPersisted,
		ACPSessionID: "acp-1",
		AgentsJSON:   `{"claude":{"acpSessionId":"acp-1"}}`,
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
	if err := store.SaveProject(ctx, "proj-a", ProjectConfig{}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := store.SaveRouteBinding(ctx, "proj-a", "im:feishu:chat-1", "sess-1"); err != nil {
		t.Fatalf("SaveRouteBinding() error = %v", err)
	}
	if err := store.SaveSession(ctx, &SessionRecord{
		ID:           "sess-1",
		ProjectName:  "proj-a",
		Status:       SessionSuspended,
		ACPSessionID: "acp-1",
		AgentsJSON:   `{"claude":{"acpSessionId":"acp-1","title":"Persisted"}}`,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	cfg, err := store.LoadProject(ctx, "proj-a")
	if err != nil {
		t.Fatalf("LoadProject() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadProject() = nil, want config")
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
