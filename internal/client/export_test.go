package client

// export_test.go exposes internal helpers for package client_test.
// This file is compiled only during `go test`, keeping production code clean.

import (
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
