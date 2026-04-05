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
