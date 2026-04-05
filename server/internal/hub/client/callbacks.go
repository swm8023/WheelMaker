package client

import (
	"context"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agentv2"
)

// compile-time check: Client implements agentv2.Callbacks (delegates to Session).
var _ agentv2.Callbacks = (*Client)(nil)

// compile-time check: Session implements agentv2.Callbacks.
var _ agentv2.Callbacks = (*Session)(nil)

// --- Client delegates to activeSession ---

func (c *Client) SessionUpdate(params acp.SessionUpdateParams) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		sess.SessionUpdate(params)
	}
}

func (c *Client) SessionRequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		return sess.SessionRequestPermission(ctx, params)
	}
	return acp.PermissionResult{Outcome: "cancelled"}, nil
}

// --- Session implements the callback logic ---

// SessionUpdate receives session/update notifications from the agent.
func (s *Session) SessionUpdate(params acp.SessionUpdateParams) {
	s.mu.Lock()
	sessID := s.acpSessionID
	ch := s.prompt.updatesCh
	promptCtx := s.prompt.ctx
	replayH := s.replayH
	s.mu.Unlock()

	if replayH != nil {
		replayH(params)
	}

	if params.SessionID != sessID {
		return
	}

	derived := acp.ParseSessionUpdateParams(params)

	if len(derived.AvailableCommands) > 0 || len(derived.ConfigOptions) > 0 || derived.Title != "" || derived.UpdatedAt != "" {
		s.mu.Lock()
		if len(derived.AvailableCommands) > 0 {
			s.sessionMeta.AvailableCommands = derived.AvailableCommands
		}
		if len(derived.ConfigOptions) > 0 {
			s.sessionMeta.ConfigOptions = derived.ConfigOptions
		}
		if derived.Title != "" {
			s.sessionMeta.Title = derived.Title
		}
		if derived.UpdatedAt != "" {
			s.sessionMeta.UpdatedAt = derived.UpdatedAt
		}
		s.mu.Unlock()
	}

	if derived.TrackAddToolCall != "" || derived.TrackDoneToolCall != "" {
		s.mu.Lock()
		if derived.TrackAddToolCall != "" {
			s.prompt.activeTCs[derived.TrackAddToolCall] = struct{}{}
		}
		if derived.TrackDoneToolCall != "" {
			delete(s.prompt.activeTCs, derived.TrackDoneToolCall)
		}
		s.mu.Unlock()
	}

	if ch == nil {
		return
	}
	if promptCtx == nil {
		ch <- derived.Update
		return
	}
	select {
	case ch <- derived.Update:
	case <-promptCtx.Done():
	}
}

// SessionRequestPermission responds to session/request_permission agent requests.
func (s *Session) SessionRequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	s.mu.Lock()
	pCtx := s.prompt.ctx
	snap := acp.SessionConfigSnapshotFromOptions(s.sessionMeta.ConfigOptions)
	s.mu.Unlock()
	if pCtx != nil {
		ctx = pCtx
	}
	return s.permRouter.decide(ctx, params, snap.Mode)
}
