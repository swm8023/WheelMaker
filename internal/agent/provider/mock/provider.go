// Package mock implements an in-process ACP mock MockProvider for testing.
package mock

import (
	"context"

	acp "github.com/swm8023/wheelmaker/internal/agent/provider"
)

const providerName = "mock"

// MockProvider is a stateless factory for in-memory mock ACP connections.
type MockProvider struct{}

// NewProvider creates a mock provider.
func NewProvider() *MockProvider {
	return &MockProvider{}
}

// Name returns the MockProvider identifier.
func (a *MockProvider) Name() string { return providerName }

// Connect creates and starts a new in-memory mock ACP connection.
func (a *MockProvider) Connect(_ context.Context) (*acp.Conn, error) {
	conn := acp.NewInMemory(runInMemoryMockServer)
	if err := conn.Start(); err != nil {
		return nil, err
	}
	return conn, nil
}

// Close is a no-op for the stateless mock provider.
func (a *MockProvider) Close() error { return nil }



