// Package mock implements an in-process ACP mock agent for testing.
package mock

import (
	"context"
	"encoding/json"
	"strings"

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

// HandlePermission resolves permission by current mode:
// - reject/deny/read -> reject_once
// - ask/manual/user  -> cancelled (explicit user decision required)
// - others           -> allow_once
func (a *Agent) HandlePermission(_ context.Context, params acp.PermissionRequestParams, mode string) (acp.PermissionResult, error) {
	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	preferredKind := "allow_once"
	switch normalizedMode {
	case "reject", "deny", "read":
		preferredKind = "reject_once"
	case "ask", "manual", "user":
		return acp.PermissionResult{Outcome: "cancelled"}, nil
	}

	optionID := ""
	for _, opt := range params.Options {
		if opt.Kind == preferredKind {
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
func (a *Agent) NormalizeParams(_ string, params json.RawMessage) json.RawMessage { return params }
