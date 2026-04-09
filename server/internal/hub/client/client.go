package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	currentCh <-chan acp.Update // tracked for draining during switchAgent
	activeTCs map[string]struct{}
}

// Client is the top-level coordinator for a single WheelMaker project.
// Agent initialization is lazy: the first incoming message triggers ensureInstance(),
// which connects the active agent and creates the ACP forwarder.
type Client struct {
	projectName string
	cwd         string
	yolo        bool

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
}

// New creates a Client for the given project.
func New(store Store, projectName string, cwd string) *Client {
	return &Client{
		projectName:    projectName,
		cwd:            cwd,
		registry:       agent.DefaultACPFactory(),
		store:          store,
		sessions:       make(map[string]*Session),
		routeMap:       make(map[string]string),
		suspendTimeout: 5 * time.Minute,
		stopPersistCh:  make(chan struct{}),
	}
}

// SetYOLO enables/disables always-approve permission mode for this project.
func (c *Client) SetYOLO(enabled bool) {
	c.mu.Lock()
	c.yolo = enabled
	store := c.store
	projectName := c.projectName
	sessions := make([]*Session, 0, len(c.sessions))
	for _, s := range c.sessions {
		sessions = append(sessions, s)
	}
	c.mu.Unlock()
	for _, sess := range sessions {
		sess.mu.Lock()
		sess.yolo = enabled
		sess.mu.Unlock()
	}
	if store != nil {
		if err := store.SaveProject(context.Background(), projectName, ProjectConfig{YOLO: enabled}); err != nil {
			hubLogger(projectName).Warn("save project config failed err=%v", err)
		}
	}
}

// Start loads persisted state.
// Agent initialization is deferred until the first incoming IM event (lazy init).
func (c *Client) Start(ctx context.Context) error {
	cfg, err := c.store.LoadProject(ctx, c.projectName)
	if err != nil {
		return fmt.Errorf("client: load project config: %w", err)
	}
	bindings, err := c.store.LoadRouteBindings(ctx, c.projectName)
	if err != nil {
		return fmt.Errorf("client: load route bindings: %w", err)
	}
	c.mu.Lock()
	if cfg != nil {
		c.yolo = cfg.YOLO
	}
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
				hubLogger(c.projectName).Warn("suspend session during close session=%s err=%v", sess.ID, err)
			}
			continue
		}
		if err := sess.persistSession(ctx); err != nil {
			hubLogger(c.projectName).Warn("persist session during close session=%s err=%v", sess.ID, err)
		}
	}
	if store != nil {
		return store.Close()
	}
	return nil
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
		store := c.store
		c.mu.Unlock()
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
		restored.lastActiveAt = time.Now()
		c.mu.Lock()
		c.sessions[restored.ID] = restored
		c.mu.Unlock()
		return restored, nil
	}
	c.mu.Unlock()

	sess := c.newWiredSession("")
	if err := c.persistBoundSession(routeKey, sess); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.sessions[sess.ID] = sess
	c.routeMap[routeKey] = sess.ID
	c.mu.Unlock()
	return sess, nil
}

// newWiredSession creates a Session with all Client back-references wired.
// Does NOT add it to c.sessions. Caller may hold c.mu.
func (c *Client) newWiredSession(id string) *Session {
	sess := newSession(id, c.cwd)
	c.wireSession(sess)
	return sess
}

func (c *Client) wireSession(sess *Session) {
	sess.projectName = c.projectName
	sess.registry = c.registry
	sess.imRouter = c.imRouter
	sess.yolo = c.yolo
	sess.store = c.store
	if strings.TrimSpace(sess.activeAgent) == "" {
		sess.activeAgent = c.preferredAvailableAgent()
	}
}

func (c *Client) preferredAvailableAgent() string {
	if c.registry == nil {
		return ""
	}
	return strings.TrimSpace(c.registry.PreferredName())
}

