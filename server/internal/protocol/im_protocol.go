package protocol

import (
	"encoding/json"
	"strings"
)

const (
	// Shared inbound/outbound methods.
	IMMethodPrompt     = "prompt"
	IMMethodPermission = "permission"

	// Outbound session update methods.
	IMMethodAgentMessage = SessionUpdateAgentMessageChunk
	IMMethodAgentThought = SessionUpdateAgentThoughtChunk
	IMMethodAgentPlan    = SessionUpdatePlan
	IMMethodToolCall     = SessionUpdateToolCall
	IMMethodPromptDone   = "prompt_done"
)

// IMMessage is the minimal IM boundary payload.
//
// This protocol uses method-driven payload typing:
//   - method=prompt:
//     request is IMPromptRequest
//   - method=permission:
//     request is IMPermissionRequest (IM -> Hub)
//     result is IMPermissionResult (Hub -> IM)
//   - method=agent_message_chunk / agent_thought_chunk:
//     result is IMTextResult
//   - method=tool_call:
//     result is IMToolResult
//   - method=agent_plan:
//     result is []IMPlanResult
//   - method=prompt_done:
//     result is IMStopReasonResult
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

type IMPermissionRequest struct {
	Selected string `json:"selected,omitempty"`
}

type IMRequestOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
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

type IMStopReasonResult struct {
	StopReason string `json:"stopReason"`
}

type IMPermissionResult struct {
	ToolCallID string            `json:"toolCallId,omitempty"`
	Title      string            `json:"title,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Status     string            `json:"status,omitempty"`
	Options    []IMRequestOption `json:"options,omitempty"`
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

func IsIMPermissionMethod(method string) bool {
	return NormalizeIMMethod(method) == IMMethodPermission
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
