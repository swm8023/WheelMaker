// Package client provides the top-level coordinator for WheelMaker.
package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
//   - "active_agent"    → "activeAdapter"   (Load copies old key if new is absent)
//   - "acp_session_ids" → "session_ids"      (Load copies old key if new is absent)
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

// Store persists and loads WheelMaker state.
type Store interface {
	Load() (*State, error)
	Save(s *State) error
}

// JSONStore persists State to a local JSON file.
type JSONStore struct {
	Path string
}

// NewJSONStore creates a JSONStore at the given path.
// The directory is created automatically on first Save.
func NewJSONStore(path string) *JSONStore {
	return &JSONStore{Path: path}
}

// Load reads and unmarshals State from disk.
// If the file does not exist a default State is returned.
// Migrates legacy JSON keys from the old hub.State format.
func (s *JSONStore) Load() (*State, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return defaultState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("store load: %w", err)
	}

	// Parse into a raw map first to detect legacy keys.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("store unmarshal: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("store unmarshal state: %w", err)
	}

	// Migrate legacy key: "active_agent" → ActiveAdapter
	if state.ActiveAdapter == "" {
		if v, ok := raw["active_agent"]; ok {
			var legacy string
			if err := json.Unmarshal(v, &legacy); err == nil && legacy != "" {
				state.ActiveAdapter = legacy
			}
		}
	}

	// Migrate legacy key: "acp_session_ids" → SessionIDs
	if len(state.SessionIDs) == 0 {
		if v, ok := raw["acp_session_ids"]; ok {
			var legacy map[string]string
			if err := json.Unmarshal(v, &legacy); err == nil && len(legacy) > 0 {
				state.SessionIDs = legacy
			}
		}
	}

	// Ensure maps are non-nil.
	if state.Adapters == nil {
		state.Adapters = map[string]AdapterConfig{}
	}
	if state.SessionIDs == nil {
		state.SessionIDs = map[string]string{}
	}

	return &state, nil
}

// Save marshals and writes State to disk, creating directories as needed.
// Only writes new key names (never writes legacy hub.State keys).
func (s *JSONStore) Save(state *State) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("store mkdir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("store marshal: %w", err)
	}
	if err := os.WriteFile(s.Path, data, 0o600); err != nil {
		return fmt.Errorf("store write: %w", err)
	}
	return nil
}
