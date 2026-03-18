package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/im"
)

const idleTimeout = 30 * time.Minute

// AgentFactory creates a new agent instance.
// The exePath and env arguments are provided for compatibility; hub-registered
// factories typically ignore them and use closure-captured config instead.
type AgentFactory func(exePath string, env map[string]string) agent.Agent

// Client is the top-level coordinator for a single WheelMaker project.
// Agent initialization is lazy: the first incoming message triggers ensureForwarder(),
// which connects the active agent and creates the ACP forwarder. After 30 minutes of idle
// the agent is disconnected (idleClose) and re-created on the next message.
type Client struct {
	projectName string
	cwd         string

	agentFacs map[string]AgentFactory

	store Store
	state *ProjectState
	imRun im.Channel // nil when no IM channel configured

	debugLog io.Writer // optional ACP JSON debug logger; nil = disabled

	idleTimer *time.Timer // fires idleClose after idleTimeout of inactivity

	mu       sync.Mutex
	promptMu sync.Mutex // serializes handlePrompt and switchAgent

	// active connection
	currentAgent     agent.Agent    // active agent factory; nil until ensureForwarder
	currentAgentName string         // registered name (key in agentFacs/state.Agents)
	forwarder        *acp.Forwarder // active transport; nil until first connection

	// session state (all owned by client)
	sessionID       string
	ready           bool
	initializing    bool
	initCond        *sync.Cond
	initMeta        clientInitMeta
	sessionMeta     clientSessionMeta
	lastReply       string
	loadHistory     []acp.Update
	activeToolCalls map[string]struct{}
	promptCtx       context.Context
	promptCancel    context.CancelFunc
	promptUpdatesCh chan<- acp.Update

	currentPromptCh <-chan acp.Update // tracked for draining during switchAgent

	replayHandler func(acp.SessionUpdateParams) // set temporarily during session/load

	terminals *terminalManager

	permRouter *permissionRouter

	// sessionOverride is set only in tests (via InjectSession in export_test.go).
	// When non-nil, promptStream and cancelPrompt delegate to it instead of the forwarder.
	sessionOverride Session
}

// New creates a Client for the given project.
//   - store: persistent state store scoped to this project
//   - imProvider: IM channel; nil means Run() returns an error (use Hub with a console project)
//   - projectName: identifier used in logs and state keys
//   - cwd: working directory for agent sessions
func New(store Store, imProvider im.Channel, projectName string, cwd string) *Client {
	c := &Client{
		projectName:     projectName,
		cwd:             cwd,
		agentFacs:       make(map[string]AgentFactory),
		store:           store,
		imRun:           imProvider,
		activeToolCalls: make(map[string]struct{}),
	}
	c.initCond = sync.NewCond(&c.mu)
	c.terminals = newTerminalManager()
	c.permRouter = newPermissionRouter(c)
	return c
}

// SetDebugLogger enables ACP JSON debug logging on every subsequent agent connection.
// Pass nil to disable. The writer is injected into acp.Conn at connect time.
func (c *Client) SetDebugLogger(w io.Writer) {
	c.mu.Lock()
	c.debugLog = w
	c.mu.Unlock()
}

// RegisterAgent registers a AgentFactory under the given name.
func (c *Client) RegisterAgent(name string, factory AgentFactory) {
	c.mu.Lock()
	c.agentFacs[name] = factory
	c.mu.Unlock()
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
	c.mu.Unlock()

	if c.imRun != nil {
		c.imRun.OnMessage(c.HandleMessage)
	}
	return nil
}

// Run blocks until ctx is cancelled, delegating to the IM channel's Run loop.
// Returns an error if no IM channel is configured.
func (c *Client) Run(ctx context.Context) error {
	if c.imRun != nil {
		return c.imRun.Run(ctx)
	}
	return errors.New("no IM channel configured; add a console project to config.json")
}

// Close saves state and shuts down the active agent.
// Stops the idle timer to prevent double-close races.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.idleTimer != nil {
		c.idleTimer.Stop()
		c.idleTimer = nil
	}
	c.mu.Unlock()

	c.mu.Lock()
	fwd := c.forwarder
	c.mu.Unlock()
	if fwd != nil {
		c.saveSessionState()
		_ = fwd.Close()
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
// Known commands (/use, /cancel, /status, /mode, /model) are dispatched to handleCommand;
// everything else — including lines starting with "/" that are not known commands —
// is forwarded to the agent as a prompt.
func (c *Client) HandleMessage(msg im.Message) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	if cmd, args, ok := parseCommand(text); ok {
		c.handleCommand(msg, cmd, args)
		return
	}
	if c.permRouter != nil && c.permRouter.resolveIncomingReply(msg.ChatID, text) {
		return
	}
	c.handlePrompt(msg, text)
}

