package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swm8023/wheelmaker/internal/acp"
)

// compile-time check: Client implements acp.ClientCallbacks.
var _ acp.ClientCallbacks = (*Client)(nil)

// SessionUpdate receives session/update notifications from the Forwarder.
// Routes the update to the active promptUpdatesCh if the session ID matches.
func (c *Client) SessionUpdate(params acp.SessionUpdateParams) {
	c.mu.Lock()
	sessID := c.session.id
	ch := c.prompt.updatesCh
	promptCtx := c.prompt.ctx
	replayH := c.session.replayH
	c.mu.Unlock()

	if replayH != nil {
		replayH(params)
	}

	if params.SessionID != sessID {
		return
	}

	derived := acp.ParseSessionUpdateParams(params)

	// Always update sessionMeta for matching session (even outside an active prompt).
	// This ensures config_option_update from set_config_option is captured correctly.
	if len(derived.AvailableCommands) > 0 || len(derived.ConfigOptions) > 0 || derived.Title != "" || derived.UpdatedAt != "" {
		c.mu.Lock()
		if len(derived.AvailableCommands) > 0 {
			c.sessionMeta.AvailableCommands = derived.AvailableCommands
		}
		if len(derived.ConfigOptions) > 0 {
			c.sessionMeta.ConfigOptions = derived.ConfigOptions
		}
		if derived.Title != "" {
			c.sessionMeta.Title = derived.Title
		}
		if derived.UpdatedAt != "" {
			c.sessionMeta.UpdatedAt = derived.UpdatedAt
		}
		c.mu.Unlock()
	}

	if derived.TrackAddToolCall != "" || derived.TrackDoneToolCall != "" {
		c.mu.Lock()
		if derived.TrackAddToolCall != "" {
			c.prompt.activeTCs[derived.TrackAddToolCall] = struct{}{}
		}
		if derived.TrackDoneToolCall != "" {
			delete(c.prompt.activeTCs, derived.TrackDoneToolCall)
		}
		c.mu.Unlock()
	}

	// Route update to the active prompt channel if one exists.
	if ch == nil {
		return
	}
	// Preserve update ordering and avoid lossy drops under high-frequency streams.
	// If prompt is already cancelled, skip blocking send.
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
// Substitutes promptCtx so that Cancel() unblocks pending permission dialogs.
func (c *Client) SessionRequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	c.mu.Lock()
	pCtx := c.prompt.ctx
	snap := acp.SessionConfigSnapshotFromOptions(c.sessionMeta.ConfigOptions)
	c.mu.Unlock()
	if pCtx != nil {
		ctx = pCtx
	}
	return c.permRouter.decide(ctx, params, snap.Mode)
}

// FSRead responds to fs/read_text_file agent requests.
func (c *Client) FSRead(params acp.FSReadTextFileParams) (acp.FSReadTextFileResult, error) {
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
func (c *Client) FSWrite(params acp.FSWriteTextFileParams) error {
	if err := os.MkdirAll(filepath.Dir(params.Path), 0o755); err != nil {
		return fmt.Errorf("fs/write: mkdir: %w", err)
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0o644); err != nil {
		return fmt.Errorf("fs/write: %w", err)
	}
	return nil
}

// TerminalCreate responds to terminal/create agent requests.
func (c *Client) TerminalCreate(params acp.TerminalCreateParams) (acp.TerminalCreateResult, error) {
	return c.terminals.Create(params)
}

// TerminalOutput responds to terminal/output agent requests.
func (c *Client) TerminalOutput(params acp.TerminalOutputParams) (acp.TerminalOutputResult, error) {
	return c.terminals.Output(params.TerminalID)
}

// TerminalWaitForExit responds to terminal/wait_for_exit agent requests.
func (c *Client) TerminalWaitForExit(params acp.TerminalWaitForExitParams) (acp.TerminalWaitForExitResult, error) {
	return c.terminals.WaitForExit(params.TerminalID)
}

// TerminalKill responds to terminal/kill agent requests.
func (c *Client) TerminalKill(params acp.TerminalKillParams) error {
	return c.terminals.Kill(params.TerminalID)
}

// TerminalRelease responds to terminal/release agent requests.
func (c *Client) TerminalRelease(params acp.TerminalReleaseParams) error {
	return c.terminals.Release(params.TerminalID)
}
