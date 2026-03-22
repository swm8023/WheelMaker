package client

// export_test.go exposes internal helpers for package client_test.
// This file is compiled only during `go test`, keeping production code clean.

import (
	"strings"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/im"
)

// InjectForwarder sets an active Forwarder and marks the session as ready
// with a preset session ID. Used by client_test to bypass Start() when
// testing with a mock agent.
func (c *Client) InjectForwarder(f *acp.Forwarder, sessionID string) {
	c.mu.Lock()
	name := defaultAgentName
	if c.state != nil && strings.TrimSpace(c.state.ActiveAgent) != "" {
		name = c.state.ActiveAgent
	}
	c.conn = &agentConn{name: name, forwarder: f}
	c.session.id = sessionID
	c.session.ready = true
	if c.state == nil {
		c.state = defaultProjectState()
	}
	c.mu.Unlock()
	f.SetCallbacks(c)
}

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
