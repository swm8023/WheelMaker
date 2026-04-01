package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	shared "github.com/swm8023/wheelmaker/internal/shared"
)

const updaterRetryDelay = 10 * time.Minute
const updaterSignalPollInterval = 5 * time.Second
const updaterWaitHeartbeatInterval = 60 * time.Second
const updaterRoundTimeout = 20 * time.Minute

type UpdaterConfig struct {
	RepoDir    string
	InstallDir string
	DailyTime  string
	SignalFile string
	Once       bool
}

type commandRunner interface {
	CombinedOutput(ctx context.Context, dir string, name string, args ...string) (string, error)
}

type osCommandRunner struct{}

func (osCommandRunner) CombinedOutput(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, trimmed)
	}
	return strings.TrimSpace(string(out)), nil
}

func RunUpdater(ctx context.Context, cfg UpdaterConfig) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if err := validateCommands(); err != nil {
		return err
	}

	hour, minute, err := parseDailyTime(cfg.DailyTime)
	if err != nil {
		return err
	}

	runner := osCommandRunner{}
	if cfg.Once {
		return runUpdateRound(ctx, cfg, runner, false)
	}

	shared.Info("[updater] started daily schedule at %02d:%02d", hour, minute)
	for {
		next := nextRunTime(time.Now(), hour, minute)
		shared.Info("[updater] next run at %s", next.Format(time.RFC3339))

		triggerReason, waitErr := waitForTrigger(ctx, cfg.SignalFile, next)
		if waitErr != nil {
			return waitErr
		}
		shared.Info("[updater] trigger=%s", triggerReason)

		skipUpdate := triggerReason == "manual-signal"
		roundCtx, cancelRound := context.WithTimeout(ctx, updaterRoundTimeout)
		err = runUpdateRound(roundCtx, cfg, runner, skipUpdate)
		cancelRound()
		if err != nil {
			shared.Error("[updater] update round failed: %v", err)
			retry := time.NewTimer(updaterRetryDelay)
			select {
			case <-ctx.Done():
				retry.Stop()
				return nil
			case <-retry.C:
				retryCtx, cancelRetry := context.WithTimeout(ctx, updaterRoundTimeout)
				retryErr := runUpdateRound(retryCtx, cfg, runner, skipUpdate)
				cancelRetry()
				if retryErr != nil {
					shared.Error("[updater] retry update round failed: %v", retryErr)
				}
			}
		}
	}
}

func waitForTrigger(ctx context.Context, signalPath string, next time.Time) (string, error) {
	timer := time.NewTimer(time.Until(next))
	ticker := time.NewTicker(updaterSignalPollInterval)
	heartbeat := time.NewTicker(updaterWaitHeartbeatInterval)
	defer timer.Stop()
	defer ticker.Stop()
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", nil
		case <-timer.C:
			return "scheduled", nil
		case <-heartbeat.C:
			remaining := time.Until(next).Round(time.Second)
			if remaining < 0 {
				remaining = 0
			}
			shared.Info("[updater] waiting for next run (remaining=%s)", remaining.String())
		case <-ticker.C:
			triggered, err := consumeManualSignal(signalPath)
			if err != nil {
				shared.Warn("[updater] manual trigger check failed: %v", err)
				continue
			}
			if triggered {
				shared.Info("[updater] manual trigger signal consumed: %s", signalPath)
				return "manual-signal", nil
			}
		}
	}
}

func consumeManualSignal(path string) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("manual signal path is a directory: %s", path)
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}

func runUpdateRound(ctx context.Context, cfg UpdaterConfig, runner commandRunner, skipUpdate bool) error {
	shared.Info("[updater] run refresh script begin")

	refreshScript := filepath.Join(cfg.RepoDir, "scripts", "refresh_server.ps1")
	if _, err := os.Stat(refreshScript); err != nil {
		return fmt.Errorf("refresh script missing: %w", err)
	}

	args := []string{
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", refreshScript,
		"-InstallDir", cfg.InstallDir,
		"-SkipUpdaterInstall",
		"-SkipServiceConfig",
	}
	if skipUpdate {
		args = append(args, "-SkipUpdate")
	}
	shared.Info("[updater] invoke powershell %s", strings.Join(args, " "))

	_, err := runner.CombinedOutput(ctx, cfg.RepoDir, "powershell", args...)
	if err != nil {
		return err
	}

	shared.Info("[updater] run refresh script complete")
	return nil
}

func validateConfig(cfg UpdaterConfig) error {
	if strings.TrimSpace(cfg.RepoDir) == "" {
		return errors.New("repo path is required")
	}
	repo := strings.TrimSpace(cfg.RepoDir)
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return fmt.Errorf("repo is invalid (missing .git): %w", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "server", "go.mod")); err != nil {
		return fmt.Errorf("repo is invalid (missing server/go.mod): %w", err)
	}
	if strings.TrimSpace(cfg.InstallDir) == "" {
		return errors.New("install-dir is required")
	}
	if err := os.MkdirAll(cfg.InstallDir, 0o755); err != nil {
		return fmt.Errorf("create install-dir: %w", err)
	}
	return nil
}

func validateCommands() error {
	for _, name := range []string{"powershell"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("required command not found: %s", name)
		}
	}
	return nil
}

func parseDailyTime(v string) (int, int, error) {
	v = strings.TrimSpace(v)
	parts := strings.Split(v, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, 0, fmt.Errorf("invalid time format %q (expected HH:mm)", v)
	}
	t, err := time.Parse("15:04", v)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid time %q: %w", v, err)
	}
	return t.Hour(), t.Minute(), nil
}

func nextRunTime(now time.Time, hour int, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func shortSHA(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 8 {
		return v
	}
	return v[:8]
}
