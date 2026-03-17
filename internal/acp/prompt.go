package acp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// Prompt sends a prompt to the agent and returns a channel of streaming updates.
// The caller must drain the channel until a Update with Done=true is received.
// The channel is closed after the Done update.
func (a *Agent) Prompt(ctx context.Context, text string) (<-chan Update, error) {
	// Clear lastReply so SwitchWithContext never sees a stale value from a prior prompt.
	a.mu.Lock()
	a.lastReply = ""
	a.mu.Unlock()

	if err := a.ensureReady(ctx); err != nil {
		return nil, err
	}

	a.mu.Lock()
	conn := a.conn
	sessID := a.sessionID
	// FL2: create per-prompt context so Cancel() can unblock pending permission requests.
	promptCtx, promptCancel := context.WithCancel(ctx)
	a.promptCtx = promptCtx
	a.promptCancel = promptCancel
	a.mu.Unlock()

	updates := make(chan Update, 32)

	// Store the write end of the updates channel so Cancel() can emit tool_call_cancelled updates.
	a.mu.Lock()
	a.promptUpdatesCh = updates
	a.mu.Unlock()

	// promptDone is closed by the response goroutine just before it closes updates.
	// The notification handler selects on it so it never sends to a closed channel
	// in the ctx-cancelled case (where conn.Send returns early before the wire response).
	promptDone := make(chan struct{})

	// replyMu protects replyBuf, which accumulates text for lastReply.
	var replyMu sync.Mutex
	var replyBuf strings.Builder

	// Subscribe to session/update notifications for this session.
	// The handler runs synchronously inside Conn.dispatch(), which is called
	// on the readLoop goroutine. Because dispatch() is synchronous, all notifications
	// received before a response on the wire are fully processed before conn.Send()
	// returns — so under normal completion the response goroutine closes the channel
	// only after all prior notifications are handled. Do NOT call conn.Send() from
	// this handler (would deadlock readLoop).
	cancelSub := conn.Subscribe(func(n Notification) {
		if n.Method != "session/update" {
			return
		}
		normalized := a.hooks.NormalizeParams(n.Method, n.Params)
		var p SessionUpdateParams
		if err := json.Unmarshal(normalized, &p); err != nil {
			return
		}
		if p.SessionID != sessID {
			return
		}

		// Track available commands in agent state so the client can persist them.
		if p.Update.SessionUpdate == "available_commands_update" && len(p.Update.AvailableCommands) > 0 {
			a.setAvailableCommands(p.Update.AvailableCommands)
		}
		// Track config options so the client can persist them.
		if p.Update.SessionUpdate == "config_option_update" && len(p.Update.ConfigOptions) > 0 {
			a.setConfigOptions(p.Update.ConfigOptions)
		}
		// Track session title and updatedAt.
		if p.Update.SessionUpdate == "session_info_update" {
			a.setSessionInfo(p.Update.Title, p.Update.UpdatedAt)
		}
		// Track active tool calls for Cancel() — protocol §7.4 requires marking them cancelled.
		if id := p.Update.ToolCallID; id != "" {
			switch p.Update.SessionUpdate {
			case "tool_call":
				// tool_call creates the entry; a non-standard terminal status removes it defensively.
				if s := p.Update.Status; s == "completed" || s == "failed" {
					a.mu.Lock()
					delete(a.activeToolCalls, id)
					a.mu.Unlock()
				} else {
					a.mu.Lock()
					a.activeToolCalls[id] = struct{}{}
					a.mu.Unlock()
				}
			case "tool_call_update":
				// tool_call_update is the standard path for status transitions (§9.2).
				if s := p.Update.Status; s == "completed" || s == "failed" {
					a.mu.Lock()
					delete(a.activeToolCalls, id)
					a.mu.Unlock()
				}
			}
		}

		u := sessionUpdateToUpdate(p.Update, normalized)

		// Accumulate text content for SwitchWithContext.
		if u.Type == UpdateText && u.Content != "" {
			replyMu.Lock()
			replyBuf.WriteString(u.Content)
			replyMu.Unlock()
		}

		// Send directly to preserve wire ordering. The channel is buffered (32);
		// if full, readLoop pauses until the caller drains — no deadlock because
		// the caller drains from a separate goroutine.
		//
		// We need to handle two cases where updates may be closed before this
		// handler runs:
		//   1. ctx cancelled: conn.Send returns early, response goroutine closes
		//      updates before all in-flight notifications are dispatched.
		//   2. Concurrent Prompts sharing a session: another prompt's handler
		//      fires after our channel is already closed.
		// recover() catches the "send on closed channel" panic from either case.
		// promptDone serves as a fast-path signal to skip the send without a
		// panic in the common cancellation scenario.
		func() {
			defer func() { recover() }() //nolint:errcheck
			select {
			case updates <- u:
			case <-ctx.Done():
			case <-promptDone:
			}
		}()
	})

	// Goroutine: send session/prompt; emit Done or Error update when complete.
	go func() {
		defer cancelSub()
		defer func() {
			// FL2: clear per-prompt context when this goroutine exits.
			// Also clear tool-call tracking and the updates channel write end.
			a.mu.Lock()
			if a.promptCancel != nil {
				a.promptCtx = nil
				a.promptCancel = nil
			}
			a.activeToolCalls = make(map[string]struct{})
			a.promptUpdatesCh = nil
			a.mu.Unlock()
			promptCancel()
		}()

		var result SessionPromptResult
		// F2 fix: Prompt is []ContentBlock per spec (was plain string).
		err := conn.Send(ctx, "session/prompt", SessionPromptParams{
			SessionID: sessID,
			Prompt:    []ContentBlock{{Type: "text", Text: text}},
		}, &result)

		// Always update lastReply (even if empty) to clear stale values from previous prompts.
		replyMu.Lock()
		reply := replyBuf.String()
		replyMu.Unlock()
		a.mu.Lock()
		a.lastReply = reply
		a.mu.Unlock()

		var finalUpdate Update
		if err != nil {
			finalUpdate = Update{Type: UpdateError, Err: err, Done: true}
		} else {
			finalUpdate = Update{Type: UpdateDone, Content: result.StopReason, Done: true}
		}
		select {
		case updates <- finalUpdate:
		case <-ctx.Done():
		}
		// Signal handlers to stop sending, then close the channel.
		close(promptDone)
		close(updates)
	}()

	return updates, nil
}

