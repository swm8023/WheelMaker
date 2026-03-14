package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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

	// Migrate legacy key: "agents[name].exe_path" → Adapters[name].ExePath
	if len(state.Adapters) == 0 {
		if v, ok := raw["agents"]; ok {
			var legacyAgents map[string]struct {
				ExePath string `json:"exe_path"`
			}
			if err := json.Unmarshal(v, &legacyAgents); err == nil && len(legacyAgents) > 0 {
				state.Adapters = make(map[string]AdapterConfig, len(legacyAgents))
				for name, ag := range legacyAgents {
					if ag.ExePath != "" {
						state.Adapters[name] = AdapterConfig{ExePath: ag.ExePath}
					}
				}
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
