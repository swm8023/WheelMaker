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
type Channel interface {
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

// DebugSender can send debug messages through IM-specific debug channel behavior.
// Implementations may format or route differently from normal text messages.
type DebugSender interface {
	SendDebug(chatID, text string) error
}

// SystemSender can send system-level messages through a dedicated rendering path
// when the platform supports distinguishing system vs agent text streams.
type SystemSender interface {
	SendSystem(chatID, text string) error
}

// DebugLoggerSetter installs an optional writer for unified debug logs.
type DebugLoggerSetter interface {
	SetDebugLogger(w io.Writer)
}

// ToolCallSender can render per-tool-call streaming cards/messages.
type ToolCallSender interface {
	SendToolCall(chatID string, update ToolCallUpdate) error
}

// DoneMarker can mark the final outbound message when a prompt stops.
type DoneMarker interface {
	MarkDone(chatID string) error
}

// StreamingMarker toggles a per-chat "streaming in progress" UI marker.
type StreamingMarker interface {
	SetStreaming(chatID string, active bool) error
}
