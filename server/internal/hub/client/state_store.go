package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store persists and loads a single project's WheelMaker state.
type Store interface {
	Load() (*ProjectState, error)
	Save(s *ProjectState) error
}

// JSONStore persists ProjectState to a local JSON file in the multi-project FileState format.
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
func (s *JSONStore) Load() (*ProjectState, error) {
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

	return defaultProjectState(), nil
}

// Save writes the ProjectState for this store's project into the shared FileState file.
// Other projects in the file are preserved. Always writes the new multi-project format.
func (s *JSONStore) Save(state *ProjectState) error {
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

// SessionStore abstracts persistent session storage (e.g. SQLite).
// A nil SessionStore means in-memory only - sessions are lost on process exit.
type SessionStore interface {
	Save(ctx context.Context, snap *SessionSnapshot) error
	Load(ctx context.Context, sessionID string) (*SessionSnapshot, error)
	List(ctx context.Context) ([]SessionSummaryEntry, error)
	Delete(ctx context.Context, sessionID string) error
	Close() error
}

// SessionSnapshot captures the full state of a Session for persistence.
type SessionSnapshot struct {
	ID           string                        `json:"id"`
	ProjectName  string                        `json:"projectName"`
	Status       SessionStatus                 `json:"status"`
	ActiveAgent  string                        `json:"activeAgent"`
	LastReply    string                        `json:"lastReply"`
	ACPSessionID string                        `json:"acpSessionId"`
	CreatedAt    time.Time                     `json:"createdAt"`
	LastActiveAt time.Time                     `json:"lastActiveAt"`
	Agents       map[string]*SessionAgentState `json:"agents,omitempty"`
	SessionMeta  clientSessionMeta             `json:"sessionMeta"`
	InitMeta     clientInitMeta                `json:"initMeta"`
}

// SessionSummaryEntry is a lightweight listing entry for session browsing.
type SessionSummaryEntry struct {
	ID           string    `json:"id"`
	ActiveAgent  string    `json:"activeAgent"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
}

var errSessionStoreNotConfigured = errors.New("session store not configured")

// ClientStateStore centralizes persistence access for project state and session snapshots.
// Client/Session should mutate in-memory state first, then persist via this boundary.
type ClientStateStore interface {
	LoadProjectState() (*ProjectState, error)
	SaveProjectState(state *ProjectState) error

	LoadSession(ctx context.Context, sessionID string) (*SessionSnapshot, error)
	SaveSession(ctx context.Context, snap *SessionSnapshot) error
	ListSessions(ctx context.Context) ([]SessionSummaryEntry, error)
	DeleteSession(ctx context.Context, sessionID string) error

	SetSessionStore(ss SessionStore)
	SessionStoreEnabled() bool
	Close() error
}

type defaultClientStateStore struct {
	projectStore Store

	mu           sync.RWMutex
	sessionStore SessionStore
}

// NewClientStateStore wires project-level and optional session-level persistence.
func NewClientStateStore(projectStore Store, sessionStore SessionStore) ClientStateStore {
	return &defaultClientStateStore{
		projectStore: projectStore,
		sessionStore: sessionStore,
	}
}

func (s *defaultClientStateStore) LoadProjectState() (*ProjectState, error) {
	return s.projectStore.Load()
}

func (s *defaultClientStateStore) SaveProjectState(state *ProjectState) error {
	return s.projectStore.Save(state)
}

func (s *defaultClientStateStore) SetSessionStore(ss SessionStore) {
	s.mu.Lock()
	s.sessionStore = ss
	s.mu.Unlock()
}

func (s *defaultClientStateStore) SessionStoreEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionStore != nil
}

func (s *defaultClientStateStore) currentSessionStore() SessionStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionStore
}

func (s *defaultClientStateStore) LoadSession(ctx context.Context, sessionID string) (*SessionSnapshot, error) {
	ss := s.currentSessionStore()
	if ss == nil {
		return nil, errSessionStoreNotConfigured
	}
	return ss.Load(ctx, sessionID)
}

func (s *defaultClientStateStore) SaveSession(ctx context.Context, snap *SessionSnapshot) error {
	ss := s.currentSessionStore()
	if ss == nil {
		return errSessionStoreNotConfigured
	}
	return ss.Save(ctx, snap)
}

func (s *defaultClientStateStore) ListSessions(ctx context.Context) ([]SessionSummaryEntry, error) {
	ss := s.currentSessionStore()
	if ss == nil {
		return nil, errSessionStoreNotConfigured
	}
	return ss.List(ctx)
}

func (s *defaultClientStateStore) DeleteSession(ctx context.Context, sessionID string) error {
	ss := s.currentSessionStore()
	if ss == nil {
		return errSessionStoreNotConfigured
	}
	return ss.Delete(ctx, sessionID)
}

func (s *defaultClientStateStore) Close() error {
	ss := s.currentSessionStore()
	if ss == nil {
		return nil
	}
	return ss.Close()
}
