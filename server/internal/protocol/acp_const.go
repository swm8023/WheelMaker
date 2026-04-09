package protocol

import "strings"

// --- ACP session update names ---

const (
	SessionUpdateAgentMessageChunk       = "agent_message_chunk"
	SessionUpdateUserMessageChunk        = "user_message_chunk"
	SessionUpdateAgentThoughtChunk       = "agent_thought_chunk"
	SessionUpdateToolCall                = "tool_call"
	SessionUpdateToolCallUpdate          = "tool_call_update"
	SessionUpdatePlan                    = "plan"
	SessionUpdateConfigOptionUpdate      = "config_option_update"
	SessionUpdateAvailableCommandsUpdate = "available_commands_update"
	SessionUpdateSessionInfoUpdate       = "session_info_update"
	SessionUpdateCurrentModeUpdate       = "current_mode_update"
)

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

// --- ACP update types ---

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
	UpdateConfigOption UpdateType = UpdateType(SessionUpdateConfigOptionUpdate)
	// UpdateAvailableCommands is emitted when the agent sends available_commands_update.
	UpdateAvailableCommands UpdateType = UpdateType(SessionUpdateAvailableCommandsUpdate)
	// UpdateSessionInfo is emitted when the agent sends session_info_update.
	UpdateSessionInfo UpdateType = UpdateType(SessionUpdateSessionInfoUpdate)
	// UpdateUserChunk is a user message reflection chunk (user_message_chunk).
	// Most integrations can ignore this; it is exposed for completeness.
	UpdateUserChunk UpdateType = UpdateType(SessionUpdateUserMessageChunk)
	// UpdateModeChange is a legacy mode switch notification
	// (current_mode_update). New integrations should use UpdateConfigOption.
	UpdateModeChange UpdateType = "mode_change"
	// UpdateDone signals the end of a prompt; Content holds the stopReason.
	UpdateDone UpdateType = "done"
	// UpdateError signals a transport or protocol error; Err is non-nil.
	UpdateError UpdateType = "error"
)

// --- ACP method names ---

const (
	MethodInitialize        = "initialize"
	MethodSessionNew        = "session/new"
	MethodSessionPrompt     = "session/prompt"
	MethodSessionCancel     = "session/cancel"
	MethodSessionLoad       = "session/load"
	MethodSessionList       = "session/list"
	MethodSetConfigOption   = "session/set_config_option"
	MethodRequestPermission = "session/request_permission"
	MethodFSRead            = "fs/read_text_file"
	MethodFSWrite           = "fs/write_text_file"
	MethodTerminalCreate    = "terminal/create"
	MethodTerminalOutput    = "terminal/output"
	MethodTerminalWaitExit  = "terminal/wait_for_exit"
	MethodTerminalKill      = "terminal/kill"
	MethodTerminalRelease   = "terminal/release"
	MethodSessionUpdate     = "session/update"
)

// --- ACP statuses ---

const (
	ToolCallStatusPending    = "pending"
	ToolCallStatusInProgress = "in_progress"
	ToolCallStatusCompleted  = "completed"
	ToolCallStatusFailed     = "failed"
)

// --- ACP tool kinds ---

const (
	ToolKindRead    = "read"
	ToolKindWrite   = "write"
	ToolKindExecute = "execute"
	ToolKindOther   = "other"
)

// --- ACP payload literals ---

const (
	ContentBlockTypeText         = "text"
	ContentBlockTypeImage        = "image"
	ContentBlockTypeAudio        = "audio"
	ContentBlockTypeResource     = "resource"
	ContentBlockTypeResourceLink = "resource_link"

	ConfigOptionIDMode            = "mode"
	ConfigOptionIDModel           = "model"
	ConfigOptionIDThoughtLevel    = "thought_level"
	ConfigOptionCategoryMode      = "mode"
	ConfigOptionCategoryModel     = "model"
	ConfigOptionCategoryThoughtLv = "thought_level"
)

// --- ACP stop reasons ---

const (
	StopReasonEndTurn         = "end_turn"
	StopReasonMaxTokens       = "max_tokens"
	StopReasonMaxTurnRequests = "max_turn_requests"
	StopReasonRefusal         = "refusal"
	StopReasonCancelled       = "cancelled"
)

// ACPProvider identifies a built-in ACP provider preset.
type ACPProvider string

const (
	ACPProviderCodex   ACPProvider = "codex"
	ACPProviderClaude  ACPProvider = "claude"
	ACPProviderCopilot ACPProvider = "copilot"
)

var acpProviders = []ACPProvider{ACPProviderCodex, ACPProviderClaude, ACPProviderCopilot}

// ParseACPProvider parses a provider name (case-insensitive).
func ParseACPProvider(name string) (ACPProvider, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case string(ACPProviderCodex):
		return ACPProviderCodex, true
	case string(ACPProviderClaude):
		return ACPProviderClaude, true
	case string(ACPProviderCopilot):
		return ACPProviderCopilot, true
	default:
		return "", false
	}
}

// ACPProviderNames returns built-in provider names in stable order.
func ACPProviderNames() []string {
	out := make([]string, 0, len(acpProviders))
	for _, p := range acpProviders {
		out = append(out, string(p))
	}
	return out
}
