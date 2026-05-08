package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

const commandTimeout = 30 * time.Second

const acpClientProtocolVersion = 1

var acpClientInfo = &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}

type promptState struct {
	ctx       context.Context
	cancel    context.CancelFunc
	updatesCh chan<- acp.SessionUpdateParams
	currentCh <-chan promptStreamEvent // tracked for prompt lifecycle cleanup
}

type promptStreamEvent struct {
	update *acp.SessionUpdateParams
	result *acp.SessionPromptResult
	err    error
}

type IMRouter interface {
	Bind(ctx context.Context, chat im.ChatRef, sessionID string, opts im.BindOptions) error
	PublishSessionUpdate(ctx context.Context, target im.SendTarget, params acp.SessionUpdateParams) error
	PublishPromptResult(ctx context.Context, target im.SendTarget, result acp.SessionPromptResult) error
	SystemNotify(ctx context.Context, target im.SendTarget, payload im.SystemPayload) error
	Run(ctx context.Context) error
}

type IMSessionMessageRouter interface {
	PublishSessionMessage(ctx context.Context, target im.SendTarget, message acp.IMTurnMessage) error
}

// Client is the top-level coordinator for a single WheelMaker project.
// Agent initialization is lazy: the first incoming message triggers ensureInstance(),
// which connects the active agent and creates the ACP forwarder.
type Client struct {
	projectName string
	cwd         string

	registry *agent.ACPFactory

	store    Store
	imRouter IMRouter

	mu sync.Mutex

	// sessions maps session IDs to Session objects.
	sessions map[string]*Session

	// routeMap maps IM routing keys to Session IDs.
	// Multiple routes can point to the same Session.
	routeMap map[string]string

	// suspendTimeout is how long a Suspended session stays in memory before
	// being persisted to SQLite and evicted. Default: 5 minutes.
	suspendTimeout time.Duration
	stopPersistCh  chan struct{} // closed to stop the persist timer goroutine

	sessionRecorder *SessionRecorder
	viewSink        SessionViewSink
	httpClient      *http.Client
	deepSeekBaseURL string
}

// New creates a Client for the given project.
func New(store Store, projectName string, cwd string) *Client {
	c := &Client{
		projectName:     projectName,
		cwd:             cwd,
		registry:        agent.DefaultACPFactory(),
		store:           store,
		sessions:        make(map[string]*Session),
		routeMap:        make(map[string]string),
		suspendTimeout:  5 * time.Minute,
		stopPersistCh:   make(chan struct{}),
		httpClient:      &http.Client{Timeout: 15 * time.Second},
		deepSeekBaseURL: "https://api.deepseek.com",
	}
	c.sessionRecorder = newSessionRecorder(projectName, store, func(ctx context.Context) ([]SessionRecord, error) {
		return c.ListSessions(ctx)
	})
	c.sessionRecorder.modelLookup = func(sessionID string) string {
		options := c.sessionConfigOptions(context.Background(), sessionID)
		for _, opt := range options {
			if opt.ID == "model" {
				return opt.CurrentValue
			}
		}
		return ""
	}
	c.viewSink = c.sessionRecorder
	return c
}

func (c *Client) ProjectName() string {
	return c.projectName
}

// Start loads persisted state.
// Agent initialization is deferred until the first incoming IM event (lazy init).
func (c *Client) Start(ctx context.Context) error {
	if err := c.store.SaveProjectDefaultAgent(ctx, c.projectName, ""); err != nil {
		return fmt.Errorf("client: ensure project row: %w", err)
	}
	bindings, err := c.store.LoadRouteBindings(ctx, c.projectName)
	if err != nil {
		return fmt.Errorf("client: load route bindings: %w", err)
	}
	c.mu.Lock()
	c.routeMap = bindings
	c.mu.Unlock()
	go c.persistLoop()
	return nil
}

// Run blocks until ctx is cancelled, delegating to the IM router's Run loop.
// Returns an error if no IM router is configured.
func (c *Client) Run(ctx context.Context) error {
	if c.imRouter != nil {
		return c.imRouter.Run(ctx)
	}
	return errors.New("no IM router configured")
}

// Close persists all in-memory sessions and shuts down active agents.
func (c *Client) Close() error {
	// Stop the persist timer goroutine.
	select {
	case <-c.stopPersistCh:
	default:
		close(c.stopPersistCh)
	}

	c.mu.Lock()
	sessions := make([]*Session, 0, len(c.sessions))
	for _, sess := range c.sessions {
		sessions = append(sessions, sess)
	}
	store := c.store
	c.mu.Unlock()

	ctx := context.Background()
	for _, sess := range sessions {
		sess.mu.Lock()
		inst := sess.instance
		sess.mu.Unlock()
		if inst != nil {
			if err := sess.Suspend(ctx); err != nil {
				hubLogger(c.projectName).Warn("suspend session during close session=%s err=%v", sess.acpSessionID, err)
			}
			continue
		}
		if err := sess.persistSession(ctx); err != nil {
			hubLogger(c.projectName).Warn("persist session during close session=%s err=%v", sess.acpSessionID, err)
		}
	}
	if c.sessionRecorder != nil {
		c.sessionRecorder.Close()
	}
	if store != nil {
		return store.Close()
	}
	return nil
}

func loadProjectAgentPreferenceState(store Store, projectName string, agentName string) PreferenceState {
	if store == nil || strings.TrimSpace(agentName) == "" {
		return PreferenceState{}
	}
	rec, err := store.LoadAgentPreference(context.Background(), projectName, strings.TrimSpace(agentName))
	if err != nil || rec == nil || strings.TrimSpace(rec.PreferenceJSON) == "" {
		return PreferenceState{}
	}
	var pref PreferenceState
	if err := json.Unmarshal([]byte(rec.PreferenceJSON), &pref); err != nil {
		hubLogger(projectName).Warn("decode agent preference failed agent=%s err=%v", agentName, err)
		return PreferenceState{}
	}
	pref.ConfigOptions = sanitizePreferenceConfigOptions(pref.ConfigOptions)
	return pref
}

