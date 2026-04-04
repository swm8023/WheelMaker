package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swm8023/wheelmaker/internal/hub/acp"
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

func (c *Client) FSRead(params acp.FSReadTextFileParams) (acp.FSReadTextFileResult, error) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		return sess.FSRead(params)
	}
	return acp.FSReadTextFileResult{}, fmt.Errorf("no active session")
}

func (c *Client) FSWrite(params acp.FSWriteTextFileParams) error {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		return sess.FSWrite(params)
	}
	return fmt.Errorf("no active session")
}

func (c *Client) TerminalCreate(params acp.TerminalCreateParams) (acp.TerminalCreateResult, error) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		return sess.TerminalCreate(params)
	}
	return acp.TerminalCreateResult{}, fmt.Errorf("no active session")
}

func (c *Client) TerminalOutput(params acp.TerminalOutputParams) (acp.TerminalOutputResult, error) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		return sess.TerminalOutput(params)
	}
	return acp.TerminalOutputResult{}, fmt.Errorf("no active session")
}

func (c *Client) TerminalWaitForExit(params acp.TerminalWaitForExitParams) (acp.TerminalWaitForExitResult, error) {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		return sess.TerminalWaitForExit(params)
	}
	return acp.TerminalWaitForExitResult{}, fmt.Errorf("no active session")
}

func (c *Client) TerminalKill(params acp.TerminalKillParams) error {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		return sess.TerminalKill(params)
	}
	return fmt.Errorf("no active session")
}

func (c *Client) TerminalRelease(params acp.TerminalReleaseParams) error {
	c.mu.Lock()
	sess := c.activeSession
	c.mu.Unlock()
	if sess != nil {
		return sess.TerminalRelease(params)
	}
	return fmt.Errorf("no active session")
}

// --- Session implements the actual callback logic ---

// SessionUpdate receives session/update notifications from the Forwarder.
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

// FSRead responds to fs/read_text_file agent requests.
func (s *Session) FSRead(params acp.FSReadTextFileParams) (acp.FSReadTextFileResult, error) {
	data, err := os.ReadFile(params.Path)
	if err != nil {
		return acp.FSReadTextFileResult{}, fmt.Errorf("fs/read: %w", err)
	}
	content := string(data)
	if params.Line != nil || params.Limit != nil {
		lines := strings.Split(content, "\n")
		start := 0
		if params.Line != nil {
			start = *params.Line - 1
			if start < 0 {
				start = 0
			}
			if start > len(lines) {
				start = len(lines)
			}
		}
		end := len(lines)
		if params.Limit != nil {
			end = start + *params.Limit
			if end > len(lines) {
				end = len(lines)
			}
		}
		content = strings.Join(lines[start:end], "\n")
	}
	return acp.FSReadTextFileResult{Content: content}, nil
}

// FSWrite responds to fs/write_text_file agent requests.
func (s *Session) FSWrite(params acp.FSWriteTextFileParams) error {
	if err := os.MkdirAll(filepath.Dir(params.Path), 0o755); err != nil {
		return fmt.Errorf("fs/write: mkdir: %w", err)
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0o644); err != nil {
		return fmt.Errorf("fs/write: %w", err)
	}
	return nil
}

// TerminalCreate responds to terminal/create agent requests.
func (s *Session) TerminalCreate(params acp.TerminalCreateParams) (acp.TerminalCreateResult, error) {
	return s.terminals.Create(params)
}

// TerminalOutput responds to terminal/output agent requests.
func (s *Session) TerminalOutput(params acp.TerminalOutputParams) (acp.TerminalOutputResult, error) {
	return s.terminals.Output(params.TerminalID)
}

// TerminalWaitForExit responds to terminal/wait_for_exit agent requests.
func (s *Session) TerminalWaitForExit(params acp.TerminalWaitForExitParams) (acp.TerminalWaitForExitResult, error) {
	return s.terminals.WaitForExit(params.TerminalID)
}

// TerminalKill responds to terminal/kill agent requests.
func (s *Session) TerminalKill(params acp.TerminalKillParams) error {
	return s.terminals.Kill(params.TerminalID)
}

// TerminalRelease responds to terminal/release agent requests.
func (s *Session) TerminalRelease(params acp.TerminalReleaseParams) error {
	return s.terminals.Release(params.TerminalID)
}
