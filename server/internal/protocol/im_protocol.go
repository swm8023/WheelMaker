package protocol

import "encoding/json"

// IM protocol (draft) is a lightweight Hub<->IM contract.
//
// Design goals:
//  1. Keep IM transport payloads explicit and compact.
//  2. Avoid ACP JSON-RPC wrappers (method/params/update nesting) at IM boundary.
//  3. Preserve only fields with clear rendering or audit value.
//
// This file only defines schema and payloads. Runtime wiring can migrate
// incrementally from ACP-shaped IM callbacks to this contract.

const (
	IMSchemaV1  = "wm.im"
	IMVersionV1 = "1.0-draft"
)

type IMMessageType string

const (
	IMMessageTypeRequest  IMMessageType = "request"
	IMMessageTypeResponse IMMessageType = "response"
	IMMessageTypeEvent    IMMessageType = "event"
	IMMessageTypeError    IMMessageType = "error"
)

type IMEnvelope struct {
	Schema        string          `json:"schema"`
	Version       string          `json:"version"`
	Type          IMMessageType   `json:"type"`
	Method        string          `json:"method"`
	RequestID     int64           `json:"requestId,omitempty"`
	CorrelationID string          `json:"correlationId,omitempty"`
	ProjectID     string          `json:"projectId,omitempty"`
	SessionID     string          `json:"sessionId,omitempty"`
	Chat          IMChatRef       `json:"chat"`
	TS            int64           `json:"ts,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Error         *IMError        `json:"error,omitempty"`
}

type IMChatRef struct {
	ChannelID string `json:"channelId"`
	ChatID    string `json:"chatId"`
}

type IMError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

const (
	// Inbound (IM -> Hub)
	IMMethodChatSend          = "im.chat.send"
	IMMethodCommandRun        = "im.command.run"
	IMMethodPermissionRespond = "im.permission.respond"
	IMMethodSessionList       = "im.session.list"
	IMMethodSessionRead       = "im.session.read"

	// Outbound (Hub -> IM)
	IMMethodMessageDelta     = "im.message.delta"
	IMMethodMessageDone      = "im.message.done"
	IMMethodToolUpdate       = "im.tool.update"
	IMMethodPermissionAsk    = "im.permission.ask"
	IMMethodSystemNotify     = "im.system.notify"
	IMMethodSessionUpsert    = "im.session.upsert"
	IMMethodSessionListReply = "im.session.list.reply"
	IMMethodSessionReadReply = "im.session.read.reply"
)

// --- Shared enum-like literals ---

type IMMessageRole string

const (
	IMRoleUser      IMMessageRole = "user"
	IMRoleAssistant IMMessageRole = "assistant"
	IMRoleSystem    IMMessageRole = "system"
)

type IMMessageKind string

const (
	IMKindText       IMMessageKind = "text"
	IMKindThought    IMMessageKind = "thought"
	IMKindTool       IMMessageKind = "tool"
	IMKindPermission IMMessageKind = "permission"
	IMKindPromptDone IMMessageKind = "prompt_result"
	IMKindSystem     IMMessageKind = "system"
)

type IMMessageStatus string

const (
	IMStatusStreaming   IMMessageStatus = "streaming"
	IMStatusDone        IMMessageStatus = "done"
	IMStatusNeedsAction IMMessageStatus = "needs_action"
	IMStatusFailed      IMMessageStatus = "failed"
)

type IMToolStatus string

const (
	IMToolStatusPending    IMToolStatus = "pending"
	IMToolStatusInProgress IMToolStatus = "in_progress"
	IMToolStatusCompleted  IMToolStatus = "completed"
	IMToolStatusFailed     IMToolStatus = "failed"
	IMToolStatusCancelled  IMToolStatus = "cancelled"
)

type IMContentType string

const (
	IMContentText  IMContentType = "text"
	IMContentImage IMContentType = "image"
	IMContentAudio IMContentType = "audio"
	IMContentFile  IMContentType = "file"
)

type IMContentBlock struct {
	Type        IMContentType `json:"type"`
	Text        string        `json:"text,omitempty"`
	MimeType    string        `json:"mimeType,omitempty"`
	Data        string        `json:"data,omitempty"`
	URI         string        `json:"uri,omitempty"`
	Name        string        `json:"name,omitempty"`
	Title       string        `json:"title,omitempty"`
	Description string        `json:"description,omitempty"`
	Size        int           `json:"size,omitempty"`
}

// --- IM -> Hub payloads ---

type IMChatSendPayload struct {
	ChatID      string           `json:"chatId"`
	SessionID   string           `json:"sessionId,omitempty"`
	Text        string           `json:"text,omitempty"`
	Blocks      []IMContentBlock `json:"blocks,omitempty"`
	ClientMsgID string           `json:"clientMsgId,omitempty"`
}

type IMCommandRunPayload struct {
	ChatID    string `json:"chatId"`
	SessionID string `json:"sessionId,omitempty"`
	Name      string `json:"name"`
	Args      string `json:"args,omitempty"`
	Raw       string `json:"raw,omitempty"`
}

type IMPermissionRespondPayload struct {
	ChatID    string `json:"chatId"`
	SessionID string `json:"sessionId,omitempty"`
	RequestID int64  `json:"requestId"`
	OptionID  string `json:"optionId"`
}

type IMSessionListPayload struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

type IMSessionReadPayload struct {
	ChatID      string `json:"chatId,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	BeforeMsgID string `json:"beforeMsgId,omitempty"`
}

