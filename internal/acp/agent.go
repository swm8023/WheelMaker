// Package acp implements the ACP protocol session layer.
// It defines the Session interface, Agent runtime, and SwitchMode.
//
// Relationships:
//
//	client.Client -> acp.Session (narrow interface, mockable)
//	client.Client -> *acp.Agent  (concrete type, for Switch calls only)
//	acp.Agent     -> *acp.Conn   (low-level transport, owns subprocess)
//	acp.Agent     -> backendHooks (per-backend customization hooks)
package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
)

// backendHooks is the per-backend customization interface for acp.Agent.
// Implementations are provided by the caller (backend.Backend) and injected
// via New / NewWithSessionID / Switch.
type backendHooks interface {

	// HandlePermission responds to session/request_permission callbacks.
	HandlePermission(ctx context.Context, params PermissionRequestParams, mode string) (PermissionResult, error)

	// NormalizeParams is called before acp processes each incoming session/update
	// notification. Translate legacy protocol fields to modern format here.
	// Return params unchanged for pass-through (default behaviour).
	NormalizeParams(method string, params json.RawMessage) json.RawMessage
}

// noopHooks is the default no-op implementation of backendHooks.
// HandlePermission auto-selects allow_once,
// and NormalizeParams passes notifications through unchanged.
type noopHooks struct{}

func (noopHooks) HandlePermission(_ context.Context, params PermissionRequestParams, mode string) (PermissionResult, error) {
	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	preferredKind := "allow_once"
	switch normalizedMode {
	case "reject", "deny", "read":
		preferredKind = "reject_once"
	case "ask", "manual", "user":
		// "ask/manual/user" means require human decision; no synchronous UI path yet.
		return PermissionResult{Outcome: "cancelled"}, nil
	}
	optionID := ""
	for _, opt := range params.Options {
		if opt.Kind == preferredKind {
			optionID = opt.OptionID
			break
		}
	}
	if optionID == "" {
		return PermissionResult{Outcome: "cancelled"}, nil
	}
	return PermissionResult{Outcome: "selected", OptionID: optionID}, nil
}

func (noopHooks) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }

// hooksOrDefault returns h if non-nil, otherwise noopHooks{}.
func hooksOrDefault(h backendHooks) backendHooks {
	if h == nil {
		return noopHooks{}
	}
	return h
}

// Session is the narrow interface used by client.Client for day-to-day operations.
// acp.Agent implements this interface; tests can inject a mock.
type Session interface {
	// Prompt sends a prompt and returns a channel of streaming updates.
	// The caller must drain the channel until an Update with Done=true is received.
	Prompt(ctx context.Context, text string) (<-chan Update, error)

	// Cancel aborts any in-progress prompt.
	Cancel() error

	// SetMode switches the agent's operating mode.
	SetMode(ctx context.Context, modeID string) error

	// BackendName returns the name of the current backend (e.g. "claude").
	BackendName() string

	// AgentName returns the name of the current backend.
	// Deprecated: use BackendName.
	AgentName() string

	// SessionID returns the current ACP session ID for state persistence.
	SessionID() string

	// Close shuts down the agent and its underlying subprocess.
	Close() error
}

// SwitchMode controls how an agent switch affects session context.
type SwitchMode int

const (
	// SwitchClean discards the current session; new conn is lazily initialized on next Prompt.
	SwitchClean SwitchMode = iota
	// SwitchWithContext passes the last reply as bootstrap context to the new session.
	// Falls back to SwitchClean behavior if lastReply is empty or Prompt fails.
	SwitchWithContext
)

// InitMeta holds agent-level metadata from the initialize handshake.
// Both the agent's response fields and the client-side params we sent are stored
// so the client can persist the full connection config.
type InitMeta struct {
	// Agent-side: from initialize response.
	ProtocolVersion   string
	AgentCapabilities AgentCapabilities
	AgentInfo         *AgentInfo
	AuthMethods       []AuthMethod

	// Client-side: what we sent in the initialize request.
	ClientProtocolVersion int
	ClientCapabilities    ClientCapabilities
	ClientInfo            *AgentInfo
}

