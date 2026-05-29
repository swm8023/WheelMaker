package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type runMode string

const (
	modeDeploy          runMode = "deploy"
	modeBootstrapUpdate runMode = "bootstrap-update"
	modeUpdate          runMode = "update"
	modeUpgradeUpdater  runMode = "upgrade-updater"
	modeService         runMode = "service"
	modeDoctor          runMode = "doctor"
)

type deployConfig struct {
	Mode          runMode
	ServiceAction string
	RepoRoot      string
	HomeDir       string
	InstallDir    string
	BuildRoot     string
	UpdaterTime   string
	NoPull        bool
	NoNPM         bool
	NoBuild       bool
	NoInstall     bool
	NoRestart     bool
	NoConfig      bool
	NoWeb         bool
	NoUpdater     bool
}

type commandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, text)
	}
	return strings.TrimSpace(string(out)), nil
}

type deployService interface {
	CheckDeployPrerequisites(context.Context) error
	Configure(context.Context) error
	Start(context.Context, bool) error
	Stop(context.Context, bool) error
	Restart(context.Context, bool) error
	Status(context.Context) error
}

type noopServices struct{}

func (noopServices) CheckDeployPrerequisites(context.Context) error { return nil }
func (noopServices) Configure(context.Context) error                { return nil }
func (noopServices) Start(context.Context, bool) error              { return nil }
func (noopServices) Stop(context.Context, bool) error               { return nil }
func (noopServices) Restart(context.Context, bool) error            { return nil }
func (noopServices) Status(context.Context) error                   { return nil }

type deployDeps struct {
	Runner   commandRunner
	Services deployService
	Now      func() time.Time
	Record   func(string)
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "wheelmaker-deploy: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}
	switch cfg.Mode {
	case modeDeploy:
		return runDeploy(ctx, cfg)
	case modeBootstrapUpdate:
		return runBootstrapUpdate(ctx, cfg)
	case modeUpdate:
		return runUpdate(ctx, cfg)
	case modeUpgradeUpdater:
		return errors.New("upgrade-updater is not implemented in this transitional CLI")
	case modeService:
		return runService(ctx, cfg)
	case modeDoctor:
		return runDoctor(ctx, cfg)
	default:
		return fmt.Errorf("unsupported mode: %s", cfg.Mode)
	}
}

func parseArgs(args []string) (deployConfig, error) {
	if len(args) == 0 {
		return deployConfig{}, errors.New("command is required")
	}
	mode := runMode(args[0])
	cfg := deployConfig{
		Mode:        mode,
		UpdaterTime: "03:00",
	}
	fs := flag.NewFlagSet("wheelmaker-deploy "+string(mode), flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.RepoRoot, "repo", "", "WheelMaker repository root")
	fs.StringVar(&cfg.InstallDir, "bin", "", "WheelMaker install directory")
	fs.StringVar(&cfg.UpdaterTime, "time", cfg.UpdaterTime, "updater daily time in HH:mm")
	fs.BoolVar(&cfg.NoPull, "no-pull", false, "skip git pull")
	fs.BoolVar(&cfg.NoNPM, "no-npm", false, "skip app npm dependency sync")
	fs.BoolVar(&cfg.NoBuild, "no-build", false, "skip Go builds")
	fs.BoolVar(&cfg.NoInstall, "no-install", false, "skip binary install")
	fs.BoolVar(&cfg.NoRestart, "no-restart", false, "skip service restart")
	fs.BoolVar(&cfg.NoConfig, "no-config", false, "skip service configuration")
	fs.BoolVar(&cfg.NoWeb, "no-web", false, "skip Web publish")
	fs.BoolVar(&cfg.NoUpdater, "no-updater", false, "skip updater build/install/config")
	if err := fs.Parse(args[1:]); err != nil {
		return deployConfig{}, err
	}

	switch mode {
	case modeDeploy, modeBootstrapUpdate, modeUpdate, modeUpgradeUpdater, modeDoctor:
	case modeService:
		rest := fs.Args()
		if len(rest) == 0 {
			return deployConfig{}, errors.New("service action is required")
		}
		cfg.ServiceAction = rest[0]
	default:
		return deployConfig{}, fmt.Errorf("unsupported command: %s", mode)
	}

	if cfg.NoWeb {
		cfg.NoNPM = true
	}
	if mode == modeUpdate {
		cfg.NoPull = true
		cfg.NoConfig = true
		cfg.NoUpdater = true
	}
	if mode == modeBootstrapUpdate {
		cfg.NoNPM = true
		cfg.NoInstall = true
		cfg.NoRestart = true
		cfg.NoConfig = true
		cfg.NoWeb = true
		cfg.NoUpdater = true
	}
	return cfg, nil
}

