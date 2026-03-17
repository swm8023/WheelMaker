// Package mock implements an in-process ACP mock Backend for testing.
package mock

import (
	"context"
	"encoding/json"

	"github.com/swm8023/wheelmaker/internal/acp"
)

const backendName = "mock"

// Backend is a stateless factory for in-memory mock ACP connections.
type Backend struct{}

// Mock is kept as a compatibility alias.
// Deprecated: use Backend.
type Mock = Backend

// New creates a mock backend.
func New() *Backend {
	return &Backend{}
}

// Name returns the Mock identifier.
func (a *Backend) Name() string { return backendName }

// Connect creates and starts a new in-memory mock ACP connection.
func (a *Backend) Connect(_ context.Context) (*acp.Conn, error) {
	conn := acp.NewInMemoryConn(runInMemoryMockServer)
	if err := conn.Start(); err != nil {
		return nil, err
	}
	return conn, nil
}

// Close is a no-op for the stateless mock backend.
func (a *Backend) Close() error { return nil }

// HandlePermission auto-selects allow_once.
func (a *Backend) HandlePermission(_ context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	optionID := ""
	for _, opt := range params.Options {
		if opt.Kind == "allow_once" {
			optionID = opt.OptionID
			break
		}
	}
	if optionID == "" {
		// No allow_once option means we should not guess a selection.
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}
	return acp.PermissionResult{Outcome: "selected", OptionID: optionID}, nil
}

// NormalizeParams passes notifications through unchanged.
func (a *Backend) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }
