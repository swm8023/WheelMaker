package im

import (
	"context"
	"strings"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

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

type SendTarget struct {
	ChannelID string
	ChatID    string
	SessionID string
	Source    *ChatRef
}

type Command struct {
	Name string
	Args string
	Raw  string
}

type SystemPayload struct {
	Kind  string
	Title string
	Body  string
	Meta  map[string]string
}

type Channel interface {
	ID() string

	OnPrompt(func(ctx context.Context, source ChatRef, params acp.SessionPromptParams) error)
	OnCommand(func(ctx context.Context, source ChatRef, cmd Command) error)
	OnPermissionResponse(func(ctx context.Context, source ChatRef, requestID int64, result acp.PermissionResponse) error)

	PublishSessionUpdate(ctx context.Context, target SendTarget, params acp.SessionUpdateParams) error
	PublishPromptResult(ctx context.Context, target SendTarget, result acp.SessionPromptResult) error
	PublishPermissionRequest(ctx context.Context, target SendTarget, requestID int64, params acp.PermissionRequestParams) error
	SystemNotify(ctx context.Context, target SendTarget, payload SystemPayload) error

	Run(ctx context.Context) error
}

type InboundHandler interface {
	HandleIMPrompt(ctx context.Context, source ChatRef, params acp.SessionPromptParams) error
	HandleIMCommand(ctx context.Context, source ChatRef, cmd Command) error
	HandleIMPermissionResponse(ctx context.Context, source ChatRef, requestID int64, result acp.PermissionResponse) error
}

func ParseCommand(text string) (Command, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Command{}, false
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return Command{}, false
	}
	switch parts[0] {
	case "/use", "/cancel", "/status", "/mode", "/model", "/config", "/list", "/new", "/load":
		return Command{
			Name: parts[0],
			Args: strings.Join(parts[1:], " "),
			Raw:  trimmed,
		}, true
	default:
		return Command{}, false
	}
}