func (c *Client) persistBoundSession(routeKey string, sess *Session) error {
	if err := sess.persistSession(context.Background()); err != nil {
		hubLogger(c.projectName).Error("save session failed route=%s session=%s err=%v",
			routeKey, sess.ID, err)
		return fmt.Errorf("save session: %w", err)
	}
	if err := c.store.SaveRouteBinding(context.Background(), c.projectName, routeKey, sess.ID); err != nil {
		hubLogger(c.projectName).Error("save route binding failed route=%s session=%s err=%v",
			routeKey, sess.ID, err)
		return fmt.Errorf("save route binding: %w", err)
	}
	return nil
}

// ClientNewSession suspends the current session for the given route,
// creates a new session, and rebinds the route. Returns the new session.
func (c *Client) ClientNewSession(routeKey string) (*Session, error) {
	routeKey, err := normalizeRouteKey(routeKey)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	oldSessID := c.routeMap[routeKey]
	oldSess := c.sessions[oldSessID]
	c.mu.Unlock()

	// Suspend old session if it is active and has an agent.
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
		oldSess.lastActiveAt = time.Now()
		oldSess.mu.Unlock()
	}

	sess := c.newWiredSession("")
	if err := c.persistBoundSession(routeKey, sess); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.sessions[sess.ID] = sess
	c.routeMap[routeKey] = sess.ID
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
		if oldSess != nil && oldSess.ID != target.ID {
			oldSess.mu.Lock()
			hasInst := oldSess.instance != nil
			oldSess.mu.Unlock()
			if hasInst {
				_ = oldSess.Suspend(context.Background())
			}
			oldSess.mu.Lock()
			oldSess.Status = SessionSuspended
			oldSess.lastActiveAt = time.Now()
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
	if oldSess != nil && oldSess.ID != target.ID {
		oldSess.mu.Lock()
		hasInst := oldSess.instance != nil
		oldSess.mu.Unlock()
		if hasInst {
			_ = oldSess.Suspend(context.Background())
		}
		oldSess.mu.Lock()
		oldSess.Status = SessionSuspended
		oldSess.lastActiveAt = time.Now()
		oldSess.mu.Unlock()
	}

	restored, err := sessionFromRecord(rec, c.cwd)
	if err != nil {
		return nil, err
	}
	c.wireSession(restored)
	c.mu.Lock()
	restored.Status = SessionActive
	restored.lastActiveAt = time.Now()
	c.sessions[restored.ID] = restored
	c.routeMap[routeKey] = restored.ID
	c.mu.Unlock()
	if err := c.store.SaveRouteBinding(context.Background(), c.projectName, routeKey, restored.ID); err != nil {
		return nil, fmt.Errorf("save route binding: %w", err)
	}
	return restored, nil
}

// clientListSessions returns a merged list of in-memory and persisted sessions,
// sorted by last active time (most recent first). Duplicates are deduplicated
// favoring in-memory sessions.
func (c *Client) clientListSessions() ([]sessionListEntry, error) {
	c.mu.Lock()
	memEntries := make([]sessionListEntry, 0, len(c.sessions))
	memIDs := make(map[string]bool, len(c.sessions))
	for _, sess := range c.sessions {
		sess.mu.Lock()
		agentName := sess.currentAgentNameLocked()
		title := ""
		if state := sess.agents[agentName]; state != nil {
			title = state.Title
		}
		e := sessionListEntry{
			ID:           sess.ID,
			Agent:        agentName,
			Title:        title,
			Status:       sess.Status,
			CreatedAt:    sess.createdAt,
			LastActiveAt: sess.lastActiveAt,
			InMemory:     true,
		}
		sess.mu.Unlock()
		memEntries = append(memEntries, e)
		memIDs[sess.ID] = true
	}
	store := c.store
	c.mu.Unlock()

	entries := memEntries

	// Merge persisted sessions.
	stored, err := store.ListSessions(context.Background(), c.projectName)
	if err != nil {
		return nil, fmt.Errorf("list persisted sessions: %w", err)
	}
	for _, s := range stored {
		if memIDs[s.ID] {
			continue
		}
		entries = append(entries, sessionListEntry{
			ID:           s.ID,
			Agent:        s.Agent,
			Title:        s.Title,
			Status:       SessionPersisted,
			CreatedAt:    s.CreatedAt,
			LastActiveAt: s.LastActiveAt,
			InMemory:     false,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LastActiveAt.After(entries[j].LastActiveAt)
	})
	return entries, nil
}

// sessionListEntry holds merged in-memory + persisted session information.
type sessionListEntry struct {
	ID           string
	Agent        string
	Title        string
	Status       SessionStatus
	CreatedAt    time.Time
	LastActiveAt time.Time
	InMemory     bool
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
			hubLogger(c.projectName).Warn("persist session failed session=%s err=%v", sess.ID, err)
			continue
		}

		c.mu.Lock()
		sess.mu.Lock()
		sess.Status = SessionPersisted
		sess.mu.Unlock()

		// Remove from sessions map but keep route mapping pointing to the ID
		// so we can look it up later for restoration.
		delete(c.sessions, sess.ID)
		c.mu.Unlock()

		hubLogger(c.projectName).Info("evicted suspended session to sqlite session=%s", sess.ID)
	}
}

