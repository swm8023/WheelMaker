// Package im defines the interface for instant messaging channels.
package im

import (
	"context"
)

// Message represents an incoming message from an IM platform.
type Message struct {
	ChatID    string
	MessageID string
	UserID    string
	Text      string
	RouteKey  string // IM-provided routing key; defaults to ChatID if empty
}

// Card is the sealed sum type for all card payloads sent via Channel.SendCard.
// Use the concrete variant types (RawCard, OptionsCard, ToolCallCard) to construct values.
type Card interface{ isCard() }

// RawCard is a platform-native card built directly as a key-value map.
// Use for pre-built card JSON such as help pages and stream cards.
type RawCard map[string]any

func (RawCard) isCard() {}

// OptionsCard presents a decision prompt with selectable options.
// Channel implementations render it according to their UI capabilities.
type OptionsCard struct {
	Title   string
	Body    string
	Options []DecisionOption
	Meta    map[string]string
}

func (OptionsCard) isCard() {}

// ToolCallCard conveys a tool-call stream update.
// Channel implementations may render a rich interactive card or a plain-text fallback.
type ToolCallCard struct {
	Update ToolCallUpdate
}

func (ToolCallCard) isCard() {}

// MessageHandler is a callback invoked when a message is received.
type MessageHandler func(Message)

// TextKind distinguishes the rendering path for outbound text messages.
type TextKind uint8

const (
	TextNormal  TextKind = iota // regular agent reply
	TextThought                 // model thinking / reasoning stream
	TextDebug                   // debug channel (collapsible, smaller font, etc.)
	TextSystem                  // system/lifecycle notice (visually distinct from agent text)
)

// Channel abstracts an IM platform for sending and receiving messages.
// All methods are required; implementations that don't support a feature
// should provide a no-op or fallback to SendText/Send.
type Channel interface {
	// OnMessage registers the handler to be called for each received message.
	OnMessage(handler MessageHandler)

	// OnCardAction registers the handler called for interactive card button events.
	// Implementations that don't support card actions should provide a no-op.
	OnCardAction(func(CardActionEvent))

	// Send sends a text message. kind controls the rendering path.
	Send(chatID, text string, kind TextKind) error

	// SendCard sends (or updates) a card. If messageID is empty a new card is
	// posted; otherwise the card identified by messageID is updated in place.
	// Implementations that don't support in-place update should post a new card.
	// card must be one of: RawCard, OptionsCard, ToolCallCard.
	SendCard(chatID, messageID string, card Card) error

	// SendReaction adds an emoji reaction to a message.
	SendReaction(messageID, emoji string) error

	// MarkDone marks the final outbound message when a prompt stops.
	// Implementations that have no such concept should return nil.
	MarkDone(chatID string) error

	// Run starts the event loop. It blocks until ctx is cancelled.
	Run(ctx context.Context) error
}