func sanitizePreferenceConfigOptions(options []PreferenceConfigOption) []PreferenceConfigOption {
	if len(options) == 0 {
		return nil
	}
	out := make([]PreferenceConfigOption, 0, len(options))
	seen := make(map[string]struct{}, len(options))
	for _, opt := range options {
		id := strings.TrimSpace(opt.ID)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, PreferenceConfigOption{
			ID:           id,
			CurrentValue: opt.CurrentValue,
		})
	}
	return out
}

func (c *Client) createSessionState(ctx context.Context, agentType, title string) (*createdSessionState, error) {
	creator := c.registry.CreatorByName(agentType)
	if creator == nil {
		fallback := ""
		if c.registry != nil {
			fallback = strings.TrimSpace(c.registry.PreferredName())
		}
		if fallback == "" {
			return nil, fmt.Errorf("no available ACP provider")
		}
		if !strings.EqualFold(fallback, agentType) {
			hubLogger(c.projectName).Warn("requested agent unavailable requested=%s fallback=%s", agentType, fallback)
		}
		agentType = fallback
		creator = c.registry.CreatorByName(agentType)
		if creator == nil {
			return nil, fmt.Errorf("no agent registered for %q", agentType)
		}
	}

	inst, err := creator(ctx, c.cwd)
	if err != nil {
		return nil, fmt.Errorf("create session instance: %w", err)
	}
	closeOnErr := true
	defer func() {
		if closeOnErr {
			_ = inst.Close()
		}
	}()

	clientCaps := acp.ClientCapabilities{
		FS: &acp.FSCapabilities{
			ReadTextFile:  true,
			WriteTextFile: true,
		},
		Terminal: true,
	}
	initResult, err := inst.Initialize(ctx, acp.InitializeParams{
		ProtocolVersion:    acpClientProtocolVersion,
		ClientCapabilities: clientCaps,
		ClientInfo:         acpClientInfo,
	})
	if err != nil {
		return nil, fmt.Errorf("create session initialize: %w", err)
	}

	preference := loadProjectAgentPreferenceState(c.store, c.projectName, agentType)

	newResult, err := inst.SessionNew(ctx, acp.SessionNewParams{
		CWD:        c.cwd,
		MCPServers: emptyMCPServers(),
	})
	if err != nil {
		return nil, fmt.Errorf("create session new: %w", err)
	}
	sessionID := strings.TrimSpace(newResult.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("create session new: empty session id")
	}

	resolved := append([]acp.ConfigOption(nil), newResult.ConfigOptions...)
	if len(preference.ConfigOptions) > 0 {
		resolved = applyStoredConfigOptions(ctx, c.projectName, inst, sessionID, resolved, preference.ConfigOptions)
	}

	state := SessionAgentState{
		ConfigOptions:     append([]acp.ConfigOption(nil), resolved...),
		Commands:          []acp.AvailableCommand{},
		Title:             strings.TrimSpace(title),
		AgentCapabilities: initResult.AgentCapabilities,
		AgentInfo:         cloneAgentInfo(initResult.AgentInfo),
		AuthMethods:       append([]acp.AuthMethod(nil), initResult.AuthMethods...),
	}

	closeOnErr = false
	return &createdSessionState{
		sessionID: sessionID,
		agentType: agentType,
		state:     state,
		instance:  inst,
		createdAt: time.Now(),
	}, nil
}

func (c *Client) CreateSession(ctx context.Context, agentType, title string) (*Session, error) {
	agentType = strings.TrimSpace(agentType)
	if agentType == "" {
		return nil, fmt.Errorf("agent type is required")
	}
	created, err := c.createSessionState(ctx, agentType, title)
	if err != nil {
		return nil, err
	}
	sess, err := c.newWiredSession(created.sessionID, created.agentType)
	if err != nil {
		_ = created.instance.Close()
		return nil, err
	}
	sess.mu.Lock()
	sess.instance = created.instance
	sess.agentState = created.state
	sess.createdAt = created.createdAt
	// New sessions already completed initialize + session/new before Session construction,
	// so they start ready without re-entering ensureReady.
	sess.ready = true
	sess.mu.Unlock()
	created.instance.SetCallbacks(sess)
	sess.persistAgentPreferenceState(created.agentType, created.state.ConfigOptions)
	if err := sess.persistSession(ctx); err != nil {
		sess.mu.Lock()
		sess.instance = nil
		sess.ready = false
		sess.mu.Unlock()
		_ = created.instance.Close()
		return nil, fmt.Errorf("save session: %w", err)
	}
	c.mu.Lock()
	c.sessions[sess.acpSessionID] = sess
	c.mu.Unlock()
	return sess, nil
}

