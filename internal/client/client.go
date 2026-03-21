package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/im"
)

// AgentFactory creates a new agent instance.
// The exePath and env arguments are provided for compatibility; hub-registered
// factories typically ignore them and use closure-captured config instead.
type AgentFactory func(exePath string, env map[string]string) agent.Agent

const commandTimeout = 30 * time.Second

// Client is the top-level coordinator for a single WheelMaker project.
// Agent initialization is lazy: the first incoming message triggers ensureForwarder(),
// which connects the active agent and creates the ACP forwarder.
type Client struct {
	projectName string
	cwd         string

	agentFacs map[string]AgentFactory

	store Store
	state *ProjectState
	imRun im.Channel // nil when no IM channel configured

	debugLog     io.Writer // optional ACP JSON debug logger sink
	debugEnabled bool      // project-level debug toggle from config

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
	debugSink  *agentDebugSink

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
	c.debugSink = newAgentDebugSink(c)
	return c
}

// SetDebugLogger enables ACP JSON debug logging on every subsequent agent connection.
// Pass nil to disable. The writer is injected into acp.Conn at connect time.
func (c *Client) SetDebugLogger(w io.Writer) {
	c.mu.Lock()
	c.debugLog = w
	c.debugEnabled = w != nil
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
		if hs, ok := c.imRun.(im.HelpResolverSetter); ok {
			hs.SetHelpResolver(c.resolveHelpModel)
		}
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
func (c *Client) Close() error {
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
// Known commands (/use, /cancel, /status, /mode, /model, /list, /new, /load) are dispatched to handleCommand;
// everything else — including lines starting with "/" that are not known commands —
// is forwarded to the agent as a prompt.
func (c *Client) HandleMessage(msg im.Message) {
	c.bindDebugChat(c.resolveCurrentAgentName(), msg.ChatID)

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	if cmd, args, ok := parseCommand(text); ok {
		c.handleCommand(msg, cmd, args)
		return
	}
	c.handlePrompt(msg, text)
}

// --- internal ---

// parseCommand checks whether text is a recognized WheelMaker command.
// Only exact first-word matches (/use, /cancel, /status, /mode, /model, /list, /new, /load, /debug) are treated as commands;
// all other "/" lines fall through to the agent (fixing the "code starting with /" bug).
func parseCommand(text string) (cmd, args string, ok bool) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "/use", "/cancel", "/status", "/mode", "/model", "/list", "/new", "/load", "/debug":
		return parts[0], strings.Join(parts[1:], " "), true
	}
	return
}

// handleCommand processes recognized "/" commands.
func (c *Client) handleCommand(msg im.Message, cmd, args string) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

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
		if err := c.switchAgent(ctx, msg.ChatID, name, mode); err != nil {
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

	case "/list":
		c.promptMu.Lock()
		defer c.promptMu.Unlock()
		lines, err := c.listSessions(ctx)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("List error: %v", err))
			return
		}
		c.reply(msg.ChatID, strings.Join(lines, "\n"))

	case "/new":
		c.promptMu.Lock()
		defer c.promptMu.Unlock()
		sid, err := c.createNewSession(ctx)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("New error: %v", err))
			return
		}
		c.reply(msg.ChatID, fmt.Sprintf("Created new session: %s", sid))

	case "/load":
		idxStr := strings.TrimSpace(args)
		if idxStr == "" {
			c.reply(msg.ChatID, "Usage: /load <index>  (see /list)")
			return
		}
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx <= 0 {
			c.reply(msg.ChatID, "Load error: index must be a positive integer")
			return
		}
		c.promptMu.Lock()
		defer c.promptMu.Unlock()
		sid, err := c.loadSessionByIndex(ctx, idx)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Load error: %v", err))
			return
		}
		c.reply(msg.ChatID, fmt.Sprintf("Loaded session: %s", sid))

	case "/mode":
		c.handleConfigCommand(ctx, msg.ChatID, args, "Usage: /mode <mode-id-or-name>", "Mode", resolveModeArg)

	case "/model":
		c.handleConfigCommand(ctx, msg.ChatID, args, "Usage: /model <model-id-or-name>", "Model", resolveModelArg)

	case "/debug":
		if err := c.handleDebugCommand(msg.ChatID, args); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Debug error: %v", err))
		}
	}
}

