package im2

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
	Run(ctx context.Context) error
}

type InboundHandler interface {
	HandleIM2Inbound(ctx context.Context, event InboundEvent) error
}
