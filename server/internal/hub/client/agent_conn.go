package client

import (
	"context"
	"fmt"
	"io"
	"sync"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agent"
)

// ConnMode distinguishes whether an AgentConn is exclusively owned by one
// AgentInstance or shared across multiple instances (multi-session).
type ConnMode int

const (
	// ConnOwned means the connection is exclusively owned by one AgentInstance.
	ConnOwned ConnMode = iota
	// ConnShared means multiple AgentInstances share the same connection.
	// Callbacks are dispatched by acpSessionId.
	ConnShared
)

// AgentConn wraps an agent.Agent subprocess and its ACP Forwarder.
// In owned mode, callbacks are routed directly to the single AgentInstance.
// In shared mode, callbacks are dispatched by acpSessionId to the correct
// AgentInstance via the instances map.
type AgentConn struct {
	agent     agent.Agent
	forwarder *acp.Forwarder
	mode      ConnMode
	debugLog  io.Writer

	// shared mode: dispatch callbacks by acpSessionId.
	mu        sync.RWMutex
	instances map[string]*AgentInstance // acpSessionId -> AgentInstance
}

// newOwnedAgentConn creates an AgentConn in owned mode.
func newOwnedAgentConn(a agent.Agent, conn *acp.Conn, debugLog io.Writer) *AgentConn {
	if debugLog != nil {
		conn.SetDebugLogger(debugLog)
	}
	fwd := acp.NewForwarder(conn, nil)
	return &AgentConn{
		agent:     a,
		forwarder: fwd,
		mode:      ConnOwned,
		debugLog:  debugLog,
		instances: make(map[string]*AgentInstance),
	}
}

// newSharedAgentConn creates an AgentConn in shared mode.
// The AgentConn itself is registered as the callback handler so it can
// dispatch callbacks by acpSessionId.
func newSharedAgentConn(a agent.Agent, conn *acp.Conn, debugLog io.Writer) *AgentConn {
	if debugLog != nil {
		conn.SetDebugLogger(debugLog)
	}
	fwd := acp.NewForwarder(conn, nil)
	ac := &AgentConn{
		agent:     a,
		forwarder: fwd,
		mode:      ConnShared,
		debugLog:  debugLog,
		instances: make(map[string]*AgentInstance),
	}
	// In shared mode, the AgentConn itself handles callback dispatch.
	fwd.SetCallbacks(ac)
	return ac
}

// SetCallbacks wires the callback handler on the underlying forwarder.
// In owned mode this is the AgentInstance; in shared mode this is the AgentConn itself.
func (ac *AgentConn) SetCallbacks(h acp.ClientCallbacks) {
	ac.forwarder.SetCallbacks(h)
}

// Close terminates the forwarder and underlying connection.
func (ac *AgentConn) Close() error {
	if ac.forwarder == nil {
		return nil
	}
	return ac.forwarder.Close()
}

// RegisterInstance adds an AgentInstance to the shared dispatch map.
// Only used in ConnShared mode.
func (ac *AgentConn) RegisterInstance(acpSessionID string, inst *AgentInstance) {
	ac.mu.Lock()
	ac.instances[acpSessionID] = inst
	ac.mu.Unlock()
}

// UnregisterInstance removes an AgentInstance from the shared dispatch map.
func (ac *AgentConn) UnregisterInstance(acpSessionID string) {
	ac.mu.Lock()
	delete(ac.instances, acpSessionID)
	ac.mu.Unlock()
}

// UnregisterAllForInstance removes all acpSessionId entries that point to the
// given AgentInstance. Used during shared-mode Close().
func (ac *AgentConn) UnregisterAllForInstance(inst *AgentInstance) {
	ac.mu.Lock()
	for k, v := range ac.instances {
		if v == inst {
			delete(ac.instances, k)
		}
	}
	ac.mu.Unlock()
}

// lookupInstance finds the AgentInstance for a given acpSessionId (shared mode).
func (ac *AgentConn) lookupInstance(acpSessionID string) *AgentInstance {
	ac.mu.RLock()
	inst := ac.instances[acpSessionID]
	ac.mu.RUnlock()
	return inst
}

