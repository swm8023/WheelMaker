package im2

import (
	"context"
	"sync"
	"time"
)

const (
	HistoryInbound  = "inbound"
	HistoryOutbound = "outbound"
)

type HistoryEvent struct {
	SessionID string
	Direction string
	Source    *ChatRef
	Targets   []ChatRef
	Kind      OutboundKind
	Payload   any
	Text      string
	CreatedAt time.Time
}

type HistoryQuery struct {
	Limit int
}

type SessionHistoryStore interface {
	Append(ctx context.Context, event HistoryEvent) error
	List(ctx context.Context, sessionID string, query HistoryQuery) ([]HistoryEvent, error)
}

type NoopHistoryStore struct{}

func (NoopHistoryStore) Append(context.Context, HistoryEvent) error { return nil }

func (NoopHistoryStore) List(context.Context, string, HistoryQuery) ([]HistoryEvent, error) {
	return nil, nil
}

type MemoryHistoryStore struct {
	mu     sync.Mutex
	events []HistoryEvent
}

func NewMemoryHistoryStore() *MemoryHistoryStore {
	return &MemoryHistoryStore{}
}

func (s *MemoryHistoryStore) Append(_ context.Context, event HistoryEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	return nil
}

func (s *MemoryHistoryStore) List(_ context.Context, sessionID string, query HistoryQuery) ([]HistoryEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []HistoryEvent
	for _, event := range s.events {
		if event.SessionID == sessionID {
			out = append(out, event)
		}
	}
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[len(out)-query.Limit:]
	}
	return append([]HistoryEvent(nil), out...), nil
}
