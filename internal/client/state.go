// Package client provides the top-level coordinator for WheelMaker.
package client

import "github.com/swm8023/wheelmaker/internal/agent/acp"

// AgentSessionState holds per-session metadata captured from session/new and session/update.
type AgentSessionState struct {
	Modes             *acp.ModeState          `json:"modes,omitempty"`
	Models            *acp.ModelState         `json:"models,omitempty"`
	ConfigOptions     []acp.ConfigOption      `json:"configOptions,omitempty"`
	AvailableCommands []acp.AvailableCommand  `json:"availableCommands,omitempty"`
}

// AgentState holds all persisted metadata for one adapter type.
// It stores agent-level info from initialize and per-session info keyed by session ID.
type AgentState struct {
	LastSessionID     string                        `json:"lastSessionId,omitempty"`
	ProtocolVersion   string                        `json:"protocolVersion,omitempty"`
	AgentCapabilities acp.AgentCapabilities         `json:"agentCapabilities,omitempty"`
	AgentInfo         *acp.AgentInfo                `json:"agentInfo,omitempty"`
	AuthMethods       []acp.AuthMethod              `json:"authMethods,omitempty"`
	Sessions          map[string]*AgentSessionState `json:"sessions,omitempty"`
}

// ProjectState is the persisted state for a single WheelMaker project.
// It stores the active adapter name and last-known ACP session IDs.
type ProjectState struct {
	// ActiveAdapter is the name of the currently active adapter (e.g. "codex").
	ActiveAdapter string `json:"activeAdapter,omitempty"`

	// SessionIDs maps adapter names to their last known ACP sessionId.
	// Used to attempt session/load on restart or after idle timeout.
	// Deprecated: new code should use Agents[name].LastSessionID.
	// Kept for backward compatibility with existing state files.
	SessionIDs map[string]string `json:"session_ids,omitempty"`

	// Agents maps adapter names to their persisted metadata.
	Agents map[string]*AgentState `json:"agents,omitempty"`
}

// State is a backward-compatibility alias for ProjectState.
// Existing code and tests can continue to use client.State.
type State = ProjectState

// FileState is the top-level on-disk state format for multi-project setups.
// It maps project names to their ProjectState.
type FileState struct {
	Projects map[string]*ProjectState `json:"projects"`
}

// defaultProjectState returns a ProjectState with sensible defaults.
func defaultProjectState() *ProjectState {
	return &ProjectState{
		ActiveAdapter: "codex",
		SessionIDs:    map[string]string{},
		Agents:        map[string]*AgentState{},
	}
}

// defaultState is an alias for defaultProjectState, kept for test compatibility.
func defaultState() *State {
	return defaultProjectState()
}
