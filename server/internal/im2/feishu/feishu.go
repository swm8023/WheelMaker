package feishu

import (
	"context"
	"fmt"

	hubim "github.com/swm8023/wheelmaker/internal/hub/im"
	hubfeishu "github.com/swm8023/wheelmaker/internal/hub/im/feishu"
	"github.com/swm8023/wheelmaker/internal/im2"
)

type Config = hubfeishu.Config

type Channel struct {
	inner *hubfeishu.Channel
}

func New(cfg Config) *Channel {
	return &Channel{inner: hubfeishu.New(cfg)}
}

func (c *Channel) ID() string { return "feishu" }

func (c *Channel) OnMessage(handler func(context.Context, string, string) error) {
	c.inner.OnMessage(func(msg hubim.Message) {
		if handler != nil {
			_ = handler(context.Background(), msg.ChatID, msg.Text)
		}
	})
}

func (c *Channel) Send(ctx context.Context, chatID string, event im2.OutboundEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	text, ok := event.Payload.(string)
	if !ok {
		text = fmt.Sprint(event.Payload)
	}
	switch event.Kind {
	case im2.OutboundACP:
		return c.inner.Send(chatID, text, hubim.TextDebug)
	case im2.OutboundSystem:
		return c.inner.Send(chatID, text, hubim.TextSystem)
	default:
		return c.inner.Send(chatID, text, hubim.TextNormal)
	}
}

func (c *Channel) Run(ctx context.Context) error {
	return c.inner.Run(ctx)
}