// --- Shared mode: AgentConn implements acp.ClientCallbacks for dispatch ---

var _ acp.ClientCallbacks = (*AgentConn)(nil)

func (ac *AgentConn) SessionUpdate(params acp.SessionUpdateParams) {
	inst := ac.lookupInstance(params.SessionID)
	if inst != nil && inst.callbacks != nil {
		inst.callbacks.SessionUpdate(params)
	}
}

func (ac *AgentConn) SessionRequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	inst := ac.lookupInstance(params.SessionID)
	if inst != nil && inst.callbacks != nil {
		return inst.callbacks.SessionRequestPermission(ctx, params)
	}
	return acp.PermissionResult{Outcome: "cancelled"}, nil
}

func (ac *AgentConn) FSRead(params acp.FSReadTextFileParams) (acp.FSReadTextFileResult, error) {
	// FS callbacks don't carry sessionId — in shared mode, route to first instance.
	ac.mu.RLock()
	var inst *AgentInstance
	for _, i := range ac.instances {
		inst = i
		break
	}
	ac.mu.RUnlock()
	if inst != nil && inst.callbacks != nil {
		return inst.callbacks.FSRead(params)
	}
	return acp.FSReadTextFileResult{}, fmt.Errorf("no active instance for FS callback")
}

func (ac *AgentConn) FSWrite(params acp.FSWriteTextFileParams) error {
	ac.mu.RLock()
	var inst *AgentInstance
	for _, i := range ac.instances {
		inst = i
		break
	}
	ac.mu.RUnlock()
	if inst != nil && inst.callbacks != nil {
		return inst.callbacks.FSWrite(params)
	}
	return fmt.Errorf("no active instance for FS callback")
}

func (ac *AgentConn) TerminalCreate(params acp.TerminalCreateParams) (acp.TerminalCreateResult, error) {
	ac.mu.RLock()
	var inst *AgentInstance
	for _, i := range ac.instances {
		inst = i
		break
	}
	ac.mu.RUnlock()
	if inst != nil && inst.callbacks != nil {
		return inst.callbacks.TerminalCreate(params)
	}
	return acp.TerminalCreateResult{}, fmt.Errorf("no active instance for terminal callback")
}

func (ac *AgentConn) TerminalOutput(params acp.TerminalOutputParams) (acp.TerminalOutputResult, error) {
	ac.mu.RLock()
	var inst *AgentInstance
	for _, i := range ac.instances {
		inst = i
		break
	}
	ac.mu.RUnlock()
	if inst != nil && inst.callbacks != nil {
		return inst.callbacks.TerminalOutput(params)
	}
	return acp.TerminalOutputResult{}, fmt.Errorf("no active instance for terminal callback")
}

func (ac *AgentConn) TerminalWaitForExit(params acp.TerminalWaitForExitParams) (acp.TerminalWaitForExitResult, error) {
	ac.mu.RLock()
	var inst *AgentInstance
	for _, i := range ac.instances {
		inst = i
		break
	}
	ac.mu.RUnlock()
	if inst != nil && inst.callbacks != nil {
		return inst.callbacks.TerminalWaitForExit(params)
	}
	return acp.TerminalWaitForExitResult{}, fmt.Errorf("no active instance for terminal callback")
}

func (ac *AgentConn) TerminalKill(params acp.TerminalKillParams) error {
	ac.mu.RLock()
	var inst *AgentInstance
	for _, i := range ac.instances {
		inst = i
		break
	}
	ac.mu.RUnlock()
	if inst != nil && inst.callbacks != nil {
		return inst.callbacks.TerminalKill(params)
	}
	return fmt.Errorf("no active instance for terminal callback")
}

func (ac *AgentConn) TerminalRelease(params acp.TerminalReleaseParams) error {
	ac.mu.RLock()
	var inst *AgentInstance
	for _, i := range ac.instances {
		inst = i
		break
	}
	ac.mu.RUnlock()
	if inst != nil && inst.callbacks != nil {
		return inst.callbacks.TerminalRelease(params)
	}
	return fmt.Errorf("no active instance for terminal callback")
}
