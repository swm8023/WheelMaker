package protocol

import (
	"encoding/json"
	"strings"
)

const (
	IMSchemaV1  = "wm.im"
	IMVersionV1 = "1.0-draft"
)

const (
	// Inbound: IM -> Hub
	IMMethodPrompt = "prompt"

	// Outbound: Hub -> IM (session update flavors)
	IMMethodAgentMessageChunk = SessionUpdateAgentMessageChunk
	IMMethodAgentThoughtChunk = SessionUpdateAgentThoughtChunk
	IMMethodToolCall          = SessionUpdateToolCall

	// Outbound: Hub -> IM lifecycle/permission
	IMMethodPromptDone       = "prompt_done"
	IMMethodPermissionAsk    = "permission_request"
	IMMethodPermissionResult = "permission_result"
)

// IMMessage is a lightweight IM boundary payload.
//
// Field mapping by method:
//   - method=prompt:
//     request = acp.SessionPromptParams JSON
//   - method in {agent_message_chunk, agent_thought_chunk}:
//     text = rendered text
//   - method=tool_call:
//     tool = {cmd, kind, status, output}
//     (both ACP tool_call and tool_call_update should be mapped to this method)
//   - method=prompt_done:
//     result = acp.SessionPromptResult JSON
//   - method=permission_request:
//     request = acp.PermissionRequestParams JSON
//   - method=permission_result:
//     result = acp.PermissionResponse JSON
//
// RequestID is optional and mainly used to correlate permission request/result.
type IMMessage struct {
	Schema    string          `json:"schema,omitempty"`
	Version   string          `json:"version,omitempty"`
	Method    string          `json:"method"`
	SessionID string          `json:"sessionId,omitempty"`
	ChatID    string          `json:"chatId,omitempty"`
	RequestID int64           `json:"requestId,omitempty"`
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
