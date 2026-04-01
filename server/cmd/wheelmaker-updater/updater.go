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

type UpdaterConfig struct {
	RepoDir    string
	InstallDir string
	DailyTime  string
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
		return runUpdateRound(ctx, cfg, runner)
	}

	shared.Info("[updater] started daily schedule at %02d:%02d", hour, minute)
	for {
		next := nextRunTime(time.Now(), hour, minute)
		wait := time.Until(next)
		shared.Info("[updater] next run at %s", next.Format(time.RFC3339))

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}

		if err := runUpdateRound(ctx, cfg, runner); err != nil {
			shared.Error("[updater] update round failed: %v", err)
			retry := time.NewTimer(updaterRetryDelay)
			select {
			case <-ctx.Done():
				retry.Stop()
				return nil
			case <-retry.C:
				if retryErr := runUpdateRound(ctx, cfg, runner); retryErr != nil {
					shared.Error("[updater] retry update round failed: %v", retryErr)
				}
			}
		}
	}
}

func runUpdateRound(ctx context.Context, cfg UpdaterConfig, runner commandRunner) error {
	shared.Info("[updater] update check begin")

	if _, err := runner.CombinedOutput(ctx, cfg.RepoDir, "git", "fetch", "origin"); err != nil {
		return err
	}

	branch, err := runner.CombinedOutput(ctx, cfg.RepoDir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "HEAD" {
		return errors.New("git repository is in detached HEAD state")
	}

	localHead, err := runner.CombinedOutput(ctx, cfg.RepoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	remoteHead, err := runner.CombinedOutput(ctx, cfg.RepoDir, "git", "rev-parse", "origin/"+branch)
	if err != nil {
		return err
	}

	if strings.TrimSpace(localHead) == strings.TrimSpace(remoteHead) {
		shared.Info("[updater] already up-to-date branch=%s head=%s", branch, shortSHA(localHead))
		return nil
	}

	shared.Info("[updater] updates found branch=%s %s -> %s", branch, shortSHA(localHead), shortSHA(remoteHead))
	if _, err := runner.CombinedOutput(ctx, cfg.RepoDir, "git", "pull", "--ff-only", "origin", branch); err != nil {
		return err
	}

	refreshScript := filepath.Join(cfg.RepoDir, "scripts", "refresh_server.ps1")
	if _, err := os.Stat(refreshScript); err != nil {
		return fmt.Errorf("refresh script missing: %w", err)
	}

	_, err = runner.CombinedOutput(
		ctx,
		cfg.RepoDir,
		"powershell",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", refreshScript,
		"-SkipGitPull",
		"-InstallDir", cfg.InstallDir,
		"-SkipUpdaterInstall",
		"-SkipServiceConfig",
	)
	if err != nil {
		return err
	}

	shared.Info("[updater] deploy complete")
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
	for _, name := range []string{"git", "go", "powershell"} {
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
