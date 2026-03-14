// Package hub provides the core dispatcher that connects IM adapters to agents.
package hub

// AgentConfig holds configuration for a single agent.
type AgentConfig struct {
	// ExePath is the path to the agent binary.
	// If empty, tools.ResolveBinary will be used to locate it automatically.
	ExePath string `json:"exe_path,omitempty"`

	// Env contains extra environment variables passed to the agent process.
	// Example: {"OPENAI_API_KEY": "sk-..."}
	Env map[string]string `json:"env,omitempty"`
}

// State is the persisted global state of WheelMaker.
type State struct {
	// ActiveAgent is the name of the currently active agent (e.g. "codex").
	ActiveAgent string `json:"active_agent,omitempty"`

	// Agents maps agent names to their configurations.
	Agents map[string]AgentConfig `json:"agents,omitempty"`

	// ACPSessionIDs maps agent names to their last known ACP sessionId.
	// Used to attempt session/load on restart.
	ACPSessionIDs map[string]string `json:"acp_session_ids,omitempty"`
}

// defaultState returns a State with sensible defaults.
func defaultState() *State {
	return &State{
		ActiveAgent:   "codex",
		Agents:        map[string]AgentConfig{},
		ACPSessionIDs: map[string]string{},
	}
}
