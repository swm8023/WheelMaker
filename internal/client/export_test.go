package client

// export_test.go exposes internal helpers for package client_test.
// This file is compiled only during `go test`, keeping production code clean.

import (
	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/im"
)

// InjectSession sets the active session and initialises a default state.
// Used by client_test to bypass Start() when testing with a mock session.
func (c *Client) InjectSession(sess agent.Session) {
	c.mu.Lock()
	c.session = sess
	if c.state == nil {
		c.state = defaultState()
	}
	c.mu.Unlock()
}

// InjectState replaces the persisted state.
func (c *Client) InjectState(st *State) {
	c.mu.Lock()
	c.state = st
	c.mu.Unlock()
}

// InjectIMAdapter sets the IM adapter and registers the HandleMessage callback.
func (c *Client) InjectIMAdapter(a im.Adapter) {
	c.imRun = a
}

// DefaultState returns a freshly initialised default state.
func DefaultState() *State {
	return defaultState()
}
