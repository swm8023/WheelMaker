package app

import (
	"context"
	"errors"

	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

var ErrNotImplemented = errors.New("im app channel: not implemented")

type Channel struct {
	onPrompt           func(context.Context, im.ChatRef, acp.SessionPromptParams) error
	onCommand          func(context.Context, im.ChatRef, im.Command) error
	onPermissionResult func(context.Context, im.ChatRef, int64, acp.PermissionResponse) error
}

func New() *Channel {
	return &Channel{}
}

func (c *Channel) ID() string { return "app" }

func (c *Channel) OnPrompt(handler func(context.Context, im.ChatRef, acp.SessionPromptParams) error) {
	c.onPrompt = handler
}

func (c *Channel) OnCommand(handler func(context.Context, im.ChatRef, im.Command) error) {
	c.onCommand = handler
}

func (c *Channel) OnPermissionResponse(handler func(context.Context, im.ChatRef, int64, acp.PermissionResponse) error) {
	c.onPermissionResult = handler
}

func (c *Channel) PublishSessionUpdate(context.Context, im.SendTarget, acp.SessionUpdateParams) error {
	return ErrNotImplemented
}

func (c *Channel) PublishPromptResult(context.Context, im.SendTarget, acp.SessionPromptResult) error {
	return ErrNotImplemented
}

func (c *Channel) PublishPermissionRequest(context.Context, im.SendTarget, int64, acp.PermissionRequestParams) error {
	return ErrNotImplemented
}

func (c *Channel) SystemNotify(context.Context, im.SendTarget, im.SystemPayload) error {
	return ErrNotImplemented
}

func (c *Channel) Run(context.Context) error {
	return ErrNotImplemented
}
