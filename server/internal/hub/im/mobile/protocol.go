// Package mobile implements an im.Channel backed by a WebSocket HTTP server.
// Mobile clients (iOS/Android) connect over WebSocket and exchange JSON messages.
package mobile

import "github.com/swm8023/wheelmaker/internal/im"

// inboundMsg is an App→Daemon WebSocket message.
type inboundMsg struct {
	Type       string `json:"type"`                 // "auth"|"message"|"option"|"ping"
	Token      string `json:"token,omitempty"`      // required for "auth"
	Text       string `json:"text,omitempty"`       // required for "message"
	DecisionID string `json:"decisionId,omitempty"` // required for "option"
	OptionID   string `json:"optionId,omitempty"`   // required for "option"
}

// outboundMsg is a Daemon→App WebSocket message.
type outboundMsg struct {
	Type       string           `json:"type"` // "text"|"card"|"options"|"debug"|"pong"|"error"|"ready"|"auth_required"
	ChatID     string           `json:"chatId,omitempty"`
	Text       string           `json:"text,omitempty"`
	Title      string           `json:"title,omitempty"`
	Body       string           `json:"body,omitempty"`
	Card       any              `json:"card,omitempty"`
	Options    []outboundOption `json:"options,omitempty"`
	DecisionID string           `json:"decisionId,omitempty"` // set on "options"; app must echo back in "option"
	Message    string           `json:"message,omitempty"`    // error description
}

// outboundOption is one item in an "options" message.
type outboundOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var _ im.Channel = (*Channel)(nil)
