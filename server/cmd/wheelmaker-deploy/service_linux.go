//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	systemdHubService     = "wheelmaker-hub.service"
	systemdMonitorService = "wheelmaker-monitor.service"
	systemdUpdaterService = "wheelmaker-updater.service"
)

type serviceManager struct {
	cfg    deployConfig
	runner commandRunner
}

func newServiceManager(cfg deployConfig, runner commandRunner) serviceManager {
	return serviceManager{cfg: resolveDefaults(cfg), runner: runner}
}

func (m serviceManager) CheckDeployPrerequisites(ctx context.Context) error {
	if _, err := m.runner.Run(ctx, "", "systemctl", "--user", "show-environment"); err != nil {
		return fmt.Errorf("systemctl --user is not available for this session: %w", err)
	}
	userName := os.Getenv("USER")
	if strings.TrimSpace(userName) == "" {
		out, err := m.runner.Run(ctx, "", "id", "-un")
		if err != nil {
			return fmt.Errorf("resolve linux user: %w", err)
		}
		userName = strings.TrimSpace(out)
	}
	out, err := m.runner.Run(ctx, "", "loginctl", "show-user", userName, "-p", "Linger")
	if err != nil {
		return fmt.Errorf("check lingering: %w", err)
	}
	if !strings.Contains(strings.TrimSpace(out), "Linger=yes") {
		return errors.New(`linux deploy requires lingering so systemd user services survive logout; run: sudo loginctl enable-linger "$USER"`)
	}
	return nil
}

func (m serviceManager) Configure(ctx context.Context) error {
	unitDir := filepath.Join(m.cfg.HomeDir, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return fmt.Errorf("create systemd user dir: %w", err)
	}
	envFile := filepath.Join(wheelMakerHome(m.cfg), "systemd.env")
	if err := writeSystemdEnv(envFile); err != nil {
		return err
	}
	units := []struct {
		name        string
		description string
		binary      string
		args        string
	}{
		{systemdHubService, "WheelMaker Hub", filepath.Join(m.cfg.InstallDir, "wheelmaker"), "-d"},
		{systemdMonitorService, "WheelMaker Monitor", filepath.Join(m.cfg.InstallDir, "wheelmaker-monitor"), ""},
	}
	if !m.cfg.NoUpdater {
		args := fmt.Sprintf("--repo %s --install-dir %s --time %s", systemdQuote(m.cfg.RepoRoot), systemdQuote(m.cfg.InstallDir), systemdQuote(m.cfg.UpdaterTime))
		units = append(units, struct {
			name        string
			description string
			binary      string
			args        string
		}{systemdUpdaterService, "WheelMaker Updater", filepath.Join(m.cfg.InstallDir, "wheelmaker-updater"), args})
	}
	for _, unit := range units {
		body := linuxUnitContent(unit.description, m.cfg.RepoRoot, envFile, unit.binary, unit.args)
		if err := os.WriteFile(filepath.Join(unitDir, unit.name), []byte(body), 0o644); err != nil {
			return fmt.Errorf("write unit %s: %w", unit.name, err)
		}
	}
	if _, err := m.runner.Run(ctx, "", "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	for _, unit := range units {
		if _, err := m.runner.Run(ctx, "", "systemctl", "--user", "enable", unit.name); err != nil {
			return err
		}
	}
	return nil
}

func (m serviceManager) Start(ctx context.Context, includeUpdater bool) error {
	for _, service := range m.services(includeUpdater) {
		if _, err := m.runner.Run(ctx, "", "systemctl", "--user", "start", service); err != nil {
			return err
		}
	}
	return nil
}

func (m serviceManager) Stop(ctx context.Context, includeUpdater bool) error {
	for _, service := range m.services(includeUpdater) {
		_, _ = m.runner.Run(ctx, "", "systemctl", "--user", "stop", service)
	}
	return nil
}

func (m serviceManager) Restart(ctx context.Context, includeUpdater bool) error {
	for _, service := range m.services(includeUpdater) {
		if _, err := m.runner.Run(ctx, "", "systemctl", "--user", "restart", service); err != nil {
			return err
		}
	}
	return nil
}

func (m serviceManager) Status(ctx context.Context) error {
	for _, service := range []string{systemdHubService, systemdMonitorService, systemdUpdaterService} {
		if _, err := m.runner.Run(ctx, "", "systemctl", "--user", "show", service, "--property=LoadState,ActiveState,UnitFileState"); err != nil {
			return err
		}
	}
	return nil
}

func (m serviceManager) services(includeUpdater bool) []string {
	services := []string{systemdHubService, systemdMonitorService}
	if includeUpdater && !m.cfg.NoUpdater {
		services = append(services, systemdUpdaterService)
	}
	return services
}

func writeSystemdEnv(path string) error {
	body := fmt.Sprintf("HOME=%q\nPATH=%q\n", os.Getenv("HOME"), os.Getenv("PATH"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create systemd env dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write systemd env: %w", err)
	}
	return nil
}
