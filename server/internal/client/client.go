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

const (
	lifecycleStartNotice    = "WheelMaker server started."
	lifecycleShutdownNotice = "WheelMaker server stopping."
)

var acpClientInfo = &acp.AgentInfo{Name: "wheelmaker", Version: "0.1"}

type sessionState struct {
	id           string
	ready        bool
	initializing bool
	lastReply    string
	replayH      func(acp.SessionUpdateParams)
}

type promptState struct {
	ctx       context.Context
	cancel    context.CancelFunc
	updatesCh chan<- acp.Update
	currentCh <-chan acp.Update // tracked for draining during switchAgent
	activeTCs map[string]struct{}
}

// Client is the top-level coordinator for a single WheelMaker project.
// Agent initialization is lazy: the first incoming message triggers ensureForwarder(),
// which connects the active agent and creates the ACP forwarder.
type Client struct {
	projectName string
	cwd         string
	yolo        bool

	registry *agentRegistry

	store    Store
	state    *ProjectState
	imBridge *im.ImAdapter // nil when no IM channel configured

	debugLog io.Writer // optional ACP JSON debug logger; nil = disabled

	mu       sync.Mutex
	promptMu sync.Mutex // serializes handlePrompt and switchAgent

	// active connection; nil until first ensureForwarder
	conn *agentConn

	initCond *sync.Cond
	session  sessionState
	prompt   promptState
	initMeta clientInitMeta
	// sessionMeta tracks runtime session snapshot from session/update.
	sessionMeta clientSessionMeta

	terminals *terminalManager

	permRouter *permissionRouter
	debugSink  *agentDebugSink
}

// New creates a Client for the given project.
//   - store: persistent state store scoped to this project
//   - imProvider: IM bridge; nil means Run() returns an error (use Hub with a console project)
//   - projectName: identifier used in logs and state keys
//   - cwd: working directory for agent sessions
func New(store Store, imProvider *im.ImAdapter, projectName string, cwd string) *Client {
	c := &Client{
		projectName: projectName,
		cwd:         cwd,
		registry:    newAgentRegistry(),
		store:       store,
		imBridge:    imProvider,
		prompt: promptState{
			activeTCs: make(map[string]struct{}),
		},
	}
	c.initCond = sync.NewCond(&c.mu)
	c.terminals = newTerminalManager()
	c.permRouter = newPermissionRouter(c)
	c.debugSink = newAgentDebugSink(c)
	return c
}

// SetYOLO enables/disables always-approve permission mode for this project.
func (c *Client) SetYOLO(enabled bool) {
	c.mu.Lock()
	c.yolo = enabled
	c.mu.Unlock()
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
		c.notifyLifecycle(lifecycleStartNotice)
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
	c.notifyLifecycle(lifecycleShutdownNotice)

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

func (c *Client) notifyLifecycle(text string) {
	chatID := c.permRouter.currentChatIDOrFallback()
	if strings.TrimSpace(chatID) == "" {
		return
	}
	c.reply(chatID, text)
}

// HandleMessage routes an incoming IM message to the appropriate handler.
// Known commands (/use, /cancel, /status, /mode, /model, /list, /new, /load) are dispatched to handleCommand;
// everything else — including lines starting with "/" that are not known commands —
// is forwarded to the agent as a prompt.
func (c *Client) HandleMessage(msg im.Message) {
	c.permRouter.setLastChatID(msg.ChatID)
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

	ctx := context.Background()
	for attempt := 1; attempt <= 2; attempt++ {
		// Lazily initialize the agent if no forwarder exists yet.
		if err := c.ensureForwarder(ctx); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
			return
		}

		if err := c.ensureReadyAndNotify(ctx, msg.ChatID); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <agent> to connect.", err))
			return
		}

		updates, err := c.promptStream(ctx, text)
		if err != nil {
			// Keepalive: recover dead agent subprocess and retry current prompt once.
			if c.resetDeadConnection(err) && attempt == 1 {
				c.reply(msg.ChatID, "Agent disconnected, reconnecting and retrying once...")
				continue
			}
			c.reply(msg.ChatID, fmt.Sprintf("Prompt error: %v", err))
			return
		}

		c.mu.Lock()
		c.prompt.currentCh = updates
		c.mu.Unlock()

		var buf strings.Builder
		c.mu.Lock()
		emitter := c.imBridge
		hasEmitter := emitter != nil
		sid := c.session.id
		c.mu.Unlock()

		sawSandboxRefresh := false
		sawText := false

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
			if hasSandboxRefreshError(u) {
				sawSandboxRefresh = true
			}
			if u.Type == acp.UpdateText && strings.TrimSpace(u.Content) != "" {
				sawText = true
			}
			if u.Err != nil {
				if c.resetDeadConnection(u.Err) {
					// Warm reconnect so the next user message can continue immediately.
					if recErr := c.ensureForwarder(ctx); recErr == nil {
						_ = c.ensureReadyAndNotify(ctx, msg.ChatID)
					}
				}
				c.reply(msg.ChatID, fmt.Sprintf("Agent error: %v", u.Err))
				c.mu.Lock()
				c.prompt.currentCh = nil
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
		c.prompt.currentCh = nil
		c.mu.Unlock()

		// Persist after prompt completes: session ID and config metadata may have changed.
		c.saveSessionState()

		if !hasEmitter && buf.Len() > 0 {
			c.reply(msg.ChatID, buf.String())
		}

		// Auto-degrade: retry once after reconnect when sandbox setup refresh occurs
		// and no meaningful text answer was produced.
		if attempt == 1 && sawSandboxRefresh && !sawText {
			c.reply(msg.ChatID, "Detected sandbox refresh failure, reconnecting and retrying once...")
			c.forceReconnect()
			continue
		}
		return
	}
}

// reply sends a text response to the chat via the IM channel.
func (c *Client) reply(chatID, text string) {
	if c.imBridge != nil {
		if sender, ok := any(c.imBridge).(im.SystemSender); ok {
			_ = sender.SendSystem(chatID, text)
			return
		}
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

// resetDeadConnection clears the current agent connection when it is known-dead.
// Returns true when a reset happened.
func (c *Client) resetDeadConnection(err error) bool {
	if !isAgentExitError(err) {
		return false
	}
	c.mu.Lock()
	old := c.conn
	c.conn = nil
	c.session.ready = false
	c.session.initializing = false
	c.prompt.ctx = nil
	c.prompt.cancel = nil
	c.prompt.updatesCh = nil
	c.prompt.currentCh = nil
	c.prompt.activeTCs = make(map[string]struct{})
	c.mu.Unlock()
	if old != nil {
		_ = old.close()
	}
	return true
}

func (c *Client) forceReconnect() {
	c.mu.Lock()
	old := c.conn
	c.conn = nil
	c.session.ready = false
	c.session.initializing = false
	c.prompt.ctx = nil
	c.prompt.cancel = nil
	c.prompt.updatesCh = nil
	c.prompt.currentCh = nil
	c.prompt.activeTCs = make(map[string]struct{})
	c.mu.Unlock()
	if old != nil {
		_ = old.close()
	}
}

func hasSandboxRefreshError(u acp.Update) bool {
	s := strings.ToLower(u.Content + " " + string(u.Raw))
	return strings.Contains(s, "windows sandbox: spawn setup refresh")
}