func (c *Client) SessionByID(ctx context.Context, sessionID string) (*Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	c.mu.Lock()
	if sess := c.sessions[sessionID]; sess != nil {
		c.mu.Unlock()
		return sess, nil
	}
	store := c.store
	c.mu.Unlock()

	rec, err := store.LoadSession(ctx, c.projectName, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session %q: %w", sessionID, err)
	}
	if rec == nil {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	restored, err := sessionFromRecord(rec, c.cwd)
	if err != nil {
		return nil, err
	}
	c.wireSession(restored)
	restored.Status = SessionActive
	c.mu.Lock()
	c.sessions[restored.acpSessionID] = restored
	c.mu.Unlock()
	return restored, nil
}

func normalizeChatRef(source im.ChatRef) im.ChatRef {
	return im.ChatRef{ChannelID: strings.TrimSpace(source.ChannelID), ChatID: strings.TrimSpace(source.ChatID)}
}

func hasChatRef(source im.ChatRef) bool {
	return source.ChannelID != "" && source.ChatID != ""
}

func (c *Client) PromptToSession(ctx context.Context, sessionID string, source im.ChatRef, blocks []acp.ContentBlock) error {
	sess, err := c.SessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	source = normalizeChatRef(source)
	if hasChatRef(source) {
		sess.setIMSource(source)
		if err := c.bindIM(ctx, source, sess.acpSessionID); err != nil {
			return err
		}
		if err := c.store.SaveRouteBinding(ctx, c.projectName, imRouteKey(source), sess.acpSessionID); err != nil {
			return fmt.Errorf("save route binding: %w", err)
		}
	}
	sess.handlePromptBlocks(blocks)
	return nil
}

func promptTitleFromBlocks(blocks []acp.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Type) == acp.ContentBlockTypeText && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	for _, block := range blocks {
		if strings.TrimSpace(block.Type) == acp.ContentBlockTypeImage {
			return "Sent an image"
		}
	}
	return ""
}

func cloneSessionContentBlocks(blocks []acp.ContentBlock) []acp.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	return cloneJSON(blocks)
}

func cloneSessionPermissionOptions(options []acp.PermissionOption) []acp.PermissionOption {
	if len(options) == 0 {
		return nil
	}
	return cloneJSON(options)
}

func cloneJSON[T any](value T) T {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Errorf("clone JSON: %w", err))
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(fmt.Errorf("clone JSON: %w", err))
	}
	return out
}

func (c *Client) SetIMRouter(router IMRouter) {
	c.mu.Lock()
	c.imRouter = router
	for _, sess := range c.sessions {
		sess.imRouter = router
	}
	c.mu.Unlock()
}

func (c *Client) HasIMRouter() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.imRouter != nil
}

func (c *Client) SetSessionViewSink(sink SessionViewSink) {
	if sink == nil {
		sink = c.sessionRecorder
	}
	c.mu.Lock()
	c.viewSink = sink
	for _, sess := range c.sessions {
		sess.viewSink = sink
	}
	c.mu.Unlock()
}

func (c *Client) SetSessionEventPublisher(publish func(method string, payload any) error) {
	c.sessionRecorder.SetEventPublisher(publish)
}

func (c *Client) ResetSessionPromptState() {
	if c == nil || c.sessionRecorder == nil {
		return
	}
	c.sessionRecorder.ResetPromptState()
}

func (c *Client) RecordEvent(ctx context.Context, event SessionViewEvent) error {
	return c.sessionRecorder.RecordEvent(ctx, event)
}

