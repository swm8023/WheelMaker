package protocol

import "encoding/json"

// UpdateType identifies the kind of streaming update from the agent.
type UpdateType string

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
func ParseSessionUpdateParams(params SessionUpdateParams) SessionUpdateDerived {
	return parseSessionUpdate(params.Update)
}

func parseSessionUpdate(u SessionUpdate) SessionUpdateDerived {
	d := SessionUpdateDerived{
		Update: sessionUpdateToUpdate(u),
	}

	switch u.SessionUpdate {
	case SessionUpdateAvailableCommandsUpdate:
		if len(u.AvailableCommands) > 0 {
			d.AvailableCommands = u.AvailableCommands
		}
	case SessionUpdateConfigOptionUpdate:
		if len(u.ConfigOptions) > 0 {
			d.ConfigOptions = u.ConfigOptions
		}
	case SessionUpdateSessionInfoUpdate:
		d.Title = u.Title
		d.UpdatedAt = u.UpdatedAt
	case SessionUpdateToolCall:
		if u.ToolCallID != "" {
			if s := u.Status; s == ToolCallStatusCompleted || s == ToolCallStatusFailed {
				d.TrackDoneToolCall = u.ToolCallID
			} else {
				d.TrackAddToolCall = u.ToolCallID
			}
		}
	case SessionUpdateToolCallUpdate:
		if u.ToolCallID != "" {
			if s := u.Status; s == ToolCallStatusCompleted || s == ToolCallStatusFailed {
				d.TrackDoneToolCall = u.ToolCallID
			}
		}
	}

	return d
}

func sessionUpdateToUpdate(u SessionUpdate) Update {
	switch u.SessionUpdate {
	case SessionUpdateAgentMessageChunk:
		text := ""
		if u.Content != nil {
			var cb ContentBlock
			if err := json.Unmarshal(u.Content, &cb); err == nil && cb.Type == ContentBlockTypeText {
				text = cb.Text
			}
		}
		return Update{Type: UpdateText, Content: text}

	case SessionUpdateUserMessageChunk:
		// User message reflection - expose as its own type so callers can ignore it
		// without hitting the default branch. No caller today renders this.
		return Update{Type: UpdateUserChunk}

	case SessionUpdateAgentThoughtChunk:
		text := ""
		if u.Content != nil {
			var cb ContentBlock
			if err := json.Unmarshal(u.Content, &cb); err == nil {
				text = cb.Text
			}
		}
		return Update{Type: UpdateThought, Content: text}

	case SessionUpdateToolCall, SessionUpdateToolCallUpdate:
		raw, _ := json.Marshal(u)
		return Update{Type: UpdateToolCall, Raw: raw}

	case SessionUpdatePlan:
		raw, _ := json.Marshal(u)
		return Update{Type: UpdatePlan, Raw: raw}

	case SessionUpdateConfigOptionUpdate:
		raw, _ := json.Marshal(u)
		return Update{Type: UpdateConfigOption, Raw: raw}

	case SessionUpdateCurrentModeUpdate:
		// Legacy path: normalize layer should map this into config_option_update
		// before parse for regular message handling.
		raw, _ := json.Marshal(u)
		return Update{Type: UpdateModeChange, Raw: raw}

	default:
		raw, _ := json.Marshal(u)
		return Update{Type: UpdateType(u.SessionUpdate), Raw: raw}
	}
}
