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
	SessionUpdateUsageUpdate             = "usage_update"
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
	ToolCallStatusCancelled  = StopReasonCancelled
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
	ConfigOptionIDReasoningEffort = "reasoning_effort"
	ConfigOptionCategoryMode      = "mode"
	ConfigOptionCategoryModel     = "model"
	ConfigOptionCategoryThoughtLv = "thought_level"
	ConfigOptionCategoryReasoning = "reasoning_effort"
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
	ACPProviderCodex       ACPProvider = "codex"
	ACPProviderClaude      ACPProvider = "claude"
	ACPProviderCopilot     ACPProvider = "copilot"
	ACPProviderCodeflicker ACPProvider = "codeflicker"
	ACPProviderOpenCode    ACPProvider = "opencode"
	ACPProviderCodeBuddy   ACPProvider = "codebuddy"
)

var acpProviders = []ACPProvider{ACPProviderCodex, ACPProviderClaude, ACPProviderCopilot, ACPProviderCodeflicker, ACPProviderOpenCode, ACPProviderCodeBuddy}

// ParseACPProvider parses a provider name (case-insensitive).
func ParseACPProvider(name string) (ACPProvider, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case string(ACPProviderCodex):
		return ACPProviderCodex, true
	case string(ACPProviderClaude):
		return ACPProviderClaude, true
	case string(ACPProviderCopilot):
		return ACPProviderCopilot, true
	case string(ACPProviderCodeflicker):
		return ACPProviderCodeflicker, true
	case string(ACPProviderOpenCode):
		return ACPProviderOpenCode, true
	case string(ACPProviderCodeBuddy):
		return ACPProviderCodeBuddy, true
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