func runDeploy(ctx context.Context, cfg deployConfig) error {
	return runDeployWithDeps(ctx, cfg, defaultDeps(cfg))
}

func runBootstrapUpdate(ctx context.Context, cfg deployConfig) error {
	return runBootstrapUpdateWithDeps(ctx, cfg, defaultDeps(cfg))
}

func runUpdate(ctx context.Context, cfg deployConfig) error {
	return runUpdateWithDeps(ctx, cfg, defaultDeps(cfg))
}

func runService(_ context.Context, cfg deployConfig) error {
	if cfg.ServiceAction == "uninstall" {
		return errors.New("service uninstall is not implemented in this transitional CLI")
	}
	ctx := context.Background()
	cfg = resolveDefaults(cfg)
	deps := defaultDeps(cfg)
	switch cfg.ServiceAction {
	case "start":
		return deps.Services.Start(ctx, true)
	case "stop":
		return deps.Services.Stop(ctx, true)
	case "restart":
		return deps.Services.Restart(ctx, true)
	case "status":
		return deps.Services.Status(ctx)
	default:
		return fmt.Errorf("unsupported service action: %s", cfg.ServiceAction)
	}
}

func runDoctor(context.Context, deployConfig) error {
	return errors.New("doctor is not implemented")
}

func defaultDeps(cfg deployConfig) deployDeps {
	runner := execRunner{}
	return deployDeps{
		Runner:   runner,
		Services: newServiceManager(cfg, runner),
		Now:      func() time.Time { return time.Now().UTC() },
	}
}

func (d deployDeps) record(event string) {
	if d.Record != nil {
		d.Record(event)
	}
}

func runDeployWithDeps(ctx context.Context, cfg deployConfig, deps deployDeps) error {
	cfg.Mode = modeDeploy
	cfg = resolveDefaults(cfg)
	deps = resolveDeps(cfg, deps)
	if err := deps.Services.CheckDeployPrerequisites(ctx); err != nil {
		return err
	}
	if err := pullLatest(ctx, cfg, deps); err != nil {
		return err
	}
	if err := syncAppNPM(ctx, cfg, deps); err != nil {
		return err
	}
	if err := buildBinaries(ctx, cfg, deps, !cfg.NoUpdater); err != nil {
		return err
	}
	if err := publishWeb(ctx, cfg, deps); err != nil {
		return err
	}
	if !cfg.NoRestart {
		if err := deps.Services.Stop(ctx, false); err != nil {
			return err
		}
	}
	if err := installBuiltBinaries(cfg, deps, !cfg.NoUpdater); err != nil {
		return err
	}
	if _, err := ensureConfig(cfg, deps); err != nil {
		return err
	}
	if err := writeHelperWrappers(cfg, deps); err != nil {
		return err
	}
	if !cfg.NoConfig {
		if err := deps.Services.Configure(ctx); err != nil {
			return err
		}
	}
	if err := writeReleaseManifest(ctx, cfg, deps); err != nil {
		return err
	}
	if !cfg.NoRestart {
		if err := deps.Services.Start(ctx, true); err != nil {
			return err
		}
	}
	return nil
}

