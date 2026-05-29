package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	logger "github.com/swm8023/wheelmaker/internal/shared"
)

const updaterRetryDelay = 10 * time.Minute
const updaterSignalPollInterval = 5 * time.Second
const updaterWaitHeartbeatInterval = 60 * time.Second
const updaterRoundTimeout = 20 * time.Minute

const (
	triggerReasonScheduled                  = "scheduled"
	triggerReasonManualSignalSkipWebPublish = "manual-signal-skip-web-publish"
	triggerReasonManualFullUpdate           = "manual-full-update"
	fullUpdateSignalToken                   = "full-update"
	skipWebPublishSignalToken               = "skip-web-publish"
)

var runtimeGOOS = runtime.GOOS

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

type updateRoundOptions struct {
	skipUpdate     bool
	skipWebPublish bool
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

	logger.Info("[updater] started daily schedule at %02d:%02d", hour, minute)
	for {
		next := nextRunTime(time.Now(), hour, minute)
		logger.Info("[updater] next run at %s", next.Format(time.RFC3339))

		triggerReason, waitErr := waitForTrigger(ctx, cfg.SignalFile, next)
		if waitErr != nil {
			return waitErr
		}
		logger.Info("[updater] trigger=%s", triggerReason)

		opts := updateRoundOptions{
			skipUpdate:     triggerReason == triggerReasonManualSignalSkipWebPublish,
			skipWebPublish: triggerReason == triggerReasonManualSignalSkipWebPublish,
		}
		roundCtx, cancelRound := context.WithTimeout(ctx, updaterRoundTimeout)
		err = runUpdateRoundWithOptions(roundCtx, cfg, runner, opts)
		cancelRound()
		if err != nil {
			logger.Error("[updater] update round failed: %v", err)
			retry := time.NewTimer(updaterRetryDelay)
			select {
			case <-ctx.Done():
				retry.Stop()
				return nil
			case <-retry.C:
				retryCtx, cancelRetry := context.WithTimeout(ctx, updaterRoundTimeout)
				retryErr := runUpdateRoundWithOptions(retryCtx, cfg, runner, opts)
				cancelRetry()
				if retryErr != nil {
					logger.Error("[updater] retry update round failed: %v", retryErr)
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
			return triggerReasonScheduled, nil
		case <-heartbeat.C:
			remaining := time.Until(next).Round(time.Second)
			if remaining < 0 {
				remaining = 0
			}
			logger.Info("[updater] waiting for next run (remaining=%s)", remaining.String())
		case <-ticker.C:
			reason, triggered, err := consumeManualSignal(signalPath)
			if err != nil {
				logger.Warn("[updater] manual trigger check failed: %v", err)
				continue
			}
			if triggered {
				logger.Info("[updater] manual trigger signal consumed: %s", signalPath)
				return reason, nil
			}
		}
	}
}

func consumeManualSignal(path string) (string, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("manual signal path is a directory: %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	if err := os.Remove(path); err != nil {
		return "", false, err
	}
	return parseManualSignalReason(string(raw)), true, nil
}

func parseManualSignalReason(raw string) string {
	if strings.Contains(strings.ToLower(raw), fullUpdateSignalToken) {
		return triggerReasonManualFullUpdate
	}
	if strings.Contains(strings.ToLower(raw), skipWebPublishSignalToken) {
		return triggerReasonManualSignalSkipWebPublish
	}
	return triggerReasonManualFullUpdate
}

func runUpdateRound(ctx context.Context, cfg UpdaterConfig, runner commandRunner, skipUpdate bool) error {
	return runUpdateRoundWithOptions(ctx, cfg, runner, updateRoundOptions{skipUpdate: skipUpdate})
}

func runUpdateRoundWithOptions(ctx context.Context, cfg UpdaterConfig, runner commandRunner, opts updateRoundOptions) error {
	logger.Info("[updater] run deploy cli begin")

	invocation, err := deployInvocationForOS(cfg, opts, runtimeGOOS)
	if err != nil {
		return err
	}
	if _, err := os.Stat(invocation.command); err != nil {
		return fmt.Errorf("wheelmaker-deploy missing: %w", err)
	}

	logger.Info("[updater] invoke %s %s", invocation.command, strings.Join(invocation.args, " "))

	_, err = runner.CombinedOutput(ctx, cfg.RepoDir, invocation.command, invocation.args...)
	if err != nil {
		return err
	}

	logger.Info("[updater] run deploy cli complete")
	return nil
}

type deployInvocation struct {
	command string
	args    []string
}

func deployInvocationForOS(cfg UpdaterConfig, opts updateRoundOptions, goos string) (deployInvocation, error) {
	binaryName := "wheelmaker-deploy"
	switch goos {
	case "windows":
		binaryName += ".exe"
	case "darwin", "linux":
	default:
		return deployInvocation{}, fmt.Errorf("unsupported updater platform: %s", goos)
	}
	args := []string{
		"bootstrap-update",
		"--repo", cfg.RepoDir,
		"--bin", cfg.InstallDir,
		"--time", cfg.DailyTime,
	}
	if opts.skipWebPublish {
		args = append(args, "--no-web")
	}
	return deployInvocation{
		command: filepath.Join(cfg.InstallDir, binaryName),
		args:    args,
	}, nil
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
	for _, name := range requiredCommandsForOS(runtimeGOOS) {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("required command not found: %s", name)
		}
	}
	return nil
}

func requiredCommandsForOS(goos string) []string {
	switch goos {
	case "windows", "darwin", "linux":
		return []string{}
	default:
		return nil
	}
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
