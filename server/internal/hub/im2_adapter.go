package hub

import (
	"context"
	"strings"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/im"
	"github.com/swm8023/wheelmaker/internal/im2"
)

// IM2Bridge routes inbound messages from legacy IM adapter into IM2 router.
// It does not modify legacy IM implementation details.
type IM2Bridge struct {
	adapter     *im.ImAdapter
	router      *im2.Router
	defaultType string
}

func NewIM2Bridge(adapter *im.ImAdapter, router *im2.Router, defaultType string) *IM2Bridge {
	return &IM2Bridge{
		adapter:     adapter,
		router:      router,
		defaultType: strings.ToLower(strings.TrimSpace(defaultType)),
	}
}

func (b *IM2Bridge) Start() {
	if b == nil || b.adapter == nil || b.router == nil {
		return
	}
	b.adapter.OnMessage(func(m im.Message) {
		kind := im2.InboundPrompt
		if strings.HasPrefix(strings.TrimSpace(m.Text), "/") {
			kind = im2.InboundCommand
		}
		_ = b.router.HandleInbound(context.Background(), im2.InboundEvent{
			Kind:       kind,
			IMType:     b.defaultType,
			ChatID:     m.ChatID,
			RouteKey:   m.RouteKey,
			MessageID:  m.MessageID,
			Text:       m.Text,
			ReceivedAt: time.Now().UTC(),
		})
	})
}

type im2OutboundChannel struct {
	adapter *im.ImAdapter
}

func (c *im2OutboundChannel) PublishToChat(_ context.Context, chatID string, event im2.OutboundEvent) error {
	if c == nil || c.adapter == nil {
		return nil
	}
	text := strings.TrimSpace(event.Text)
	if text == "" && len(event.Payload) > 0 {
		text = strings.TrimSpace(string(event.Payload))
	}
	if text == "" {
		return nil
	}
	return c.adapter.SendText(chatID, text)
}

var _ im2.Channel = (*im2OutboundChannel)(nil)