// --- internal ---

// parseCommand checks whether text is a recognized WheelMaker command.
// Only exact first-word matches (/use, /cancel, /status, /mode, /model) are treated as commands;
// all other "/" lines fall through to the agent (fixing the "code starting with /" bug).
func parseCommand(text string) (cmd, args string, ok bool) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "/use", "/cancel", "/status", "/mode", "/model":
		return parts[0], strings.Join(parts[1:], " "), true
	}
	return
}

// handleCommand processes recognized "/" commands.
func (c *Client) handleCommand(msg im.Message, cmd, args string) {
	switch cmd {
	case "/use":
		if args == "" {
			c.reply(msg.ChatID, "Usage: /use <agent-name> [--continue]  (e.g. /use claude)")
			return
		}
		parts := strings.Fields(args)
		name := strings.ToLower(parts[0])
		mode := SwitchClean
		for _, p := range parts[1:] {
			if p == "--continue" {
				mode = SwitchWithContext
			}
		}
		if err := c.switchAgent(context.Background(), msg.ChatID, name, mode); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Switch error: %v", err))
		}

	case "/cancel":
		c.mu.Lock()
		active := c.currentAgent != nil
		c.mu.Unlock()
		if !active {
			c.reply(msg.ChatID, "No active session.")
			return
		}
		if err := c.cancelPrompt(); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Cancel error: %v", err))
			return
		}
		c.reply(msg.ChatID, "Cancelled.")

	case "/status":
		c.mu.Lock()
		agentName := ""
		if c.currentAgent != nil {
			agentName = c.currentAgent.Name()
		}
		sid := c.sessionID
		active := c.currentAgent != nil
		c.mu.Unlock()
		if !active {
			c.reply(msg.ChatID, "No active session.")
			return
		}
		status := fmt.Sprintf("Active agent: %s", agentName)
		if sid != "" {
			status += fmt.Sprintf("\nACP session: %s", sid)
		}
		c.reply(msg.ChatID, status)

	case "/mode":
		if strings.TrimSpace(args) == "" {
			c.reply(msg.ChatID, "Usage: /mode <mode-id-or-name>")
			return
		}
		c.promptMu.Lock()
		defer c.promptMu.Unlock()

		c.mu.Lock()
		if err := c.ensureForwarder(context.Background()); err != nil {
			c.mu.Unlock()
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
			return
		}
		agentName := ""
		if c.currentAgent != nil {
			agentName = c.currentAgent.Name()
		}
		var sessionState *SessionState
		if c.state != nil && c.state.Agents != nil {
			if as := c.state.Agents[agentName]; as != nil && as.Session != nil {
				sessionState = as.Session
			}
		}
		c.resetIdleTimer()
		c.mu.Unlock()

		if err := c.ensureReadyAndNotify(context.Background(), msg.ChatID); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Mode error: %v", err))
			return
		}
		modeConfigID, modeValue, err := resolveModeArg(strings.TrimSpace(args), sessionState)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Mode error: %v", err))
			return
		}
		c.mu.Lock()
		fwd := c.forwarder
		sid := c.sessionID
		c.mu.Unlock()
		if fwd == nil {
			c.reply(msg.ChatID, "Mode error: no active forwarder")
			return
		}
		if _, err := fwd.SessionSetConfigOption(context.Background(), acp.SessionSetConfigOptionParams{
			SessionID: sid,
			ConfigID:  modeConfigID,
			Value:     modeValue,
		}); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Mode error: %v", err))
			return
		}
		c.saveSessionState()
		c.reply(msg.ChatID, fmt.Sprintf("Mode set to: %s", modeValue))

	case "/model":
		if strings.TrimSpace(args) == "" {
			c.reply(msg.ChatID, "Usage: /model <model-id-or-name>")
			return
		}
		c.promptMu.Lock()
		defer c.promptMu.Unlock()

		c.mu.Lock()
		if err := c.ensureForwarder(context.Background()); err != nil {
			c.mu.Unlock()
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
			return
		}
		agentName := ""
		if c.currentAgent != nil {
			agentName = c.currentAgent.Name()
		}
		var sessionState *SessionState
		if c.state != nil && c.state.Agents != nil {
			if as := c.state.Agents[agentName]; as != nil {
				sessionState = as.Session
			}
		}
		c.resetIdleTimer()
		c.mu.Unlock()

		if err := c.ensureReadyAndNotify(context.Background(), msg.ChatID); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Model error: %v", err))
			return
		}
		configID, modelValue, err := resolveModelArg(strings.TrimSpace(args), sessionState)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Model error: %v", err))
			return
		}
		c.mu.Lock()
		fwd := c.forwarder
		sid := c.sessionID
		c.mu.Unlock()
		if fwd == nil {
			c.reply(msg.ChatID, "Model error: no active forwarder")
			return
		}
		if _, err := fwd.SessionSetConfigOption(context.Background(), acp.SessionSetConfigOptionParams{
			SessionID: sid,
			ConfigID:  configID,
			Value:     modelValue,
		}); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Model error: %v", err))
			return
		}
		c.saveSessionState()
		c.reply(msg.ChatID, fmt.Sprintf("Model set to: %s", modelValue))
	}
}

