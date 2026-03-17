package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/backend"
	"github.com/swm8023/wheelmaker/internal/im"
)

const idleTimeout = 30 * time.Minute

// BackendFactory creates a new backend instance.
// The exePath and env arguments are provided for compatibility; hub-registered
// factories typically ignore them and use closure-captured config instead.
type BackendFactory func(exePath string, env map[string]string) backend.Backend

// Client is the top-level coordinator for a single WheelMaker project.
// It holds a pool of BackendFactory functions and two references to the active backend:
//   - ag      *backend.Backend   ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â concrete type for Switch (to avoid type assertion on mock).
//
// Backend initialization is lazy: the first incoming message triggers ensureBackend(),
// which connects the active backend and creates the ACP session wrapper. After 30 minutes of idle
// the backend is disconnected (idleClose) and re-created on the next message.
type Client struct {
	projectName string
	cwd         string

	backendFacs map[string]BackendFactory
	session     acp.Session // narrow interface, can be mock in tests
	ag          *acp.Agent  // concrete type, used for Switch only; nil when mock injected

	store Store
	state *ProjectState
	imRun im.Provider // nil when no IM provider configured

	debugLog io.Writer // optional ACP JSON debug logger; nil = disabled

	idleTimer *time.Timer // fires idleClose after idleTimeout of inactivity

	mu       sync.Mutex
	promptMu sync.Mutex // serializes handlePrompt and switchBackend

	currentPromptCh <-chan acp.Update // tracked for draining during switchBackend
}

// New creates a Client for the given project.
//   - store: persistent state store scoped to this project
//   - imProvider: IM provider; nil means Run() returns an error (use Hub with a console project)
//   - projectName: identifier used in logs and state keys
//   - cwd: working directory for backend sessions
func New(store Store, imProvider im.Provider, projectName string, cwd string) *Client {
	return &Client{
		projectName: projectName,
		cwd:         cwd,
		backendFacs: make(map[string]BackendFactory),
		store:       store,
		imRun:       imProvider,
	}
}

// SetDebugLogger enables ACP JSON debug logging on every subsequent backend connection.
// Pass nil to disable. The writer is injected into acp.Conn at connect time.
func (c *Client) SetDebugLogger(w io.Writer) {
	c.mu.Lock()
	c.debugLog = w
	c.mu.Unlock()
}

// RegisterBackend registers a BackendFactory under the given name.
func (c *Client) RegisterBackend(name string, factory BackendFactory) {
	c.mu.Lock()
	c.backendFacs[name] = factory
	c.mu.Unlock()
}

// Start loads persisted state and registers the IM message callback.
// Backend initialization is deferred until the first incoming message (lazy init).
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

// Run blocks until ctx is cancelled, delegating to the IM provider's Run loop.
// Returns an error if no IM provider is configured.
func (c *Client) Run(ctx context.Context) error {
	if c.imRun != nil {
		return c.imRun.Run(ctx)
	}
	return errors.New("no IM provider configured; add a console project to config.json")
}

