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
	sessID := c.sessionID
	ch := c.promptUpdatesCh
	replayH := c.replayHandler
	c.mu.Unlock()

	if replayH != nil {
		replayH(params)
	}

	if params.SessionID != sessID || ch == nil {
		return
	}

	u := sessionUpdateToUpdate(params.Update)

	// Track session metadata updates.
	switch params.Update.SessionUpdate {
	case "available_commands_update":
		if len(params.Update.AvailableCommands) > 0 {
			c.mu.Lock()
			c.sessionMeta.AvailableCommands = params.Update.AvailableCommands
			c.mu.Unlock()
		}
	case "config_option_update":
		if len(params.Update.ConfigOptions) > 0 {
			c.mu.Lock()
			c.sessionMeta.ConfigOptions = params.Update.ConfigOptions
			c.mu.Unlock()
		}
	case "session_info_update":
		c.mu.Lock()
		if params.Update.Title != "" {
			c.sessionMeta.Title = params.Update.Title
		}
		if params.Update.UpdatedAt != "" {
			c.sessionMeta.UpdatedAt = params.Update.UpdatedAt
		}
		c.mu.Unlock()
	}

	// Track active tool calls for cancelPrompt.
	if id := params.Update.ToolCallID; id != "" {
		switch params.Update.SessionUpdate {
		case "tool_call":
			if s := params.Update.Status; s == "completed" || s == "failed" {
				c.mu.Lock()
				delete(c.activeToolCalls, id)
				c.mu.Unlock()
			} else {
				c.mu.Lock()
				c.activeToolCalls[id] = struct{}{}
				c.mu.Unlock()
			}
		case "tool_call_update":
			if s := params.Update.Status; s == "completed" || s == "failed" {
				c.mu.Lock()
				delete(c.activeToolCalls, id)
				c.mu.Unlock()
			}
		}
	}

	// Send update to the active prompt channel. Use recover() to handle the
	// case where the channel is already closed (ctx cancelled race).
	func() {
		defer func() { recover() }() //nolint:errcheck
		select {
		case ch <- u:
		default:
		}
	}()
}

// SessionRequestPermission responds to session/request_permission agent requests.
// Substitutes promptCtx so that Cancel() unblocks pending permission dialogs.
func (c *Client) SessionRequestPermission(ctx context.Context, params acp.PermissionRequestParams) (acp.PermissionResult, error) {
	c.mu.Lock()
	pCtx := c.promptCtx
	snap := acp.SessionConfigSnapshotFromOptions(c.sessionMeta.ConfigOptions)
	ag := c.currentAgent
	c.mu.Unlock()
	if pCtx != nil {
		ctx = pCtx
	}
	return c.permRouter.decide(ctx, params, snap.Mode, ag)
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
