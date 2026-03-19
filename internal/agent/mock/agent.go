// Package mock implements an in-process ACP mock agent for testing.
package mock

import (
	"context"
	"encoding/json"

	"github.com/swm8023/wheelmaker/internal/acp"
)

const agentName = "mock"

// Agent is a stateless factory for in-memory mock ACP connections.
type Agent struct{}

// Mock is kept as a compatibility alias.
// Deprecated: use Agent.
type Mock = Agent

// New creates a mock agent.
func New() *Agent {
	return &Agent{}
}

// Name returns the Mock identifier.
func (a *Agent) Name() string { return agentName }

// Connect creates and starts a new in-memory mock ACP connection.
func (a *Agent) Connect(_ context.Context) (*acp.Conn, error) {
	conn := acp.NewInMemoryConn(runInMemoryMockServer)
	if err := conn.Start(); err != nil {
		return nil, err
	}
	return conn, nil
}

// Close is a no-op for the stateless mock agent.
func (a *Agent) Close() error { return nil }

// NormalizeParams passes notifications through unchanged.
func (a *Agent) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }
