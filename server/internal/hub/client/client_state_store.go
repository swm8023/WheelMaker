package client

import (
	"context"
	"errors"
	"sync"
)

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
