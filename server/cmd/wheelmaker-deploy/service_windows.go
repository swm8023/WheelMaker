//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	windowsHubService     = "WheelMaker"
	windowsMonitorService = "WheelMakerMonitor"
	windowsUpdaterService = "WheelMakerUpdater"
)

type serviceManager struct {
	cfg    deployConfig
	runner commandRunner
}

func newServiceManager(cfg deployConfig, runner commandRunner) serviceManager {
	return serviceManager{cfg: resolveDefaults(cfg), runner: runner}
}

func (m serviceManager) CheckDeployPrerequisites(ctx context.Context) error {
	if m.cfg.NoConfig {
		return nil
	}
	out, err := m.runner.Run(ctx, "", "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)")
	if err != nil {
		return fmt.Errorf("check administrator privileges: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(out), "true") {
		return errors.New("windows service configuration requires an Administrator terminal; rerun deploy.bat or wheelmaker-deploy deploy elevated")
	}
	return nil
}

func (m serviceManager) Configure(ctx context.Context) error {
	if err := m.ensureService(ctx, windowsHubService, filepath.Join(m.cfg.InstallDir, "wheelmaker.exe"), ""); err != nil {
		return err
	}
	if err := m.ensureService(ctx, windowsMonitorService, filepath.Join(m.cfg.InstallDir, "wheelmaker-monitor.exe"), ""); err != nil {
		return err
	}
	if !m.cfg.NoUpdater {
		if err := m.ensureService(ctx, windowsUpdaterService, filepath.Join(m.cfg.InstallDir, "wheelmaker-updater.exe"), windowsUpdaterArgs(m.cfg.RepoRoot, m.cfg.InstallDir, m.cfg.UpdaterTime)); err != nil {
			return err
		}
	}
	return nil
}

func (m serviceManager) Start(ctx context.Context, includeUpdater bool) error {
	for _, name := range m.serviceNames(includeUpdater) {
		if _, err := m.runner.Run(ctx, "", "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", fmt.Sprintf("Start-Service -Name %s -ErrorAction Stop", psQuote(name))); err != nil {
			return err
		}
	}
	return nil
}

func (m serviceManager) Stop(ctx context.Context, includeUpdater bool) error {
	for _, name := range m.serviceNames(includeUpdater) {
		_, _ = m.runner.Run(ctx, "", "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", fmt.Sprintf("$svc=Get-Service -Name %s -ErrorAction SilentlyContinue; if ($null -ne $svc -and $svc.Status -ne 'Stopped') { Stop-Service -Name %s -Force -ErrorAction Stop }", psQuote(name), psQuote(name)))
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
	for _, name := range []string{windowsHubService, windowsMonitorService, windowsUpdaterService} {
		if _, err := m.runner.Run(ctx, "", "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", fmt.Sprintf("Get-Service -Name %s -ErrorAction SilentlyContinue | Format-Table -AutoSize", psQuote(name))); err != nil {
			return err
		}
	}
	return nil
}

func (m serviceManager) ensureService(ctx context.Context, name string, binary string, args string) error {
	_ = m.Stop(ctx, name == windowsUpdaterService)
	_, _ = m.runner.Run(ctx, "", "sc.exe", "delete", name)
	binPath := `"` + binary + `"`
	if strings.TrimSpace(args) != "" {
		binPath += " " + strings.TrimSpace(args)
	}
	if _, err := m.runner.Run(ctx, "", "sc.exe", "create", name, "binPath=", binPath, "start=", "auto"); err != nil {
		return fmt.Errorf("create service %s: %w", name, err)
	}
	return nil
}

func (m serviceManager) serviceNames(includeUpdater bool) []string {
	names := []string{windowsHubService, windowsMonitorService}
	if includeUpdater && !m.cfg.NoUpdater {
		names = append(names, windowsUpdaterService)
	}
	return names
}

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