// parseCommand checks whether text is a recognized WheelMaker command.
// Only exact first-word matches (/use, /cancel, /status, /mode, /model, /config, /list, /new, /load) are treated as commands;
// all other "/" lines fall through to the agent (fixing the "code starting with /" bug).
func parseCommand(text string) (cmd, args string, ok bool) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "/use", "/cancel", "/status", "/mode", "/model", "/config", "/list", "/new", "/load":
		return parts[0], strings.Join(parts[1:], " "), true
	}
	return
}

func normalizeSessionPromptBlocks(blocks []acp.ContentBlock) []acp.ContentBlock {
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

// handlePrompt sends text to the active (or lazily initialized) session and streams the reply.
// promptMu is held for the full duration, serializing with switchAgent.
func (s *Session) handlePrompt(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	s.handlePromptBlocks([]acp.ContentBlock{{Type: acp.ContentBlockTypeText, Text: text}})
}

// handlePromptBlocks sends content blocks to the active (or lazily initialized) session.
// promptMu is held for the full duration, serializing with switchAgent.
func (s *Session) handlePromptBlocks(blocks []acp.ContentBlock) {
	blocks = normalizeSessionPromptBlocks(blocks)
	if len(blocks) == 0 {
		return
	}
	s.promptMu.Lock()
	defer s.promptMu.Unlock()

	ctx := context.Background()
	for attempt := 1; attempt <= 2; attempt++ {
		// Lazily initialize the agent if no forwarder exists yet.
		if err := s.ensureInstance(ctx); err != nil {
			s.reply(fmt.Sprintf("No active session: %v. %s", err, s.connectHint()))
			return
		}

		if err := s.ensureReadyAndNotify(ctx); err != nil {
			if attempt == 1 && s.shouldReconnectOnRecoverableErr(err) {
				s.reply("Agent init failed transiently, reconnecting and retrying once...")
				s.forceReconnect()
				continue
			}
			s.reply(fmt.Sprintf("No active session: %v. %s", err, s.connectHint()))
			return
		}

		updates, err := s.promptStream(ctx, blocks)
		if err != nil {
			if attempt == 1 && s.shouldReconnectOnRecoverableErr(err) {
				s.reply("Agent disconnected, reconnecting and retrying once...")
				s.forceReconnect()
				continue
			}
			if isAgentExitError(err) && !s.agentProcessAlive() {
				_ = s.resetDeadConnection(err)
			}
			s.reply(fmt.Sprintf("Prompt error: %v", err))
			return
		}

		s.mu.Lock()
		s.prompt.currentCh = updates
		s.mu.Unlock()

		var buf strings.Builder
		imRouter, imSource, hasIMEmitter := s.imContext()
		s.mu.Lock()
		acpSessionID := s.acpSessionID
		s.mu.Unlock()

		sawSandboxRefresh := false
		sawText := false
		observe := newPromptObserveState(time.Now())
		observeTicker := time.NewTicker(promptObserveInterval)

		retryStream := false
		streamDone := false
		for !streamDone {
			select {
			case u, ok := <-updates:
				if !ok {
					streamDone = true
					break
				}
				observe.MarkActivity(time.Now(), true)
				if u.Err != nil {
					if attempt == 1 && isUnsupportedReasoningEffortError(u.Err) && s.tryCopilotReasoningFallback(ctx) {
						s.reply("Detected incompatible reasoning settings, switched model and retrying once...")
						retryStream = true
						break
					}
					if attempt == 1 && !sawText && s.shouldReconnectOnRecoverableErr(u.Err) {
						s.reply("Agent disconnected during stream, reconnecting and retrying once...")
						s.forceReconnect()
						retryStream = true
						break
					}
					recovered := false
					if !s.agentProcessAlive() && s.resetDeadConnection(u.Err) {
						if recErr := s.ensureInstance(ctx); recErr == nil {
							_ = s.ensureReadyAndNotify(ctx)
							recovered = true
						}
					}
					if recovered {
						s.reply("Agent process exited and was reconnected. Please resend if this reply was interrupted.")
					} else {
						s.reply(fmt.Sprintf("Agent error: %v", u.Err))
					}
					s.mu.Lock()
					s.prompt.currentCh = nil
					s.mu.Unlock()
					observeTicker.Stop()
					return
				}
				if hasIMEmitter {
					target := im.SendTarget{SessionID: s.ID, Source: &imSource}
					var emitErr error
					switch u.Type {
					case acp.UpdateDone:
						emitErr = imRouter.PublishPromptResult(ctx, target, acp.SessionPromptResult{StopReason: u.Content})
					default:
						params, ok := sessionUpdateParamsFromUpdate(acpSessionID, u)
						if ok {
							emitErr = imRouter.PublishSessionUpdate(ctx, target, params)
						}
					}
					if emitErr != nil {
						s.reply(fmt.Sprintf("IM emit error: %v", emitErr))
					}
				}
				if hasSandboxRefreshError(u) {
					sawSandboxRefresh = true
				}
				if u.Type == acp.UpdateText && strings.TrimSpace(u.Content) != "" {
					sawText = true
				}
				if u.Type == acp.UpdateConfigOption && !hasIMEmitter {
					s.reply(formatConfigOptionUpdateMessage(u.Raw))
					s.persistSessionBestEffort()
				}
				if u.Type == acp.UpdateText && !hasIMEmitter {
					buf.WriteString(u.Content)
				}
				if u.Done {
					streamDone = true
				}
			case <-observeTicker.C:
				ev := observe.Eval(time.Now(), observe.Started())
				if ev.WarnFirstWait {
					hubLogger(s.projectName).Warn("timeout warn category=timeout stage=stream kind=first_wait session=%s", s.ID)
				}
				if ev.ErrorFirstWait {
					s.reportTimeoutError("stream", "first_wait")
				}
				if ev.WarnSilence {
					hubLogger(s.projectName).Warn("timeout warn category=timeout stage=stream kind=silence session=%s", s.ID)
				}
				if ev.ErrorSilence {
					s.reportTimeoutError("stream", "silence")
				}
			}
		}
		observeTicker.Stop()
		if retryStream {
			continue
		}

		s.mu.Lock()
		s.prompt.currentCh = nil
		s.mu.Unlock()

		s.persistSessionBestEffort()

		if !hasIMEmitter && buf.Len() > 0 {
			s.reply(buf.String())
		}

		if attempt == 1 && sawSandboxRefresh && !sawText && !s.agentProcessAlive() {
			s.reply("Detected sandbox refresh failure, reconnecting and retrying once...")
			s.forceReconnect()
			continue
		}
		return
	}
}

func normalizeRouteKey(routeKey string) (string, error) {
	routeKey = strings.TrimSpace(routeKey)
	if routeKey == "" {
		return "", fmt.Errorf("route key is required")
	}
	return routeKey, nil
}

func renderUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

func (s *Session) reportTimeoutError(stage string, kind string) {
	now := time.Now()
	s.mu.Lock()
	agent := s.currentAgentNameLocked()
	sid := s.acpSessionID
	allow := true
	if s.timeoutLimiter != nil {
		allow = s.timeoutLimiter.Allow(kind, now)
	}
	s.mu.Unlock()

	hubLogger(s.projectName).Error("timeout error category=timeout stage=%s kind=%s agent=%s session=%s",
		stage, kind, renderUnknown(agent), renderUnknown(sid))
	if !allow {
		return
	}

	router, source, ok := s.imContext()
	if !ok {
		return
	}
	body := fmt.Sprintf(
		"category=timeout stage=%s agent=%s sessionID=%s action=check /status then retry",
		stage,
		renderUnknown(agent),
		renderUnknown(sid),
	)
	if err := router.SystemNotify(
		context.Background(),
		im.SendTarget{SessionID: s.ID, Source: &source},
		im.SystemPayload{Kind: "message", Body: body},
	); err != nil {
		hubLogger(s.projectName).Error("timeout im notify failed stage=%s kind=%s session=%s err=%v",
			stage, kind, s.ID, err)
	}
}

func (s *Session) connectHint() string {
	agentName := s.preferredAgentName()
	if agentName == "" {
		if s.registry != nil {
			names := s.registry.Names()
			if len(names) > 0 {
				return fmt.Sprintf("Run `/use <agent>` to connect. Available: %s", strings.Join(names, ", "))
			}
		}
		return "No available ACP provider. Check environment and restart wheelmaker."
	}
	return fmt.Sprintf("Run `%s` to connect.", "/use "+agentName)
}

func (s *Session) preferredAgentName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.instance != nil && strings.TrimSpace(s.instance.Name()) != "" {
		return strings.TrimSpace(s.instance.Name())
	}
	if strings.TrimSpace(s.activeAgent) != "" {
		return strings.TrimSpace(s.activeAgent)
	}
	if s.registry != nil {
		return strings.TrimSpace(s.registry.PreferredName())
	}
	return ""
}

