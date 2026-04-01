package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	shared "github.com/swm8023/wheelmaker/internal/shared"
	"github.com/swm8023/wheelmaker/internal/shared/winsvc"
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
	signalFile := fs.String("signal-file", "", "manual trigger signal file path (default: ~/.wheelmaker/update-now.signal)")
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
	stateDir := resolveStateDir(home, *installDir)
	if *signalFile == "" {
		*signalFile = filepath.Join(stateDir, "update-now.signal")
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
		LogFile:      filepath.Join(stateDir, "updater.log"),
		DebugLogFile: "",
	}); err != nil {
		return fmt.Errorf("logger setup: %w", err)
	}
	defer shared.Close()

	cfg := UpdaterConfig{
		RepoDir:    *repo,
		InstallDir: *installDir,
		DailyTime:  *at,
		SignalFile: *signalFile,
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

func runAsWindowsServiceIfNeeded(cfg UpdaterConfig) (bool, error) {
	return winsvc.RunIfWindowsService(
		updaterServiceName,
		func(ctx context.Context) error {
			return runUpdaterService(ctx, cfg)
		},
		nil,
	)
}

func runUpdaterService(ctx context.Context, cfg UpdaterConfig) error {
	return RunUpdater(ctx, cfg)
}

func resolveStateDir(home string, installDir string) string {
	trimmedInstall := strings.TrimSpace(installDir)
	if trimmedInstall != "" {
		cleanInstall := filepath.Clean(trimmedInstall)
		if strings.EqualFold(filepath.Base(cleanInstall), "bin") {
			return filepath.Dir(cleanInstall)
		}
		return cleanInstall
	}
	return filepath.Join(home, ".wheelmaker")
}
