// Command wheelmaker runs the WheelMaker daemon.
// It reads ~/.wheelmaker/config.json to configure projects and IM providers.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/swm8023/wheelmaker/internal/hub"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "wheelmaker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	logPath := filepath.Join(home, ".wheelmaker", "wheelmaker.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", logPath, err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags)

	cfgPath := filepath.Join(home, ".wheelmaker", "config.json")
	statePath := filepath.Join(home, ".wheelmaker", "state.json")

	cfg, err := hub.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("cannot load config.json at %s: %w\n\nCreate one based on config.example.json in the project root.", cfgPath, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	h := hub.New(cfg, statePath)
	if err := h.Start(ctx); err != nil {
		return err
	}
	defer h.Close()

	return h.Run(ctx)
}
