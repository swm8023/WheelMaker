// Package client provides the top-level coordinator for WheelMaker.
package client

// AdapterConfig holds configuration for a single adapter.
type AdapterConfig struct {
	// ExePath is the path to the adapter binary.
	// If empty, tools.ResolveBinary will locate it automatically.
	ExePath string `json:"exePath,omitempty"`

	// Env contains extra environment variables passed to the adapter process.
	Env map[string]string `json:"env,omitempty"`
}

// State is the persisted global state of WheelMaker.
//
// Migration note (backward compatibility with old hub.State JSON keys):
//   - "active_agent"          → "activeAdapter"        (Load copies old key if new is absent)
//   - "acp_session_ids"       → "session_ids"           (Load copies old key if new is absent)
//   - "agents[name].exe_path" → "adapters[name].exePath" (Load migrates old adapter configs)
//
// Save() writes only the new keys.
type State struct {
	// ActiveAdapter is the name of the currently active adapter (e.g. "codex").
	ActiveAdapter string `json:"activeAdapter,omitempty"`

	// Adapters maps adapter names to their configurations.
	Adapters map[string]AdapterConfig `json:"adapters,omitempty"`

	// SessionIDs maps adapter names to their last known ACP sessionId.
	// Used to attempt session/load on restart.
	SessionIDs map[string]string `json:"session_ids,omitempty"`
}

// defaultState returns a State with sensible defaults.
func defaultState() *State {
	return &State{
		ActiveAdapter: "codex",
		Adapters:      map[string]AdapterConfig{},
		SessionIDs:    map[string]string{},
	}
}
