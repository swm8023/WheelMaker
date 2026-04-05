package protocol

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

// --- ACP statuses ---

const (
	ToolCallStatusCompleted = "completed"
	ToolCallStatusFailed    = "failed"
)

// --- ACP payload literals ---

const (
	ContentBlockTypeText     = "text"
	ConfigOptionIDMode       = "mode"
	ConfigOptionCategoryMode = "mode"
)
