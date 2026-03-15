package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store persists and loads a single project's WheelMaker state.
type Store interface {
	Load() (*State, error)
	Save(s *State) error
}

// JSONStore persists State to a local JSON file in the multi-project FileState format.
// It reads and writes only the entry for its configured projectName, leaving all
// other projects' data untouched.
type JSONStore struct {
	Path        string
	projectName string
}

// NewJSONStore creates a JSONStore for the "default" project at the given path.
// Use NewProjectJSONStore when managing named projects (e.g. from hub config).
func NewJSONStore(path string) *JSONStore {
	return &JSONStore{Path: path, projectName: "default"}
}

// NewProjectJSONStore creates a JSONStore scoped to a specific project name.
func NewProjectJSONStore(path, projectName string) *JSONStore {
	return &JSONStore{Path: path, projectName: projectName}
}

// Load reads the state file and returns the ProjectState for this store's project.
// If the file does not exist, a default ProjectState is returned.
// Migrates legacy flat-format state files (pre-multi-project) into the "default" project.
func (s *JSONStore) Load() (*State, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return defaultProjectState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("store load: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("store unmarshal: %w", err)
	}

	// New multi-project format: {"projects": {...}}
	if rawProjects, ok := raw["projects"]; ok {
		var projects map[string]*ProjectState
		if err := json.Unmarshal(rawProjects, &projects); err != nil {
			return nil, fmt.Errorf("store unmarshal projects: %w", err)
		}
		if ps := projects[s.projectName]; ps != nil {
			ensureStateMaps(ps)
			return ps, nil
		}
		return defaultProjectState(), nil
	}

	// Legacy flat format: migrate to ProjectState.
	// Only the "default" project inherits the migrated state; other names get empty defaults.
	if s.projectName != "default" {
		return defaultProjectState(), nil
	}

	ps := &ProjectState{}

	if v, ok := raw["activeAdapter"]; ok {
		_ = json.Unmarshal(v, &ps.ActiveAdapter)
	}
	if ps.ActiveAdapter == "" {
		if v, ok := raw["active_agent"]; ok {
			_ = json.Unmarshal(v, &ps.ActiveAdapter)
		}
	}

	// Migrate legacy session ID maps into Agents[name].LastSessionID.
	var sessionIDs map[string]string
	if v, ok := raw["session_ids"]; ok {
		_ = json.Unmarshal(v, &sessionIDs)
	}
	if len(sessionIDs) == 0 {
		if v, ok := raw["acp_session_ids"]; ok {
			_ = json.Unmarshal(v, &sessionIDs)
		}
	}

	ensureStateMaps(ps)
	for name, sid := range sessionIDs {
		if sid == "" {
			continue
		}
		if ps.Agents[name] == nil {
			ps.Agents[name] = &AgentState{}
		}
		if ps.Agents[name].LastSessionID == "" {
			ps.Agents[name].LastSessionID = sid
		}
	}

	return ps, nil
}

// Save writes the ProjectState for this store's project into the shared FileState file.
// Other projects in the file are preserved. Always writes the new multi-project format.
func (s *JSONStore) Save(state *State) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("store mkdir: %w", err)
	}

	// Read existing FileState so other projects are not overwritten.
	var projects map[string]*ProjectState
	if data, err := os.ReadFile(s.Path); err == nil {
		var raw map[string]json.RawMessage
		if json.Unmarshal(data, &raw) == nil {
			if rawProjects, ok := raw["projects"]; ok {
				_ = json.Unmarshal(rawProjects, &projects)
			}
		}
	}
	if projects == nil {
		projects = map[string]*ProjectState{}
	}
	projects[s.projectName] = state

	fs := FileState{Projects: projects}
	data, err := json.MarshalIndent(fs, "", "  ")
	if err != nil {
		return fmt.Errorf("store marshal: %w", err)
	}
	if err := os.WriteFile(s.Path, data, 0o600); err != nil {
		return fmt.Errorf("store write: %w", err)
	}
	return nil
}

// ensureStateMaps initialises nil maps in a ProjectState to avoid nil-dereference panics.
func ensureStateMaps(ps *ProjectState) {
	if ps.Agents == nil {
		ps.Agents = map[string]*AgentState{}
	}
}
