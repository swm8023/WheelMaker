// Package agent implements the ACP protocol layer:
// Session interface (used by client), Agent concrete struct, and SwitchMode.
//
// Relationships:
//   client.Client → agent.Session (narrow interface, mockable)
//   client.Client → *agent.Agent  (concrete type, for Switch calls only)
//   agent.Agent   → agent/acp.Conn (low-level transport, owns subprocess)
//   agent.Agent   → adapter.Adapter (not stored; provided once by client on New/Switch)
package agent

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
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

	// AdapterName returns the name of the current adapter (e.g. "codex").
	AdapterName() string

	// SessionID returns the current ACP session ID for state persistence.
	SessionID() string

	// Close shuts down the agent and its underlying subprocess.
	Close() error
}

// SwitchMode controls how an adapter switch affects session context.
type SwitchMode int

const (
	// SwitchClean discards the current session; new conn is lazily initialized on next Prompt.
	SwitchClean SwitchMode = iota
	// SwitchWithContext passes the last reply as bootstrap context to the new session.
	// Falls back to SwitchClean behavior if lastReply is empty or Prompt fails.
	SwitchWithContext
)

// InitMeta holds agent-level metadata captured from the initialize handshake.
type InitMeta struct {
	ProtocolVersion   string
	AgentCapabilities acp.AgentCapabilities
	AgentInfo         *acp.AgentInfo
	AuthMethods       []acp.AuthMethod
}

// SessionMeta holds session-level metadata captured from session/new.
type SessionMeta struct {
	Modes             *acp.ModeState
	Models            *acp.ModelState
	ConfigOptions     []acp.ConfigOption
	AvailableCommands []acp.AvailableCommand
}

// Agent is the complete ACP protocol encapsulation.
// It owns an active *acp.Conn and handles all outbound ACP calls and inbound callbacks.
// Adapter is not stored here; after Connect(), the Conn owns the subprocess lifecycle.
type Agent struct {
	name       string                // current adapter name (for identification)
	conn       *acp.Conn             // active ACP connection (owns the subprocess)
	caps       acp.AgentCapabilities // capabilities declared during initialize
	sessionID  string                // active ACP session ID
	cwd        string                // working directory for session/new
	mcpServers []acp.MCPServer       // MCP server list for session/new

	initMeta    InitMeta    // metadata from initialize handshake
	sessionMeta SessionMeta // metadata from session/new

	permission PermissionHandler // injectable; defaults to AutoAllowHandler
	terminals  *terminalManager

	lastReply    string // most recent complete agent reply, used for SwitchWithContext
	mu           sync.Mutex
	ready        bool       // true after initialize + session/new or session/load
	initCond     *sync.Cond // guards single-flight initialization (associated with mu)
	initializing bool       // true while one goroutine is running ensureReady I/O

	// FL2: per-prompt context; cancelled by Cancel() to unblock pending permission requests.
	promptCtx    context.Context
	promptCancel context.CancelFunc
}

// New creates an Agent using an already-started *acp.Conn.
// The caller (Client) is responsible for calling adapter.Connect() first.
func New(name string, conn *acp.Conn, cwd string) *Agent {
	ag := &Agent{
		name:       name,
		conn:       conn,
		cwd:        cwd,
		mcpServers: []acp.MCPServer{},
		permission: &AutoAllowHandler{},
		terminals:  newTerminalManager(),
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
func NewWithSessionID(name string, conn *acp.Conn, cwd string, sessionID string) *Agent {
	ag := New(name, conn, cwd)
	ag.sessionID = sessionID
	return ag
}

// --- Session interface implementation ---

// Cancel sends a session/cancel notification to abort the current prompt.
// It is a no-op when the agent is not yet ready (handshake not complete),
// because sessionID seeded by NewWithSessionID must not be used before initialize.
func (a *Agent) Cancel() error {
	a.mu.Lock()
	sessID := a.sessionID
	conn := a.conn
	ready := a.ready
	cancel := a.promptCancel // FL2
	a.mu.Unlock()

	// FL2: cancel per-prompt context to unblock any pending permission request.
	if cancel != nil {
		cancel()
	}

	if sessID == "" || !ready {
		return nil
	}
	return conn.Notify("session/cancel", acp.SessionCancelParams{SessionID: sessID})
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

// AdapterName returns the name of the current adapter.
func (a *Agent) AdapterName() string {
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
		acp.SessionSetConfigOptionParams{
			SessionID: sessID,
			ConfigID:  configID,
			Value:     value,
		}, nil)
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
func (a *Agent) Switch(ctx context.Context, name string, newConn *acp.Conn, mode SwitchMode, savedSessionID string) error {
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
	a.mu.Unlock()

	// Wake up any goroutines waiting in ensureReady so they retry with the new conn.
	a.initCond.Broadcast()

	// Close old conn outside the lock to avoid holding it during slow I/O.
	if oldConn != nil {
		_ = oldConn.Close()
	}

	// For SwitchWithContext, bootstrap the new session with the previous reply.
	// Drain the channel synchronously so that this call blocks until the bootstrap
	// completes. The caller (Client.switchAdapter) holds promptMu for the duration,
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

// setAvailableCommands updates the session metadata with the latest command list.
// Called from the prompt subscription handler when an available_commands_update arrives.
func (a *Agent) setAvailableCommands(cmds []acp.AvailableCommand) {
	a.mu.Lock()
	a.sessionMeta.AvailableCommands = cmds
	a.mu.Unlock()
}

// setConfigOptions updates the session metadata with the latest config option list.
// Called from the prompt subscription handler when a config_option_update arrives.
func (a *Agent) setConfigOptions(opts []acp.ConfigOption) {
	a.mu.Lock()
	a.sessionMeta.ConfigOptions = opts
	a.mu.Unlock()
}

// compile-time check: Agent implements Session.
var _ Session = (*Agent)(nil)
