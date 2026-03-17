// Package backend defines the Backend interface for ACP-compatible CLI backends.
package backend

import (
	"context"
	"encoding/json"

	"github.com/swm8023/wheelmaker/internal/acp"
)

// Backend is a stateless connection factory for an ACP-compatible CLI backend.
// Connect() starts a new binary subprocess on each call and returns its acp.Conn.
// The subprocess lifecycle is owned by the returned *acp.Conn; Backend.Close()
// is only used for cleanup if Connect() fails.
// After a successful Connect(), calling Close() is a no-op.
//
// Connect() calls Conn.Start() internally; callers must NOT call Start() again.
type Backend interface {
	// Name returns the identifier for this backend (e.g. "claude").
	Name() string

	// Connect starts a new subprocess and returns an initialized *acp.Conn.
	// The conn is started (subprocess running) when Connect returns.
	Connect(ctx context.Context) (*acp.Conn, error)

	// Close cleans up any resources held by the backend.
	// No-op after a successful Connect().
	Close() error

	// HandlePermission responds to session/request_permission callbacks.
	HandlePermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error)

	// NormalizeParams is called before acp processes each incoming session/update
	// notification. Translate legacy protocol fields to modern format here.
	// Return params unchanged for pass-through (default behaviour).
	NormalizeParams(method string, params json.RawMessage) json.RawMessage
}
