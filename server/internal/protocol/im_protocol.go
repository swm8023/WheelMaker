package protocol

import (
	"encoding/json"
	"strings"
)

const (
	// Shared inbound/outbound methods.
	IMMethodPrompt = "prompt"
	IMMethodSystem = "system"

	// Outbound session update methods.
	IMMethodAgentMessage = SessionUpdateAgentMessageChunk
	IMMethodAgentThought = SessionUpdateAgentThoughtChunk
	IMMethodAgentPlan    = SessionUpdatePlan
	IMMethodToolCall     = SessionUpdateToolCall
)

// IMMessage is the minimal IM boundary payload.
//
// This protocol uses method-driven payload typing:
//   - method=prompt:
//     request is IMPromptRequest
//     result is IMPromptResult
//   - method=agent_message_chunk / agent_thought_chunk:
//     result is IMTextResult
//   - method=tool_call:
//     result is IMToolResult
//   - method=agent_plan:
//     result is []IMPlanResult
//
// Request and Result are inlined (no extra type wrapper map).
// Index is a string sequence marker for ordering/replay.
type IMMessage struct {
	Method  string          `json:"method"`
	Index   string          `json:"index,omitempty"`
	Request json.RawMessage `json:"request,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
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
	Output string `json:"output,omitempty"`
}

type IMPlanResult struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

func NormalizeIMMethod(method string) string {
	return strings.TrimSpace(method)
}

func IsIMPromptMethod(method string) bool {
	return NormalizeIMMethod(method) == IMMethodPrompt
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
