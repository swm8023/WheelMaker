// Package agent defines the Agent interface for ACP-compatible CLI backends.
package agent

import "context"

// Agent is a stateless connection factory for an ACP-compatible CLI backend.
// Connect() starts a new binary subprocess on each call and returns its Conn.
// The subprocess lifecycle is owned by the returned *Conn; Agent.Close()
// is only used for cleanup if Connect() fails.
// After a successful Connect(), calling Close() is a no-op.
//
// Connect() calls Conn.Start() internally; callers must NOT call Start() again.
type Agent interface {
	// Name returns the identifier for this agent (e.g. "claude").
	Name() string

	// Connect starts a new subprocess and returns an initialized *Conn.
	// The conn is started (subprocess running) when Connect returns.
	Connect(ctx context.Context) (*Conn, error)

	// Close cleans up any resources held by the agent.
	// No-op after a successful Connect().
	Close() error
}