// Close saves state and shuts down the active backend.
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
		c.saveBackendState(ag)
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
// Known commands (/use, /cancel, /status, /mode, /model) are dispatched to handleCommand;
// everything else ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â including lines starting with "/" that are not known commands ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â
// is forwarded to the backend as a prompt.
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
// Only exact first-word matches (/use, /cancel, /status, /mode, /model) are treated as commands;
// all other "/" lines fall through to the backend (fixing the "code starting with /" bug).
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
			c.reply(msg.ChatID, "Usage: /use <backend-name> [--continue]  (e.g. /use claude)")
			return
		}
		parts := strings.Fields(args)
		name := strings.ToLower(parts[0])
		mode := acp.SwitchClean
		for _, p := range parts[1:] {
			if p == "--continue" {
				mode = acp.SwitchWithContext
			}
		}
		if err := c.switchBackend(context.Background(), msg.ChatID, name, mode); err != nil {
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
		status := fmt.Sprintf("Active backend: %s", sess.BackendName())
		if sid := sess.SessionID(); sid != "" {
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
		if err := c.ensureBackend(context.Background()); err != nil {
			c.mu.Unlock()
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <backend> to connect.", err))
			return
		}
		sess := c.session
		ag := c.ag
		backendName := sess.BackendName()
		var sessionState *SessionState
		if c.state != nil && c.state.Backends != nil {
			if as := c.state.Backends[backendName]; as != nil && as.Session != nil {
				sessionState = as.Session
			}
		}
		c.resetIdleTimer()
		c.mu.Unlock()

		if ag == nil {
			c.reply(msg.ChatID, "Mode error: no concrete backend instance available")
			return
		}
		if err := c.ensureReadyAndNotify(context.Background(), msg.ChatID, ag); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Mode error: %v", err))
			return
		}
		modeConfigID, modeValue, err := resolveModeArg(strings.TrimSpace(args), sessionState)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Mode error: %v", err))
			return
		}
		if err := ag.SetConfigOption(context.Background(), modeConfigID, modeValue); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Mode error: %v", err))
			return
		}
		c.saveBackendState(ag)
		c.reply(msg.ChatID, fmt.Sprintf("Mode set to: %s", modeValue))

	case "/model":
		if strings.TrimSpace(args) == "" {
			c.reply(msg.ChatID, "Usage: /model <model-id-or-name>")
			return
		}
		c.promptMu.Lock()
		defer c.promptMu.Unlock()

		c.mu.Lock()
		if err := c.ensureBackend(context.Background()); err != nil {
			c.mu.Unlock()
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <backend> to connect.", err))
			return
		}
		ag := c.ag
		backendName := ""
		if c.session != nil {
			backendName = c.session.BackendName()
		}
		var sessionState *SessionState
		if c.state != nil && c.state.Backends != nil {
			if as := c.state.Backends[backendName]; as != nil {
				sessionState = as.Session
			}
		}
		c.resetIdleTimer()
		c.mu.Unlock()

		if ag == nil {
			c.reply(msg.ChatID, "Model error: no concrete backend instance available")
			return
		}
		if err := c.ensureReadyAndNotify(context.Background(), msg.ChatID, ag); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Model error: %v", err))
			return
		}
		configID, modelValue, err := resolveModelArg(strings.TrimSpace(args), sessionState)
		if err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Model error: %v", err))
			return
		}
		if err := ag.SetConfigOption(context.Background(), configID, modelValue); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("Model error: %v", err))
			return
		}
		c.saveBackendState(ag)
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
// promptMu is held for the full duration, serializing with switchBackend.
func (c *Client) handlePrompt(msg im.Message, text string) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	// Lazily initialize the backend if no session exists yet.
	c.mu.Lock()
	if err := c.ensureBackend(context.Background()); err != nil {
		c.mu.Unlock()
		c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <backend> to connect.", err))
		return
	}
	sess := c.session
	ag := c.ag         // capture early so error paths can persist the session ID
	c.resetIdleTimer() // refresh timeout before sending prompt
	c.mu.Unlock()

	ctx := context.Background()
	if ag != nil {
		if err := c.ensureReadyAndNotify(ctx, msg.ChatID, ag); err != nil {
			c.reply(msg.ChatID, fmt.Sprintf("No active session: %v. Use /use <backend> to connect.", err))
			return
		}
	}
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
			c.reply(msg.ChatID, fmt.Sprintf("Backend error: %v", u.Err))
			c.mu.Lock()
			c.currentPromptCh = nil
			c.mu.Unlock()
			return
		}
		if u.Type == acp.UpdateConfigOption {
			c.reply(msg.ChatID, formatConfigOptionUpdateMessage(u.Raw))
			c.saveBackendState(ag) // persist immediately; don't wait for prompt to finish
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
	c.saveBackendState(ag)

	if buf.Len() > 0 {
		c.reply(msg.ChatID, buf.String())
	}
}

