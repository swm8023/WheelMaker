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
	"github.com/swm8023/wheelmaker/internal/registry"
	shared "github.com/swm8023/wheelmaker/internal/shared"
	"github.com/swm8023/wheelmaker/internal/shared/winsvc"
)

const daemonWorkerArg = "--daemon-worker"
const hubWorkerArg = "--hub-worker"
const registryWorkerArg = "--registry-worker"
const wheelmakerWindowsServiceName = "WheelMaker"

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

	if !*registryServer && !*registryWorker && !*hubWorker && !*daemonWorker {
		ranAsService, err := runAsWindowsServiceIfNeeded(fs.Args())
		if err != nil {
			return err
		}
		if ranAsService {
			return nil
		}
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
		restoreStdio, err := redirectProcessStdioToDevNull()
		if err != nil {
			return err
		}
		defer restoreStdio()
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

	cfg, err := shared.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("cannot load config.json at %s: %w\n\nCreate one based on config.example.json in the project root.", cfgPath, err)
	}
	hubLog := filepath.Join(wheelmakerLogDir(home), "hub.log")
	hubDebugLog := filepath.Join(wheelmakerLogDir(home), "hub.debug.log")
	_ = shared.MigrateLegacyLogFile(filepath.Join(home, ".wheelmaker", "hub.log"), hubLog)
	_ = shared.MigrateLegacyLogFile(filepath.Join(home, ".wheelmaker", "hub.debug.log"), hubDebugLog)

	if err := shared.Setup(shared.LoggerConfig{
		Level:        shared.ParseLevel(cfg.Log.Level),
		LogFile:      hubLog,
		DebugLogFile: hubDebugLog,
	}); err != nil {
		return fmt.Errorf("logger setup: %w", err)
	}
	defer shared.Close()

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
	cfg, err := shared.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("cannot load config.json at %s: %w\n\nCreate one based on config.example.json in the project root.", cfgPath, err)
	}
	regLog := filepath.Join(wheelmakerLogDir(home), "registry.log")
	regDebugLog := filepath.Join(wheelmakerLogDir(home), "registry.debug.log")
	_ = shared.MigrateLegacyLogFile(filepath.Join(home, ".wheelmaker", "registry.log"), regLog)
	_ = shared.MigrateLegacyLogFile(filepath.Join(home, ".wheelmaker", "registry.debug.log"), regDebugLog)

	if err := shared.Setup(shared.LoggerConfig{
		Level:        shared.ParseLevel(cfg.Log.Level),
		LogFile:      regLog,
		DebugLogFile: regDebugLog,
	}); err != nil {
		return fmt.Errorf("logger setup: %w", err)
	}
	defer shared.Close()

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

func wheelmakerLogDir(home string) string {
	return filepath.Join(home, ".wheelmaker", "log")
}

func runAsWindowsServiceIfNeeded(workerArgs []string) (bool, error) {
	sanitizedArgs := sanitizeWorkerArgs(workerArgs)
	return winsvc.RunIfWindowsService(
		wheelmakerWindowsServiceName,
		func(ctx context.Context) error {
			return runGuardianWithContext(ctx, sanitizedArgs)
		},
		nil,
	)
}
