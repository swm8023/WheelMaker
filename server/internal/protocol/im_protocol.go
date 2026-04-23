package protocol

import (
	"encoding/json"
	"strings"
)

const (
	// Inbound/Outbound shared methods
	IMMethodPrompt     = "prompt"
	IMMethodPermission = "permission"

	// Outbound session update flavors
	IMMethodAgentMessageChunk = SessionUpdateAgentMessageChunk
	IMMethodAgentThoughtChunk = SessionUpdateAgentThoughtChunk
	IMMethodToolCall          = SessionUpdateToolCall
)

// IMMessage is a minimal IM boundary payload.
//
// Field mapping by method:
//   - method=prompt:
//     request = acp.SessionPromptParams JSON (IM -> Hub)
//     result  = acp.SessionPromptResult JSON (Hub -> IM)
//   - method=permission:
//     request = acp.PermissionRequestParams JSON (Hub -> IM)
//     result  = acp.PermissionResponse JSON (IM -> Hub)
//   - method in {agent_message_chunk, agent_thought_chunk}:
//     text = rendered text
//   - method=tool_call:
//     tool = {cmd, kind, status, output}
//     (both ACP tool_call and tool_call_update should be mapped to this method)
//
// Index is a string sequence marker used by IM side for ordering/replay.
type IMMessage struct {
	Method    string          `json:"method"`
	SessionID string          `json:"sessionId,omitempty"`
	Index     string          `json:"index,omitempty"`
	Text      string          `json:"text,omitempty"`
	Tool      *IMToolPayload  `json:"tool,omitempty"`
	Request   json.RawMessage `json:"request,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

type IMToolPayload struct {
	Cmd    string `json:"cmd,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status,omitempty"`
	Output string `json:"output,omitempty"`
}

func NormalizeIMMethod(method string) string {
	return strings.TrimSpace(method)
}

func IsIMSessionUpdateMethod(method string) bool {
	switch NormalizeIMMethod(method) {
	case IMMethodAgentMessageChunk, IMMethodAgentThoughtChunk, IMMethodToolCall:
		return true
	default:
		return false
	}
}

func IsIMTextMethod(method string) bool {
	switch NormalizeIMMethod(method) {
	case IMMethodAgentMessageChunk, IMMethodAgentThoughtChunk:
		return true
	default:
		return false
	}
}

func IsIMToolMethod(method string) bool {
	switch NormalizeIMMethod(method) {
	case IMMethodToolCall:
		return true
	default:
		return false
	}
}
