package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
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
	a.mu.Unlock()

	updates := make(chan Update, 32)

	// sendMu and channelClosed coordinate the subscriber and the close operation.
	// Sending to a closed channel panics even inside a select, so we use a flag
	// guarded by a mutex to prevent any send after close.
	var sendMu sync.Mutex
	var channelClosed bool

	// replyMu protects replyBuf, which accumulates text for lastReply.
	var replyMu sync.Mutex
	var replyBuf strings.Builder

	// Subscribe to session/update notifications for this session.
	cancelSub := conn.Subscribe(func(n acp.Notification) {
		if n.Method != "session/update" {
			return
		}
		var p acp.SessionUpdateParams
		if err := json.Unmarshal(n.Params, &p); err != nil {
			return
		}
		if p.SessionID != sessID {
			return
		}

		u := sessionUpdateToUpdate(p.Update, n.Params)

		// Accumulate text content for SwitchWithContext.
		if u.Type == UpdateText && u.Content != "" {
			replyMu.Lock()
			replyBuf.WriteString(u.Content)
			replyMu.Unlock()
		}

		// Guard against send-on-closed-channel: sending to a closed channel panics
		// even inside a select. The mutex ensures close() and send are mutually exclusive.
		sendMu.Lock()
		if !channelClosed {
			select {
			case updates <- u:
			case <-ctx.Done():
			}
		}
		sendMu.Unlock()
	})

	// Goroutine: send session/prompt; emit Done or Error update when complete.
	go func() {
		defer cancelSub()

		var result acp.SessionPromptResult
		err := conn.Send(ctx, "session/prompt", acp.SessionPromptParams{
			SessionID: sessID,
			Prompt:    text,
		}, &result)

		// Always update lastReply (even if empty) to clear stale values from previous prompts.
		replyMu.Lock()
		reply := replyBuf.String()
		replyMu.Unlock()
		a.mu.Lock()
		a.lastReply = reply
		a.mu.Unlock()

		// Acquire sendMu to close the channel safely: set channelClosed first so
		// concurrent subscriber goroutines see it and stop sending, then close.
		var finalUpdate Update
		if err != nil {
			finalUpdate = Update{Type: UpdateError, Err: err, Done: true}
		} else {
			finalUpdate = Update{Type: UpdateDone, Content: result.StopReason, Done: true}
		}
		sendMu.Lock()
		channelClosed = true
		select {
		case updates <- finalUpdate:
		case <-ctx.Done():
		}
		close(updates)
		sendMu.Unlock()
	}()

	return updates, nil
}

// sessionUpdateToUpdate converts an ACP SessionUpdate notification into an agent Update.
// rawParams is the full notification params JSON, used to populate Raw for structured types.
func sessionUpdateToUpdate(u acp.SessionUpdate, rawParams json.RawMessage) Update {
	switch u.SessionUpdate {
	case "agent_message_chunk":
		text := ""
		if u.Content != nil && u.Content.Type == "text" {
			text = u.Content.Text
		}
		return Update{Type: UpdateText, Content: text}

	case "agent_thought_chunk":
		text := ""
		if u.Content != nil {
			text = u.Content.Text
		}
		return Update{Type: UpdateThought, Content: text}

	case "tool_call", "tool_call_update":
		return Update{Type: UpdateToolCall, Raw: rawParams}

	case "plan":
		return Update{Type: UpdatePlan, Raw: rawParams}

	case "current_mode_update":
		return Update{Type: UpdateModeChange, Raw: rawParams}

	default:
		return Update{Type: UpdateType(u.SessionUpdate), Raw: rawParams}
	}
}
