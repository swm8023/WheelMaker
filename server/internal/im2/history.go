package im2

import "context"

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