func resolveModeArg(input string, st *SessionState) (configID, value string, err error) {
	mode := strings.TrimSpace(input)
	if mode == "" {
		return "", "", errors.New("empty mode")
	}
	if st != nil {
		for i := range st.ConfigOptions {
			opt := &st.ConfigOptions[i]
			if opt.ID == "mode" || strings.EqualFold(opt.Category, "mode") {
				if len(opt.Options) == 0 {
					return opt.ID, mode, nil
				}
				for _, v := range opt.Options {
					if mode == v.Value || strings.EqualFold(mode, v.Name) {
						return opt.ID, v.Value, nil
					}
				}
				values := make([]string, 0, len(opt.Options))
				for _, v := range opt.Options {
					values = append(values, v.Value)
				}
				slices.Sort(values)
				return "", "", fmt.Errorf("unknown mode %q (available: %s)", mode, strings.Join(values, ", "))
			}
		}
	}
	// configOptions is the only source of truth for mode.
	return "mode", mode, nil
}

func resolveModelArg(input string, st *SessionState) (configID, value string, err error) {
	model := strings.TrimSpace(input)
	if model == "" {
		return "", "", errors.New("empty model")
	}
	configID = "model"

	var modelOpt *acp.ConfigOption
	if st != nil {
		for i := range st.ConfigOptions {
			opt := &st.ConfigOptions[i]
			if opt.ID == "model" || strings.EqualFold(opt.Category, "model") {
				modelOpt = opt
				configID = opt.ID
				break
			}
		}
	}

	if modelOpt != nil && len(modelOpt.Options) > 0 {
		for _, opt := range modelOpt.Options {
			if model == opt.Value || strings.EqualFold(model, opt.Name) {
				return configID, opt.Value, nil
			}
		}
		values := make([]string, 0, len(modelOpt.Options))
		for _, opt := range modelOpt.Options {
			values = append(values, opt.Value)
		}
		slices.Sort(values)
		return "", "", fmt.Errorf("unknown model %q (available: %s)", model, strings.Join(values, ", "))
	}

	return configID, model, nil
}

// handlePrompt sends text to the active (or lazily initialized) session and streams the reply.
// promptMu is held for the full duration, serializing with switchAgent.
func (c *Client) handlePrompt(msg im.Message, text string) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	if c.permRouter != nil {
		c.permRouter.setLastChatID(msg.ChatID)
		defer c.permRouter.clearLastChatID(msg.ChatID)
	}

	ctx := context.Background()

	// When a session override is injected (test-only), skip forwarder initialization.
	c.mu.Lock()
	hasOverride := c.sessionOverride != nil
	c.mu.Unlock()

	if !hasOverride {
		// Lazily initialize the agent if no forwarder exists yet.
		c.mu.Lock()
		if err := c.ensureForwarder(ctx); err != nil {
			c.mu.Unlock()
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
			return
		}
		c.resetIdleTimer() // refresh timeout before sending prompt
		c.mu.Unlock()

		if err := c.ensureReadyAndNotify(ctx, msg.ChatID); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
			return
		}
	}

	updates, err := c.promptStream(ctx, text)
	if err != nil {
		c.reply(msg.ChatID, fmt.Sprintf("Prompt error: %v", err))
		return
	}

	c.mu.Lock()
	c.currentPromptCh = updates
	c.mu.Unlock()

	var buf strings.Builder
	for u := range updates {
		if u.Err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Agent error: %v", u.Err))
			c.mu.Lock()
			c.currentPromptCh = nil
			c.mu.Unlock()
			return
		}
		if u.Type == acp.UpdateConfigOption {
			c.reply(msg.ChatID, formatConfigOptionUpdateMessage(u.Raw))
			c.saveSessionState() // persist immediately; don't wait for prompt to finish
		}
		if u.Type == acp.UpdateText {
			buf.WriteString(u.Content)
		}
		if u.Done {
			break
		}
	}

	c.mu.Lock()
	c.currentPromptCh = nil
	c.resetIdleTimer() // reset after prompt completes: 30 min from last activity
	c.mu.Unlock()

	// Persist after prompt completes: session ID and config metadata may have changed.
	c.saveSessionState()

	if buf.Len() > 0 {
		c.reply(msg.ChatID, buf.String())
	}
}

