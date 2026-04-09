package protocol

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