// ensureBackend connects the active backend and creates the ACP session wrapper if not already running.
// Must be called while holding c.mu.
func (c *Client) ensureBackend(ctx context.Context) error {
	if c.session != nil {
		return nil
	}
	if c.state == nil {
		return errors.New("state not loaded")
	}
	name := c.state.ActiveBackend
	if name == "" {
		name = "claude"
	}
	fac := c.backendFacs[name]
	if fac == nil {
		return fmt.Errorf("no backend registered for %q", name)
	}
	backend := fac("", nil)
	conn, err := backend.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}
	if c.debugLog != nil {
		conn.SetDebugLogger(c.debugLog)
	}
	savedSID := ""
	if as := c.state.Backends[name]; as != nil {
		savedSID = as.LastSessionID
	}
	var ag *acp.Agent
	if savedSID != "" {
		ag = acp.NewWithSessionID(name, conn, c.cwd, savedSID, backend)
	} else {
		ag = acp.New(name, conn, c.cwd, backend)
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
// the backend subprocess.
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

	c.saveBackendState(ag) // persist + save outside lock (file I/O should not hold c.mu)

	c.mu.Lock()
	_ = ag.Close()
	c.session = nil
	c.ag = nil
	c.mu.Unlock()
}

// switchBackend cancels any in-progress prompt, waits for it to finish via
// promptMu, connects a new backend binary, and calls ag.Switch() to replace
// the connection. Always uses c.ag (concrete type) for Switch.
//
// Ordering: Cancel() ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢ promptMu.Lock() ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢ drain ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢ ag-refresh ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢ outgoing-snapshot ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢
// Connect() ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢ ag.Switch() ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢ persist ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢ resetIdleTimer().
func (c *Client) switchBackend(ctx context.Context, chatID, name string, mode acp.SwitchMode) error {
	c.mu.Lock()
	fac := c.backendFacs[name]
	sess := c.session
	c.mu.Unlock()

	if fac == nil {
		return fmt.Errorf("unknown backend: %q (registered: %v)", name, c.registeredBackendNames())
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
	// while we were waiting may have installed a new backend (nil-ag creation path).
	c.mu.Lock()
	ag := c.ag
	sess = c.session
	c.mu.Unlock()

	// Outgoing backend state is captured in step 3 below via persistBackendMeta(ag),
	// before ag.Switch() resets initMeta/sessionMeta to zero.

	// Step 2: read saved session ID for the incoming backend and connect.
	c.mu.Lock()
	var savedSID string
	if c.state != nil {
		if as := c.state.Backends[name]; as != nil {
			savedSID = as.LastSessionID
		}
	}
	c.mu.Unlock()

	newBackend := fac("", nil)
	newConn, err := newBackend.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect %q: %w", name, err)
	}

	c.mu.Lock()
	dw := c.debugLog
	c.mu.Unlock()
	if dw != nil {
		newConn.SetDebugLogger(dw)
	}

	// Step 3: persist outgoing backend's full state NOW, while ag still holds its
	// name/initMeta/sessionMeta. After ag.Switch() those fields are reset to zero.
	// This is also where the outgoing LastSessionID is captured.
	if ag != nil {
		c.persistBackendMeta(ag)
	}

	// Step 4: replace the connection via the concrete Agent type.
	if ag != nil {
		if err := ag.Switch(ctx, name, newConn, mode, savedSID, newBackend); err != nil {
			return fmt.Errorf("switch %q: %w", name, err)
		}
		// After Switch(), ag.name == name and initMeta/sessionMeta are reset.
		// For SwitchWithContext the bootstrap prompt ran; capture the new session data.
		c.persistBackendMeta(ag)
	} else {
		// No active backend (lazy init never ran or idleClose fired): create from scratch.
		var newAg *acp.Agent
		if savedSID != "" {
			newAg = acp.NewWithSessionID(name, newConn, c.cwd, savedSID, newBackend)
		} else {
			newAg = acp.New(name, newConn, c.cwd, newBackend)
		}
		c.mu.Lock()
		c.ag = newAg
		c.session = newAg
		c.mu.Unlock()
	}

	// Update ActiveBackend and trigger a single save for all accumulated mutations.
	c.mu.Lock()
	if c.state != nil {
		c.state.ActiveBackend = name
	}
	c.resetIdleTimer()
	s := c.state
	c.mu.Unlock()
	if s != nil {
		_ = c.store.Save(s)
	}

	c.reply(chatID, fmt.Sprintf("Switched to backend: %s", name))
	if ag != nil {
		if snap, ok := ag.SessionConfigSnapshot(); ok {
			c.reply(chatID, fmt.Sprintf("Session ready: mode=%s model=%s", renderUnknown(snap.Mode), renderUnknown(snap.Model)))
		}
	}
	return nil
}