// ensureForwarder connects the active agent and sets up the Forwarder if not already running.
// Must be called while holding c.mu.
func (c *Client) ensureForwarder(ctx context.Context) error {
	if c.forwarder != nil {
		return nil
	}
	if c.state == nil {
		return errors.New("state not loaded")
	}
	name := c.state.ActiveAgent
	if name == "" {
		name = "claude"
	}
	fac := c.agentFacs[name]
	if fac == nil {
		return fmt.Errorf("no agent registered for %q", name)
	}
	baseAgent := fac("", nil)
	conn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if c.debugLog != nil {
		conn.SetDebugLogger(c.debugLog)
	}
	fwd := acp.NewForwarder(conn, makePrefilter(baseAgent))
	fwd.SetCallbacks(c)
	c.forwarder = fwd
	c.currentAgent = baseAgent
	c.currentAgentName = name
	c.ready = false
	// Restore saved session ID if present.
	if c.state.Agents != nil {
		if as := c.state.Agents[name]; as != nil && as.LastSessionID != "" {
			c.sessionID = as.LastSessionID
		}
	}
	c.resetIdleTimer()
	return nil
}

// makePrefilter returns a Forwarder prefilter that applies ag.NormalizeParams
// to every incoming notification, ensuring the client receives standard ACP.
func makePrefilter(ag agent.Agent) acp.Prefilter {
	return func(ctx context.Context, msg acp.ForwardMessage) (acp.ForwardMessage, bool, error) {
		if msg.Direction == acp.DirectionToClient && msg.Kind == acp.KindNotification {
			msg.Params = ag.NormalizeParams(msg.Method, msg.Params)
		}
		return msg, true, nil
	}
}

// resetIdleTimer restarts the 30-minute idle timer.
// Must be called while holding c.mu.
func (c *Client) resetIdleTimer() {
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}
	c.idleTimer = time.AfterFunc(idleTimeout, c.idleClose)
}

