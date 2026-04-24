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
	Kind     string
	Title    string
	Body     string
	Meta     map[string]string
	HelpCard *HelpCardPayload
}

type HelpOption struct {
	Label   string
	Command string
	Value   string
	MenuID  string
}

type HelpMenu struct {
	Title   string
	Body    string
	Options []HelpOption
	Parent  string
}

type HelpModel struct {
	Title    string
	Body     string
	Options  []HelpOption
	RootMenu string
	Menus    map[string]HelpMenu
}

type HelpCardPayload struct {
	Model  HelpModel
	MenuID string
	Page   int
}

type Channel interface {
	ID() string

	OnPrompt(func(ctx context.Context, source ChatRef, params acp.SessionPromptParams) error)
	OnCommand(func(ctx context.Context, source ChatRef, cmd Command) error)

	PublishSessionUpdate(ctx context.Context, target SendTarget, params acp.SessionUpdateParams) error
	PublishPromptResult(ctx context.Context, target SendTarget, result acp.SessionPromptResult) error
	SystemNotify(ctx context.Context, target SendTarget, payload SystemPayload) error

	Run(ctx context.Context) error
}

type InboundHandler interface {
	HandleIMPrompt(ctx context.Context, source ChatRef, params acp.SessionPromptParams) error
	HandleIMCommand(ctx context.Context, source ChatRef, cmd Command) error
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
	case "/use", "/cancel", "/status", "/mode", "/model", "/config", "/list", "/new", "/load", "/help":
		return Command{
			Name: parts[0],
			Args: strings.Join(parts[1:], " "),
			Raw:  trimmed,
		}, true
	default:
		return Command{}, false
	}
}
