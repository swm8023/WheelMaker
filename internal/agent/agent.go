// Package agent defines the interface for AI coding assistant agents.
package agent

import "context"

// Update represents a single streaming update from an agent.
type Update struct {
	// Type is one of: "text", "thought", "tool_call", "error"
	Type    string
	Content string
	Done    bool
	Err     error
}

// Agent represents an AI coding assistant that can receive prompts and stream responses.
type Agent interface {
	// Name returns the agent's identifier (e.g. "codex", "claude").
	Name() string

	// Prompt sends a prompt and returns a channel of streaming updates.
	// The caller must read the channel until a Update with Done=true is received.
	// The channel is closed after Done=true.
	Prompt(ctx context.Context, text string) (<-chan Update, error)

	// Cancel cancels any in-progress prompt.
	Cancel() error

	// SetMode switches the agent's operating mode.
	SetMode(modeID string) error

	// Close shuts down the agent and cleans up resources.
	Close() error
}
