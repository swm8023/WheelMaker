// Package im defines the interface for instant messaging platform providers.
package im

import "context"

// Message represents an incoming message from an IM platform.
type Message struct {
	ChatID    string
	MessageID string
	UserID    string
	Text      string
}

// Card represents a rich interactive message card (platform-specific format).
type Card map[string]any

// MessageHandler is a callback invoked when a message is received.
type MessageHandler func(Message)

// Provider abstracts an IM platform for sending and receiving messages.
type Provider interface {
	// OnMessage registers the handler to be called for each received message.
	OnMessage(handler MessageHandler)

	// SendText sends a plain text message to the given chat.
	SendText(chatID, text string) error

	// SendCard sends a rich interactive card to the given chat.
	SendCard(chatID string, card Card) error

	// SendReaction adds an emoji reaction to a message.
	SendReaction(messageID, emoji string) error

	// Run starts the event loop. It blocks until ctx is cancelled.
	Run(ctx context.Context) error
}
