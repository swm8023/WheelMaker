package client

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/hub/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
	logger "github.com/swm8023/wheelmaker/internal/shared"
)

const commandTimeout = 30 * time.Second

const defaultAgentName = string(acp.ACPProviderClaude)

const acpClientProtocolVersion = 1

var acpClientInfo = &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}

type promptState struct {
	ctx       context.Context
	cancel    context.CancelFunc
	updatesCh chan<- acp.Update
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

	stateStore ClientStateStore
	state      *ProjectState
	imBridge   *im.ImAdapter // nil when no IM channel configured

	mu sync.Mutex

	// sessions maps session IDs to Session objects.
	sessions map[string]*Session

	// routeMap maps IM routing keys to Session IDs.
	// Multiple routes can point to the same Session.
	routeMap map[string]string

	// activeSession is the Session currently handling messages.
	activeSession *Session

	// suspendTimeout is how long a Suspended session stays in memory before
	// being persisted to SQLite and evicted. Default: 5 minutes.
	suspendTimeout time.Duration
	stopPersistCh  chan struct{} // closed to stop the persist timer goroutine

	// sessionCounter generates unique session IDs.
	sessionCounter int

	imBlockedUpdates map[string]struct{}
}

// New creates a Client for the given project.
//   - store: persistent state store scoped to this project
//   - imProvider: IM bridge; nil means Run() returns an error (use Hub with a console project)
//   - projectName: identifier used in logs and state keys
//   - cwd: working directory for agent sessions
func New(store Store, imProvider *im.ImAdapter, projectName string, cwd string) *Client {
	c := &Client{
		projectName:      projectName,
		cwd:              cwd,
		registry:         agent.DefaultACPFactory(),
		stateStore:       NewClientStateStore(store, nil),
		imBridge:         imProvider,
		imBlockedUpdates: map[string]struct{}{},
		sessions:         make(map[string]*Session),
		routeMap:         make(map[string]string),
		suspendTimeout:   5 * time.Minute,
		stopPersistCh:    make(chan struct{}),
	}
	sess := newSession("default", cwd)
	sess.persistence = c.stateStore
	sess.registry = c.registry
	sess.imBridge = imProvider
	sess.imBlockedUpdates = c.imBlockedUpdates
	sess.permRouter = newPermissionRouter(sess)
	c.activeSession = sess
	c.sessions["default"] = sess
	return c
}

// SetYOLO enables/disables always-approve permission mode for this project.
func (c *Client) SetYOLO(enabled bool) {
	c.mu.Lock()
	c.yolo = enabled
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
}

// SetIMUpdateBlockList configures outbound IM update types to suppress.
// Values are case-insensitive; aliases: "tool" -> "tool_call", "system" -> "error".
func (c *Client) SetIMUpdateBlockList(types []string) {
	blocked := make(map[string]struct{}, len(types))
	for _, t := range types {
		k := canonicalIMBlockType(t)
		if k == "" {
			continue
		}
		blocked[k] = struct{}{}
	}
	c.mu.Lock()
	c.imBlockedUpdates = blocked
	sessions := make([]*Session, 0, len(c.sessions))
	for _, s := range c.sessions {
		sessions = append(sessions, s)
	}
	c.mu.Unlock()
	for _, sess := range sessions {
		sess.mu.Lock()
		sess.imBlockedUpdates = blocked
		sess.mu.Unlock()
	}
}

// SetSessionStore sets an optional persistent session store (e.g. SQLite).
// Must be called before Start(). A nil store means in-memory only.
func (c *Client) SetSessionStore(ss SessionStore) {
	c.stateStore.SetSessionStore(ss)
}

// Start loads persisted state and registers the IM message callback.
// Agent initialization is deferred until the first incoming message (lazy init).
func (c *Client) Start(ctx context.Context) error {
	state, err := c.stateStore.LoadProjectState()
	if err != nil {
		return fmt.Errorf("client: load state: %w", err)
	}
	c.mu.Lock()
	c.state = state
	// Wire state into all sessions.
	for _, sess := range c.sessions {
		sess.mu.Lock()
		sess.state = state
		sess.mu.Unlock()
	}
	activeSess := c.activeSession
	c.mu.Unlock()

	if c.imBridge != nil {
		c.imBridge.OnMessage(c.HandleMessage)
		if activeSess != nil {
			c.imBridge.SetHelpResolver(activeSess.resolveHelpModel)
		}
	}

	// Start background persist timer for suspended sessions.
	if c.stateStore.SessionStoreEnabled() {
		go c.persistLoop()
	}

	return nil
}

