package feishu

import (
	"encoding/json"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type Message struct {
	ChatID    string
	MessageID string
	UserID    string
	Text      string
	RouteKey  string
}

type MessageHandler func(Message)

type Card interface{ isCard() }

type RawCard map[string]any

func (RawCard) isCard() {}

type OptionsCard struct {
	Title   string
	Body    string
	Options []PermissionOption
	Meta    map[string]string
}

func (OptionsCard) isCard() {}

type ToolCallCard struct {
	Update ToolCallUpdate
}

func (ToolCallCard) isCard() {}

type TextKind uint8

const (
	TextNormal TextKind = iota
	TextThought
	TextSystem
	TextDivider // insert divider in unified stream
)

type ToolCallContent struct {
	Type       string          `json:"type"`
	Content    json.RawMessage `json:"content,omitempty"`
	TerminalID string          `json:"terminalId,omitempty"`
	Path       string          `json:"path,omitempty"`
	OldText    *string         `json:"oldText,omitempty"`
	NewText    string          `json:"newText,omitempty"`
}

type ToolCallUpdate struct {
	SessionUpdate   string            `json:"sessionUpdate"`
	ToolCallID      string            `json:"toolCallId"`
	Title           string            `json:"title,omitempty"`
	Kind            string            `json:"kind,omitempty"`
	Status          string            `json:"status,omitempty"`
	RawInput        json.RawMessage   `json:"rawInput,omitempty"`
	RawOutput       json.RawMessage   `json:"rawOutput,omitempty"`
	ToolCallContent []ToolCallContent `json:"toolCallContent,omitempty"`
}

type CardActionEvent struct {
	ChatID    string
	MessageID string
	UserID    string
	Tag       string
	Option    string
	Value     map[string]string
}

type PermissionOption = acp.PermissionOption