// idleClose is called by the idle timer when no activity has occurred for idleTimeout.
// It acquires promptMu (to wait for any in-progress prompt) then saves state and closes
// the agent subprocess.
func (c *Client) idleClose() {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	c.mu.Lock()
	if c.forwarder == nil {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	c.saveSessionState()
	c.terminals.KillAll()

	c.mu.Lock()
	fwd := c.forwarder
	if fwd != nil {
		_ = fwd.Close()
	}
	c.forwarder = nil
	c.currentAgent = nil
	c.currentAgentName = ""
	c.ready = false
	c.sessionID = ""
	c.initMeta = clientInitMeta{}
	c.sessionMeta = clientSessionMeta{}
	c.mu.Unlock()
}

// switchAgent cancels any in-progress prompt, waits for it to finish via
// promptMu, connects a new agent binary, and replaces the forwarder.
func (c *Client) switchAgent(ctx context.Context, chatID, name string, mode SwitchMode) error {
	c.mu.Lock()
	fac := c.agentFacs[name]
	c.mu.Unlock()
	if fac == nil {
		return fmt.Errorf("unknown agent: %q (registered: %v)", name, c.registeredAgentNames())
	}

	// Cancel in-progress prompt, wait for handlePrompt to release promptMu.
	_ = c.cancelPrompt()
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	// Belt-and-suspenders: drain any channel published between cancelPrompt and promptMu.Lock.
	c.mu.Lock()
	promptCh := c.currentPromptCh
	c.mu.Unlock()
	if promptCh != nil {
		for range promptCh {
		}
		c.mu.Lock()
		c.currentPromptCh = nil
		c.mu.Unlock()
	}

	// Capture outgoing state.
	c.mu.Lock()
	oldFwd := c.forwarder
	savedLastReply := c.lastReply
	c.mu.Unlock()
	c.persistMeta() // save outgoing agent state before reset

	// Read saved session ID for incoming agent.
	c.mu.Lock()
	var savedSID string
	if c.state != nil && c.state.Agents != nil {
		if as := c.state.Agents[name]; as != nil {
			savedSID = as.LastSessionID
		}
	}
	dw := c.debugLog
	c.mu.Unlock()

	// Connect new agent.
	baseAgent := fac("", nil)
	newConn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if dw != nil {
		newConn.SetDebugLogger(dw)
	}
	newFwd := acp.NewForwarder(newConn, makePrefilter(baseAgent))
	newFwd.SetCallbacks(c)

	// Replace forwarder atomically; kill terminals; close old conn.
	c.mu.Lock()
	c.terminals.KillAll()
	c.forwarder = newFwd
	c.currentAgent = baseAgent
	c.currentAgentName = name
	c.ready = false
	c.initializing = false
	c.sessionID = savedSID
	c.lastReply = ""
	c.loadHistory = nil
	c.activeToolCalls = make(map[string]struct{})
	c.promptUpdatesCh = nil
	c.initMeta = clientInitMeta{}
	c.sessionMeta = clientSessionMeta{}
	c.mu.Unlock()
	c.initCond.Broadcast()

	if oldFwd != nil {
		_ = oldFwd.Close()
	}

	// SwitchWithContext — bootstrap new session with previous reply.
	if mode == SwitchWithContext && savedLastReply != "" {
		ch, err := c.promptStream(ctx, "[context] "+savedLastReply)
		if err != nil {
			log.Printf("client: SwitchWithContext bootstrap prompt failed: %v", err)
		} else {
			for u := range ch {
				if u.Err != nil {
					log.Printf("client: SwitchWithContext bootstrap prompt failed: %v", u.Err)
				}
			}
		}
		c.persistMeta()
	}

	// Update ActiveAgent, save, reset timer.
	c.mu.Lock()
	if c.state != nil {
		c.state.ActiveAgent = name
	}
	c.resetIdleTimer()
	s := c.state
	c.mu.Unlock()
	if s != nil {
		_ = c.store.Save(s)
	}

	c.reply(chatID, fmt.Sprintf("Switched to agent: %s", name))
	snap := c.sessionConfigSnapshot()
	if snap.Mode != "" || snap.Model != "" {
		c.reply(chatID, fmt.Sprintf("Session ready: mode=%s model=%s",
			renderUnknown(snap.Mode), renderUnknown(snap.Model)))
	}
	return nil
}

// registeredAgentNames returns all registered agent names (for error messages).
func (c *Client) registeredAgentNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.agentFacs))
	for n := range c.agentFacs {
		names = append(names, n)
	}
	return names
}

// reply sends a text response to the chat via the IM channel.
func (c *Client) reply(chatID, text string) {
	if c.imRun != nil {
		_ = c.imRun.SendText(chatID, text)
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

func formatConfigOptionUpdateMessage(raw []byte) string {
	if len(raw) == 0 {
		return "Config options updated."
	}
	// Raw may be a SessionUpdate (marshaled directly) or a SessionUpdateParams wrapper.
	// Try SessionUpdate first (the common case from sessionUpdateToUpdate).
	var opts []acp.ConfigOption
	var u acp.SessionUpdate
	if err := json.Unmarshal(raw, &u); err == nil && len(u.ConfigOptions) > 0 {
		opts = u.ConfigOptions
	} else {
		var p acp.SessionUpdateParams
		if err := json.Unmarshal(raw, &p); err == nil {
			opts = p.Update.ConfigOptions
		}
	}
	if len(opts) == 0 {
		return "Config options updated."
	}
	mode := ""
	model := ""
	for _, opt := range opts {
		if mode == "" && (opt.ID == "mode" || strings.EqualFold(opt.Category, "mode")) {
			mode = strings.TrimSpace(opt.CurrentValue)
		}
		if model == "" && (opt.ID == "model" || strings.EqualFold(opt.Category, "model")) {
			model = strings.TrimSpace(opt.CurrentValue)
		}
	}
	if mode == "" && model == "" {
		return "Config options updated."
	}
	return fmt.Sprintf("Config options updated: mode=%s model=%s", renderUnknown(mode), renderUnknown(model))
}
