package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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

