package feishu

import (
	"encoding/json"

	"github.com/swm8023/wheelmaker/internal/im"
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
	Options []DecisionOption
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
	TextDebug
	TextSystem
)

type ToolCallContent struct {
	Type       string          `json:"type"`
	Content    json.RawMessage `json:"content,omitempty"`
	TerminalID string          `json:"terminalId,omitempty"`
	Path       string          `json:"path,omitempty"`
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

type DecisionOption = im.DecisionOption
