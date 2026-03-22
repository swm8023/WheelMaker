package client

// export_test.go exposes internal helpers for package client_test.
// This file is compiled only during `go test`, keeping production code clean.

import (
	"context"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/im"
)

// InjectForwarder sets an active Forwarder and marks the session as ready
// with a preset session ID. Used by client_test to bypass Start() when
// testing with a mock agent.
func (c *Client) InjectForwarder(f *acp.Forwarder, sessionID string) {
	c.mu.Lock()
	c.conn = &agentConn{forwarder: f}
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
	// Use a stub agentConn to satisfy c.conn != nil checks.
	c.conn = &agentConn{name: s.AgentName(), agent: &stubAgent{name: s.AgentName()}}
	c.mu.Unlock()
}

// stubAgent is a minimal agent.Agent used by InjectSession.
type stubAgent struct{ name string }

func (a *stubAgent) Name() string { return a.name }
func (a *stubAgent) Connect(_ context.Context) (*acp.Conn, error) {
	return nil, nil
}
func (a *stubAgent) Close() error { return nil }

// InjectState replaces the persisted state.
func (c *Client) InjectState(st *ProjectState) {
	c.mu.Lock()
	c.state = st
	c.mu.Unlock()
}

// InjectIMChannel sets the IM bridge over the provided IM channel.
func (c *Client) InjectIMChannel(p im.Channel) {
	c.imBridge = im.NewBridge(p)
}

// DefaultState returns a freshly initialised default state.
func DefaultState() *ProjectState {
	return defaultProjectState()
}
