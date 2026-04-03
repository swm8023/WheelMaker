package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/hub/im"
)

// AgentFactory creates a new agent instance.
// The exePath and env arguments are provided for compatibility; hub-registered
// factories typically ignore them and use closure-captured config instead.
type AgentFactory func(exePath string, env map[string]string) agent.Agent

// agentRegistry maps agent names to their factories.
// It carries its own mutex so Client.mu need not protect registration.
type agentRegistry struct {
	mu     sync.Mutex
	facs   map[string]AgentFactory
	v2facs map[string]AgentFactoryV2
}

func newAgentRegistry() *agentRegistry {
	return &agentRegistry{
		facs:   make(map[string]AgentFactory),
		v2facs: make(map[string]AgentFactoryV2),
	}
}

func (r *agentRegistry) register(name string, f AgentFactory) {
	r.mu.Lock()
	r.facs[name] = f
	r.v2facs[name] = wrapLegacyFactory(name, f)
	r.mu.Unlock()
}

func (r *agentRegistry) get(name string) AgentFactory {
	r.mu.Lock()
	f := r.facs[name]
	r.mu.Unlock()
	return f
}

func (r *agentRegistry) getV2(name string) AgentFactoryV2 {
	r.mu.Lock()
	f := r.v2facs[name]
	r.mu.Unlock()
	return f
}

func (r *agentRegistry) names() []string {
	r.mu.Lock()
	ns := make([]string, 0, len(r.facs))
	for n := range r.facs {
		ns = append(ns, n)
	}
	r.mu.Unlock()
	return ns
}

const commandTimeout = 30 * time.Second

const defaultAgentName = "claude"

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

	registry *agentRegistry

	store        Store
	sessionStore SessionStore // optional; nil = in-memory only
	state        *ProjectState
	imBridge     *im.ImAdapter // nil when no IM channel configured

	debugLog io.Writer // optional ACP JSON debug logger; nil = disabled

	mu sync.Mutex

	// sessions maps session IDs to Session objects.
	sessions map[string]*Session

	// routeMap maps IM routing keys to Session IDs.
	// Multiple routes can point to the same Session.
	routeMap map[string]string

	// activeSession is the Session currently handling messages.
	activeSession *Session

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
		registry:         newAgentRegistry(),
		store:            store,
		imBridge:         imProvider,
		imBlockedUpdates: map[string]struct{}{},
		sessions:         make(map[string]*Session),
		routeMap:         make(map[string]string),
	}
	sess := newSession("default", cwd)
	sess.store = store
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

// SetDebugLogger enables ACP JSON debug logging on every subsequent agent connection.
// Pass nil to disable. The writer is injected into acp.Conn at connect time.
func (c *Client) SetDebugLogger(w io.Writer) {
	c.mu.Lock()
	c.debugLog = w
	sessions := make([]*Session, 0, len(c.sessions))
	for _, s := range c.sessions {
		sessions = append(sessions, s)
	}
	c.mu.Unlock()
	for _, sess := range sessions {
		sess.mu.Lock()
		sess.debugLog = w
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
	c.mu.Lock()
	c.sessionStore = ss
	c.mu.Unlock()
}

// RegisterAgent registers an AgentFactory under the given name.
func (c *Client) RegisterAgent(name string, factory AgentFactory) {
	c.registry.register(name, factory)
}

// Start loads persisted state and registers the IM message callback.
// Agent initialization is deferred until the first incoming message (lazy init).
func (c *Client) Start(ctx context.Context) error {
	state, err := c.store.Load()
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
	c.mu.Lock()
	sessions := make([]*Session, 0, len(c.sessions))
	for _, sess := range c.sessions {
		sessions = append(sessions, sess)
	}
	ss := c.sessionStore
	pn := c.projectName
	c.mu.Unlock()

	ctx := context.Background()
	for _, sess := range sessions {
		sess.mu.Lock()
		inst := sess.instance
		sess.mu.Unlock()
		if inst != nil {
			sess.saveSessionState()
			if ss != nil {
				_ = sess.Suspend(ctx, ss, pn)
			} else {
				_ = inst.Close()
			}
		}
	}

	c.mu.Lock()
	s := c.state
	c.mu.Unlock()
	if s != nil {
		return c.store.Save(s)
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

	sess := newSession(id, c.cwd)
	sess.store = c.store
	sess.registry = c.registry
	sess.imBridge = c.imBridge
	sess.imBlockedUpdates = c.imBlockedUpdates
	sess.debugLog = c.debugLog
	sess.yolo = c.yolo
	sess.state = c.state
	sess.permRouter = newPermissionRouter(sess)

	c.sessions[id] = sess
	c.routeMap[routeKey] = id
	c.activeSession = sess
	return sess
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
				s.saveSessionState()
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

		s.saveSessionState()

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
	s.saveSessionState()
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
