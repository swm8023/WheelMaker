package client

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// agentConn bundles an active agent subprocess with its ACP forwarder.
// Client holds at most one agentConn at a time; nil means no connection yet.
type agentConn struct {
	name      string         // registered name (key in state.Agents)
	agent     agent.Agent    // backing agent (owns the subprocess)
	forwarder *acp.Forwarder // ACP transport; nil only in test injection
}

func (ac *agentConn) close() error {
	if ac.forwarder == nil {
		return nil
	}
	return ac.forwarder.Close()
}

// agentRegistry maps agent names to their factories.
// It carries its own mutex so Client.mu need not protect registration.
type agentRegistry struct {
	mu   sync.Mutex
	facs map[string]AgentFactory
}

func newAgentRegistry() *agentRegistry {
	return &agentRegistry{facs: make(map[string]AgentFactory)}
}

func (r *agentRegistry) register(name string, f AgentFactory) {
	r.mu.Lock()
	r.facs[name] = f
	r.mu.Unlock()
}

func (r *agentRegistry) get(name string) AgentFactory {
	r.mu.Lock()
	f := r.facs[name]
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

// Client is the top-level coordinator for a single WheelMaker project.
// Agent initialization is lazy: the first incoming message triggers ensureForwarder(),
// which connects the active agent and creates the ACP forwarder.
type Client struct {
	projectName string
	cwd         string

	registry *agentRegistry

	store    Store
	state    *ProjectState
	imBridge *im.Bridge // nil when no IM channel configured

	debugLog io.Writer // optional ACP JSON debug logger; nil = disabled

	mu       sync.Mutex
	promptMu sync.Mutex // serializes handlePrompt and switchAgent

	// active connection; nil until first ensureForwarder
	conn *agentConn

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
//   - imProvider: IM bridge; nil means Run() returns an error (use Hub with a console project)
//   - projectName: identifier used in logs and state keys
//   - cwd: working directory for agent sessions
func New(store Store, imProvider *im.Bridge, projectName string, cwd string) *Client {
	c := &Client{
		projectName:     projectName,
		cwd:             cwd,
		registry:        newAgentRegistry(),
		store:           store,
		imBridge:        imProvider,
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
	c.mu.Unlock()

	if c.imBridge != nil {
		c.imBridge.OnMessage(c.HandleMessage)
		c.imBridge.SetHelpResolver(c.resolveHelpModel)
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

// Close saves state and shuts down the active agent.
func (c *Client) Close() error {
	c.mu.Lock()
	ac := c.conn
	c.mu.Unlock()
	if ac != nil {
		c.saveSessionState()
		_ = ac.close()
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
	emitter := c.imBridge
	hasEmitter := emitter != nil
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

// reply sends a text response to the chat via the IM channel.
func (c *Client) reply(chatID, text string) {
	if c.imBridge != nil {
		_ = c.imBridge.SendText(chatID, text)
		return
	}
	fmt.Println(text)
}

// replyDebug sends debug text via IM debug channel when available.
func (c *Client) replyDebug(chatID, text string) {
	if c.imBridge != nil {
		_ = c.imBridge.SendDebug(chatID, text)
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
