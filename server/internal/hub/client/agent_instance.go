package client

import (
	"context"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
)

// SessionCallbacks is the interface that Session implements to receive
// ACP callbacks dispatched by AgentInstance. It embeds acp.ClientCallbacks.
type SessionCallbacks interface {
	acp.ClientCallbacks
}

// AgentInstance is the ACP interface visible to Session.
// It wraps an AgentConn and exposes typed ACP methods.
// Session never touches AgentConn or acp.Forwarder directly.
type AgentInstance struct {
	name      string
	agentConn *AgentConn
	callbacks SessionCallbacks // owner Session (receives callbacks)
	initMeta  clientInitMeta   // cached initialize result
}

// Name returns the registered agent name.
func (ai *AgentInstance) Name() string { return ai.name }

// Initialize sends the ACP initialize request and caches the result.
func (ai *AgentInstance) Initialize(ctx context.Context, params acp.InitializeParams) (acp.InitializeResult, error) {
	return ai.agentConn.forwarder.Initialize(ctx, params)
}

// SessionNew creates a new ACP session.
func (ai *AgentInstance) SessionNew(ctx context.Context, params acp.SessionNewParams) (acp.SessionNewResult, error) {
	return ai.agentConn.forwarder.SessionNew(ctx, params)
}

// SessionLoad loads an existing ACP session.
func (ai *AgentInstance) SessionLoad(ctx context.Context, params acp.SessionLoadParams) (acp.SessionLoadResult, error) {
	return ai.agentConn.forwarder.SessionLoad(ctx, params)
}

// SessionList lists available ACP sessions.
func (ai *AgentInstance) SessionList(ctx context.Context, params acp.SessionListParams) (acp.SessionListResult, error) {
	return ai.agentConn.forwarder.SessionList(ctx, params)
}

// SessionPrompt sends a prompt to the ACP session.
func (ai *AgentInstance) SessionPrompt(ctx context.Context, params acp.SessionPromptParams) (acp.SessionPromptResult, error) {
	return ai.agentConn.forwarder.SessionPrompt(ctx, params)
}

// SessionCancel cancels an in-progress prompt.
func (ai *AgentInstance) SessionCancel(sessionID string) error {
	return ai.agentConn.forwarder.SessionCancel(sessionID)
}

// SessionSetConfigOption sets a config option on the ACP session.
func (ai *AgentInstance) SessionSetConfigOption(ctx context.Context, params acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
	return ai.agentConn.forwarder.SessionSetConfigOption(ctx, params)
}

// Close terminates the underlying AgentConn.
func (ai *AgentInstance) Close() error {
	return ai.agentConn.Close()
}

// SetDebugLogger sets the debug logger on the underlying conn.
func (ai *AgentInstance) SetDebugLogger(w interface{ Write([]byte) (int, error) }) {
	ai.agentConn.debugLog = w
}