// --- Hub -> IM payloads ---

type IMMessageDeltaEvent struct {
	ChatID    string          `json:"chatId"`
	SessionID string          `json:"sessionId"`
	MessageID string          `json:"messageId,omitempty"`
	Role      IMMessageRole   `json:"role"`
	Kind      IMMessageKind   `json:"kind"`
	Delta     string          `json:"delta"`
	Status    IMMessageStatus `json:"status,omitempty"`
	CreatedAt string          `json:"createdAt,omitempty"`
	UpdatedAt string          `json:"updatedAt,omitempty"`
}

type IMMessageDoneEvent struct {
	ChatID     string          `json:"chatId"`
	SessionID  string          `json:"sessionId"`
	MessageID  string          `json:"messageId,omitempty"`
	Status     IMMessageStatus `json:"status,omitempty"`
	StopReason string          `json:"stopReason,omitempty"`
	UpdatedAt  string          `json:"updatedAt,omitempty"`
}

type IMToolLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line,omitempty"`
}

type IMToolUpdateEvent struct {
	ChatID     string           `json:"chatId"`
	SessionID  string           `json:"sessionId"`
	ToolCallID string           `json:"toolCallId"`
	Title      string           `json:"title,omitempty"`
	Kind       string           `json:"kind,omitempty"`
	Status     IMToolStatus     `json:"status"`
	Command    string           `json:"command,omitempty"`
	Output     string           `json:"output,omitempty"`
	Input      map[string]any   `json:"input,omitempty"`     // rawInput, optional for audit/replay
	OutputRaw  map[string]any   `json:"outputRaw,omitempty"` // rawOutput, optional for audit/replay
	Locations  []IMToolLocation `json:"locations,omitempty"`
	UpdatedAt  string           `json:"updatedAt,omitempty"`
}

type IMPermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

type IMPermissionAskEvent struct {
	ChatID     string               `json:"chatId"`
	SessionID  string               `json:"sessionId"`
	RequestID  int64                `json:"requestId"`
	ToolCallID string               `json:"toolCallId,omitempty"`
	ToolTitle  string               `json:"toolTitle,omitempty"`
	ToolKind   string               `json:"toolKind,omitempty"`
	Options    []IMPermissionOption `json:"options"`
	CreatedAt  string               `json:"createdAt,omitempty"`
}

type IMSystemNotifyEvent struct {
	ChatID    string            `json:"chatId"`
	SessionID string            `json:"sessionId,omitempty"`
	Kind      string            `json:"kind"`
	Title     string            `json:"title,omitempty"`
	Body      string            `json:"body,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

type IMSessionSummary struct {
	ChatID       string `json:"chatId"`
	SessionID    string `json:"sessionId"`
	Title        string `json:"title,omitempty"`
	Preview      string `json:"preview,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
	MessageCount int    `json:"messageCount,omitempty"`
}

type IMSessionUpsertEvent struct {
	Session IMSessionSummary `json:"session"`
}

type IMSessionListReply struct {
	Items      []IMSessionSummary `json:"items"`
	NextCursor string             `json:"nextCursor,omitempty"`
}

type IMMessageItem struct {
	MessageID string          `json:"messageId"`
	Role      IMMessageRole   `json:"role"`
	Kind      IMMessageKind   `json:"kind"`
	Text      string          `json:"text,omitempty"`
	Status    IMMessageStatus `json:"status,omitempty"`
	CreatedAt string          `json:"createdAt,omitempty"`
	UpdatedAt string          `json:"updatedAt,omitempty"`
}

type IMSessionReadReply struct {
	Session IMSessionSummary `json:"session"`
	Items   []IMMessageItem  `json:"items"`
}
