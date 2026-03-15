package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/adapter"
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/im"
)

const idleTimeout = 30 * time.Minute

// AdapterFactory creates a new Adapter instance.
// The exePath and env arguments are provided for compatibility; hub-registered
// factories typically ignore them and use closure-captured config instead.
type AdapterFactory func(exePath string, env map[string]string) adapter.Adapter

// Client is the top-level coordinator for a single WheelMaker project.
// It holds a pool of AdapterFactory functions and two references to the active Agent:
//   - session agent.Session  — narrow interface for Prompt/Cancel/SetMode, mockable in tests.
//   - ag      *agent.Agent   — concrete type for Switch (to avoid type assertion on mock).
//
// Agent initialization is lazy: the first incoming message triggers ensureAgent(),
// which connects the active adapter and creates the agent. After 30 minutes of idle
// the agent is disconnected (idleClose) and re-created on the next message.
type Client struct {
	projectName string
	cwd         string

	adapterFacs map[string]AdapterFactory
	session     agent.Session // narrow interface, can be mock in tests
	ag          *agent.Agent  // concrete type, used for Switch only; nil when mock injected

	store Store
	state *State
	imRun im.Adapter // nil when no IM adapter configured

	debugLog io.Writer // optional ACP JSON debug logger; nil = disabled

	idleTimer *time.Timer // fires idleClose after idleTimeout of inactivity

	mu       sync.Mutex
	promptMu sync.Mutex // serializes handlePrompt and switchAdapter

	currentPromptCh <-chan agent.Update // tracked for draining during switchAdapter
}

// New creates a Client for the given project.
//   - store: persistent state store scoped to this project
//   - imAdapter: IM adapter; nil means Run() returns an error (use Hub with a console project)
//   - projectName: identifier used in logs and state keys
//   - cwd: working directory for agent sessions
func New(store Store, imAdapter im.Adapter, projectName string, cwd string) *Client {
	return &Client{
		projectName: projectName,
		cwd:         cwd,
		adapterFacs: make(map[string]AdapterFactory),
		store:       store,
		imRun:       imAdapter,
	}
}

// SetDebugLogger enables ACP JSON debug logging on every subsequent adapter connection.
// Pass nil to disable. The writer is injected into acp.Conn at connect time.
func (c *Client) SetDebugLogger(w io.Writer) {
	c.mu.Lock()
	c.debugLog = w
	c.mu.Unlock()
}

// RegisterAdapter registers an AdapterFactory under the given name.
func (c *Client) RegisterAdapter(name string, factory AdapterFactory) {
	c.mu.Lock()
	c.adapterFacs[name] = factory
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

// Run blocks until ctx is cancelled, delegating to the IM adapter's Run loop.
// Returns an error if no IM adapter is configured.
func (c *Client) Run(ctx context.Context) error {
	if c.imRun != nil {
		return c.imRun.Run(ctx)
	}
	return errors.New("no IM adapter configured; add a console project to config.json")
}

// Close saves state and shuts down the active agent.
// Stops the idle timer to prevent double-close races.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.idleTimer != nil {
		c.idleTimer.Stop()
		c.idleTimer = nil
	}
	ag := c.ag
	c.mu.Unlock()

	if ag != nil {
		c.persistAgentMeta(ag)
		_ = ag.Close()
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
// Known commands (/use, /cancel, /status) are dispatched to handleCommand;
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
	c.handlePrompt(msg, text)
}

// --- internal ---

// parseCommand checks whether text is a recognized WheelMaker command.
// Only exact first-word matches (/use, /cancel, /status) are treated as commands;
// all other "/" lines fall through to the agent (fixing the "code starting with /" bug).
func parseCommand(text string) (cmd, args string, ok bool) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "/use", "/cancel", "/status":
		return parts[0], strings.Join(parts[1:], " "), true
	}
	return
}

// handleCommand processes recognized "/" commands.
func (c *Client) handleCommand(msg im.Message, cmd, args string) {
	switch cmd {
	case "/use":
		if args == "" {
			c.reply(msg.ChatID, "Usage: /use <adapter-name> [--continue]  (e.g. /use codex)")
			return
		}
		parts := strings.Fields(args)
		name := strings.ToLower(parts[0])
		mode := agent.SwitchClean
		for _, p := range parts[1:] {
			if p == "--continue" {
				mode = agent.SwitchWithContext
			}
		}
		if err := c.switchAdapter(context.Background(), msg.ChatID, name, mode); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Switch error: %v", err))
		}

	case "/cancel":
		c.mu.Lock()
		sess := c.session
		c.mu.Unlock()
		if sess == nil {
			c.reply(msg.ChatID, "No active session.")
			return
		}
		if err := sess.Cancel(); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Cancel error: %v", err))
			return
		}
		c.reply(msg.ChatID, "Cancelled.")

	case "/status":
		c.mu.Lock()
		sess := c.session
		c.mu.Unlock()
		if sess == nil {
			c.reply(msg.ChatID, "No active session.")
			return
		}
		status := fmt.Sprintf("Active adapter: %s", sess.AdapterName())
		if sid := sess.SessionID(); sid != "" {
			status += fmt.Sprintf("\nACP session: %s", sid)
		}
		c.reply(msg.ChatID, status)
	}
}

