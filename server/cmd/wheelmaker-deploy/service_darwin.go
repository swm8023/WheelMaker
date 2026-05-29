//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const (
	launchHubLabel     = "com.wheelmaker.hub"
	launchMonitorLabel = "com.wheelmaker.monitor"
	launchUpdaterLabel = "com.wheelmaker.updater"
)

type serviceManager struct {
	cfg    deployConfig
	runner commandRunner
}

func newServiceManager(cfg deployConfig, runner commandRunner) serviceManager {
	return serviceManager{cfg: resolveDefaults(cfg), runner: runner}
}

func (m serviceManager) CheckDeployPrerequisites(ctx context.Context) error {
	_, err := m.runner.Run(ctx, "", "launchctl", "print", launchDomain())
	return err
}

func (m serviceManager) Configure(ctx context.Context) error {
	dir := filepath.Join(m.cfg.HomeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	agents := []struct {
		label  string
		binary string
		args   []string
	}{
		{launchHubLabel, filepath.Join(m.cfg.InstallDir, "wheelmaker"), []string{"-d"}},
		{launchMonitorLabel, filepath.Join(m.cfg.InstallDir, "wheelmaker-monitor"), nil},
	}
	if !m.cfg.NoUpdater {
		agents = append(agents, struct {
			label  string
			binary string
			args   []string
		}{launchUpdaterLabel, filepath.Join(m.cfg.InstallDir, "wheelmaker-updater"), []string{"--repo", m.cfg.RepoRoot, "--install-dir", m.cfg.InstallDir, "--time", m.cfg.UpdaterTime}})
	}
	for _, agent := range agents {
		path := launchPlistPath(m.cfg.HomeDir, agent.label)
		body := launchAgentPlistContent(agent.label, m.cfg.RepoRoot, agent.binary, agent.args)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func (m serviceManager) Start(ctx context.Context, includeUpdater bool) error {
	for _, label := range m.labels(includeUpdater) {
		_, _ = m.runner.Run(ctx, "", "launchctl", "bootout", launchTarget(label))
		if _, err := m.runner.Run(ctx, "", "launchctl", "bootstrap", launchDomain(), launchPlistPath(m.cfg.HomeDir, label)); err != nil {
			return err
		}
		if _, err := m.runner.Run(ctx, "", "launchctl", "kickstart", "-k", launchTarget(label)); err != nil {
			return err
		}
	}
	return nil
}

func (m serviceManager) Stop(ctx context.Context, includeUpdater bool) error {
	for _, label := range m.labels(includeUpdater) {
		_, _ = m.runner.Run(ctx, "", "launchctl", "bootout", launchTarget(label))
	}
	return nil
}

func (m serviceManager) Restart(ctx context.Context, includeUpdater bool) error {
	if err := m.Stop(ctx, includeUpdater); err != nil {
		return err
	}
	return m.Start(ctx, includeUpdater)
}

func (m serviceManager) Status(ctx context.Context) error {
	for _, label := range []string{launchHubLabel, launchMonitorLabel, launchUpdaterLabel} {
		if _, err := m.runner.Run(ctx, "", "launchctl", "print", launchTarget(label)); err != nil {
			return err
		}
	}
	return nil
}

func (m serviceManager) labels(includeUpdater bool) []string {
	labels := []string{launchHubLabel, launchMonitorLabel}
	if includeUpdater && !m.cfg.NoUpdater {
		labels = append(labels, launchUpdaterLabel)
	}
	return labels
}

func launchDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func launchTarget(label string) string {
	return launchDomain() + "/" + label
}

func launchPlistPath(home string, label string) string {
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}