func runUpdateWithDeps(ctx context.Context, cfg deployConfig, deps deployDeps) error {
	cfg.Mode = modeUpdate
	cfg.NoUpdater = true
	cfg.NoConfig = true
	cfg = resolveDefaults(cfg)
	deps = resolveDeps(cfg, deps)
	if err := pullLatest(ctx, cfg, deps); err != nil {
		return err
	}
	if err := syncAppNPM(ctx, cfg, deps); err != nil {
		return err
	}
	if err := buildBinaries(ctx, cfg, deps, false); err != nil {
		return err
	}
	if err := publishWeb(ctx, cfg, deps); err != nil {
		return err
	}
	if !cfg.NoRestart {
		if err := deps.Services.Stop(ctx, false); err != nil {
			return err
		}
	}
	if err := installBuiltBinaries(cfg, deps, false); err != nil {
		return err
	}
	if err := writeReleaseManifest(ctx, cfg, deps); err != nil {
		return err
	}
	if !cfg.NoRestart {
		if err := deps.Services.Start(ctx, false); err != nil {
			return err
		}
	}
	return nil
}

func runBootstrapUpdateWithDeps(ctx context.Context, cfg deployConfig, deps deployDeps) error {
	cfg.Mode = modeBootstrapUpdate
	cfg = resolveDefaults(cfg)
	deps = resolveDeps(cfg, deps)
	if err := pullLatest(ctx, cfg, deps); err != nil {
		return err
	}
	next := filepath.Join(cfg.BuildRoot, "bootstrap", binaryName("wheelmaker-deploy-next"))
	if err := os.MkdirAll(filepath.Dir(next), 0o755); err != nil {
		return fmt.Errorf("create bootstrap build dir: %w", err)
	}
	if _, err := deps.Runner.Run(ctx, filepath.Join(cfg.RepoRoot, "server"), "go", "build", "-o", next, "./cmd/wheelmaker-deploy"); err != nil {
		return err
	}
	args := []string{"wheelmaker-deploy-next", "update", "--repo", cfg.RepoRoot, "--bin", cfg.InstallDir, "--time", cfg.UpdaterTime, "--no-pull", "--no-config", "--no-updater"}
	_, err := deps.Runner.Run(ctx, cfg.RepoRoot, "exec", args...)
	return err
}

func resolveDeps(cfg deployConfig, deps deployDeps) deployDeps {
	if deps.Runner == nil {
		deps.Runner = execRunner{}
	}
	if deps.Services == nil {
		deps.Services = newServiceManager(cfg, deps.Runner)
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}
	_ = cfg
	return deps
}

func resolveDefaults(cfg deployConfig) deployConfig {
	if strings.TrimSpace(cfg.HomeDir) == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.HomeDir = home
		}
	}
	if strings.TrimSpace(cfg.RepoRoot) == "" {
		if cwd, err := os.Getwd(); err == nil {
			cfg.RepoRoot = cwd
		}
	}
	if abs, err := filepath.Abs(cfg.RepoRoot); err == nil {
		cfg.RepoRoot = abs
	}
	if strings.TrimSpace(cfg.InstallDir) == "" {
		cfg.InstallDir = filepath.Join(cfg.HomeDir, ".wheelmaker", "bin")
	}
	if strings.TrimSpace(cfg.BuildRoot) == "" {
		cfg.BuildRoot = filepath.Join(cfg.HomeDir, ".wheelmaker", "build", runtime.GOOS+"_"+runtime.GOARCH)
	}
	if strings.TrimSpace(cfg.UpdaterTime) == "" {
		cfg.UpdaterTime = "03:00"
	}
	return cfg
}

