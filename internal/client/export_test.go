package client

// export_test.go exposes internal helpers for package client_test.
// This file is compiled only during `go test`, keeping production code clean.

import (
	"context"
	"encoding/json"

	"github.com/swm8023/wheelmaker/internal/acp"
	"github.com/swm8023/wheelmaker/internal/im"
)

// mockSessionAgent wraps an acp.Session to satisfy agent.Agent for test injection.
// Used by InjectSession so that /status and /cancel work against the mock session.
type mockSessionAgent struct {
	sess acp.Session
}

func (a *mockSessionAgent) Name() string { return a.sess.AgentName() }
func (a *mockSessionAgent) Connect(_ context.Context) (*acp.Conn, error) {
	return nil, nil // never called via InjectSession path
}
func (a *mockSessionAgent) Close() error { return nil }
func (a *mockSessionAgent) HandlePermission(_ context.Context, _ acp.PermissionRequestParams, _ string) (acp.PermissionResult, error) {
	return acp.PermissionResult{}, nil
}
func (a *mockSessionAgent) NormalizeParams(_ string, params json.RawMessage) json.RawMessage {
	return params
}

var _ acp.ClientCallbacks = (*Client)(nil) // ensure compile-time check still holds

// InjectSession sets the active session state from an acp.Session mock.
// Used by client_test to bypass Start() when testing with a mock session.
// Sets currentAgent (for Name()), sessionID, and ready so that /cancel,
// /status, and handlePrompt commands work with the injected mock.
//
// Note: this sets forwarder to a non-nil sentinel so that nil-checks pass.
// The actual prompt/cancel behaviour for mock-based tests is wired via
// injectedPromptFn and injectedCancelFn on the Client.
func (c *Client) InjectSession(sess acp.Session) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == nil {
		c.state = defaultProjectState()
	}

	c.sessionID = sess.SessionID()
	c.currentAgent = &mockSessionAgent{sess: sess}
	c.currentAgentName = sess.AgentName()
	c.ready = true

	// Set up cancel delegation through promptCancel.
	_, cancel := context.WithCancel(context.Background())
	c.promptCancel = func() {
		_ = sess.Cancel()
		cancel()
	}

	// Store the mock prompt function for use by handlePrompt.
	c.injectedPromptFn = func(ctx context.Context, text string) (<-chan acp.Update, error) {
		return sess.Prompt(ctx, text)
	}

	// forwarder is left nil; /cancel and /status now guard on currentAgent != nil.
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
