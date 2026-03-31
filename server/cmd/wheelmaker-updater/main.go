package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	shared "github.com/swm8023/wheelmaker/internal/shared"
)

const updaterServiceName = "WheelMakerUpdater"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "wheelmaker-updater: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("wheelmaker-updater", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	repo := fs.String("repo", "", "WheelMaker repository root")
	installDir := fs.String("install-dir", "", "WheelMaker install directory (default: ~/.wheelmaker/bin)")
	at := fs.String("time", "03:00", "daily update time in HH:mm")
	once := fs.Bool("once", false, "run one update round then exit")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	if *installDir == "" {
		*installDir = filepath.Join(home, ".wheelmaker", "bin")
	}
	if *repo == "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return fmt.Errorf("resolve repo path: %w", cwdErr)
		}
		*repo = cwd
	}

	if err := shared.Setup(shared.LoggerConfig{
		Level:        shared.LevelInfo,
		LogFile:      filepath.Join(home, ".wheelmaker", "auto_update.log"),
		DebugLogFile: "",
	}); err != nil {
		return fmt.Errorf("logger setup: %w", err)
	}
	defer shared.Close()

	cfg := UpdaterConfig{
		RepoDir:    *repo,
		InstallDir: *installDir,
		DailyTime:  *at,
		Once:       *once,
	}

	ranAsService, err := runAsWindowsServiceIfNeeded(cfg)
	if err != nil {
		return err
	}
	if ranAsService {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return RunUpdater(ctx, cfg)
}
