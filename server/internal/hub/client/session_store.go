package client

import (
	"context"
	"time"
)

// SessionStore abstracts persistent session storage (e.g. SQLite).
// A nil SessionStore means in-memory only — sessions are lost on process exit.
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
