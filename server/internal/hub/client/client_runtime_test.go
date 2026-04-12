package client

import (
	"bytes"
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
	logger "github.com/swm8023/wheelmaker/internal/shared"
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

func (f *fakeIMRouter) PublishPermissionRequest(context.Context, im.SendTarget, int64, acp.PermissionRequestParams) error {
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

func (r *TestCaptureRouter) PublishPermissionRequest(context.Context, im.SendTarget, int64, acp.PermissionRequestParams) error {
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
	return acp.SessionListResult{}, nil
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
func (s *noopStore) ListSessions(context.Context, string) ([]SessionListEntry, error) {
	return nil, nil
}
func (s *noopStore) AppendSessionMessage(context.Context, SessionMessageRecord) error { return nil }
func (s *noopStore) UpsertSessionMessage(context.Context, SessionMessageRecord) error { return nil }
func (s *noopStore) ListSessionMessages(context.Context, string, string) ([]SessionMessageRecord, error) {
	return nil, nil
}
func (s *noopStore) LoadSessionMessage(context.Context, string, string, string) (*SessionMessageRecord, error) {
	return nil, nil
}
func (s *noopStore) ListSessionMessagesAfterIndex(context.Context, string, string, int64) ([]SessionMessageRecord, error) {
	return nil, nil
}
func (s *noopStore) HasSessionMessage(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (s *noopStore) DeleteSession(context.Context, string, string) error { return nil }
func (s *noopStore) Close() error                                        { return nil }

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
	c.InjectAgentFactory(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) {
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
func (f *failingPermissionIMRouter) PublishPermissionRequest(context.Context, im.SendTarget, int64, acp.PermissionRequestParams) error {
	return errors.New("publish fail")
}
func (f *failingPermissionIMRouter) SystemNotify(context.Context, im.SendTarget, im.SystemPayload) error {
	return nil
}
func (f *failingPermissionIMRouter) Run(context.Context) error { return nil }

func TestPermissionRouter_PublishFailureLogged(t *testing.T) {
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
	_, _ = s.SessionRequestPermission(context.Background(), 1, acp.PermissionRequestParams{})
	if got := buf.String(); got == "" || !strings.Contains(got, "permission publish failed") {
		t.Fatalf("expected permission publish failure log, got: %q", got)
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
		LastReply:    "previous reply",
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
	if sess.lastReply != "previous reply" {
		t.Fatalf("lastReply = %q, want previous reply", sess.lastReply)
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
		ID:                 "persisted-only",
		ProjectName:        "proj1",
		Status:             SessionSuspended,
		AgentsJSON:         `{"claude":{"title":"Persisted Title"}}`,
		LastMessagePreview: "persisted preview",
		LastMessageAt:      lastMessageAt,
		MessageCount:       4,
		CreatedAt:          createdAt,
		LastActiveAt:       lastMessageAt,
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
	if entries[0].Preview != "persisted preview" {
		t.Fatalf("entries[0].Preview = %q, want %q", entries[0].Preview, "persisted preview")
	}
	if entries[0].MessageCount != 4 {
		t.Fatalf("entries[0].MessageCount = %d, want 4", entries[0].MessageCount)
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
		ID:                 "sess-1",
		ProjectName:        "proj1",
		Status:             SessionSuspended,
		Title:              "Persisted Title",
		LastMessagePreview: "hello from store",
		LastMessageAt:      lastMessageAt,
		MessageCount:       3,
		CreatedAt:          createdAt,
		LastActiveAt:       lastMessageAt,
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
	if entries[0].Preview != "hello from store" {
		t.Fatalf("entries[0].Preview = %q, want %q", entries[0].Preview, "hello from store")
	}
	if entries[0].MessageCount != 3 {
		t.Fatalf("entries[0].MessageCount = %d, want 3", entries[0].MessageCount)
	}
	if entries[0].Status != SessionActive {
		t.Fatalf("entries[0].Status = %v, want %v", entries[0].Status, SessionActive)
	}
	if !entries[0].InMemory {
		t.Fatal("entries[0].InMemory = false, want true")
	}
	if !entries[0].LastMessageAt.Equal(lastMessageAt) {
		t.Fatalf("entries[0].LastMessageAt = %v, want %v", entries[0].LastMessageAt, lastMessageAt)
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
	s.registry.Register(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) {
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
	s.registry.Register(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) {
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
	c.registry.Register(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) { return inst, nil })
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
	s.registry.Register(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) {
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
	s.registry.Register(acp.ACPProviderClaude, func(context.Context) (agent.Instance, error) {
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
