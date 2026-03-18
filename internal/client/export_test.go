package client

// export_test.go exposes internal helpers for package client_test.
// This file is compiled only during `go test`, keeping production code clean.

import (
	"context"
	"encoding/json"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/im"
)

// InjectForwarder sets an active Forwarder and marks the session as ready
// with a preset session ID. Used by client_test to bypass Start() when
// testing with a mock agent.
func (c *Client) InjectForwarder(f *acp.Forwarder, sessionID string) {
	c.mu.Lock()
	c.forwarder = f
	c.sessionID = sessionID
	c.ready = true
	if c.state == nil {
		c.state = defaultProjectState()
	}
	c.mu.Unlock()
}

// InjectSession installs a Session override so tests can inject a mock that
// intercepts Prompt and Cancel without spawning a real ACP subprocess.
// It also marks the client as having an active agent so command routing works.
func (c *Client) InjectSession(s Session) {
	c.mu.Lock()
	c.sessionOverride = s
	// Populate agent fields so /cancel, /status, and other commands see an active session.
	if c.state == nil {
		c.state = defaultProjectState()
	}
	c.sessionID = s.SessionID()
	c.ready = true
	// Use a stub agent.Agent to satisfy currentAgent != nil checks.
	c.currentAgent = &stubAgent{name: s.AgentName()}
	c.currentAgentName = s.AgentName()
	c.mu.Unlock()
}

// stubAgent is a minimal agent.Agent used by InjectSession.
type stubAgent struct{ name string }

func (a *stubAgent) Name() string { return a.name }
func (a *stubAgent) Connect(_ context.Context) (*acp.Conn, error) {
	return nil, nil
}
func (a *stubAgent) Close() error { return nil }
func (a *stubAgent) HandlePermission(_ context.Context, _ acp.PermissionRequestParams, _ string) (acp.PermissionResult, error) {
	return acp.PermissionResult{}, nil
}
func (a *stubAgent) NormalizeParams(_ string, p json.RawMessage) json.RawMessage { return p }

// InjectState replaces the persisted state.
func (c *Client) InjectState(st *ProjectState) {
	c.mu.Lock()
	c.state = st
	c.mu.Unlock()
}

// InjectIMChannel sets the IM channel and registers the HandleMessage callback.
func (c *Client) InjectIMChannel(p im.Channel) {
	c.imRun = p
}

// DefaultState returns a freshly initialised default state.
func DefaultState() *ProjectState {
	return defaultProjectState()
}
