// Package acp implements the ACP protocol session layer.
// It defines the Session interface, Agent runtime, and SwitchMode.
//
// Relationships:
//
//	client.Client -> agent.Session (narrow interface, mockable)
//	client.Client -> *agent.Agent (concrete type, for Switch calls only)
//	agent.Agent   -> agent/agent.Conn (low-level transport, owns subprocess)
//	agent.Agent   -> agent.Agent (not stored; provided once by client on New/Switch)
package acp

import (
	"context"
	"fmt"
	"log"
	"sync"

	agent "github.com/swm8023/wheelmaker/internal/agent"
)

// Session is the narrow interface used by client.Client for day-to-day operations.
// agent.Agent implements this interface; tests can inject a mock.
type Session interface {
	// Prompt sends a prompt and returns a channel of streaming updates.
	// The caller must drain the channel until an Update with Done=true is received.
	Prompt(ctx context.Context, text string) (<-chan Update, error)

	// Cancel aborts any in-progress prompt.
	Cancel() error

	// SetMode switches the agent's operating mode.
	SetMode(ctx context.Context, modeID string) error

	// AgentName returns the name of the current agent (e.g. "claude").
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
	Modes             *ModeState
	Models            *ModelState
	ConfigOptions     []ConfigOption
	AvailableCommands []AvailableCommand
	Title             string
	UpdatedAt         string
}

// Agent is the complete ACP protocol encapsulation.
// It owns an active *agent.Conn and handles all outbound ACP calls and inbound callbacks.
// The factory agent is not stored here; after Connect(), the Conn owns the subprocess lifecycle.
type Agent struct {
	name       string            // current agent name (for identification)
	conn       *agent.Conn       // active ACP connection (owns the subprocess)
	caps       AgentCapabilities // capabilities declared during initialize
	sessionID  string            // active ACP session ID
	cwd        string            // working directory for session/new
	mcpServers []MCPServer       // MCP server list for session/new

	initMeta    InitMeta    // metadata from initialize handshake
	sessionMeta SessionMeta // metadata from session/new

	permission PermissionHandler // injectable; defaults to AutoAllowHandler
	terminals  *terminalManager

	lastReply    string // most recent complete agent reply, used for SwitchWithContext
	loadHistory  []Update   // session/update notifications replayed during session/load
	mu           sync.Mutex
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

// New creates an Agent using an already-started *agent.Conn.
// The caller (Client) is responsible for calling agent.Connect() first.
func New(name string, conn *agent.Conn, cwd string) *Agent {
	ag := &Agent{
		name:            name,
		conn:            conn,
		cwd:             cwd,
		mcpServers:      []MCPServer{},
		permission:      &AutoAllowHandler{},
		terminals:       newTerminalManager(),
		activeToolCalls: make(map[string]struct{}),
	}
	ag.initCond = sync.NewCond(&ag.mu)
	return ag
}

// SetPermissionHandler replaces the permission handler (thread-safe).
func (a *Agent) SetPermissionHandler(h PermissionHandler) {
	a.mu.Lock()
	a.permission = h
	a.mu.Unlock()
}

// NewWithSessionID creates an Agent with a pre-existing session ID to attempt session/load.
func NewWithSessionID(name string, conn *agent.Conn, cwd string, sessionID string) *Agent {
	ag := New(name, conn, cwd)
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
	a.mu.Lock()
	sessID := a.sessionID
	conn := a.conn
	ready := a.ready
	a.mu.Unlock()
	if sessID == "" || !ready {
		return fmt.Errorf("agent: no active session")
	}
	return conn.Send(ctx, "session/set_mode",
		map[string]string{"sessionId": sessID, "modeId": modeID}, nil)
}

// AgentName returns the name of the current agent.
func (a *Agent) AgentName() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.name
}

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
	a.mu.Lock()
	sessID := a.sessionID
	conn := a.conn
	ready := a.ready
	a.mu.Unlock()
	if sessID == "" || !ready {
		return fmt.Errorf("agent: no active session")
	}
	return conn.Send(ctx, "session/set_config_option",
		SessionSetConfigOptionParams{
			SessionID: sessID,
			ConfigID:  configID,
			Value:     value,
		}, nil)
}

// Switch replaces the underlying conn with a new one, resetting session state.
//
// savedSessionID, if non-empty, is pre-seeded into the agent so that ensureReady
// will attempt session/load on the new connection (same as NewWithSessionID).
// For SwitchWithContext pass "" ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â a fresh session is created by the bootstrap prompt.
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
func (a *Agent) Switch(ctx context.Context, name string, newConn *agent.Conn, mode SwitchMode, savedSessionID string) error {
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
	a.mu.Unlock()

	// Wake up any goroutines waiting in ensureReady so they retry with the new conn.
	a.initCond.Broadcast()

	// Close old conn outside the lock to avoid holding it during slow I/O.
	if oldConn != nil {
		_ = oldConn.Close()
	}

	// For SwitchWithContext, bootstrap the new session with the previous reply.
	// Drain the channel synchronously so that this call blocks until the bootstrap
	// completes. The caller (Client.switchAgent) holds promptMu for the duration,
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
	a.mu.Lock()
	a.sessionMeta.ConfigOptions = opts
	a.mu.Unlock()
}

// setCurrentMode updates the current mode ID in session metadata.
// Called from the prompt subscription handler when a current_mode_update arrives.
func (a *Agent) setCurrentMode(modeID string) {
	a.mu.Lock()
	if a.sessionMeta.Modes == nil {
		a.sessionMeta.Modes = &ModeState{}
	}
	a.sessionMeta.Modes.CurrentModeID = modeID
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