// registeredBackendNames returns all registered backend names (for error messages).
func (c *Client) registeredBackendNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.backendFacs))
	for n := range c.backendFacs {
		names = append(names, n)
	}
	return names
}

// saveBackendState updates in-memory state via persistBackendMeta and writes to disk
// only if anything actually changed. Call this whenever backend state may have changed.
func (c *Client) saveBackendState(ag *acp.Agent) {
	if !c.persistBackendMeta(ag) {
		return
	}
	c.mu.Lock()
	s := c.state
	c.mu.Unlock()
	if s != nil {
		_ = c.store.Save(s)
	}
}

// persistBackendMeta snapshots current backend metadata into in-memory state.
// Returns true if anything changed (caller should then call store.Save).
// Only fields with non-zero values are written, so an uninitialized backend
// (e.g. right after a clean switch before ensureReady runs) never overwrites
// previously saved data.
// Must be called while NOT holding c.mu.
func (c *Client) persistBackendMeta(ag *acp.Agent) bool {
	if ag == nil {
		return false
	}
	initMeta, sessMeta := ag.Meta()
	sessionID := ag.SessionID()
	backendName := ag.BackendName()
	if backendName == "" {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == nil {
		return false
	}
	if c.state.Backends == nil {
		c.state.Backends = map[string]*BackendState{}
	}
	as := c.state.Backends[backendName]
	if as == nil {
		as = &BackendState{}
		c.state.Backends[backendName] = as
	}

	changed := false

	// Session ID: only update when a real value is available.
	if sessionID != "" && as.LastSessionID != sessionID {
		as.LastSessionID = sessionID
		changed = true
	}

	// Agent-level data: only available after initialize handshake completes.
	if initMeta.ProtocolVersion != "" {
		as.ProtocolVersion = initMeta.ProtocolVersion
		as.AgentCapabilities = initMeta.AgentCapabilities
		as.AgentInfo = initMeta.AgentInfo
		as.AuthMethods = initMeta.AuthMethods
		// Client-side connection params are the same for all Backends.
		if c.state.Connection == nil {
			c.state.Connection = &ConnectionConfig{}
		}
		c.state.Connection.ProtocolVersion = initMeta.ClientProtocolVersion
		c.state.Connection.ClientCapabilities = initMeta.ClientCapabilities
		c.state.Connection.ClientInfo = initMeta.ClientInfo
		changed = true
	}

	// Session-level data: only available after session/new or session/load.
	hasSessionData := len(sessMeta.AvailableCommands) > 0 || len(sessMeta.ConfigOptions) > 0 ||
		sessMeta.Title != "" || sessMeta.UpdatedAt != ""
	if hasSessionData {
		if as.Session == nil {
			as.Session = &SessionState{}
		}
		as.Session.ConfigOptions = sessMeta.ConfigOptions
		as.Session.AvailableCommands = sessMeta.AvailableCommands
		if sessMeta.Title != "" {
			as.Session.Title = sessMeta.Title
		}
		if sessMeta.UpdatedAt != "" {
			as.Session.UpdatedAt = sessMeta.UpdatedAt
		}
		changed = true
	}

	return changed
}

// reply sends a text response to the chat via the IM provider.
func (c *Client) reply(chatID, text string) {
	if c.imRun != nil {
		_ = c.imRun.SendText(chatID, text)
		return
	}
	fmt.Println(text)
}

func (c *Client) ensureReadyAndNotify(ctx context.Context, chatID string, ag *acp.Agent) error {
	snap, initialized, err := ag.EnsureReady(ctx)
	if err != nil {
		return err
	}
	if initialized {
		c.reply(chatID, fmt.Sprintf("Session ready: mode=%s model=%s", renderUnknown(snap.Mode), renderUnknown(snap.Model)))
		c.saveBackendState(ag)
	}
	return nil
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
	var p acp.SessionUpdateParams
	if err := json.Unmarshal(raw, &p); err != nil || len(p.Update.ConfigOptions) == 0 {
		return "Config options updated."
	}
	mode := ""
	model := ""
	for _, opt := range p.Update.ConfigOptions {
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
