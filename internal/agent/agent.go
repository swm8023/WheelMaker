// Package agent defines the Agent interface for ACP-compatible CLI agents.
// An Agent is a stateless subprocess factory: Connect() starts a new binary
// and returns its acp.Conn. Per-agent protocol hooks (NormalizeParams)
// are also provided through this interface.
package agent

import (
	"context"
	"encoding/json"

	"github.com/swm8023/wheelmaker/internal/acp"
)

// Agent is a stateless connection factory for an ACP-compatible CLI agent.
// Connect() starts a new binary subprocess on each call and returns its acp.Conn.
// The subprocess lifecycle is owned by the returned *acp.Conn; Agent.Close()
// is only used for cleanup if Connect() fails.
// After a successful Connect(), calling Close() is a no-op.
//
// Connect() calls Conn.Start() internally; callers must NOT call Start() again.
type Agent interface {
	// Name returns the identifier for this agent (e.g. "claude").
	Name() string

	// Connect starts a new subprocess and returns an initialized *acp.Conn.
	// The conn is started (subprocess running) when Connect returns.
	Connect(ctx context.Context) (*acp.Conn, error)

	// Close cleans up any resources held by the agent.
	// No-op after a successful Connect().
	Close() error

	// NormalizeParams is called before acp processes each incoming session/update
	// notification. Translate legacy protocol fields to modern format here.
	// Return params unchanged for pass-through (default behaviour).
	NormalizeParams(method string, params json.RawMessage) json.RawMessage
}