// handlePrompt sends text to the active (or lazily initialized) session and streams the reply.
// promptMu is held for the full duration, serializing with switchAdapter.
func (c *Client) handlePrompt(msg im.Message, text string) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	// Lazily initialize the agent if no session exists yet.
	c.mu.Lock()
	if err := c.ensureAgent(context.Background()); err != nil {
		c.mu.Unlock()
		c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <adapter> to connect.", err))
		return
	}
	sess := c.session
	ag := c.ag // capture early so error paths can persist the session ID
	c.resetIdleTimer() // refresh timeout before sending prompt
	c.mu.Unlock()

	// SD fix: persist session ID immediately after ensureAgent so it survives a mid-prompt crash.
	// This is a lightweight JSON write that covers both the "just created" and "already running" cases.
	c.persistAgentMeta(ag)
	_ = c.store.Save(c.state)

	ctx := context.Background()
	updates, err := sess.Prompt(ctx, text)
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
		if u.Type == agent.UpdateText {
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

	// Persist again after prompt completes: AvailableCommands etc. may have changed.
	c.persistAgentMeta(ag)
	_ = c.store.Save(c.state)

	if buf.Len() > 0 {
		c.reply(msg.ChatID, buf.String())
	}
}

// ensureAgent connects the active adapter and creates the agent if not already running.
// Must be called while holding c.mu.
func (c *Client) ensureAgent(ctx context.Context) error {
	if c.session != nil {
		return nil
	}
	if c.state == nil {
		return errors.New("state not loaded")
	}
	name := c.state.ActiveAdapter
	if name == "" {
		name = "codex"
	}
	fac := c.adapterFacs[name]
	if fac == nil {
		return fmt.Errorf("no adapter registered for %q", name)
	}
	conn, err := fac("", nil).Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if c.debugLog != nil {
		conn.SetDebugLogger(c.debugLog)
	}
	savedSID := ""
	if as := c.state.Agents[name]; as != nil {
		savedSID = as.LastSessionID
	}
	var ag *agent.Agent
	if savedSID != "" {
		ag = agent.NewWithSessionID(name, conn, c.cwd, savedSID)
	} else {
		ag = agent.New(name, conn, c.cwd)
	}
	c.ag = ag
	c.session = ag
	c.resetIdleTimer()
	return nil
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
	if c.session == nil {
		c.mu.Unlock()
		return
	}
	ag := c.ag
	c.mu.Unlock()

	c.persistAgentMeta(ag)

	c.mu.Lock()
	_ = c.store.Save(c.state)
	_ = ag.Close()
	c.session = nil
	c.ag = nil
	c.mu.Unlock()
}