// Run blocks until ctx is cancelled, delegating to the IM channel's Run loop.
// Returns an error if no IM channel is configured.
func (c *Client) Run(ctx context.Context) error {
	if c.imBridge != nil {
		return c.imBridge.Run(ctx)
	}
	return errors.New("no IM channel configured; add a console project to config.json")
}

// Close saves state and shuts down all active sessions.
// If a SessionStore is configured, active sessions are persisted before shutdown.
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
	pn := c.projectName
	stateStore := c.stateStore
	c.mu.Unlock()

	ctx := context.Background()
	for _, sess := range sessions {
		sess.mu.Lock()
		inst := sess.instance
		sess.mu.Unlock()
		if inst != nil {
			sess.syncAndPersistProjectState()
			if stateStore.SessionStoreEnabled() {
				_ = sess.Suspend(ctx, pn)
			} else {
				_ = inst.Close()
			}
		}
	}

	c.mu.Lock()
	s := c.state
	c.mu.Unlock()
	if s != nil {
		return c.stateStore.SaveProjectState(s)
	}
	return nil
}

// HandleMessage routes an incoming IM message to the appropriate handler.
// Known commands (/use, /cancel, /status, /mode, /model, /config, /list, /new, /load) are dispatched to handleCommand;
// everything else — including lines starting with "/" that are not known commands —
// is forwarded to the agent as a prompt.
func (c *Client) HandleMessage(msg im.Message) {
	// Update the active chat ID so all outbound messages route to the correct chat.
	if c.imBridge != nil && strings.TrimSpace(msg.ChatID) != "" {
		c.imBridge.SetActiveChatID(msg.ChatID)
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	sess := c.resolveSession(msg)

	if cmd, args, ok := parseCommand(text); ok {
		c.handleCommand(sess, msg, cmd, args)
		return
	}
	sess.handlePrompt(msg, text)
}

// --- internal ---

// resolveSession finds or creates the Session for a given message's route.
// Uses msg.RouteKey (falls back to msg.ChatID, then "default") to look up the
// session via routeMap. If no session exists for the route, a new one is created.
func (c *Client) resolveSession(msg im.Message) *Session {
	routeKey := msg.RouteKey
	if routeKey == "" {
		routeKey = msg.ChatID
	}
	if routeKey == "" {
		routeKey = "default"
	}

	c.mu.Lock()
	sessID := c.routeMap[routeKey]
	if sessID != "" {
		if sess := c.sessions[sessID]; sess != nil {
			c.activeSession = sess
			c.mu.Unlock()
			return sess
		}
		// Session was evicted — try to restore from store.
		stateStore := c.stateStore
		c.mu.Unlock()
		if stateStore.SessionStoreEnabled() {
			snap, err := stateStore.LoadSession(context.Background(), sessID)
			if err == nil && snap != nil {
				restored := RestoreFromSnapshot(snap, c.cwd)
				c.mu.Lock()
				restored.persistence = c.stateStore
				restored.registry = c.registry
				restored.imBridge = c.imBridge
				restored.imBlockedUpdates = c.imBlockedUpdates
				restored.yolo = c.yolo
				restored.state = c.state
				restored.permRouter = newPermissionRouter(restored)
				c.sessions[restored.ID] = restored
				c.activeSession = restored
				c.mu.Unlock()
				return restored
			}
		}
		// Could not restore — fall through to create new session.
		c.mu.Lock()
	}

	// If only one session exists and no explicit route mapping, reuse it.
	// This preserves backward compatibility for single-session setups.
	if len(c.sessions) == 1 {
		for _, sess := range c.sessions {
			c.routeMap[routeKey] = sess.ID
			c.activeSession = sess
			c.mu.Unlock()
			return sess
		}
	}

	// No session for this route — create one.
	sess := c.createSessionLocked(routeKey)
	c.mu.Unlock()
	return sess
}

// createSessionLocked creates a new Session, wires back-references, and binds
// it to the given routeKey. Caller MUST hold c.mu.
func (c *Client) createSessionLocked(routeKey string) *Session {
	id := routeKey // use routeKey as session ID for simplicity
	if existing := c.sessions[id]; existing != nil {
		c.routeMap[routeKey] = id
		c.activeSession = existing
		return existing
	}

	sess := c.newWiredSession(id)
	c.sessions[id] = sess
	c.routeMap[routeKey] = id
	c.activeSession = sess
	return sess
}

// newWiredSession creates a Session with all Client back-references wired.
// Does NOT add it to c.sessions. Caller may hold c.mu.
func (c *Client) newWiredSession(id string) *Session {
	sess := newSession(id, c.cwd)
	sess.persistence = c.stateStore
	sess.registry = c.registry
	sess.imBridge = c.imBridge
	sess.imBlockedUpdates = c.imBlockedUpdates
	sess.yolo = c.yolo
	sess.state = c.state
	sess.permRouter = newPermissionRouter(sess)
	return sess
}

// nextSessionID returns a unique session ID. Caller MUST hold c.mu.
func (c *Client) nextSessionID() string {
	c.sessionCounter++
	return fmt.Sprintf("session-%d", c.sessionCounter)
}

// ClientNewSession suspends the current session for the given route,
// creates a new session, and rebinds the route. Returns the new session.
func (c *Client) ClientNewSession(routeKey string) *Session {
	c.mu.Lock()
	oldSessID := c.routeMap[routeKey]
	oldSess := c.sessions[oldSessID]
	pn := c.projectName
	c.mu.Unlock()

	// Suspend old session if it is active and has an agent.
	if oldSess != nil {
		oldSess.mu.Lock()
		hasInst := oldSess.instance != nil
		oldSess.mu.Unlock()
		if hasInst {
			if err := oldSess.Suspend(context.Background(), pn); err != nil {
				logger.Warn("client: suspend old session %s: %v", oldSessID, err)
			}
		}
		oldSess.mu.Lock()
		oldSess.Status = SessionSuspended
		oldSess.lastActiveAt = time.Now()
		oldSess.mu.Unlock()
	}

	c.mu.Lock()
	newID := c.nextSessionID()
	sess := c.newWiredSession(newID)
	c.sessions[newID] = sess
	c.routeMap[routeKey] = newID
	c.activeSession = sess
	c.mu.Unlock()
	return sess
}

// ClientLoadSession restores a session by index from the merged list of
// in-memory + persisted sessions. Binds the loaded session to the given route.
func (c *Client) ClientLoadSession(routeKey string, index int) (*Session, error) {
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
		pn := c.projectName
		c.mu.Unlock()

		// Suspend old if different from target.
		if oldSess != nil && oldSess.ID != target.ID {
			oldSess.mu.Lock()
			hasInst := oldSess.instance != nil
			oldSess.mu.Unlock()
			if hasInst {
				_ = oldSess.Suspend(context.Background(), pn)
			}
			oldSess.mu.Lock()
			oldSess.Status = SessionSuspended
			oldSess.lastActiveAt = time.Now()
			oldSess.mu.Unlock()
		}

		c.mu.Lock()
		c.routeMap[routeKey] = target.ID
		sess.Status = SessionActive
		c.activeSession = sess
		c.mu.Unlock()
		return sess, nil
	}
	stateStore := c.stateStore
	pn := c.projectName
	c.mu.Unlock()

	// Try to load from SessionStore.
	if !stateStore.SessionStoreEnabled() {
		return nil, fmt.Errorf("session %q not in memory and no session store configured", target.ID)
	}
	snap, err := stateStore.LoadSession(context.Background(), target.ID)
	if err != nil {
		return nil, fmt.Errorf("load session %q: %w", target.ID, err)
	}
	if snap == nil {
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
			_ = oldSess.Suspend(context.Background(), pn)
		}
		oldSess.mu.Lock()
		oldSess.Status = SessionSuspended
		oldSess.lastActiveAt = time.Now()
		oldSess.mu.Unlock()
	}

	restored := RestoreFromSnapshot(snap, c.cwd)
	c.mu.Lock()
	restored.persistence = c.stateStore
	restored.registry = c.registry
	restored.imBridge = c.imBridge
	restored.imBlockedUpdates = c.imBlockedUpdates
	restored.yolo = c.yolo
	restored.state = c.state
	restored.permRouter = newPermissionRouter(restored)
	c.sessions[restored.ID] = restored
	c.routeMap[routeKey] = restored.ID
	c.activeSession = restored
	c.mu.Unlock()
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
		agentName := ""
		if sess.instance != nil {
			agentName = sess.instance.Name()
		}
		title := sess.sessionMeta.Title
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
	stateStore := c.stateStore
	c.mu.Unlock()

	entries := memEntries

	// Merge persisted sessions.
	if stateStore.SessionStoreEnabled() {
		stored, err := stateStore.ListSessions(context.Background())
		if err != nil {
			return nil, fmt.Errorf("list persisted sessions: %w", err)
		}
		for _, s := range stored {
			if memIDs[s.ID] {
				continue // already in memory
			}
			entries = append(entries, sessionListEntry{
				ID:           s.ID,
				Agent:        s.ActiveAgent,
				Title:        s.Title,
				Status:       SessionPersisted,
				CreatedAt:    s.CreatedAt,
				LastActiveAt: s.LastActiveAt,
				InMemory:     false,
			})
		}
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

// persistLoop periodically scans for Suspended sessions that have exceeded
// the suspend timeout and persists them to the SessionStore.
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
	stateStore := c.stateStore
	pn := c.projectName
	timeout := c.suspendTimeout
	if !stateStore.SessionStoreEnabled() {
		c.mu.Unlock()
		return
	}

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
		snap := sess.Snapshot(pn)
		if err := stateStore.SaveSession(context.Background(), snap); err != nil {
			logger.Warn("client: persist session %s: %v", sess.ID, err)
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

		logger.Info("client: evicted suspended session %s to SQLite", sess.ID)
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

// handlePrompt sends text to the active (or lazily initialized) session and streams the reply.
// promptMu is held for the full duration, serializing with switchAgent.
func (s *Session) handlePrompt(msg im.Message, text string) {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()

	ctx := context.Background()
	for attempt := 1; attempt <= 2; attempt++ {
		// Lazily initialize the agent if no forwarder exists yet.
		if err := s.ensureInstance(ctx); err != nil {
			s.reply(fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
			return
		}

		if err := s.ensureReadyAndNotify(ctx); err != nil {
			s.reply(fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
			return
		}

		updates, err := s.promptStream(ctx, text)
		if err != nil {
			// Keepalive: recover dead agent subprocess and retry current prompt once.
			if s.resetDeadConnection(err) && attempt == 1 {
				s.reply("Agent disconnected, reconnecting and retrying once...")
				continue
			}
			s.reply(fmt.Sprintf("Prompt error: %v", err))
			return
		}

		s.mu.Lock()
		s.prompt.currentCh = updates
		s.mu.Unlock()

		var buf strings.Builder
		s.mu.Lock()
		emitter := s.imBridge
		hasEmitter := emitter != nil
		sid := s.acpSessionID
		s.mu.Unlock()

		sawSandboxRefresh := false
		sawText := false

		retryStream := false
		for u := range updates {
			if u.Err != nil {
				if attempt == 1 && isCopilotReasoningArgError(u.Err) && s.tryCopilotReasoningFallback(ctx) {
					s.reply("Copilot model incompatibility detected, switched model and retrying once...")
					retryStream = true
					break
				}
				if attempt == 1 && !sawText && (isAgentExitError(u.Err) || isSandboxRefreshErr(u.Err)) {
					s.reply("Agent disconnected during stream, reconnecting and retrying once...")
					s.forceReconnect()
					retryStream = true
					break
				}
				recovered := false
				if s.resetDeadConnection(u.Err) {
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
				return
			}
			if s.shouldBlockIMUpdate(u.Type) {
				continue
			}
			if hasEmitter {
				emitErr := emitter.Emit(ctx, im.IMUpdate{
					SessionID:  sid,
					UpdateType: string(u.Type),
					Text:       u.Content,
					Raw:        u.Raw,
				})
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
			if u.Type == acp.UpdateConfigOption {
				s.reply(formatConfigOptionUpdateMessage(u.Raw))
				s.syncAndPersistProjectState()
			}
			if u.Type == acp.UpdateText {
				if !hasEmitter {
					buf.WriteString(u.Content)
				}
			}
			if u.Done {
				break
			}
		}
		if retryStream {
			continue
		}

		s.mu.Lock()
		s.prompt.currentCh = nil
		s.mu.Unlock()

		s.syncAndPersistProjectState()

		if !hasEmitter && buf.Len() > 0 {
			s.reply(buf.String())
		}

		if attempt == 1 && sawSandboxRefresh && !sawText {
			s.reply("Detected sandbox refresh failure, reconnecting and retrying once...")
			s.forceReconnect()
			continue
		}
		return
	}
}

func (s *Session) shouldBlockIMUpdate(updateType acp.UpdateType) bool {
	key := canonicalIMBlockType(string(updateType))
	if key == "" {
		return false
	}
	s.mu.Lock()
	_, blocked := s.imBlockedUpdates[key]
	s.mu.Unlock()
	return blocked
}

func canonicalIMBlockType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "":
		return ""
	case "tool":
		return "tool_call"
	case "tool_call_update", "tool_call_cancelled":
		return "tool_call"
	case "system":
		return "error"
	default:
		return s
	}
}

// reply delegates to the active session's reply method.
func (c *Client) reply(text string) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		sess.reply(text)
		return
	}
	fmt.Println(text)
}

func renderUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

func isAgentExitError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(err.Error()))
	if s == "" {
		return false
	}
	return strings.Contains(s, "agent process exited") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "eof")
}