func pullLatest(ctx context.Context, cfg deployConfig, deps deployDeps) error {
	if cfg.NoPull {
		return nil
	}
	branch, err := deps.Runner.Run(ctx, cfg.RepoRoot, "git", "branch", "--show-current")
	if err != nil {
		return err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return errors.New("repository is in detached HEAD state; cannot pull latest automatically")
	}
	_, err = deps.Runner.Run(ctx, cfg.RepoRoot, "git", "pull", "--ff-only", "origin", branch)
	return err
}

func syncAppNPM(ctx context.Context, cfg deployConfig, deps deployDeps) error {
	if cfg.NoWeb || cfg.NoNPM {
		return nil
	}
	_, err := deps.Runner.Run(ctx, filepath.Join(cfg.RepoRoot, "app"), "npm", "ci", "--include=dev")
	return err
}

func buildBinaries(ctx context.Context, cfg deployConfig, deps deployDeps, includeUpdater bool) error {
	if cfg.NoBuild {
		return nil
	}
	builds := []struct {
		label string
		pkg   string
	}{
		{"wheelmaker", "./cmd/wheelmaker"},
		{"wheelmaker-monitor", "./cmd/wheelmaker-monitor"},
	}
	if includeUpdater {
		builds = append(builds, struct {
			label string
			pkg   string
		}{"wheelmaker-updater", "./cmd/wheelmaker-updater"})
	}
	builds = append(builds, struct {
		label string
		pkg   string
	}{"wheelmaker-deploy", "./cmd/wheelmaker-deploy"})
	for _, build := range builds {
		out := filepath.Join(cfg.BuildRoot, binaryName(build.label))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("create build dir: %w", err)
		}
		if _, err := deps.Runner.Run(ctx, filepath.Join(cfg.RepoRoot, "server"), "go", "build", "-o", out, build.pkg); err != nil {
			return err
		}
	}
	return nil
}

func publishWeb(ctx context.Context, cfg deployConfig, deps deployDeps) error {
	if cfg.NoWeb || cfg.NoBuild || cfg.NoInstall {
		return nil
	}
	_, err := deps.Runner.Run(ctx, filepath.Join(cfg.RepoRoot, "app"), "npm", "run", "build:web:release")
	return err
}

func installBuiltBinaries(cfg deployConfig, deps deployDeps, includeUpdater bool) error {
	if cfg.NoInstall {
		return nil
	}
	names := []string{"wheelmaker", "wheelmaker-monitor"}
	if includeUpdater {
		names = append(names, "wheelmaker-updater")
	}
	names = append(names, "wheelmaker-deploy")
	if err := os.MkdirAll(cfg.InstallDir, 0o755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}
	for _, name := range names {
		deps.record("install " + name)
	}
	return nil
}

func ensureConfig(cfg deployConfig, deps deployDeps) (bool, error) {
	path := filepath.Join(wheelMakerHome(cfg), "config.json")
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat config: %w", err)
	}
	config := map[string]any{
		"projects": []map[string]string{
			{
				"name": "WheelMaker",
				"path": cfg.RepoRoot,
			},
		},
		"registry": map[string]any{
			"listen": true,
			"port":   9630,
			"server": "127.0.0.1",
			"token":  "wheelmaker-local-token",
			"hubId":  "local-hub",
		},
		"monitor": map[string]any{
			"port": 9631,
		},
		"log": map[string]string{
			"level": "warn",
		},
	}
	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return false, fmt.Errorf("write config: %w", err)
	}
	deps.record("write config")
	return true, nil
}

