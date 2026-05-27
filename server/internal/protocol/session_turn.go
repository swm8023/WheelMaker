package protocol

import (
	"encoding/json"
	"strings"
)

const (
	// Shared inbound/outbound methods.
	SessionTurnMethodPromptRequest = "prompt_request"
	SessionTurnMethodPromptDone    = "prompt_done"
	SessionTurnMethodSystem        = "system"
	SessionTurnMethodSessionInfo   = "session_info"

	// Outbound session update methods.
	SessionTurnMethodAgentMessage = SessionUpdateAgentMessageChunk
	SessionTurnMethodAgentThought = SessionUpdateAgentThoughtChunk
	SessionTurnMethodAgentPlan    = SessionUpdatePlan
	SessionTurnMethodToolCall     = SessionUpdateToolCall
)

// SessionTurnMessage is the persisted session event payload.
//
// This protocol uses method-driven payload typing:
//   - method=prompt_request:
//     param is SessionTurnPromptRequest
//   - method=prompt_done:
//     param is SessionTurnPromptResult
//   - method=agent_message_chunk / agent_thought_chunk / user_message_chunk:
//     param is SessionTurnTextResult
//   - method=tool_call:
//     param is SessionTurnToolResult
//   - method=agent_plan:
//     param is []SessionTurnPlanResult
//
// Payload is inlined in Param (no extra type wrapper map).
// Ordering metadata lives in the outer transport or persistence envelope.
type SessionTurnMessage struct {
	Method string          `json:"method"`
	Param  json.RawMessage `json:"param,omitempty"`
}

type SessionTurnPromptRequest struct {
	ContentBlocks []ContentBlock `json:"contentBlocks,omitempty"`
	ModelName     string         `json:"modelName,omitempty"`
	CreatedAt     string         `json:"createdAt,omitempty"`
}

type SessionTurnPromptResult struct {
	StopReason  string `json:"stopReason"`
	CompletedAt string `json:"completedAt,omitempty"`
	Message     string `json:"message,omitempty"`
}

type SessionTurnTextResult struct {
	Text string `json:"text"`
}

type SessionTurnToolResult struct {
	Cmd    string `json:"cmd,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status,omitempty"`
}

type SessionTurnPlanResult struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

func NormalizeSessionTurnMethod(method string) string {
	return strings.TrimSpace(method)
}

func IsSessionTurnPromptMethod(method string) bool {
	switch NormalizeSessionTurnMethod(method) {
	case SessionTurnMethodPromptRequest, SessionTurnMethodPromptDone:
		return true
	default:
		return false
	}
}

func IsSessionTurnTextResultMethod(method string) bool {
	switch NormalizeSessionTurnMethod(method) {
	case SessionTurnMethodAgentMessage, SessionTurnMethodAgentThought:
		return true
	default:
		return false
	}
}

func IsSessionTurnToolResultMethod(method string) bool {
	return NormalizeSessionTurnMethod(method) == SessionTurnMethodToolCall
}
