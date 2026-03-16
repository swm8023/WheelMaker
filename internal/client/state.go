// Package client provides the top-level coordinator for WheelMaker.
package client

import acp "github.com/swm8023/wheelmaker/internal/agent/provider"

// ConnectionConfig captures what this client declared in the initialize request.
// Persisted for auditability: version mismatches and capability gaps are easier
// to diagnose when the exact params used are recorded alongside the agent response.
type ConnectionConfig struct {
	ProtocolVersion    int                    `json:"protocolVersion"`
	ClientCapabilities acp.ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *acp.AgentInfo         `json:"clientInfo,omitempty"`
}

// SessionState holds session-level metadata populated during session/new or session/load,
// then kept up-to-date by session/update notifications throughout the session lifetime.
// Only the last session per adapter is retained.
type SessionState struct {
	// Modes is from the session/new response or current_mode_update notifications.
	// Deprecated by configOptions but retained for backward compatibility.
	Modes *acp.ModeState `json:"modes,omitempty"`

	// Models is from the session/new response (Zed-specific extension).
	Models *acp.ModelState `json:"models,omitempty"`

	// ConfigOptions is from the session/new response or config_option_update notifications.
	// Always the full list with current values (not incremental patches).
	ConfigOptions []acp.ConfigOption `json:"configOptions,omitempty"`

	// AvailableCommands is from available_commands_update notifications.
	AvailableCommands []acp.AvailableCommand `json:"availableCommands,omitempty"`

	// Title and UpdatedAt are from session_info_update notifications.
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// SessionSummary is a lightweight entry in the per-adapter session list.
// The list is populated lazily (e.g. when the user queries session history)
// and is not automatically maintained on every prompt.
type SessionSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// AgentState holds all persisted metadata for one adapter type.
// Agent-level fields come from the initialize handshake; Session holds only
// the most recently used session's state (not all sessions).
// Sessions is a lazily-populated list of known session summaries per provider.
type AgentState struct {
	// LastSessionID is passed to session/load on the next connection attempt.
	LastSessionID string `json:"lastSessionId,omitempty"`

	// Agent-level data from the initialize response.
	ProtocolVersion   string                `json:"protocolVersion,omitempty"`
	AgentCapabilities acp.AgentCapabilities `json:"agentCapabilities,omitempty"`
	AgentInfo         *acp.AgentInfo        `json:"agentInfo,omitempty"`
	AuthMethods       []acp.AuthMethod      `json:"authMethods,omitempty"`

	// Session holds the most recent session-level metadata (last used session only).
	// Updated on every session/new, session/load, and session/update notification.
	Session *SessionState `json:"session,omitempty"`

	// Sessions is a lightweight list of known sessions for this provider.
	// Populated on demand (e.g. querying session history), not on every prompt.
	Sessions []SessionSummary `json:"sessions,omitempty"`
}

// ProjectState is the persisted state for a single WheelMaker project.
type ProjectState struct {
	// ActiveAdapter is the name of the currently active adapter (e.g. "codex").
	ActiveAdapter string `json:"activeAdapter,omitempty"`

	// Connection captures what this client sent in the last initialize call.
	// Common across all adapters since WheelMaker always declares the same capabilities.
	Connection *ConnectionConfig `json:"connection,omitempty"`

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
		Agents:        map[string]*AgentState{},
	}
}

// defaultState is an alias for defaultProjectState, kept for test compatibility.
func defaultState() *State {
	return defaultProjectState()
}
