package app

import (
	"context"
	"errors"

	"github.com/swm8023/wheelmaker/internal/im2"
)

var ErrNotImplemented = errors.New("im2 app channel: not implemented")

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

func (c *Channel) Send(context.Context, string, im2.OutboundEvent) error {
	return ErrNotImplemented
}

func (c *Channel) RequestDecision(context.Context, string, im2.DecisionRequest) (im2.DecisionResult, error) {
	return im2.DecisionResult{Outcome: "invalid"}, ErrNotImplemented
}

func (c *Channel) Run(context.Context) error {
	return ErrNotImplemented
}
