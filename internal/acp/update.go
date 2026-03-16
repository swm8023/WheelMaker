package acp

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





