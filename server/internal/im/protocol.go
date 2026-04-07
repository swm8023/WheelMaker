package im

import "context"

type ChatRef struct {
	ChannelID string
	ChatID    string
}

type BindOptions struct {
	Watch bool
}

type InboundEvent struct {
	ChannelID string
	ChatID    string
	Text      string
	SessionID string
}

type OutboundKind string

const (
	OutboundMessage OutboundKind = "message"
	OutboundACP     OutboundKind = "acp"
	OutboundSystem  OutboundKind = "system"
)

type OutboundEvent struct {
	Kind    OutboundKind
	Payload any
}

type TextPayload struct {
	Text string
}

type ACPPayload struct {
	SessionID  string
	UpdateType string
	Text       string
	Raw        []byte
}

type DecisionKind string

const (
	DecisionPermission DecisionKind = "permission"
	DecisionConfirm    DecisionKind = "confirm"
	DecisionSingle     DecisionKind = "single"
	DecisionInput      DecisionKind = "input"
)

type DecisionOption struct {
	ID    string
	Label string
	Value string
}

type DecisionRequest struct {
	SessionID string
	Kind      DecisionKind
	Title     string
	Body      string
	Options   []DecisionOption
	Meta      map[string]string
	Hint      map[string]string
}

type DecisionResult struct {
	Outcome  string
	OptionID string
	Value    string
	ActorID  string
	Source   string
}

type SendTarget struct {
	ChannelID string
	ChatID    string
	SessionID string
	Source    *ChatRef
}

type Channel interface {
	ID() string
	OnMessage(func(ctx context.Context, chatID string, text string) error)
	Send(ctx context.Context, chatID string, event OutboundEvent) error
	RequestDecision(ctx context.Context, chatID string, req DecisionRequest) (DecisionResult, error)
	Run(ctx context.Context) error
}

type InboundHandler interface {
	HandleIMInbound(ctx context.Context, event InboundEvent) error
}
