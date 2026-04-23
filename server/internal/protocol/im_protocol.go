package protocol

import "strings"

const (
	// Shared inbound/outbound methods.
	IMMethodPrompt     = "prompt"
	IMMethodPermission = "permission"
)

type IMRequestType string

const (
	IMRequestTypeContentBlocks IMRequestType = "content_blocks"
	IMRequestTypeSelected      IMRequestType = "selected"
)

type IMResultType string

const (
	IMResultTypeText       IMResultType = "text"
	IMResultTypeTool       IMResultType = "tool"
	IMResultTypeStopReason IMResultType = "stop_reason"
	IMResultTypePermission IMResultType = "permission"
)

// IMMessage is the minimal IM boundary payload.
//
// Method and payload mapping:
//   - method=prompt:
//     request.type=content_blocks (IM -> Hub)
//     result.type=text|tool|stop_reason (Hub -> IM)
//   - method=permission:
//     request.type=selected (IM -> Hub)
//     result.type=permission (Hub -> IM)
//
// Index is a string sequence marker used by IM side for ordering/replay.
type IMMessage struct {
	Method    string     `json:"method"`
	SessionID string     `json:"sessionId,omitempty"`
	Index     string     `json:"index,omitempty"`
	Request   *IMRequest `json:"request,omitempty"`
	Result    *IMResult  `json:"result,omitempty"`
}

type IMRequest struct {
	Type          IMRequestType  `json:"type"`
	ContentBlocks []ContentBlock `json:"contentBlocks,omitempty"`
	Selected      string         `json:"selected,omitempty"`
}

type IMResult struct {
	Type       IMResultType        `json:"type"`
	Text       *IMTextResult       `json:"text,omitempty"`
	Tool       *IMToolResult       `json:"tool,omitempty"`
	StopReason *IMStopReasonResult `json:"stopReason,omitempty"`
	Permission *IMPermissionResult `json:"permission,omitempty"`
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
	ToolCallID string               `json:"toolCallId,omitempty"`
	Title      string               `json:"title,omitempty"`
	Kind       string               `json:"kind,omitempty"`
	Status     string               `json:"status,omitempty"`
	Options    []IMPermissionOption `json:"options,omitempty"`
}

type IMPermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name,omitempty"`
	Kind     string `json:"kind,omitempty"`
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

func NormalizeIMRequestType(t IMRequestType) IMRequestType {
	return IMRequestType(strings.TrimSpace(string(t)))
}

func NormalizeIMResultType(t IMResultType) IMResultType {
	return IMResultType(strings.TrimSpace(string(t)))
}
