package hub

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
// If the file does not exist, a default State is returned.
func (s *JSONStore) Load() (*State, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return defaultState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("store load: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("store unmarshal: %w", err)
	}
	// Fill any nil maps to avoid nil-map panics.
	if state.Agents == nil {
		state.Agents = map[string]AgentConfig{}
	}
	if state.ACPSessionIDs == nil {
		state.ACPSessionIDs = map[string]string{}
	}
	return &state, nil
}

// Save marshals and writes State to disk, creating directories as needed.
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
