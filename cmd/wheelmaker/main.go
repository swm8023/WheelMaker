// Command wheelmaker runs the WheelMaker daemon.
// In MVP mode (no IM configured), it reads messages from stdin for testing.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/swm8023/wheelmaker/internal/adapter/codex"
	"github.com/swm8023/wheelmaker/internal/client"
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

	store := client.NewJSONStore(statePath)
	c := client.New(store, nil) // no IM adapter in MVP

	// Register the Codex adapter (reads ExePath from state config if set).
	c.RegisterAdapter(codex.NewAdapter(codex.Config{}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := c.Start(ctx); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	defer c.Close()

	return c.Run(ctx) // blocks: CLI mode drives stdin loop
}
