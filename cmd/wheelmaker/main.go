// Command wheelmaker runs the WheelMaker daemon.
// In MVP mode (no IM configured), it reads messages from stdin for testing.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/swm8023/wheelmaker/internal/hub"
	"github.com/swm8023/wheelmaker/internal/im"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "wheelmaker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// State file: ~/.wheelmaker/state.json
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	statePath := filepath.Join(home, ".wheelmaker", "state.json")

	store := hub.NewJSONStore(statePath)
	h := hub.New(store, nil) // no IM adapter in MVP

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := h.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	defer h.Close()

	fmt.Fprintln(os.Stderr, "WheelMaker ready. Type a message or /status, /use <agent>, /cancel. Ctrl+C to quit.")

	// CLI test mode: read messages from stdin.
	scanner := bufio.NewScanner(os.Stdin)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		fmt.Fprint(os.Stderr, "> ")
		if !scanner.Scan() {
			return nil
		}
		text := scanner.Text()
		if text == "" {
			continue
		}

		h.HandleMessage(im.Message{
			ChatID:    "cli",
			MessageID: "cli-msg",
			UserID:    "local",
			Text:      text,
		})
	}
}
