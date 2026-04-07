package app

import (
	"context"
	"errors"

	"github.com/swm8023/wheelmaker/internal/im"
)

var ErrNotImplemented = errors.New("im app channel: not implemented")

type Channel struct {
	handler func(context.Context, string, string) error
}

func New() *Channel {
	return &Channel{}
}

func (c *Channel) ID() string { return "app" }

func (c *Channel) OnMessage(handler func(context.Context, string, string) error) {
	c.handler = handler
}

func (c *Channel) Send(context.Context, string, im.OutboundEvent) error {
	return ErrNotImplemented
}

func (c *Channel) RequestDecision(context.Context, string, im.DecisionRequest) (im.DecisionResult, error) {
	return im.DecisionResult{Outcome: "invalid"}, ErrNotImplemented
}

func (c *Channel) Run(context.Context) error {
	return ErrNotImplemented
}
