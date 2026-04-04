package client

import (
	"context"
	"errors"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agentv2"
)

// SessionCallbacks is the callback contract that Session provides to agentv2 runtime.
type SessionCallbacks interface {
	agentv2.Callbacks
}

// AgentInstance is the ACP interface visible to Session.
type AgentInstance struct {
	name      string
	runtime   agentv2.Instance
	callbacks SessionCallbacks
	initMeta  clientInitMeta
}

func (ai *AgentInstance) requireRuntime() (agentv2.Instance, error) {
	if ai.runtime == nil {
		return nil, errors.New("agent runtime is nil")
	}
	return ai.runtime, nil
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
	rt, err := ai.requireRuntime()
	if err != nil {
		return acp.InitializeResult{}, err
	}
	return rt.Initialize(ctx, params)
}

// SessionNew creates a new ACP session.
func (ai *AgentInstance) SessionNew(ctx context.Context, params acp.SessionNewParams) (acp.SessionNewResult, error) {
	rt, err := ai.requireRuntime()
	if err != nil {
		return acp.SessionNewResult{}, err
	}
	return rt.SessionNew(ctx, params)
}

// SessionLoad loads an existing ACP session.
func (ai *AgentInstance) SessionLoad(ctx context.Context, params acp.SessionLoadParams) (acp.SessionLoadResult, error) {
	rt, err := ai.requireRuntime()
	if err != nil {
		return acp.SessionLoadResult{}, err
	}
	return rt.SessionLoad(ctx, params)
}

// SessionList lists available ACP sessions.
func (ai *AgentInstance) SessionList(ctx context.Context, params acp.SessionListParams) (acp.SessionListResult, error) {
	rt, err := ai.requireRuntime()
	if err != nil {
		return acp.SessionListResult{}, err
	}
	return rt.SessionList(ctx, params)
}

// SessionPrompt sends a prompt to the ACP session.
func (ai *AgentInstance) SessionPrompt(ctx context.Context, params acp.SessionPromptParams) (acp.SessionPromptResult, error) {
	rt, err := ai.requireRuntime()
	if err != nil {
		return acp.SessionPromptResult{}, err
	}
	return rt.SessionPrompt(ctx, params)
}

// SessionCancel cancels an in-progress prompt.
func (ai *AgentInstance) SessionCancel(sessionID string) error {
	rt, err := ai.requireRuntime()
	if err != nil {
		return err
	}
	return rt.SessionCancel(sessionID)
}

// SessionSetConfigOption sets a config option on the ACP session.
func (ai *AgentInstance) SessionSetConfigOption(ctx context.Context, params acp.SessionSetConfigOptionParams) ([]acp.ConfigOption, error) {
	rt, err := ai.requireRuntime()
	if err != nil {
		return nil, err
	}
	return rt.SessionSetConfigOption(ctx, params)
}

// Close terminates the underlying runtime.
func (ai *AgentInstance) Close() error {
	if ai.runtime == nil {
		return nil
	}
	return ai.runtime.Close()
}

// SetDebugLogger kept for compatibility; runtime logger is configured at conn creation.
func (ai *AgentInstance) SetDebugLogger(_ interface{ Write([]byte) (int, error) }) {}
