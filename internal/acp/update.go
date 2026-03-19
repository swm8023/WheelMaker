package acp

import "encoding/json"

// UpdateType identifies the kind of streaming update from the agent.
type UpdateType string

const (
	// UpdateText is an agent reply text chunk (agent_message_chunk).
	UpdateText UpdateType = "text"
	// UpdateThought is an agent thought chunk (agent_thought_chunk).
	UpdateThought UpdateType = "thought"
	// UpdateToolCall is a tool invocation event (tool_call / tool_call_update).
	UpdateToolCall UpdateType = "tool_call"
	// UpdateToolCallCancelled is emitted by Cancel() for each tool call that was
	// pending or in_progress at the time of cancellation. Content holds the toolCallId.
	UpdateToolCallCancelled UpdateType = "tool_call_cancelled"
	// UpdatePlan is a plan update from the agent.
	UpdatePlan UpdateType = "plan"
	// UpdateConfigOption is emitted when the agent sends config_option_update.
	UpdateConfigOption UpdateType = "config_option_update"
	// UpdateModeChange is a mode switch notification (current_mode_update).
	UpdateModeChange UpdateType = "mode_change"
	// UpdateDone signals the end of a prompt; Content holds the stopReason.
	UpdateDone UpdateType = "done"
	// UpdateError signals a transport or protocol error; Err is non-nil.
	UpdateError UpdateType = "error"
)

// Update is a single streaming unit emitted by Agent.Prompt.
// Raw is populated only for structured update types (tool_call, plan, unknown);
// for plain-text types (text, thought) Raw is nil.
type Update struct {
	Type    UpdateType
	Content string // text content: reply text, thought text, plan text, or stopReason
	Raw     []byte // raw JSON for structured content (tool_call, plan, unknown types)
	Done    bool
	Err     error
}

// SessionUpdateDerived is a parsed, client-ready view of SessionUpdate.
// It is populated by ParseSessionUpdateParams in the transport layer.
type SessionUpdateDerived struct {
	Update            Update
	ConfigOptions     []ConfigOption
	AvailableCommands []AvailableCommand
	Title             string
	UpdatedAt         string
	TrackAddToolCall  string
	TrackDoneToolCall string
}

// ParseSessionUpdateParams parses protocol-level session/update details into a
// derived structure so upper layers can consume normalized fields only.
func ParseSessionUpdateParams(p *SessionUpdateParams) {
	if p == nil {
		return
	}
	d := parseSessionUpdate(p.Update)
	p.Derived = &d
}

func parseSessionUpdate(u SessionUpdate) SessionUpdateDerived {
	d := SessionUpdateDerived{
		Update: sessionUpdateToUpdate(u),
	}

	switch u.SessionUpdate {
	case "available_commands_update":
		if len(u.AvailableCommands) > 0 {
			d.AvailableCommands = u.AvailableCommands
		}
	case "config_option_update":
		if len(u.ConfigOptions) > 0 {
			d.ConfigOptions = u.ConfigOptions
		}
	case "session_info_update":
		d.Title = u.Title
		d.UpdatedAt = u.UpdatedAt
	case "tool_call":
		if u.ToolCallID != "" {
			if s := u.Status; s == "completed" || s == "failed" {
				d.TrackDoneToolCall = u.ToolCallID
			} else {
				d.TrackAddToolCall = u.ToolCallID
			}
		}
	case "tool_call_update":
		if u.ToolCallID != "" {
			if s := u.Status; s == "completed" || s == "failed" {
				d.TrackDoneToolCall = u.ToolCallID
			}
		}
	}

	return d
}

func sessionUpdateToUpdate(u SessionUpdate) Update {
	switch u.SessionUpdate {
	case "agent_message_chunk":
		text := ""
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
		raw, _ := json.Marshal(u)
		return Update{Type: UpdateToolCall, Raw: raw}

	case "plan":
		raw, _ := json.Marshal(u)
		return Update{Type: UpdatePlan, Raw: raw}

	case "config_option_update":
		raw, _ := json.Marshal(u)
		return Update{Type: UpdateConfigOption, Raw: raw}

	case "current_mode_update":
		raw, _ := json.Marshal(u)
		return Update{Type: UpdateModeChange, Raw: raw}

	default:
		raw, _ := json.Marshal(u)
		return Update{Type: UpdateType(u.SessionUpdate), Raw: raw}
	}
}
