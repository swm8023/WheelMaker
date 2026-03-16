// Package mock implements an in-process ACP mock MockAgent for testing.
package mock

import (
	"context"

	"github.com/swm8023/wheelmaker/internal/agent"
)

const agentName = "mock"

// MockAgent is a stateless factory for in-memory mock ACP connections.
type MockAgent struct{}

// NewAgent creates a mock agent.
func NewAgent() *MockAgent {
	return &MockAgent{}
}

// Name returns the MockAgent identifier.
func (a *MockAgent) Name() string { return agentName }

// Connect creates and starts a new in-memory mock ACP connection.
func (a *MockAgent) Connect(_ context.Context) (*agent.Conn, error) {
	conn := agent.NewInMemory(runInMemoryMockServer)
	if err := conn.Start(); err != nil {
		return nil, err
	}
	return conn, nil
}

// Close is a no-op for the stateless mock agent.
func (a *MockAgent) Close() error { return nil }