// sessionUpdateToUpdate converts an ACP SessionUpdate notification into an agent Update.
// rawParams is the full notification params JSON, used to populate Raw for structured types.
func sessionUpdateToUpdate(u SessionUpdate, rawParams json.RawMessage) Update {
	switch u.SessionUpdate {
	case "agent_message_chunk":
		text := ""
		// F4 fix: Content is now json.RawMessage (was *ContentBlock).
		if u.Content != nil {
			var cb ContentBlock
			if err := json.Unmarshal(u.Content, &cb); err == nil && cb.Type == "text" {
				text = cb.Text
			}
		}
		return Update{Type: UpdateText, Content: text}

	case "agent_thought_chunk":
		text := ""
		if u.Content != nil {
			var cb ContentBlock
			if err := json.Unmarshal(u.Content, &cb); err == nil {
				text = cb.Text
			}
		}
		return Update{Type: UpdateThought, Content: text}

	case "tool_call", "tool_call_update":
		return Update{Type: UpdateToolCall, Raw: rawParams}

	case "plan":
		return Update{Type: UpdatePlan, Raw: rawParams}

	case "config_option_update":
		return Update{Type: UpdateConfigOption, Raw: rawParams}

	case "current_mode_update":
		return Update{Type: UpdateModeChange, Raw: rawParams}

	default:
		return Update{Type: UpdateType(u.SessionUpdate), Raw: rawParams}
	}
}
