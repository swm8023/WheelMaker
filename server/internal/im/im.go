// Package im defines the interface for instant messaging channels.
package im

import (
	"context"
	"io"
)

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

// Channel abstracts an IM platform for sending and receiving messages.
// All methods are required; implementations that don't support a feature
// should provide a no-op or text-fallback implementation.
type Channel interface {
	// OnMessage registers the handler to be called for each received message.
	OnMessage(handler MessageHandler)

	// OnCardAction registers the handler called for interactive card button events.
	// Implementations that don't support card actions should provide a no-op.
	OnCardAction(func(CardActionEvent))

	// SendText sends a plain text message to the given chat.
	SendText(chatID, text string) error

	// SendCard sends a rich interactive card to the given chat.
	SendCard(chatID string, card Card) error

	// UpdateCard updates an existing card message in place.
	// Implementations that don't support in-place updates should send a new card.
	UpdateCard(chatID, messageID string, card Card) error

	// SendReaction adds an emoji reaction to a message.
	SendReaction(messageID, emoji string) error

	// SendDebug sends a debug-channel message. Implementations may render it
	// differently from normal text (e.g. smaller font, different colour).
	SendDebug(chatID, text string) error

	// SendSystem sends a system-level message through a dedicated rendering path.
	// Implementations that don't distinguish system vs agent text may call SendText.
	SendSystem(chatID, text string) error

	// SendOptions sends a structured decision prompt (title, body, selectable options).
	SendOptions(chatID, title, body string, options []DecisionOption, meta map[string]string) error

	// SendToolCall renders a tool-call stream update (card or text fallback).
	SendToolCall(chatID string, update ToolCallUpdate) error

	// MarkDone marks the final outbound message when a prompt stops.
	// Implementations that have no such concept should return nil.
	MarkDone(chatID string) error

	// Run starts the event loop. It blocks until ctx is cancelled.
	Run(ctx context.Context) error
}

// DebugLoggerSetter installs an optional writer for unified debug logs.
// Implemented by ImAdapter; not required on Channel implementations.
type DebugLoggerSetter interface {
	SetDebugLogger(w io.Writer)
}
