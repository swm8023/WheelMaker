package client

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/hub/tools"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

const acpClientProtocolVersion = 1

var acpClientInfo = &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}

var cleanupSessionArtifacts = agent.CleanupSessionArtifacts

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

// Client is the top-level coordinator for a single WheelMaker project.
// Agent initialization is lazy: the first incoming message triggers ensureInstance(),
// which connects the active agent and creates the ACP forwarder.
type Client struct {
	projectName string
	cwd         string

	registry *agent.ACPFactory

	store Store

	mu sync.Mutex

	// sessions maps session IDs to Session objects.
	sessions map[string]*Session

	// suspendTimeout is how long a Suspended session stays in memory before
	// being persisted to SQLite and evicted. Default: 5 minutes.
	suspendTimeout time.Duration
	stopPersistCh  chan struct{} // closed to stop the persist timer goroutine

	sessionRecorder *SessionRecorder
	archiveStore    *sessionArchiveStore
	sessionSearch   *sessionSearchManager
	attachments     *attachmentManager
	viewSink        SessionViewSink
}

// New creates a Client for the given project.
func New(store Store, projectName string, cwd string) *Client {
	c := &Client{
		projectName:    projectName,
		cwd:            cwd,
		registry:       agent.DefaultACPFactory(),
		store:          store,
		sessions:       make(map[string]*Session),
		suspendTimeout: 5 * time.Minute,
		stopPersistCh:  make(chan struct{}),
		attachments:    newAttachmentManager(),
	}
	c.sessionRecorder = newSessionRecorder(projectName, store, func(ctx context.Context) ([]SessionRecord, error) {
		return c.ListSessions(ctx)
	})
	c.sessionSearch = newSessionSearchManager(c)
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

func (c *Client) SetSessionHistoryRoot(root string) {
	if c == nil || c.sessionRecorder == nil {
		return
	}
	root = strings.TrimSpace(root)
	if root == "" {
		c.sessionRecorder.turnStore = nil
		c.archiveStore = nil
		return
	}
	c.sessionRecorder.turnStore = newFileSessionTurnStore(root)
	c.archiveStore = newSessionArchiveStore(filepath.Join(filepath.Dir(root), "session-archive"))
}

// Start loads persisted state.
// Agent initialization is deferred until the first incoming IM event (lazy init).
func (c *Client) Start(ctx context.Context) error {
	if err := c.store.SaveProjectDefaultAgent(ctx, c.projectName, ""); err != nil {
		return fmt.Errorf("client: ensure project row: %w", err)
	}
	go c.persistLoop()
	return nil
}

// Run blocks until ctx is cancelled.
func (c *Client) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// Close persists all in-memory sessions and shuts down active agents.
func (c *Client) Close() error {
	// Stop the persist timer goroutine.
	select {
	case <-c.stopPersistCh:
	default:
		close(c.stopPersistCh)
	}
	if c.sessionSearch != nil {
		c.sessionSearch.Close()
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
	agentType = normalizeAgentType(agentType)
	if _, ok := acp.ParseACPProvider(agentType); !ok {
		return nil, fmt.Errorf("no agent registered for %q", agentType)
	}
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

	inst, err := creator(agent.WithProjectName(ctx, c.projectName), c.cwd)
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
	agentType = normalizeAgentType(agentType)
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

func (c *Client) PromptToSession(ctx context.Context, sessionID string, blocks []acp.ContentBlock) error {
	sess, err := c.SessionByID(ctx, sessionID)
	if err != nil {
		return err
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

func (c *Client) HandleSessionRequest(ctx context.Context, method string, projectID string, payload json.RawMessage) (any, error) {
	switch strings.TrimSpace(method) {
	case "session.list":
		sessions, err := c.sessionRecorder.ListSessionViews(ctx)
		if err != nil {
			return nil, err
		}
		for i := range sessions {
			sessions[i].ConfigOptions = c.sessionConfigOptions(ctx, sessions[i].SessionID)
		}
		return map[string]any{"sessions": sessions}, nil
	case "session.read":
		var req struct {
			SessionID      string `json:"sessionId"`
			AfterTurnIndex int64  `json:"afterTurnIndex,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.read payload: %w", err)
		}
		latestTurnIndex, turns, err := c.sessionRecorder.ReadSessionTurns(ctx, req.SessionID, req.AfterTurnIndex)
		if err != nil {
			return nil, err
		}
		summary, err := c.sessionRecorder.ReadSessionSummary(ctx, req.SessionID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"sessionId": strings.TrimSpace(req.SessionID), "latestTurnIndex": latestTurnIndex, "session": summary, "turns": turns}, nil
	case "session.search":
		if c.sessionSearch == nil {
			c.sessionSearch = newSessionSearchManager(c)
		}
		return c.sessionSearch.Handle(ctx, projectID, payload)
	case "session.markRead":
		var req struct {
			SessionID         string `json:"sessionId"`
			LastReadTurnIndex int64  `json:"lastReadTurnIndex,omitempty"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.markRead payload: %w", err)
		}
		summary, err := c.sessionRecorder.MarkSessionRead(ctx, req.SessionID, req.LastReadTurnIndex)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "session": summary}, nil
	case "session.rename":
		var req struct {
			SessionID string `json:"sessionId"`
			Title     string `json:"title"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.rename payload: %w", err)
		}
		summary, err := c.sessionRecorder.RenameSessionTitle(ctx, req.SessionID, req.Title)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "sessionId": summary.SessionID, "session": summary}, nil
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
					Title:     strings.TrimSpace(req.Title),
				},
			}),
		}); err != nil {
			return nil, err
		}
		summary, err := c.sessionRecorder.ReadSessionSummary(ctx, sess.acpSessionID)
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
	case "session.archive":
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.archive payload: %w", err)
		}
		if err := c.ArchiveSession(ctx, req.SessionID); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "sessionId": strings.TrimSpace(req.SessionID)}, nil
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
	case "session.attachment.start":
		return c.handleSessionAttachmentStart(ctx, payload)
	case "session.attachment.chunk":
		return c.handleSessionAttachmentChunk(ctx, payload)
	case "session.attachment.finish":
		return c.handleSessionAttachmentFinish(ctx, payload)
	case "session.attachment.cancel":
		return c.handleSessionAttachmentCancel(ctx, payload)
	case "session.attachment.delete":
		return c.handleSessionAttachmentDelete(ctx, payload)
	case "session.token.providers":
		return map[string]any{
			"providers": []map[string]any{
				{"id": "deepseek", "name": "DeepSeek", "authMode": "api_key"},
				{"id": "codex", "name": "Codex", "authMode": "oauth"},
				{"id": "copilot", "name": "Copilot", "authMode": "github_api"},
			},
		}, nil
	case "session.token.scan":
		stats, err := tools.ScanTokenStats(ctx)
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
		stats, err := tools.FetchDeepSeekTokenStats(ctx, req.APIKey, req.RangeType, req.Month)
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
		attachmentRefs, err := c.validateSessionAttachmentBlocks(ctx, req.SessionID, blocks)
		if err != nil {
			return nil, err
		}
		sessionID := strings.TrimSpace(req.SessionID)
		if err := c.PromptToSession(ctx, sessionID, blocks); err != nil {
			return nil, err
		}
		if err := c.markSessionAttachmentsSent(attachmentRefs); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "sessionId": strings.TrimSpace(req.SessionID)}, nil
	case "session.cancel":
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := decodeSessionRequestPayload(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid session.cancel payload: %w", err)
		}
		sessionID := strings.TrimSpace(req.SessionID)
		if sessionID == "" {
			return nil, fmt.Errorf("sessionId is required")
		}
		sess, err := c.SessionByID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if err := sess.cancelPrompt(); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "sessionId": sessionID}, nil
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

// --- internal ---

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
	sess.viewSink = c.viewSink
	sess.store = c.store
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
		if strings.TrimSpace(storedEntry.SessionSyncJSON) != "" {
			entries[i].SessionSyncJSON = storedEntry.SessionSyncJSON
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
	return c.deleteActiveSession(ctx, sessionID, true)
}

func (c *Client) deleteActiveSession(ctx context.Context, sessionID string, rejectRunning bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if rejectRunning && c.sessionIsRunning(sessionID) {
		return fmt.Errorf("session %s is running", sessionID)
	}

	c.mu.Lock()
	sess := c.sessions[sessionID]
	delete(c.sessions, sessionID)
	store := c.store
	c.mu.Unlock()

	agentType := ""
	if sess != nil {
		sess.mu.Lock()
		agentType = sess.agentType
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
	if agentType == "" && store != nil {
		rec, err := store.LoadSession(ctx, c.projectName, sessionID)
		if err != nil {
			hubLogger(c.projectName).Warn("load session before artifact cleanup failed session=%s err=%v", sessionID, err)
		} else if rec != nil {
			agentType = rec.AgentType
		}
	}
	if store != nil {
		if err := store.DeleteSession(ctx, c.projectName, sessionID); err != nil {
			return err
		}
	}
	if c.sessionRecorder != nil {
		if err := c.sessionRecorder.DeleteSessionData(ctx, sessionID); err != nil {
			return fmt.Errorf("delete session data: %w", err)
		}
	}
	if cleanupSessionArtifacts != nil {
		if err := cleanupSessionArtifacts(c.projectName, agentType, sessionID); err != nil {
			hubLogger(c.projectName).Warn("cleanup session artifacts failed agent=%s session=%s err=%v", agentType, sessionID, err)
		}
	}
	return nil
}

func (c *Client) ArchiveSession(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if c.sessionIsRunning(sessionID) {
		return fmt.Errorf("session %s is running", sessionID)
	}

	rec, err := c.store.LoadSession(ctx, c.projectName, sessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	latestTurnIndex := sessionSyncLatestPersistedTurnIndex(rec.SessionSyncJSON)
	if latestTurnIndex < 3 {
		return c.deleteActiveSession(ctx, sessionID, false)
	}
	if c.archiveStore == nil {
		return fmt.Errorf("session archive store is required")
	}
	alreadyArchived, err := c.archiveStore.HasSession(ctx, c.projectName, sessionID)
	if err != nil {
		return err
	}
	if alreadyArchived {
		return c.deleteActiveSession(ctx, sessionID, false)
	}
	contents, gapCount, err := c.sessionRecorder.ReadPersistedTurnContentsForArchive(ctx, sessionID, latestTurnIndex)
	if err != nil {
		return err
	}
	if _, _, err := c.archiveStore.AppendSession(ctx, *rec, contents, gapCount); err != nil {
		return err
	}
	return c.deleteActiveSession(ctx, sessionID, false)
}

func (c *Client) sessionIsRunning(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	c.mu.Lock()
	sess := c.sessions[sessionID]
	c.mu.Unlock()
	if sess != nil && sess.isRunning() {
		return true
	}
	return c.sessionRecorder != nil && c.sessionRecorder.HasUnfinishedPrompt(sessionID)
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

		delete(c.sessions, sess.acpSessionID)
		c.mu.Unlock()

		hubLogger(c.projectName).Info("evicted suspended session to sqlite session=%s", sess.acpSessionID)
	}
}

func decodeSessionRequestPayload(raw json.RawMessage, out any) error {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func formatConfigOptionUpdateMessage(raw []byte) string {
	if len(raw) == 0 {
		return "Config options updated."
	}
	var u acp.SessionUpdate
	var opts []acp.ConfigOption
	if err := json.Unmarshal(raw, &u); err == nil {
		opts = u.ConfigOptions
	}
	if len(opts) == 0 {
		return "Config options updated."
	}
	mode := ""
	model := ""
	for _, opt := range opts {
		if mode == "" && (opt.ID == acp.ConfigOptionIDMode || strings.EqualFold(opt.Category, acp.ConfigOptionCategoryMode)) {
			mode = strings.TrimSpace(opt.CurrentValue)
		}
		if model == "" && (opt.ID == acp.ConfigOptionIDModel || strings.EqualFold(opt.Category, acp.ConfigOptionCategoryModel)) {
			model = strings.TrimSpace(opt.CurrentValue)
		}
	}
	if mode == "" && model == "" {
		return "Config options updated."
	}
	return fmt.Sprintf("Config options updated: mode=%s model=%s", renderUnknown(mode), renderUnknown(model))
}
