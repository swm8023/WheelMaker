// Package client provides the top-level coordinator for WheelMaker.
package client

import (
	"encoding/json"

	acp "github.com/swm8023/wheelmaker/internal/acp"
)

// ConnectionConfig captures what this client declared in the initialize request.
// Persisted for auditability: version mismatches and capability gaps are easier
// to diagnose when the exact params used are recorded alongside the backend response.
type ConnectionConfig struct {
	ProtocolVersion    int                    `json:"protocolVersion"`
	ClientCapabilities acp.ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *acp.AgentInfo         `json:"clientInfo,omitempty"`
}

// SessionState holds session-level metadata populated during session/new or session/load,
// then kept up-to-date by session/update notifications throughout the session lifetime.
// Only the last session per backend is retained.
type SessionState struct {
	// ConfigOptions is the canonical config source from session/new
	// and config_option_update notifications.
	// Always the full list with current values (not incremental patches).
	ConfigOptions []acp.ConfigOption `json:"configOptions,omitempty"`

	// AvailableCommands is from available_commands_update notifications.
	AvailableCommands []acp.AvailableCommand `json:"availableCommands,omitempty"`

	// Title and UpdatedAt are from session_info_update notifications.
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// SessionSummary is a lightweight entry in the per-backend session list.
// The list is populated lazily (e.g. when the user queries session history)
// and is not automatically maintained on every prompt.
type SessionSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// BackendState holds all persisted metadata for one backend type.
// ACP agent-level fields come from the initialize handshake; Session holds only
// the most recently used session's state (not all sessions).
// Sessions is a lazily-populated list of known session summaries per backend.
type BackendState struct {
	// LastSessionID is passed to session/load on the next connection attempt.
	LastSessionID string `json:"lastSessionId,omitempty"`

	// ACP agent-level data from the initialize response.
	ProtocolVersion   string                `json:"protocolVersion,omitempty"`
	AgentCapabilities acp.AgentCapabilities `json:"agentCapabilities,omitempty"`
	AgentInfo         *acp.AgentInfo        `json:"agentInfo,omitempty"`
	AuthMethods       []acp.AuthMethod      `json:"authMethods,omitempty"`

	// Session holds the most recent session-level metadata (last used session only).
	// Updated on every session/new, session/load, and session/update notification.
	Session *SessionState `json:"session,omitempty"`

	// Sessions is a lightweight list of known sessions for this backend.
	// Populated on demand (e.g. querying session history), not on every prompt.
	Sessions []SessionSummary `json:"sessions,omitempty"`
}

// ProjectState is the persisted state for a single WheelMaker project.
type ProjectState struct {
	// ActiveBackend is the name of the currently active backend (e.g. "claude").
	ActiveBackend string `json:"activeBackend,omitempty"`

	// Connection captures what this client sent in the last initialize call.
	// Common across all backends since WheelMaker always declares the same capabilities.
	Connection *ConnectionConfig `json:"connection,omitempty"`

	// Backends maps backend names to their persisted metadata.
	Backends map[string]*BackendState `json:"backends,omitempty"`
}

// UnmarshalJSON supports both current keys (activeBackend/backends) and
// legacy keys (activeAgent/agents) for backward compatibility.
func (s *ProjectState) UnmarshalJSON(data []byte) error {
	type rawProjectState struct {
		ActiveBackend string                   `json:"activeBackend,omitempty"`
		Backends      map[string]*BackendState `json:"backends,omitempty"`
		ActiveAgent   string                   `json:"activeAgent,omitempty"`
		Agents        map[string]*BackendState `json:"agents,omitempty"`
		Connection    *ConnectionConfig        `json:"connection,omitempty"`
	}
	var raw rawProjectState
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	s.ActiveBackend = raw.ActiveBackend
	if s.ActiveBackend == "" {
		s.ActiveBackend = raw.ActiveAgent
	}

	s.Backends = raw.Backends
	if s.Backends == nil {
		s.Backends = raw.Agents
	}
	s.Connection = raw.Connection
	return nil
}

// FileState is the top-level on-disk state format for multi-project setups.
// It maps project names to their ProjectState.
type FileState struct {
	Projects map[string]*ProjectState `json:"projects"`
}

// defaultProjectState returns a ProjectState with sensible defaults.
func defaultProjectState() *ProjectState {
	return &ProjectState{
		ActiveBackend: "claude",
		Backends:      map[string]*BackendState{},
	}
}