func writeHelperWrappers(cfg deployConfig, deps deployDeps) error {
	home := wheelMakerHome(cfg)
	binDir := filepath.Join(home, "bin")
	windowsDeploy := filepath.Join(binDir, "wheelmaker-deploy.exe")
	unixDeploy := filepath.Join(binDir, "wheelmaker-deploy")
	windows := map[string]string{
		"start.bat":   wrapperBAT(windowsDeploy, "start"),
		"stop.bat":    wrapperBAT(windowsDeploy, "stop"),
		"restart.bat": wrapperBAT(windowsDeploy, "restart"),
		"status.bat":  wrapperBAT(windowsDeploy, "status"),
	}
	unix := map[string]string{
		"start.sh":   wrapperSH(unixDeploy, "start"),
		"stop.sh":    wrapperSH(unixDeploy, "stop"),
		"restart.sh": wrapperSH(unixDeploy, "restart"),
		"status.sh":  wrapperSH(unixDeploy, "status"),
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return fmt.Errorf("create helper dir: %w", err)
	}
	for name, body := range windows {
		if err := os.WriteFile(filepath.Join(home, name), []byte(body), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	for name, body := range unix {
		path := filepath.Join(home, name)
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	deps.record("write wrappers")
	return nil
}

func writeReleaseManifest(ctx context.Context, cfg deployConfig, deps deployDeps) error {
	if cfg.NoBuild || cfg.NoInstall || cfg.NoWeb {
		return nil
	}
	sha, err := deps.Runner.Run(ctx, cfg.RepoRoot, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	branch, err := deps.Runner.Run(ctx, cfg.RepoRoot, "git", "branch", "--show-current")
	if err != nil {
		return err
	}
	manifest := map[string]any{
		"schemaVersion": 1,
		"repo":          cfg.RepoRoot,
		"branch":        strings.TrimSpace(branch),
		"remote":        "origin",
		"sha":           strings.TrimSpace(sha),
		"publishedAt":   deps.Now().UTC().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release manifest: %w", err)
	}
	path := filepath.Join(wheelMakerHome(cfg), "release.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create release dir: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write release manifest: %w", err)
	}
	deps.record("write release")
	return nil
}

func wheelMakerHome(cfg deployConfig) string {
	return filepath.Dir(cfg.InstallDir)
}

func wrapperBAT(deployPath string, action string) string {
	return fmt.Sprintf("@echo off\r\nsetlocal\r\n\"%s\" service %s %%*\r\nexit /b %%errorlevel%%\r\n", deployPath, action)
}

func wrapperSH(deployPath string, action string) string {
	return fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\n%q service %s \"$@\"\n", deployPath, action)
}

func linuxUnitContent(description string, workingDir string, envFile string, binary string, args string) string {
	execStart := systemdQuote(binary)
	if strings.TrimSpace(args) != "" {
		execStart += " " + strings.TrimSpace(args)
	}
	return fmt.Sprintf(`[Unit]
Description=%s

[Service]
Type=simple
WorkingDirectory=%s
EnvironmentFile=%s
ExecStart=%s
Restart=always
RestartSec=5
StartLimitIntervalSec=300
StartLimitBurst=5

[Install]
WantedBy=default.target
`, description, workingDir, envFile, execStart)
}

func launchAgentPlistContent(label string, workingDir string, binary string, args []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>`)
	b.WriteString(xmlEscape(label))
	b.WriteString(`</string>
  <key>WorkingDirectory</key>
  <string>`)
	b.WriteString(xmlEscape(workingDir))
	b.WriteString(`</string>
  <key>ProgramArguments</key>
  <array>
    <string>`)
	b.WriteString(xmlEscape(binary))
	b.WriteString("</string>\n")
	for _, arg := range args {
		b.WriteString("    <string>")
		b.WriteString(xmlEscape(arg))
		b.WriteString("</string>\n")
	}
	b.WriteString(`  </array>
  <key>KeepAlive</key>
  <true/>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`)
	return b.String()
}

func windowsUpdaterArgs(repo string, bin string, dailyTime string) string {
	return fmt.Sprintf(`--repo "%s" --install-dir "%s" --time "%s"`, strings.ReplaceAll(repo, `"`, `\"`), strings.ReplaceAll(bin, `"`, `\"`), strings.ReplaceAll(dailyTime, `"`, `\"`))
}

func systemdQuote(value string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func xmlEscape(value string) string {
	return html.EscapeString(value)
}

func binaryName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		return name + ".exe"
	}
	return name
}
