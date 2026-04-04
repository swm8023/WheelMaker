package client

import (
	"context"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agentv2"
)

// SessionCallbacks is the callback contract that Session provides.
type SessionCallbacks interface {
	acp.ClientCallbacks
}

// AgentInstance is the ACP interface visible to Session.
// During migration, it can wrap either legacy AgentConn+Forwarder or agentv2.Instance runtime.
type AgentInstance struct {
	name    string
	runtime agentv2.Instance

	// legacy fields kept for transition compatibility
	agentConn *AgentConn
	callbacks SessionCallbacks
	initMeta  clientInitMeta
	shared    bool
}

// Name returns the registered agent name.
func (ai *AgentInstance) Name() string {
	if ai.runtime != nil {
		return ai.runtime.Name()
	}
	return ai.name
}

// Initialize sends the ACP initialize request and caches the result.
func (ai *AgentInstance) Initialize(ctx context.Context, params acp.InitializeParams) (acp.InitializeResult, error) {
	if ai.runtime != nil {
		return ai.runtime.Initialize(ctx, params)
	}
	return ai.agentConn.forwarder.Initialize(ctx, params)
}

// SessionNew creates a new ACP session.
func (ai *AgentInstance) SessionNew(ctx context.Context, params acp.SessionNewParams) (acp.SessionNewResult, error) {
	if ai.runtime != nil {
		return ai.runtime.SessionNew(ctx, params)
	}
	res, err := ai.agentConn.forwarder.SessionNew(ctx, params)
	if err == nil && ai.shared && res.SessionID != "" {
		ai.agentConn.RegisterInstance(res.SessionID, ai)
	}
	return res, err
}

// SessionLoad loads an existing ACP session.
func (ai *AgentInstance) SessionLoad(ctx context.Context, params acp.SessionLoadParams) (acp.SessionLoadResult, error) {
	if ai.runtime != nil {
		return ai.runtime.SessionLoad(ctx, params)
	}
	if ai.shared && params.SessionID != "" {
		ai.agentConn.RegisterInstance(params.SessionID, ai)
	}
	res, err := ai.agentConn.forwarder.SessionLoad(ctx, params)
	if err != nil && ai.shared && params.SessionID != "" {
		ai.agentConn.UnregisterInstance(params.SessionID)
	}
	return res, err
}

// SessionList lists available ACP sessions.
func (ai *AgentInstance) SessionList(ctx context.Context, params acp.SessionListParams) (acp.SessionListResult, error) {
	if ai.runtime != nil {
		return ai.runtime.SessionList(ctx, params)
	}
	return ai.agentConn.forwarder.SessionList(ctx, params)
}

// SessionPrompt sends a prompt to the ACP session.
func (ai *AgentInstance) SessionPrompt(ctx context.Context, params acp.SessionPromptParams) (acp.SessionPromptResult, error) {
	if ai.runtime != nil {
		return ai.runtime.SessionPrompt(ctx, params)
	}
	return ai.agentConn.forwarder.SessionPrompt(ctx, params)
}

// SessionCancel cancels an in-progress prompt.
func (ai *AgentInstance) SessionCancel(sessionID string) error {
	if ai.runtime != nil {
		return ai.runtime.SessionCancel(sessionID)
	}
	return ai.agentConn.forwarder.SessionCancel(sessionID)
}

// SessionSetConfigOption sets a config option on the ACP session.
func (ai *AgentInstance) SessionSetConfigOption(ctx context.Context, params acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
	if ai.runtime != nil {
		return ai.runtime.SessionSetConfigOption(ctx, params)
	}
	return ai.agentConn.forwarder.SessionSetConfigOption(ctx, params)
}

// Close terminates the underlying connection runtime.
func (ai *AgentInstance) Close() error {
	if ai.runtime != nil {
		return ai.runtime.Close()
	}
	if ai.shared {
		ai.agentConn.UnregisterAllForInstance(ai)
		return nil
	}
	return ai.agentConn.Close()
}

// SetDebugLogger sets debug logger on legacy conn path.
func (ai *AgentInstance) SetDebugLogger(w interface{ Write([]byte) (int, error) }) {
	if ai.runtime != nil {
		return
	}
	ai.agentConn.debugLog = w
}