type instanceLivenessProbe interface {
	Alive() bool
}

func (s *Session) agentProcessAlive() bool {
	s.mu.Lock()
	inst := s.instance
	s.mu.Unlock()
	if inst == nil {
		return false
	}
	probe, ok := inst.(instanceLivenessProbe)
	if !ok {
		// Backward-compatible fallback for instances without explicit liveness support.
		return true
	}
	return probe.Alive()
}

func (s *Session) shouldReconnectOnRecoverableErr(err error) bool {
	if !isAgentRecoverableRuntimeErr(err) {
		return false
	}
	return !s.agentProcessAlive()
}

func isAgentExitError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(err.Error()))
	if s == "" {
		return false
	}
	// Network handshake EOF from remote APIs does not necessarily imply the local ACP
	// subprocess died; treat it as non-exit so we don't restart blindly.
	if strings.Contains(s, "tls handshake eof") {
		return false
	}
	if s == "eof" || strings.HasSuffix(s, ": eof") {
		return true
	}
	return strings.Contains(s, "agent process exited") ||
		strings.Contains(s, "process exited") ||
		strings.Contains(s, "conn is closed") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "process stdout closed")
}

func isAgentRecoverableRuntimeErr(err error) bool {
	if err == nil {
		return false
	}
	return isAgentExitError(err) || isSandboxRefreshErr(err)
}