// switchAdapter cancels any in-progress prompt, waits for it to finish via
// promptMu, connects a new adapter binary, and calls ag.Switch() to replace
// the connection. Always uses c.ag (concrete type) for Switch.
//
// Ordering: Cancel() → promptMu.Lock() → drain → ag-refresh → outgoing-snapshot →
// Connect() → ag.Switch() → persist → resetIdleTimer().
func (c *Client) switchAdapter(ctx context.Context, chatID, name string, mode agent.SwitchMode) error {
	c.mu.Lock()
	fac := c.adapterFacs[name]
	sess := c.session
	c.mu.Unlock()

	if fac == nil {
		return fmt.Errorf("unknown adapter: %q (registered: %v)", name, c.registeredAdapterNames())
	}

	// Step 1: signal cancel so any in-progress prompt winds down quickly,
	// then wait for handlePrompt to release promptMu (covering ensureReady).
	if sess != nil {
		_ = sess.Cancel()
	}
	c.promptMu.Lock()
	defer c.promptMu.Unlock()
	// Belt-and-suspenders: drain any channel published between Cancel and promptMu.Lock.
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

	// Re-read c.ag/c.session after acquiring promptMu: a concurrent switch that completed
	// while we were waiting may have installed a new agent (nil-ag creation path).
	c.mu.Lock()
	ag := c.ag
	sess = c.session
	c.mu.Unlock()

	var outgoingName string
	var outgoingSessionID string
	if sess != nil {
		outgoingName = sess.AdapterName()
		outgoingSessionID = sess.SessionID()
	}

	// Step 2: read saved session ID for the incoming adapter and connect.
	c.mu.Lock()
	var savedSID string
	if c.state != nil {
		if as := c.state.Agents[name]; as != nil {
			savedSID = as.LastSessionID
		}
	}
	c.mu.Unlock()

	newConn, err := fac("", nil).Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}

	c.mu.Lock()
	dw := c.debugLog
	c.mu.Unlock()
	if dw != nil {
		newConn.SetDebugLogger(dw)
	}

	// Step 3: replace the connection via the concrete Agent type.
	if ag != nil {
		if err := ag.Switch(ctx, name, newConn, mode, savedSID); err != nil {
			return fmt.Errorf("switch %q: %w", name, err)
		}
	} else {
		// No active agent (lazy init never ran or idleClose fired): create from scratch.
		var newAg *agent.Agent
		if savedSID != "" {
			newAg = agent.NewWithSessionID(name, newConn, c.cwd, savedSID)
		} else {
			newAg = agent.New(name, newConn, c.cwd)
		}
		c.mu.Lock()
		c.ag = newAg
		c.session = newAg
		c.mu.Unlock()
		ag = newAg
	}

	// Persist outgoing session ID before switching active adapter.
	if outgoingName != "" && outgoingSessionID != "" {
		c.mu.Lock()
		if c.state != nil {
			if c.state.Agents == nil {
				c.state.Agents = map[string]*AgentState{}
			}
			if c.state.Agents[outgoingName] == nil {
				c.state.Agents[outgoingName] = &AgentState{}
			}
			c.state.Agents[outgoingName].LastSessionID = outgoingSessionID
		}
		c.mu.Unlock()
	}

	// For SwitchWithContext, ag.Switch bootstrapped a new session; persist its metadata.
	if ag != nil {
		c.persistAgentMeta(ag)
	}

	c.mu.Lock()
	if c.state != nil {
		c.state.ActiveAdapter = name
	}
	c.resetIdleTimer()
	c.mu.Unlock()
	_ = c.store.Save(c.state)

	c.reply(chatID, fmt.Sprintf("Switched to adapter: %s", name))
	return nil
}

// registeredAdapterNames returns all registered adapter names (for error messages).
func (c *Client) registeredAdapterNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.adapterFacs))
	for n := range c.adapterFacs {
		names = append(names, n)
	}
	return names
}

// persistAgentMeta snapshots the current agent metadata and writes it into state.
// Only fields with non-zero values are updated, so stale saved data is not overwritten
// by an uninitialized agent (e.g. immediately after a clean switch before ensureReady).
// Must be called while NOT holding c.mu (it acquires c.mu internally).
func (c *Client) persistAgentMeta(ag *agent.Agent) {
	if ag == nil {
		return
	}
	initMeta, sessMeta := ag.Meta()
	sessionID := ag.SessionID()
	adapterName := ag.AdapterName()
	if adapterName == "" {
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
	as := c.state.Agents[adapterName]
	if as == nil {
		as = &AgentState{}
		c.state.Agents[adapterName] = as
	}
	// Only overwrite LastSessionID when we have a real value; preserve the
	// previously saved ID so session/load can be attempted on the next restart.
	if sessionID != "" {
		as.LastSessionID = sessionID
	}
	// Only overwrite agent-level info when the initialize handshake has completed.
	if initMeta.ProtocolVersion != "" {
		as.ProtocolVersion = initMeta.ProtocolVersion
		as.AgentCapabilities = initMeta.AgentCapabilities
		as.AgentInfo = initMeta.AgentInfo
		as.AuthMethods = initMeta.AuthMethods
	}
	// Update session-level metadata when real data is available.
	if sessMeta.Modes != nil || len(sessMeta.AvailableCommands) > 0 || len(sessMeta.ConfigOptions) > 0 {
		as.Modes = sessMeta.Modes
		as.Models = sessMeta.Models
		as.ConfigOptions = sessMeta.ConfigOptions
		as.AvailableCommands = sessMeta.AvailableCommands
	}
	c.mu.Unlock()
}

// reply sends a text response to the chat via the IM adapter.
func (c *Client) reply(chatID, text string) {
	if c.imRun != nil {
		_ = c.imRun.SendText(chatID, text)
		return
	}
	fmt.Println(text)
}