// SessionMeta holds session-level metadata captured from session/new and
// updated by session/update notifications throughout the session lifetime.
type SessionMeta struct {
	ConfigOptions     []ConfigOption
	AvailableCommands []AvailableCommand
	Title             string
	UpdatedAt         string
}

// Agent is the complete ACP protocol encapsulation.
// It owns an active *Conn and handles all outbound ACP calls and inbound callbacks.
type Agent struct {
	name       string            // current agent name (for identification)
	conn       *Conn             // active ACP connection (owns the subprocess)
	caps       AgentCapabilities // capabilities declared during initialize
	sessionID  string            // active ACP session ID
	cwd        string            // working directory for session/new
	mcpServers []MCPServer       // MCP server list for session/new

	initMeta    InitMeta    // metadata from initialize handshake
	sessionMeta SessionMeta // metadata from session/new

	hooks     backendHooks
	terminals *terminalManager

	lastReply    string   // most recent complete agent reply, used for SwitchWithContext
	loadHistory  []Update // session/update notifications replayed during session/load
	mu           sync.Mutex
	configOptsMu sync.Mutex // serializes config option updates from different sources
	ready        bool       // true after initialize + session/new or session/load
	initCond     *sync.Cond // guards single-flight initialization (associated with mu)
	initializing bool       // true while one goroutine is running ensureReady I/O

	// FL2: per-prompt context; cancelled by Cancel() to unblock pending permission requests.
	promptCtx    context.Context
	promptCancel context.CancelFunc

	// activeToolCalls tracks tool call IDs that are pending or in_progress during a prompt.
	// Populated by the session/update notification handler; cleared when the prompt ends.
	// Used by Cancel() to emit UpdateToolCallCancelled updates for each open tool call.
	activeToolCalls map[string]struct{}
	// promptUpdatesCh is the write end of the active prompt's updates channel.
	// Set by Prompt() and cleared when the prompt goroutine exits.
	promptUpdatesCh chan<- Update
}

// New creates an Agent using an already-started *Conn.
// hooks customizes per-backend behaviour; nil uses noopHooks (auto allow_once).
func New(name string, conn *Conn, cwd string, hooks backendHooks) *Agent {
	ag := &Agent{
		name:            name,
		conn:            conn,
		cwd:             cwd,
		mcpServers:      []MCPServer{},
		hooks:           hooksOrDefault(hooks),
		terminals:       newTerminalManager(),
		activeToolCalls: make(map[string]struct{}),
	}
	ag.initCond = sync.NewCond(&ag.mu)
	return ag
}

// NewWithSessionID creates an Agent with a pre-existing session ID to attempt session/load.
func NewWithSessionID(name string, conn *Conn, cwd string, sessionID string, hooks backendHooks) *Agent {
	ag := New(name, conn, cwd, hooks)
	ag.sessionID = sessionID
	return ag
}

// --- Session interface implementation ---

// Cancel sends a session/cancel notification to abort the current prompt.
// It is a no-op when the agent is not yet ready (handshake not complete),
// because sessionID seeded by NewWithSessionID must not be used before initialize.
//
// Per protocol §7.4, Cancel also emits UpdateToolCallCancelled for each tool call
// that was pending or in_progress at the time of cancellation.
func (a *Agent) Cancel() error {
	a.mu.Lock()
	sessID := a.sessionID
	conn := a.conn
	ready := a.ready
	cancel := a.promptCancel // FL2
	ch := a.promptUpdatesCh
	var cancelIDs []string
	for id := range a.activeToolCalls {
		cancelIDs = append(cancelIDs, id)
	}
	a.mu.Unlock()

	// FL2: cancel per-prompt context to unblock any pending permission request.
	if cancel != nil {
		cancel()
	}

	// Emit cancelled updates for all open tool calls before sending session/cancel.
	// Use recover() to handle the case where the prompt channel is already closed.
	for _, id := range cancelIDs {
		u := Update{Type: UpdateToolCallCancelled, Content: id}
		func() {
			defer func() { recover() }() //nolint:errcheck
			if ch != nil {
				select {
				case ch <- u:
				default:
				}
			}
		}()
	}

	if sessID == "" || !ready {
		return nil
	}
	return conn.Notify("session/cancel", SessionCancelParams{SessionID: sessID})
}