func isUnsupportedReasoningEffortError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" || !strings.Contains(msg, "reasoning_effort") {
		return false
	}
	hints := []string{
		"unrecognized request argument",
		"unknown field",
		"not supported",
		"unsupported",
		"invalid request",
	}
	for _, hint := range hints {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}

func pickAlternativeModelValue(opt *acp.ConfigOption) string {
	if opt == nil || len(opt.Options) == 0 {
		return ""
	}
	current := strings.TrimSpace(opt.CurrentValue)
	for _, candidate := range opt.Options {
		value := strings.TrimSpace(candidate.Value)
		if value == "" || strings.EqualFold(value, current) {
			continue
		}
		return value
	}
	return ""
}

func (s *Session) tryCopilotReasoningFallback(ctx context.Context) bool {
	s.mu.Lock()
	if s.instance == nil || !strings.EqualFold(strings.TrimSpace(s.instance.Name()), "copilot") {
		s.mu.Unlock()
		return false
	}
	sid := strings.TrimSpace(s.acpSessionID)
	configOptions := []acp.ConfigOption(nil)
	if state := s.agents[s.currentAgentNameLocked()]; state != nil {
		configOptions = append(configOptions, state.ConfigOptions...)
	}
	s.mu.Unlock()

	if sid == "" {
		return false
	}

	var modelOpt *acp.ConfigOption
	for i := range configOptions {
		opt := &configOptions[i]
		if strings.EqualFold(strings.TrimSpace(opt.ID), acp.ConfigOptionIDModel) || strings.EqualFold(strings.TrimSpace(opt.Category), acp.ConfigOptionCategoryModel) {
			modelOpt = opt
			break
		}
	}
	target := pickAlternativeModelValue(modelOpt)
	if target == "" {
		return false
	}

	updatedOpts, err := s.instance.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{
		SessionID: sid,
		ConfigID:  modelOpt.ID,
		Value:     target,
	})
	if err != nil {
		return false
	}

	s.mu.Lock()
	if len(updatedOpts) > 0 {
		if state := s.agentStateLocked(s.currentAgentNameLocked()); state != nil {
			state.ConfigOptions = append([]acp.ConfigOption(nil), updatedOpts...)
		}
	}
	s.mu.Unlock()
	s.persistSessionBestEffort()
	return true
}