func (c *Client) handleConfigCommand(
	ctx context.Context,
	chatID string,
	args string,
	usage string,
	label string,
	resolve func(input string, st *SessionState) (configID, value string, err error),
) {
	input := strings.TrimSpace(args)
	if input == "" {
		c.reply(chatID, usage)
		return
	}

	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	if err := c.ensureForwarder(ctx); err != nil {
		c.reply(chatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
		return
	}

	c.mu.Lock()
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
	c.mu.Unlock()

	if err := c.ensureReadyAndNotify(ctx, chatID); err != nil {
		c.reply(chatID, fmt.Sprintf("%s error: %v", label, err))
		return
	}

	configID, value, err := resolve(input, sessionState)
	if err != nil {
		c.reply(chatID, fmt.Sprintf("%s error: %v", label, err))
		return
	}

	c.mu.Lock()
	fwd := c.forwarder
	sid := c.sessionID
	c.mu.Unlock()
	if fwd == nil {
		c.reply(chatID, fmt.Sprintf("%s error: no active forwarder", label))
		return
	}

	if _, err := fwd.SessionSetConfigOption(ctx, acp.SessionSetConfigOptionParams{
		SessionID: sid,
		ConfigID:  configID,
		Value:     value,
	}); err != nil {
		c.reply(chatID, fmt.Sprintf("%s error: %v", label, err))
		return
	}

	c.saveSessionState()
	c.reply(chatID, fmt.Sprintf("%s set to: %s", label, value))
}

func resolveModeArg(input string, st *SessionState) (configID, value string, err error) {
	return resolveConfigSelectArg("mode", "mode", input, st)
}

func resolveModelArg(input string, st *SessionState) (configID, value string, err error) {
	return resolveConfigSelectArg("model", "model", input, st)
}

func resolveConfigSelectArg(kind string, defaultConfigID string, input string, st *SessionState) (configID, value string, err error) {
	v := strings.TrimSpace(input)
	if v == "" {
		return "", "", fmt.Errorf("empty %s", kind)
	}

	configID = defaultConfigID
	var targetOpt *acp.ConfigOption
	if st != nil {
		for i := range st.ConfigOptions {
			opt := &st.ConfigOptions[i]
			if opt.ID == defaultConfigID || strings.EqualFold(opt.Category, kind) {
				targetOpt = opt
				configID = opt.ID
				break
			}
		}
	}

	if targetOpt != nil && len(targetOpt.Options) > 0 {
		for _, opt := range targetOpt.Options {
			if v == opt.Value || strings.EqualFold(v, opt.Name) {
				return configID, opt.Value, nil
			}
		}
		values := make([]string, 0, len(targetOpt.Options))
		for _, opt := range targetOpt.Options {
			values = append(values, opt.Value)
		}
		slices.Sort(values)
		return "", "", fmt.Errorf("unknown %s %q (available: %s)", kind, v, strings.Join(values, ", "))
	}

	return configID, v, nil
}

func (c *Client) listSessions(ctx context.Context) ([]string, error) {
	if err := c.ensureForwarder(ctx); err != nil {
		return nil, err
	}

	if err := c.ensureReady(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	fwd := c.forwarder
	cwd := c.cwd
	curSID := c.sessionID
	agentName := c.currentAgentName
	caps := c.initMeta.AgentCapabilities
	c.mu.Unlock()

	if fwd == nil {
		return nil, errors.New("no active forwarder")
	}
	if caps.SessionCapabilities == nil || caps.SessionCapabilities.List == nil {
		return nil, errors.New("agent does not support session/list")
	}

	all := make([]acp.SessionInfo, 0, 16)
	cursor := ""
	for page := 0; page < 20; page++ {
		res, err := fwd.SessionList(ctx, acp.SessionListParams{
			CWD:    cwd,
			Cursor: cursor,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, res.Sessions...)
		if strings.TrimSpace(res.NextCursor) == "" || res.NextCursor == cursor {
			break
		}
		cursor = res.NextCursor
	}

	summaries := make([]SessionSummary, 0, len(all))
	lines := make([]string, 0, len(all)+1)
	lines = append(lines, fmt.Sprintf("Sessions (%d):", len(all)))
	for i, s := range all {
		summaries = append(summaries, SessionSummary{
			ID:        s.SessionID,
			Title:     s.Title,
			UpdatedAt: s.UpdatedAt,
		})
		marker := " "
		if s.SessionID == curSID {
			marker = "*"
		}
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = "(no title)"
		}
		lines = append(lines, fmt.Sprintf("%s %d. %s  %s", marker, i+1, s.SessionID, title))
	}

	c.persistSessionSummaries(agentName, summaries)
	return lines, nil
}

func (c *Client) createNewSession(ctx context.Context) (string, error) {
	if err := c.ensureForwarder(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	fwd := c.forwarder
	cwd := c.cwd
	c.mu.Unlock()
	if fwd == nil {
		return "", errors.New("no active forwarder")
	}
	if err := c.ensureReady(ctx); err != nil {
		return "", err
	}

	res, err := fwd.SessionNew(ctx, acp.SessionNewParams{
		CWD:        cwd,
		MCPServers: []acp.MCPServer{},
	})
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.sessionID = res.SessionID
	c.ready = true
	c.lastReply = ""
	c.loadHistory = nil
	c.activeToolCalls = make(map[string]struct{})
	c.sessionMeta = clientSessionMeta{
		ConfigOptions: res.ConfigOptions,
	}
	c.mu.Unlock()
	c.saveSessionState()
	return res.SessionID, nil
}

func (c *Client) loadSessionByIndex(ctx context.Context, index int) (string, error) {
	lines, err := c.listSessions(ctx)
	if err != nil {
		return "", err
	}
	_ = lines // listSessions already refreshes and persists state

	c.mu.Lock()
	agentName := c.currentAgentName
	fwd := c.forwarder
	cwd := c.cwd
	loadCap := c.initMeta.AgentCapabilities.LoadSession
	var sessions []SessionSummary
	if c.state != nil && c.state.Agents != nil {
		if as := c.state.Agents[agentName]; as != nil {
			sessions = append(sessions, as.Sessions...)
		}
	}
	c.mu.Unlock()

	if !loadCap {
		return "", errors.New("agent does not support session/load")
	}
	if fwd == nil {
		return "", errors.New("no active forwarder")
	}
	if index < 1 || index > len(sessions) {
		return "", fmt.Errorf("index out of range (1-%d)", len(sessions))
	}
	target := sessions[index-1].ID
	if strings.TrimSpace(target) == "" {
		return "", errors.New("invalid session id")
	}

	_, err = fwd.SessionLoad(ctx, acp.SessionLoadParams{
		SessionID:  target,
		CWD:        cwd,
		MCPServers: []acp.MCPServer{},
	})
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.sessionID = target
	c.ready = true
	c.lastReply = ""
	c.loadHistory = nil
	c.activeToolCalls = make(map[string]struct{})
	c.sessionMeta = clientSessionMeta{}
	c.mu.Unlock()
	c.saveSessionState()
	return target, nil
}

func (c *Client) persistSessionSummaries(agentName string, sessions []SessionSummary) {
	if strings.TrimSpace(agentName) == "" {
		return
	}
	c.mu.Lock()
	if c.state == nil {
		c.mu.Unlock()
		return
	}
	if c.state.Agents == nil {
		c.state.Agents = map[string]*AgentState{}
	}
	as := c.state.Agents[agentName]
	if as == nil {
		as = &AgentState{}
		c.state.Agents[agentName] = as
	}
	as.Sessions = sessions
	s := c.state
	c.mu.Unlock()
	_ = c.store.Save(s)
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
		if err := c.ensureForwarder(ctx); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
			return
		}

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
	c.mu.Lock()
	emitter, hasEmitter := c.imRun.(im.UpdateEmitter)
	sid := c.sessionID
	c.mu.Unlock()
	for u := range updates {
		if hasEmitter {
			emitErr := emitter.Emit(ctx, im.IMUpdate{
				ChatID:     msg.ChatID,
				SessionID:  sid,
				UpdateType: string(u.Type),
				Text:       u.Content,
				Raw:        u.Raw,
			})
			if emitErr != nil {
				c.reply(msg.ChatID, fmt.Sprintf("IM emit error: %v", emitErr))
			}
		}
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
			if !hasEmitter {
				buf.WriteString(u.Content)
			}
		}
		if u.Done {
			break
		}
	}

	c.mu.Lock()
	c.currentPromptCh = nil
	c.mu.Unlock()

	// Persist after prompt completes: session ID and config metadata may have changed.
	c.saveSessionState()

	if !hasEmitter && buf.Len() > 0 {
		c.reply(msg.ChatID, buf.String())
	}
}

// ensureForwarder connects the active agent and sets up the Forwarder if not already running.
// Connect() is intentionally executed outside c.mu to avoid blocking unrelated operations.
func (c *Client) ensureForwarder(ctx context.Context) error {
	c.mu.Lock()
	if c.forwarder != nil {
		c.mu.Unlock()
		return nil
	}
	if c.state == nil {
		c.mu.Unlock()
		return errors.New("state not loaded")
	}
	name := c.state.ActiveAgent
	if name == "" {
		name = "claude"
	}
	fac := c.agentFacs[name]
	if fac == nil {
		c.mu.Unlock()
		return fmt.Errorf("no agent registered for %q", name)
	}
	dw := c.debugLog
	debugEnabled := c.debugEnabled
	savedSID := ""
	if c.state.Agents != nil {
		if as := c.state.Agents[name]; as != nil && as.LastSessionID != "" {
			savedSID = as.LastSessionID
		}
	}
	c.mu.Unlock()

	baseAgent := fac("", nil)
	conn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if debugWriter := c.composeDebugWriter(name, dw, debugEnabled); debugWriter != nil {
		conn.SetDebugLogger(debugWriter)
	}
	fwd := acp.NewForwarder(conn, nil)
	fwd.SetCallbacks(c)

	c.mu.Lock()
	if c.forwarder != nil {
		c.mu.Unlock()
		_ = fwd.Close()
		return nil
	}
	c.forwarder = fwd
	c.currentAgent = baseAgent
	c.currentAgentName = name
	c.ready = false
	c.sessionID = savedSID
	c.mu.Unlock()
	return nil
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
	debugEnabled := c.debugEnabled
	c.mu.Unlock()

	// Connect new agent.
	baseAgent := fac("", nil)
	newConn, err := baseAgent.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if debugWriter := c.composeDebugWriter(name, dw, debugEnabled); debugWriter != nil {
		newConn.SetDebugLogger(debugWriter)
	}
	newFwd := acp.NewForwarder(newConn, nil)
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

	// Update ActiveAgent and save.
	c.mu.Lock()
	if c.state != nil {
		c.state.ActiveAgent = name
	}
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

// replyDebug sends debug text via IM debug channel when available.
func (c *Client) replyDebug(chatID, text string) {
	if c.imRun != nil {
		if dbg, ok := c.imRun.(im.DebugSender); ok {
			_ = dbg.SendDebug(chatID, text)
			return
		}
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

func (c *Client) resolveHelpModel(ctx context.Context, _ string) (im.HelpModel, error) {
	c.mu.Lock()
	hasForwarder := c.forwarder != nil
	c.mu.Unlock()
	if !hasForwarder {
		_ = c.ensureForwarder(ctx)
	}
	_ = c.ensureReady(ctx)

	c.mu.Lock()
	opts := append([]acp.ConfigOption(nil), c.sessionMeta.ConfigOptions...)
	commands := append([]acp.AvailableCommand(nil), c.sessionMeta.AvailableCommands...)
	c.mu.Unlock()

	model := im.HelpModel{
		Title: "WheelMaker Help",
		Body:  "Choose a quick action below. Advanced commands can still be typed manually.",
	}
	model.Options = append(model.Options, im.HelpOption{Label: "Status", Command: "/status"})
	model.Options = append(model.Options, im.HelpOption{Label: "Cancel", Command: "/cancel"})
	model.Options = append(model.Options, im.HelpOption{Label: "List Sessions", Command: "/list"})
	model.Options = append(model.Options, im.HelpOption{Label: "New Session", Command: "/new"})
	model.Options = append(model.Options, im.HelpOption{Label: "Project Debug Status", Command: "/debug"})
	c.mu.Lock()
	agentNames := make([]string, 0, len(c.agentFacs))
	for name := range c.agentFacs {
		agentNames = append(agentNames, name)
	}
	c.mu.Unlock()
	for _, name := range agentNames {
		model.Options = append(model.Options, im.HelpOption{
			Label:   "Agent: " + name,
			Command: "/use",
			Value:   name,
		})
	}

	for _, opt := range opts {
		switch opt.ID {
		case "mode":
			for _, v := range opt.Options {
				model.Options = append(model.Options, im.HelpOption{
					Label:   "Mode: " + firstNonEmpty(v.Name, v.Value),
					Command: "/mode",
					Value:   v.Value,
				})
			}
		case "model":
			for _, v := range opt.Options {
				model.Options = append(model.Options, im.HelpOption{
					Label:   "Model: " + firstNonEmpty(v.Name, v.Value),
					Command: "/model",
					Value:   v.Value,
				})
			}
		}
	}
	if len(commands) > 0 {
		names := make([]string, 0, len(commands))
		for _, cmd := range commands {
			if strings.TrimSpace(cmd.Name) != "" {
				names = append(names, cmd.Name)
			}
		}
		if len(names) > 0 {
			model.Body += "\nAvailable slash commands: " + strings.Join(names, ", ")
		}
	}
	return model, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
