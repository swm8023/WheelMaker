// Package mock implements an in-process ACP mock adapter for testing.
package mock

import (
	"context"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
)

const adapterName = "mock"

// Adapter is a stateless factory for in-memory mock ACP connections.
type Adapter struct{}

// NewAdapter creates a mock provider.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Name returns the adapter identifier.
func (a *Adapter) Name() string { return adapterName }

// Connect creates and starts a new in-memory mock ACP connection.
func (a *Adapter) Connect(_ context.Context) (*acp.Conn, error) {
	conn := acp.NewInMemoryMock()
	if err := conn.Start(); err != nil {
		return nil, err
	}
	return conn, nil
}

// Close is a no-op for the stateless mock provider.
func (a *Adapter) Close() error { return nil }