// resetDeadConnection clears the current agent connection when it is known-dead.
func (s *Session) resetDeadConnection(err error) bool {
	if !isAgentExitError(err) {
		return false
	}
	s.mu.Lock()
	old := s.instance
	s.instance = nil
	s.ready = false
	s.initializing = false
	s.prompt.ctx = nil
	s.prompt.cancel = nil
	s.prompt.updatesCh = nil
	s.prompt.currentCh = nil
	s.prompt.activeTCs = make(map[string]struct{})
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return true
}

func (s *Session) forceReconnect() {
	s.mu.Lock()
	old := s.instance
	s.instance = nil
	s.ready = false
	s.initializing = false
	s.prompt.ctx = nil
	s.prompt.cancel = nil
	s.prompt.updatesCh = nil
	s.prompt.currentCh = nil
	s.prompt.activeTCs = make(map[string]struct{})
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

func hasSandboxRefreshError(u acp.Update) bool {
	s := strings.ToLower(u.Content + " " + string(u.Raw))
	return strings.Contains(s, "windows sandbox: spawn setup refresh")
}

func sessionUpdateParamsFromUpdate(sessionID string, u acp.Update) (acp.SessionUpdateParams, bool) {
	update := acp.SessionUpdate{}
	switch u.Type {
	case acp.UpdateText:
		content, _ := json.Marshal(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: u.Content})
		update = acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentMessageChunk, Content: content}
	case acp.UpdateThought:
		content, _ := json.Marshal(acp.ContentBlock{Type: acp.ContentBlockTypeText, Text: u.Content})
		update = acp.SessionUpdate{SessionUpdate: acp.SessionUpdateAgentThoughtChunk, Content: content}
	case acp.UpdateToolCallCancelled:
		// Content holds the toolCallID; synthesize a tool_call_update with cancelled status.
		update = acp.SessionUpdate{
			SessionUpdate: acp.SessionUpdateToolCallUpdate,
			ToolCallID:    strings.TrimSpace(u.Content),
			Status:        "cancelled",
		}
	case acp.UpdateToolCall, acp.UpdateConfigOption, acp.UpdateAvailableCommands, acp.UpdateSessionInfo, acp.UpdatePlan, acp.UpdateModeChange, acp.UpdateUserChunk:
		if len(u.Raw) == 0 || json.Unmarshal(u.Raw, &update) != nil {
			return acp.SessionUpdateParams{}, false
		}
	default:
		return acp.SessionUpdateParams{}, false
	}
	return acp.SessionUpdateParams{SessionID: sessionID, Update: update}, true
}

func isSandboxRefreshErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "windows sandbox: spawn setup refresh")
}
