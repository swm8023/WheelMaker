package client

import (
	"context"

	"github.com/swm8023/wheelmaker/internal/im2"
)

// IM2Router is the client-facing bridge contract for IM 2.0 routing.
type IM2Router interface {
	HandleInbound(ctx context.Context, event im2.InboundEvent) error
	Publish(ctx context.Context, event im2.OutboundEvent) error
	RebindRouteKey(ctx context.Context, routeKey, clientSessionID string) error
}
