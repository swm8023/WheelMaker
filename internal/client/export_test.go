package client

// export_test.go exposes internal helpers for package client_test.
// This file is compiled only during `go test`, keeping production code clean.

import (
	acp "github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/im"
)

// InjectSession sets the active session and initialises a default state.
// Used by client_test to bypass Start() when testing with a mock session.
func (c *Client) InjectSession(sess acp.Session) {
	c.mu.Lock()
	c.session = sess
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

// InjectIMProvider sets the IM provider and registers the HandleMessage callback.
func (c *Client) InjectIMProvider(p im.Provider) {
	c.imRun = p
}

// DefaultState returns a freshly initialised default state.
func DefaultState() *ProjectState {
	return defaultProjectState()
}
