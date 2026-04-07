package im2

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// OutboundKind identifies events published from Client to IM router.
type OutboundKind string

const (
	OutboundMessage      OutboundKind = "message"
	OutboundACPUpdate    OutboundKind = "acp_update"
	OutboundCommandReply OutboundKind = "command_reply"
)

// InboundKind identifies events delivered from IM router to Client.
type InboundKind string

const (
	InboundPrompt          InboundKind = "prompt"
	InboundPermissionReply InboundKind = "permission_reply"
	InboundCommand         InboundKind = "command"
)

// OutboundEvent is a protocol-agnostic event emitted by Client.
// Payload semantics are transparent to IM router.
type OutboundEvent struct {
	Kind            OutboundKind
	ClientSessionID string
	TargetRouteKey  string // empty means broadcast to all online chats of the session
	Text            string
	Payload         []byte
	Meta            map[string]string
}

// InboundEvent is a normalized inbound event from IM channels.
type InboundEvent struct {
	Kind            InboundKind
	IMType          string
	ChatID          string
	RouteKey        string
	ClientSessionID string
	MessageID       string
	Text            string
	Payload         []byte
	Meta            map[string]string
	ReceivedAt      time.Time
}

// IMRouteBinding is the lightweight runtime view of one IM route binding.
type IMRouteBinding struct {
	ProjectName     string
	RouteKey        string
	IMType          string
	ChatID          string
	ClientSessionID string
	Online          bool
	LastSeenAt      time.Time
	UpdatedAt       time.Time
}

// InboundHandler consumes inbound events routed by Router.
type InboundHandler func(ctx context.Context, event InboundEvent) error

// Channel is implemented by concrete IM integrations (feishu/console/mobile/etc).
// Router keeps ACP protocol transparent and only pushes normalized events to channels.
type Channel interface {
	PublishToChat(ctx context.Context, chatID string, event OutboundEvent) error
}

// BuildRouteKey builds normalized IM route key: <imType>:<chatID>.
func BuildRouteKey(imType, chatID string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(imType))
	c := strings.TrimSpace(chatID)
	if t == "" || c == "" {
		return "", fmt.Errorf("invalid route key parts: imType=%q chatID=%q", imType, chatID)
	}
	return t + ":" + c, nil
}

// ParseRouteKey parses normalized IM route key.
func ParseRouteKey(routeKey string) (imType, chatID string, ok bool) {
	v := strings.TrimSpace(routeKey)
	if v == "" {
		return "", "", false
	}
	left, right, found := strings.Cut(v, ":")
	if !found {
		return "", "", false
	}
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return "", "", false
	}
	return left, right, true
}
