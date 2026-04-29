package protocol

import (
	"encoding/json"
	"strings"
)

const (
	// Shared inbound/outbound methods.
	IMMethodPromptRequest = "prompt_request"
	IMMethodPromptDone    = "prompt_done"
	IMMethodSystem        = "system"
	IMMethodSessionInfo   = "session_info"

	// Outbound session update methods.
	IMMethodAgentMessage = SessionUpdateAgentMessageChunk
	IMMethodAgentThought = SessionUpdateAgentThoughtChunk
	IMMethodAgentPlan    = SessionUpdatePlan
	IMMethodToolCall     = SessionUpdateToolCall
)

// IMTurnMessage is the minimal IM boundary payload.
//
// This protocol uses method-driven payload typing:
//   - method=prompt_request:
//     param is IMPromptRequest
//   - method=prompt_done:
//     param is IMPromptResult
//   - method=agent_message_chunk / agent_thought_chunk / user_message_chunk:
//     param is IMTextResult
//   - method=tool_call:
//     param is IMToolResult
//   - method=agent_plan:
//     param is []IMPlanResult
//
// Payload is inlined in Param (no extra type wrapper map).
// Ordering metadata lives in the outer transport or persistence envelope.
type IMTurnMessage struct {
	Method string          `json:"method"`
	Param  json.RawMessage `json:"param,omitempty"`
}

type IMPromptRequest struct {
	ContentBlocks []ContentBlock `json:"contentBlocks,omitempty"`
}

type IMPromptResult struct {
	StopReason string `json:"stopReason"`
}

type IMTextResult struct {
	Text string `json:"text"`
}

type IMToolResult struct {
	Cmd    string `json:"cmd,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status,omitempty"`
}

type IMPlanResult struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

func NormalizeIMMethod(method string) string {
	return strings.TrimSpace(method)
}

func IsIMPromptMethod(method string) bool {
	switch NormalizeIMMethod(method) {
	case IMMethodPromptRequest, IMMethodPromptDone:
		return true
	default:
		return false
	}
}

func IsIMTextResultMethod(method string) bool {
	switch NormalizeIMMethod(method) {
	case IMMethodAgentMessage, IMMethodAgentThought:
		return true
	default:
		return false
	}
}

func IsIMToolResultMethod(method string) bool {
	return NormalizeIMMethod(method) == IMMethodToolCall
}
