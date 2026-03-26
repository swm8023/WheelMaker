// Command wheelmaker runs hub/registry workers and guardian mode.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/swm8023/wheelmaker/internal/hub"
	"github.com/swm8023/wheelmaker/internal/logger"
	"github.com/swm8023/wheelmaker/internal/registry"
)

const daemonWorkerArg = "--daemon-worker"
const hubWorkerArg = "--hub-worker"
const registryWorkerArg = "--registry-worker"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "wheelmaker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("wheelmaker", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	daemonMode := fs.Bool("d", false, "run guardian mode (checks service every 30 seconds)")
	daemonWorker := fs.Bool("daemon-worker", false, "internal: worker mode for guardian")
	hubWorker := fs.Bool("hub-worker", false, "internal: hub worker mode for guardian")
	registryWorker := fs.Bool("registry-worker", false, "internal: registry worker mode for guardian")
	registryServer := fs.Bool("registry-server", false, "run registry websocket server mode")
	registryAddr := fs.String("registry-addr", ":9630", "registry websocket listen address")
	registryToken := fs.String("registry-token", "", "registry shared token (optional)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	switch {
	case *registryServer:
		return runRegistryServer(*registryAddr, *registryToken)
	case *registryWorker:
		return runRegistryWorker()
	case *hubWorker:
		return runHubWorker()
	case *daemonWorker:
		return runHubWorker()
	case *daemonMode:
		return runGuardian(fs.Args())
	default:
		return runHubWorker()
	}
}

func runRegistryServer(addr, token string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := registry.New(registry.Config{
		Addr:  addr,
		Token: token,
	})
	return s.Run(ctx)
}

func runHubWorker() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	cfgPath := filepath.Join(home, ".wheelmaker", "config.json")
	statePath := filepath.Join(home, ".wheelmaker", "state.json")

	cfg, err := hub.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("cannot load config.json at %s: %w\n\nCreate one based on config.example.json in the project root.", cfgPath, err)
	}

	if err := logger.Setup(logger.Config{
		Level:        logger.ParseLevel(cfg.Log.Level),
		LogFile:      filepath.Join(home, ".wheelmaker", "hub.log"),
		DebugLogFile: filepath.Join(home, ".wheelmaker", "hub.debug.log"),
	}); err != nil {
		return fmt.Errorf("logger setup: %w", err)
	}
	defer logger.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	h := hub.New(cfg, statePath)
	if err := h.Start(ctx); err != nil {
		return err
	}
	defer h.Close()

	return h.Run(ctx)
}

func runRegistryWorker() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	cfgPath := filepath.Join(home, ".wheelmaker", "config.json")
	cfg, err := hub.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("cannot load config.json at %s: %w\n\nCreate one based on config.example.json in the project root.", cfgPath, err)
	}

	if err := logger.Setup(logger.Config{
		Level:        logger.ParseLevel(cfg.Log.Level),
		LogFile:      filepath.Join(home, ".wheelmaker", "registry.log"),
		DebugLogFile: filepath.Join(home, ".wheelmaker", "registry.debug.log"),
	}); err != nil {
		return fmt.Errorf("logger setup: %w", err)
	}
	defer logger.Close()

	host := cfg.Registry.Server
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Registry.Port
	if port == 0 {
		port = 9630
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	s := registry.New(registry.Config{
		Addr:  addr,
		Token: cfg.Registry.Token,
	})
	return s.Run(ctx)
}