func isCopilotReasoningArgError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(err.Error()))
	if s == "" {
		return false
	}
	return strings.Contains(s, "reasoning_effort") && strings.Contains(s, "unrecognized request argument")
}

func (s *Session) tryCopilotReasoningFallback(ctx context.Context) bool {
	s.mu.Lock()
	if s.instance == nil || !strings.EqualFold(strings.TrimSpace(s.instance.Name()), "copilot") {
		s.mu.Unlock()
		return false
	}
	sid := strings.TrimSpace(s.acpSessionID)
	configOptions := append([]acp.ConfigOption(nil), s.sessionMeta.ConfigOptions...)
	s.mu.Unlock()

	if sid == "" {
		return false
	}

	var modelOpt *acp.ConfigOption
	for i := range configOptions {
		opt := &configOptions[i]
		if strings.EqualFold(strings.TrimSpace(opt.ID), "model") || strings.EqualFold(strings.TrimSpace(opt.Category), "model") {
			modelOpt = opt
			break
		}
	}
	if modelOpt == nil || len(modelOpt.Options) == 0 {
		return false
	}

	current := strings.ToLower(strings.TrimSpace(modelOpt.CurrentValue))
	target := ""
	for _, candidate := range modelOpt.Options {
		value := strings.TrimSpace(candidate.Value)
		if value == "" || strings.EqualFold(value, modelOpt.CurrentValue) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(value), "gpt-5") {
			target = value
			break
		}
	}
	if target == "" {
		for _, candidate := range modelOpt.Options {
			value := strings.TrimSpace(candidate.Value)
			lower := strings.ToLower(value)
			if value == "" || strings.EqualFold(value, modelOpt.CurrentValue) {
				continue
			}
			if strings.HasPrefix(lower, "gpt-4") && strings.HasPrefix(current, "gpt-4") {
				continue
			}
			target = value
			break
		}
	}
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
		s.sessionMeta.ConfigOptions = updatedOpts
	}
	s.mu.Unlock()
	s.syncAndPersistProjectState()
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

func isSandboxRefreshErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "windows sandbox: spawn setup refresh")
}