// SetMode sends a session/set_mode request (ACP extension).
func (a *Agent) SetMode(ctx context.Context, modeID string) error {
	if err := a.ensureReady(ctx); err != nil {
		return err
	}
	a.mu.Lock()
	sessID := a.sessionID
	conn := a.conn
	ready := a.ready
	a.mu.Unlock()
	if sessID == "" || !ready {
		return fmt.Errorf("agent: no active session")
	}
	if err := conn.Send(ctx, "session/set_mode",
		map[string]string{"sessionId": sessID, "modeId": modeID}, nil); err != nil {
		return err
	}
	return nil
}

// BackendName returns the name of the current backend.
func (a *Agent) BackendName() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.name
}

// AgentName returns the name of the current backend.
// Deprecated: use BackendName.
func (a *Agent) AgentName() string { return a.BackendName() }

// SessionID returns the current ACP session ID.
func (a *Agent) SessionID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessionID
}

// Close kills all terminals and closes the underlying ACP connection.
func (a *Agent) Close() error {
	a.terminals.KillAll()
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

// --- Extended methods (called by Client via concrete type, not Session interface) ---

// SetConfigOption sends a session/set_config_option request to update a config value.
// This is an extended method; it is not part of the Session interface.
func (a *Agent) SetConfigOption(ctx context.Context, configID, value string) error {
	if err := a.ensureReady(ctx); err != nil {
		return err
	}
	a.mu.Lock()
	sessID := a.sessionID
	conn := a.conn
	ready := a.ready
	a.mu.Unlock()
	if sessID == "" || !ready {
		return fmt.Errorf("agent: no active session")
	}
	var raw json.RawMessage
	if err := conn.Send(ctx, "session/set_config_option",
		SessionSetConfigOptionParams{
			SessionID: sessID,
			ConfigID:  configID,
			Value:     value,
		}, &raw); err != nil {
		return err
	}

	// Backends may return either:
	// 1) []ConfigOption
	// 2) {"configOptions":[...]}
	var opts []ConfigOption
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &opts); err != nil {
			var wrapped struct {
				ConfigOptions []ConfigOption `json:"configOptions"`
			}
			if json.Unmarshal(raw, &wrapped) == nil {
				opts = wrapped.ConfigOptions
			}
		}
	}
	if len(opts) > 0 {
		a.setConfigOptions(opts)
	}
	return nil
}

// Switch replaces the underlying conn with a new one, resetting session state.
//
// savedSessionID, if non-empty, is pre-seeded into the agent so that ensureReady
// will attempt session/load on the new connection (same as NewWithSessionID).
// For SwitchWithContext pass "" — a fresh session is created by the bootstrap prompt.
//
// Concurrency contract: the caller (Client) MUST call Cancel() and drain the
// current prompt channel before calling Switch. Agent does not wait for in-progress
// Prompt goroutines internally (doing so would risk deadlock since Cancel sends
// network messages).
//
// Switch acquires mu to update fields atomically, but closes oldConn outside the
// lock to avoid blocking the lock during potentially slow I/O.
//
// For SwitchWithContext, Switch blocks until the bootstrap prompt completes.
// This preserves the caller's promptMu hold across the full switch + bootstrap,
// preventing any concurrent user prompt from racing with the hidden bootstrap.
func (a *Agent) Switch(ctx context.Context, name string, newConn *Conn, mode SwitchMode, savedSessionID string, hooks backendHooks) error {
	a.mu.Lock()
	var summary string
	if mode == SwitchWithContext && a.lastReply != "" {
		summary = a.lastReply
	}
	// Kill all running terminals under the lock (they belong to the old session).
	a.terminals.KillAll()
	oldConn := a.conn
	a.conn = newConn
	a.name = name
	a.ready = false
	a.initializing = false // reset any in-progress initialization
	a.sessionID = savedSessionID
	a.lastReply = ""
	a.loadHistory = nil
	a.activeToolCalls = make(map[string]struct{})
	a.promptUpdatesCh = nil
	// Reset metadata so the new agent's initialize handshake repopulates state
	// from scratch, preventing stale metadata from the old agent being read back.
	a.initMeta = InitMeta{}
	a.sessionMeta = SessionMeta{}
	a.hooks = hooksOrDefault(hooks)
	a.mu.Unlock()

	// Wake up any goroutines waiting in ensureReady so they retry with the new conn.
	a.initCond.Broadcast()

	// Close old conn outside the lock to avoid holding it during slow I/O.
	if oldConn != nil {
		_ = oldConn.Close()
	}

	// For SwitchWithContext, bootstrap the new session with the previous reply.
	// Drain the channel synchronously so that this call blocks until the bootstrap
	// completes. The caller (Client.switchBackend) holds promptMu for the duration,
	// which prevents any user prompt from running concurrently with the bootstrap.
	if mode == SwitchWithContext && summary != "" {
		ch, err := a.Prompt(ctx, "[context] "+summary)
		if err != nil {
			log.Printf("agent: SwitchWithContext bootstrap prompt failed: %v", err)
		} else {
			for u := range ch {
				if u.Err != nil {
					// Bootstrap Prompt() itself succeeded (ensureReady passed)
					// but the RPC call completed with an error; log and continue draining.
					log.Printf("agent: SwitchWithContext bootstrap prompt failed: %v", u.Err)
				}
			}
		}
	}
	return nil
}

