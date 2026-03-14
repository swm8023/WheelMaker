// Package adapter defines the Adapter interface for ACP-compatible CLI backends.
package adapter

import (
	"context"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
)

// Adapter is a stateless connection factory for an ACP-compatible CLI backend.
// Connect() starts a new binary subprocess on each call and returns its Conn.
// The subprocess lifecycle is owned by the returned *acp.Conn; Adapter.Close()
// is only used for cleanup if Connect() fails.
// After a successful Connect(), calling Close() is a no-op.
//
// Connect() calls Conn.Start() internally; callers must NOT call Start() again.
type Adapter interface {
	// Name returns the identifier for this adapter (e.g. "codex").
	Name() string

	// Connect starts a new subprocess and returns an initialized *acp.Conn.
	// The conn is started (subprocess running) when Connect returns.
	Connect(ctx context.Context) (*acp.Conn, error)

	// Close cleans up any resources held by the adapter.
	// No-op after a successful Connect().
	Close() error
}