func (c *Client) HandleSessionRequest(ctx context.Context, method string, _ string, payload json.RawMessage) (any, error) {
	switch strings.TrimSpace(method) {
	case "session.list":
		sessions, err := c.sessionRecorder.ListSessionViews(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"sessions": sessions}, nil
	case "session.read":
		var req struct {
			SessionID   string `json:"sessionId"`
			PromptIndex int64  `json:"promptIndex,omitempty"`
			TurnIndex   int64  `json:"turnIndex,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.read payload: %w", err)
		}
		summary, prompts, messages, err := c.sessionRecorder.ReadSessionPrompts(ctx, req.SessionID, req.PromptIndex, req.TurnIndex)
		if err != nil {
			return nil, err
		}
		summary.ConfigOptions = c.sessionConfigOptions(ctx, req.SessionID)
		return map[string]any{"session": summary, "prompts": prompts, "messages": messages}, nil
	case "session.new":
		var req struct {
			AgentType string `json:"agentType"`
			Title     string `json:"title,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.new payload: %w", err)
		}
		if strings.TrimSpace(req.AgentType) == "" {
			return nil, fmt.Errorf("agentType is required")
		}
		sess, err := c.CreateSession(ctx, req.AgentType, req.Title)
		if err != nil {
			return nil, err
		}
		if c.store != nil {
			if err := c.store.SaveProjectDefaultAgent(ctx, c.projectName, sess.agentType); err != nil {
				hubLogger(c.projectName).Warn("save project default agent failed agent=%s err=%v", sess.agentType, err)
			}
		}
		if err := c.RecordEvent(ctx, SessionViewEvent{
			Type:      SessionViewEventTypeACP,
			SessionID: sess.acpSessionID,
			Content: acp.BuildACPContentJSON(acp.MethodSessionNew, map[string]any{
				"params": sessionViewSessionNewParams{
					SessionID: sess.acpSessionID,
					AgentType: sess.agentType,
					Title:     firstNonEmpty(req.Title, sess.acpSessionID),
				},
			}),
		}); err != nil {
			return nil, err
		}
		summary, _, _, err := c.sessionRecorder.ReadSessionPrompts(ctx, sess.acpSessionID, 0, 0)
		if err != nil {
			return nil, err
		}
		summary.ConfigOptions = sess.CurrentConfigOptions()
		return map[string]any{"ok": true, "session": summary}, nil
	case "session.resume.list":
		var req struct {
			AgentType string `json:"agentType"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.resume.list payload: %w", err)
		}
		return c.recovery().ListResumableSessions(ctx, req.AgentType)
	case "session.resume.import":
		var req struct {
			SessionID string `json:"sessionId"`
			AgentType string `json:"agentType"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.resume.import payload: %w", err)
		}
		return c.recovery().ImportResumableSession(ctx, req.AgentType, req.SessionID)
	case "session.reload":
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.reload payload: %w", err)
		}
		return c.recovery().ReloadSession(ctx, req.SessionID)
	case "session.token.providers":
		return map[string]any{
			"providers": []map[string]any{
				{"id": "deepseek", "name": "DeepSeek", "authMode": "api_key"},
				{"id": "codex", "name": "Codex", "authMode": "oauth"},
				{"id": "copilot", "name": "Copilot", "authMode": "github_api"},
			},
		}, nil
	case "session.token.scan":
		stats, err := c.scanTokenStats(ctx)
		if err != nil {
			return nil, err
		}
		return stats, nil
	case "session.token.deepseek.stats":
		var req struct {
			APIKey    string `json:"apiKey"`
			RangeType string `json:"rangeType,omitempty"`
			Month     string `json:"month,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.token.deepseek.stats payload: %w", err)
		}
		stats, err := c.fetchDeepSeekTokenStats(ctx, req.APIKey, req.RangeType, req.Month)
		if err != nil {
			return nil, err
		}
		return stats, nil
	case "session.setConfig":
		var req struct {
			SessionID string `json:"sessionId"`
			ConfigID  string `json:"configId"`
			Value     string `json:"value"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.setConfig payload: %w", err)
		}
		sess, err := c.SessionByID(ctx, req.SessionID)
		if err != nil {
			return nil, err
		}
		options, err := sess.SetConfigOption(ctx, req.ConfigID, req.Value)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"ok":            true,
			"sessionId":     sess.acpSessionID,
			"configOptions": options,
		}, nil
	case "session.delete":
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.delete payload: %w", err)
		}
		if err := c.DeleteSession(ctx, req.SessionID); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "sessionId": strings.TrimSpace(req.SessionID)}, nil
	case "session.send":
		var req struct {
			SessionID string             `json:"sessionId"`
			Text      string             `json:"text,omitempty"`
			Blocks    []acp.ContentBlock `json:"blocks,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.send payload: %w", err)
		}
		blocks := req.Blocks
		if len(blocks) == 0 && strings.TrimSpace(req.Text) != "" {
			blocks = []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: req.Text}}
		}
		if strings.TrimSpace(req.SessionID) == "" {
			return nil, fmt.Errorf("sessionId is required")
		}
		if len(blocks) == 0 {
			return nil, fmt.Errorf("session prompt is empty")
		}
		if err := c.PromptToSession(ctx, req.SessionID, im.ChatRef{ChannelID: "app", ChatID: strings.TrimSpace(req.SessionID)}, blocks); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "sessionId": strings.TrimSpace(req.SessionID)}, nil
	default:
		return nil, fmt.Errorf("unsupported session method: %s", method)
	}
}

func (c *Client) listSessionViews(ctx context.Context) ([]sessionViewSummary, error) {
	return c.sessionRecorder.ListSessionViews(ctx)
}

func (c *Client) sessionConfigOptions(ctx context.Context, sessionID string) []acp.ConfigOption {
	sess, err := c.SessionByID(ctx, sessionID)
	if err != nil {
		return nil
	}
	options := sess.CurrentConfigOptions()
	if len(options) > 0 {
		return options
	}

	sess.mu.Lock()
	agentType := strings.TrimSpace(sess.agentType)
	sess.mu.Unlock()
	if agentType == "" {
		return options
	}

	preference := loadProjectAgentPreferenceState(c.store, c.projectName, agentType)
	if len(preference.ConfigOptions) == 0 {
		return options
	}
	fallback := make([]acp.ConfigOption, 0, len(preference.ConfigOptions))
	for _, pref := range preference.ConfigOptions {
		id := strings.TrimSpace(pref.ID)
		if id == "" {
			continue
		}
		fallback = append(fallback, acp.ConfigOption{
			ID:           id,
			CurrentValue: strings.TrimSpace(pref.CurrentValue),
		})
	}
	return fallback
}

func (c *Client) HandleIMPrompt(ctx context.Context, source im.ChatRef, params acp.SessionPromptParams) error {
	return c.handleIMPromptBlocks(ctx, source, params.Prompt)
}

func (c *Client) HandleIMCommand(ctx context.Context, source im.ChatRef, cmd im.Command) error {
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("client im: invalid source")
	}
	return c.handleIMCommand(ctx, source, cmd.Name, cmd.Args)
}

func (c *Client) HandleIMInbound(ctx context.Context, event im.InboundEvent) error {
	source := normalizeChatRef(im.ChatRef{ChannelID: event.ChannelID, ChatID: event.ChatID})
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("client im: invalid source")
	}
	if cmd, ok := im.ParseCommand(event.Text); ok {
		return c.HandleIMCommand(ctx, source, cmd)
	}
	return c.HandleIMPrompt(ctx, source, acp.SessionPromptParams{
		Prompt: []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: event.Text}},
	})
}

func (c *Client) bindIM(ctx context.Context, source im.ChatRef, sessionID string) error {
	c.mu.Lock()
	router := c.imRouter
	c.mu.Unlock()
	if router == nil {
		return nil
	}
	return router.Bind(ctx, source, sessionID, im.BindOptions{})
}

func (c *Client) sendIMDirect(ctx context.Context, source im.ChatRef, text string) error {
	c.mu.Lock()
	router := c.imRouter
	c.mu.Unlock()
	if router == nil {
		return nil
	}
	return router.SystemNotify(ctx, im.SendTarget{ChannelID: source.ChannelID, ChatID: source.ChatID}, im.SystemPayload{
		Kind: "message",
		Body: text,
	})
}

func (c *Client) loadSessionForIM(ctx context.Context, source im.ChatRef, routeKey, args string) (*Session, error) {
	idxStr := strings.TrimSpace(args)
	if idxStr == "" {
		return nil, fmt.Errorf("Usage: /load <index>  (see /list)")
	}
	idx, err := parsePositiveIndex(idxStr)
	if err != nil {
		return nil, err
	}
	loaded, err := c.ClientLoadSession(routeKey, idx)
	if err != nil {
		return nil, err
	}
	return loaded, c.bindIM(ctx, source, loaded.acpSessionID)
}

func imRouteKey(source im.ChatRef) string {
	source = normalizeChatRef(source)
	return "im:" + strings.ToLower(source.ChannelID) + ":" + source.ChatID
}
func normalizeIMPromptBlocks(blocks []acp.ContentBlock) []acp.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]acp.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		kind := strings.TrimSpace(block.Type)
		if kind == "" {
			continue
		}
		if kind == acp.ContentBlockTypeText {
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			block.Text = text
		}
		block.Type = kind
		out = append(out, block)
	}
	return out
}

func singleTextIMPrompt(blocks []acp.ContentBlock) (string, bool) {
	if len(blocks) != 1 || blocks[0].Type != acp.ContentBlockTypeText {
		return "", false
	}
	text := strings.TrimSpace(blocks[0].Text)
	if text == "" {
		return "", false
	}
	return text, true
}

func (c *Client) handleIMPromptBlocks(ctx context.Context, source im.ChatRef, blocks []acp.ContentBlock) error {
	source = normalizeChatRef(source)
	if source.ChannelID == "" || source.ChatID == "" {
		return fmt.Errorf("client im: invalid source")
	}
	blocks = normalizeIMPromptBlocks(blocks)
	if len(blocks) == 0 {
		return nil
	}
	if text, ok := singleTextIMPrompt(blocks); ok {
		if cmd, parsed := im.ParseCommand(text); parsed {
			return c.handleIMCommand(ctx, source, cmd.Name, cmd.Args)
		}
	}
	routeKey := imRouteKey(source)
	sess := c.resolveOrCreateIMSession(ctx, source, routeKey)
	if sess == nil {
		return nil
	}
	sess.setIMSource(source)
	sess.handlePromptBlocks(blocks)
	return nil
}

func (c *Client) handleIMText(ctx context.Context, source im.ChatRef, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return c.handleIMPromptBlocks(ctx, source, []acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}})
}

func (c *Client) handleIMCommand(ctx context.Context, source im.ChatRef, cmd, args string) error {
	routeKey := imRouteKey(source)

	if cmd == "/list" {
		body, err := c.formatSessionList("")
		if err != nil {
			body = fmt.Sprintf("List error: %v", err)
		}
		return c.sendIMDirect(ctx, source, body)
	}
	if cmd == "/new" {
		agentType := strings.TrimSpace(args)
		if agentType == "" {
			model, _, err := c.helpModelForRoute(ctx, source, routeKey)
			if err != nil {
				return c.sendIMDirect(ctx, source, fmt.Sprintf("New error: %v", err))
			}
			return c.sendHelpCard(ctx, source, model, "menu:new", 0)
		}
		sess, err := c.clientNewSessionWithOptions(routeKey, agentType, false)
		if err != nil {
			return c.sendIMDirect(ctx, source, fmt.Sprintf("New error: %v", err))
		}
		if err := c.bindIM(ctx, source, sess.acpSessionID); err != nil {
			return err
		}
		sess.setIMSource(source)
		sess.reply(fmt.Sprintf("Created new session: %s", sess.acpSessionID))
		return nil
	}
	if cmd == "/load" {
		loaded, err := c.loadSessionForIM(ctx, source, routeKey, args)
		if err != nil {
			return c.sendIMDirect(ctx, source, fmt.Sprintf("Load error: %v", err))
		}
		loaded.setIMSource(source)
		loaded.reply(fmt.Sprintf("Loaded session: %s", loaded.acpSessionID))
		return nil
	}
	if cmd == "/help" {
		menuID, page := parseHelpArgs(args)
		model, _, err := c.helpModelForRoute(ctx, source, routeKey)
		if err != nil {
			return c.sendIMDirect(ctx, source, fmt.Sprintf("Help error: %v", err))
		}
		return c.sendHelpCard(ctx, source, model, menuID, page)
	}

	sess := c.resolveOrCreateIMSession(ctx, source, routeKey)
	if sess == nil {
		return nil
	}
	sess.setIMSource(source)
	c.handleCommand(sess, routeKey, cmd, args)
	return nil
}

func (c *Client) resolveOrCreateIMSession(ctx context.Context, source im.ChatRef, routeKey string) *Session {
	c.mu.Lock()
	sessID := c.routeMap[routeKey]
	c.mu.Unlock()
	if sessID != "" {
		sess, err := c.resolveSession(routeKey)
		if err != nil {
			_ = c.sendIMDirect(ctx, source, fmt.Sprintf("Route error: %v", err))
			return nil
		}
		return sess
	}
	agentType := c.preferredAgentForAutoCreate()
	if agentType == "" {
		_ = c.sendIMDirect(ctx, source, "No available agent.")
		return nil
	}
	sess, err := c.clientNewSessionWithOptions(routeKey, agentType, false)
	if err != nil {
		_ = c.sendIMDirect(ctx, source, fmt.Sprintf("New error: %v", err))
		return nil
	}
	if err := c.bindIM(ctx, source, sess.acpSessionID); err != nil {
		return nil
	}
	return sess
}

func parseHelpArgs(args string) (menuID string, page int) {
	parts := strings.Fields(args)
	if len(parts) >= 1 {
		menuID = parts[0]
	}
	if len(parts) >= 2 {
		if n, err := strconv.Atoi(parts[1]); err == nil {
			page = n
		}
	}
	return
}

func (c *Client) sendHelpCard(ctx context.Context, source im.ChatRef, model im.HelpModel, menuID string, page int) error {
	c.mu.Lock()
	router := c.imRouter
	c.mu.Unlock()
	if router == nil {
		return nil
	}
	return router.SystemNotify(ctx, im.SendTarget{ChannelID: source.ChannelID, ChatID: source.ChatID}, im.SystemPayload{
		Kind: "help_card",
		HelpCard: &im.HelpCardPayload{
			Model:  model,
			MenuID: menuID,
			Page:   page,
		},
	})
}

// --- internal ---

// resolveSession finds or creates the Session for a given route key.
// If no session exists for the route, a new one is created.
func (c *Client) resolveSession(routeKey string) (*Session, error) {
	routeKey, err := normalizeRouteKey(routeKey)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	sessID := c.routeMap[routeKey]
	if sessID != "" {
		if sess := c.sessions[sessID]; sess != nil {
			c.mu.Unlock()
			return sess, nil
		}
	}
	store := c.store
	c.mu.Unlock()
	if sessID != "" {
		rec, err := store.LoadSession(context.Background(), c.projectName, sessID)
		if err != nil {
			return nil, fmt.Errorf("load session %q: %w", sessID, err)
		}
		if rec == nil {
			return nil, fmt.Errorf("bound session %q for route %q not found", sessID, routeKey)
		}
		restored, err := sessionFromRecord(rec, c.cwd)
		if err != nil {
			return nil, err
		}
		c.wireSession(restored)
		restored.Status = SessionActive
		c.mu.Lock()
		c.sessions[restored.acpSessionID] = restored
		c.mu.Unlock()
		return restored, nil
	}

	return nil, fmt.Errorf("route %q is not bound to a session", routeKey)
}

func (c *Client) helpModelForRoute(ctx context.Context, source im.ChatRef, routeKey string) (HelpModel, *Session, error) {
	routeKey, err := normalizeRouteKey(routeKey)
	if err != nil {
		return HelpModel{}, nil, err
	}

	c.mu.Lock()
	hasBoundSession := strings.TrimSpace(c.routeMap[routeKey]) != ""
	c.mu.Unlock()
	if hasBoundSession {
		sess, err := c.resolveSession(routeKey)
		if err != nil {
			return HelpModel{}, nil, err
		}
		sess.setIMSource(source)
		model, err := sess.resolveHelpModel(ctx, source.ChatID)
		return model, sess, err
	}

	return c.resolveDetachedHelpModel(), nil, nil
}

func (c *Client) resolveDetachedHelpModel() HelpModel {
	model := HelpModel{
		Title:    "WheelMaker",
		RootMenu: "root",
		Menus:    map[string]HelpMenu{},
	}

	newMenuID := "menu:new"
	model.Options = append(model.Options, HelpOption{Label: "New Conversation", MenuID: newMenuID})
	newMenu := HelpMenu{
		Title:  "New Conversation",
		Body:   "Choose an agent for the new conversation.",
		Parent: model.RootMenu,
	}
	if c.registry != nil {
		for _, name := range c.registry.Names() {
			newMenu.Options = append(newMenu.Options, HelpOption{Label: "Agent: " + name, Command: "/new", Value: name})
		}
	}
	if len(newMenu.Options) == 0 {
		newMenu.Body = "No agents available."
	}
	model.Menus[newMenuID] = newMenu

	sessionMenuID := "menu:sessions"
	model.Options = append(model.Options, HelpOption{Label: "Session List", MenuID: sessionMenuID})
	sessionMenu := HelpMenu{
		Title:  "Sessions",
		Body:   "Select a session to load.",
		Parent: model.RootMenu,
	}
	entries, err := c.clientListSessions()
	if err == nil {
		for i, entry := range entries {
			title := strings.TrimSpace(entry.Title)
			if title == "" {
				title = "(no title)"
			}
			sessionMenu.Options = append(sessionMenu.Options, HelpOption{
				Label:   fmt.Sprintf("%d. %s", i+1, title),
				Command: "/load",
				Value:   strconv.Itoa(i + 1),
			})
		}
	}
	if len(sessionMenu.Options) == 0 {
		sessionMenu.Body = "No sessions available."
	}
	model.Menus[sessionMenuID] = sessionMenu
	model.Options = append(model.Options, HelpOption{Label: "Status", Command: "/status"})

	return model
}

// newWiredSession creates a Session with all Client back-references wired.
// Does NOT add it to c.sessions. Caller may hold c.mu.
func (c *Client) newWiredSession(id, agentType string) (*Session, error) {
	sess, err := newSession(id, c.cwd, agentType)
	if err != nil {
		return nil, err
	}
	c.wireSession(sess)
	return sess, nil
}

func (c *Client) wireSession(sess *Session) {
	sess.projectName = c.projectName
	sess.registry = c.registry
	sess.imRouter = c.imRouter
	sess.viewSink = c.viewSink
	sess.store = c.store
}

func (c *Client) preferredAvailableAgent() string {
	if c.registry == nil {
		return ""
	}
	return strings.TrimSpace(c.registry.PreferredName())
}

func (c *Client) preferredAgentForAutoCreate() string {
	if c.store != nil {
		agentType, err := c.store.LoadProjectDefaultAgent(context.Background(), c.projectName)
		if err != nil {
			hubLogger(c.projectName).Warn("load project default agent failed err=%v", err)
		} else if agentType != "" && c.registry != nil && c.registry.CreatorByName(agentType) != nil {
			return agentType
		}
	}
	return c.preferredAvailableAgent()
}

func (c *Client) persistBoundSession(routeKey string, sess *Session) error {
	if err := sess.persistSession(context.Background()); err != nil {
		hubLogger(c.projectName).Error("save session failed route=%s session=%s err=%v",
			routeKey, sess.acpSessionID, err)
		return fmt.Errorf("save session: %w", err)
	}
	if err := c.store.SaveRouteBinding(context.Background(), c.projectName, routeKey, sess.acpSessionID); err != nil {
		hubLogger(c.projectName).Error("save route binding failed route=%s session=%s err=%v",
			routeKey, sess.acpSessionID, err)
		return fmt.Errorf("save route binding: %w", err)
	}
	return nil
}

// ClientNewSession suspends the current session for the given route,
// creates a new session, and rebinds the route. Returns the new session.
func (c *Client) ClientNewSession(routeKey, agentType string) (*Session, error) {
	return c.clientNewSessionWithOptions(routeKey, agentType, true)
}

func (c *Client) clientNewSessionWithOptions(routeKey, agentType string, persistDefault bool) (*Session, error) {
	routeKey, err := normalizeRouteKey(routeKey)
	if err != nil {
		return nil, err
	}
	agentType = strings.TrimSpace(agentType)
	if agentType == "" {
		return nil, fmt.Errorf("agent type is required")
	}
	c.mu.Lock()
	oldSessID := c.routeMap[routeKey]
	oldSess := c.sessions[oldSessID]
	c.mu.Unlock()

	if oldSess != nil {
		oldSess.mu.Lock()
		hasInst := oldSess.instance != nil
		oldSess.mu.Unlock()
		if hasInst {
			if err := oldSess.Suspend(context.Background()); err != nil {
				hubLogger(c.projectName).Warn("suspend old session failed session=%s err=%v", oldSessID, err)
			}
		}
		oldSess.mu.Lock()
		oldSess.Status = SessionSuspended
		oldSess.mu.Unlock()
	}

	sess, err := c.CreateSession(context.Background(), agentType, "")
	if err != nil {
		return nil, err
	}
	if persistDefault && c.store != nil {
		if err := c.store.SaveProjectDefaultAgent(context.Background(), c.projectName, agentType); err != nil {
			hubLogger(c.projectName).Warn("save project default agent failed agent=%s err=%v", agentType, err)
		}
	}
	if err := c.store.SaveRouteBinding(context.Background(), c.projectName, routeKey, sess.acpSessionID); err != nil {
		return nil, fmt.Errorf("save route binding: %w", err)
	}
	c.mu.Lock()
	c.routeMap[routeKey] = sess.acpSessionID
	c.mu.Unlock()
	return sess, nil
}

// ClientLoadSession restores a session by index from the merged list of
// in-memory + persisted sessions. Binds the loaded session to the given route.
func (c *Client) ClientLoadSession(routeKey string, index int) (*Session, error) {
	routeKey, err := normalizeRouteKey(routeKey)
	if err != nil {
		return nil, err
	}
	entries, err := c.clientListSessions()
	if err != nil {
		return nil, err
	}
	if index < 1 || index > len(entries) {
		return nil, fmt.Errorf("index out of range (1-%d)", len(entries))
	}
	target := entries[index-1]

	// Check if session is already in memory.
	c.mu.Lock()
	if sess := c.sessions[target.ID]; sess != nil {
		// Already in memory — just rebind the route.
		oldSessID := c.routeMap[routeKey]
		oldSess := c.sessions[oldSessID]
		c.mu.Unlock()

		// Suspend old if different from target.
		if oldSess != nil && oldSess.acpSessionID != target.ID {
			oldSess.mu.Lock()
			hasInst := oldSess.instance != nil
			oldSess.mu.Unlock()
			if hasInst {
				_ = oldSess.Suspend(context.Background())
			}
			oldSess.mu.Lock()
			oldSess.Status = SessionSuspended
			oldSess.mu.Unlock()
		}

		c.mu.Lock()
		c.routeMap[routeKey] = target.ID
		sess.Status = SessionActive
		c.mu.Unlock()
		if err := c.store.SaveRouteBinding(context.Background(), c.projectName, routeKey, target.ID); err != nil {
			return nil, fmt.Errorf("save route binding: %w", err)
		}
		return sess, nil
	}
	c.mu.Unlock()

	rec, err := c.store.LoadSession(context.Background(), c.projectName, target.ID)
	if err != nil {
		return nil, fmt.Errorf("load session %q: %w", target.ID, err)
	}
	if rec == nil {
		return nil, fmt.Errorf("session %q not found in session store", target.ID)
	}

	// Suspend old session.
	c.mu.Lock()
	oldSessID := c.routeMap[routeKey]
	oldSess := c.sessions[oldSessID]
	c.mu.Unlock()
	if oldSess != nil && oldSess.acpSessionID != target.ID {
		oldSess.mu.Lock()
		hasInst := oldSess.instance != nil
		oldSess.mu.Unlock()
		if hasInst {
			_ = oldSess.Suspend(context.Background())
		}
		oldSess.mu.Lock()
		oldSess.Status = SessionSuspended
		oldSess.mu.Unlock()
	}

	restored, err := sessionFromRecord(rec, c.cwd)
	if err != nil {
		return nil, err
	}
	c.wireSession(restored)
	c.mu.Lock()
	restored.Status = SessionActive
	c.sessions[restored.acpSessionID] = restored
	c.routeMap[routeKey] = restored.acpSessionID
	c.mu.Unlock()
	if err := c.store.SaveRouteBinding(context.Background(), c.projectName, routeKey, restored.acpSessionID); err != nil {
		return nil, fmt.Errorf("save route binding: %w", err)
	}
	return restored, nil
}

// clientListSessions returns a merged list of in-memory and persisted sessions,
// sorted by last active time (most recent first). Duplicates are deduplicated
// favoring in-memory sessions.
func (c *Client) ListSessions(ctx context.Context) ([]SessionRecord, error) {
	c.mu.Lock()
	memEntries := make([]SessionRecord, 0, len(c.sessions))
	memIDs := make(map[string]bool, len(c.sessions))
	for _, sess := range c.sessions {
		sess.mu.Lock()
		agentType := sess.agentType
		title := ""
		title = sess.agentState.Title
		e := SessionRecord{
			ID:           sess.acpSessionID,
			ProjectName:  c.projectName,
			AgentType:    agentType,
			Agent:        agentType,
			Title:        title,
			Status:       sess.Status,
			CreatedAt:    sess.createdAt,
			LastActiveAt: sess.lastActiveAt,
			InMemory:     true,
		}
		sess.mu.Unlock()
		memEntries = append(memEntries, e)
		memIDs[sess.acpSessionID] = true
	}
	store := c.store
	c.mu.Unlock()

	entries := memEntries

	stored, err := store.ListSessions(ctx, c.projectName)
	if err != nil {
		return nil, fmt.Errorf("list persisted sessions: %w", err)
	}
	storedByID := make(map[string]SessionRecord, len(stored))
	for _, s := range stored {
		storedByID[s.ID] = s
	}
	for i := range entries {
		storedEntry, ok := storedByID[entries[i].ID]
		if !ok {
			continue
		}
		if entries[i].Agent == "" {
			entries[i].Agent = storedEntry.Agent
		}
		if strings.TrimSpace(storedEntry.Title) != "" {
			entries[i].Title = storedEntry.Title
		}
		if !storedEntry.LastActiveAt.IsZero() {
			entries[i].LastActiveAt = storedEntry.LastActiveAt
		}
	}
	for _, s := range stored {
		if memIDs[s.ID] {
			continue
		}
		s.InMemory = false
		s.Status = SessionPersisted
		entries = append(entries, s)
	}

	sort.Slice(entries, func(i, j int) bool {
		left := entries[i].LastActiveAt
		right := entries[j].LastActiveAt
		if left.Equal(right) {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		}
		return left.After(right)
	})
	return entries, nil
}

func (c *Client) DeleteSession(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}

	c.mu.Lock()
	sess := c.sessions[sessionID]
	for routeKey, mappedSessionID := range c.routeMap {
		if strings.TrimSpace(mappedSessionID) == sessionID {
			delete(c.routeMap, routeKey)
		}
	}
	delete(c.sessions, sessionID)
	store := c.store
	c.mu.Unlock()

	if sess != nil {
		sess.mu.Lock()
		inst := sess.instance
		sess.instance = nil
		sess.ready = false
		sess.initializing = false
		sess.Status = SessionSuspended
		sess.mu.Unlock()
		if inst != nil {
			_ = inst.Close()
		}
	}
	if c.sessionRecorder != nil {
		c.sessionRecorder.RemovePromptState(sessionID)
	}
	if store != nil {
		if err := store.DeleteSession(ctx, c.projectName, sessionID); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) clientListSessions() ([]SessionRecord, error) {
	return c.ListSessions(context.Background())
}

// persistLoop periodically scans for Suspended sessions and evicts old ones from memory.
func (c *Client) persistLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopPersistCh:
			return
		case <-ticker.C:
			c.evictSuspendedSessions()
		}
	}
}

// evictSuspendedSessions finds Suspended sessions that have exceeded the
// suspend timeout, persists them to SQLite, and removes them from memory.
func (c *Client) evictSuspendedSessions() {
	c.mu.Lock()
	timeout := c.suspendTimeout

	var toEvict []*Session
	for _, sess := range c.sessions {
		sess.mu.Lock()
		if sess.Status == SessionSuspended && time.Since(sess.lastActiveAt) > timeout {
			toEvict = append(toEvict, sess)
		}
		sess.mu.Unlock()
	}
	c.mu.Unlock()

	for _, sess := range toEvict {
		if err := sess.persistSession(context.Background()); err != nil {
			hubLogger(c.projectName).Warn("persist session failed session=%s err=%v", sess.acpSessionID, err)
			continue
		}

		c.mu.Lock()
		sess.mu.Lock()
		sess.Status = SessionPersisted
		sess.mu.Unlock()

		// Remove from sessions map but keep route mapping pointing to the ID
		// so we can look it up later for restoration.
		delete(c.sessions, sess.acpSessionID)
		c.mu.Unlock()

		hubLogger(c.projectName).Info("evicted suspended session to sqlite session=%s", sess.acpSessionID)
	}
}

// parseCommand checks whether text is a recognized WheelMaker command.
// Only exact first-word matches (/cancel, /status, /mode, /model, /config, /list, /new, /load, /skills) are treated as commands;
// all other "/" lines fall through to the agent (fixing the "code starting with /" bug).
func parseCommand(text string) (cmd, args string, ok bool) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "/cancel", "/status", "/mode", "/model", "/config", "/list", "/new", "/load", "/skills":
		return parts[0], strings.Join(parts[1:], " "), true
	}
	return
}
func normalizeRouteKey(routeKey string) (string, error) {
	routeKey = strings.TrimSpace(routeKey)
	if routeKey == "" {
		return "", fmt.Errorf("route key is required")
	}
	return routeKey, nil
}

func decodeSessionRequestPayload(raw json.RawMessage, out any) error {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	return json.Unmarshal(raw, out)
}