// Meta returns a snapshot of the agent's current init and session metadata.
// Returns zero-value structs if the agent has not completed initialization.
func (a *Agent) Meta() (InitMeta, SessionMeta) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.initMeta, a.sessionMeta
}

// SessionConfigSnapshot returns the current mode/model values and whether the
// agent is ready.
func (a *Agent) SessionConfigSnapshot() (SessionConfigSnapshot, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.ready {
		return SessionConfigSnapshot{}, false
	}
	return sessionConfigSnapshotFromOptions(a.sessionMeta.ConfigOptions), true
}

// EnsureReady initializes the ACP session when needed and returns the current
// mode/model snapshot. initialized is true only when this call observed the
// transition from not-ready to ready.
func (a *Agent) EnsureReady(ctx context.Context) (snap SessionConfigSnapshot, initialized bool, err error) {
	a.mu.Lock()
	wasReady := a.ready
	a.mu.Unlock()

	if err := a.ensureReady(ctx); err != nil {
		return SessionConfigSnapshot{}, false, err
	}
	snap, _ = a.SessionConfigSnapshot()
	return snap, !wasReady, nil
}

// LoadHistory returns the session/update notifications that were replayed during
// session/load. Returns nil when the session was created via session/new (no history).
// Safe to call after the first Prompt() completes initialization.
func (a *Agent) LoadHistory() []Update {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loadHistory
}

// setAvailableCommands updates the session metadata with the latest command list.
// Called from the prompt subscription handler when an available_commands_update arrives.
func (a *Agent) setAvailableCommands(cmds []AvailableCommand) {
	a.mu.Lock()
	a.sessionMeta.AvailableCommands = cmds
	a.mu.Unlock()
}

// setConfigOptions updates the session metadata with the latest config option list.
// Called from the prompt subscription handler when a config_option_update arrives.
func (a *Agent) setConfigOptions(opts []ConfigOption) {
	a.configOptsMu.Lock()
	defer a.configOptsMu.Unlock()
	a.mu.Lock()
	a.sessionMeta.ConfigOptions = opts
	a.mu.Unlock()
}

// setSessionInfo updates the session title and updatedAt in session metadata.
// Called from the prompt subscription handler when a session_info_update arrives.
func (a *Agent) setSessionInfo(title, updatedAt string) {
	a.mu.Lock()
	if title != "" {
		a.sessionMeta.Title = title
	}
	if updatedAt != "" {
		a.sessionMeta.UpdatedAt = updatedAt
	}
	a.mu.Unlock()
}

// compile-time check: Agent implements Session.
var _ Session = (*Agent)(nil)
