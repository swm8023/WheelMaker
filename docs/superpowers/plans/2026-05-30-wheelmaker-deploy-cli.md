# WheelMaker Deploy CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a cross-platform `wheelmaker-deploy` Go CLI and move new deploy/update orchestration to it while preserving old refresh scripts for compatibility.

**Architecture:** The new command lives in `server/cmd/wheelmaker-deploy` with command parsing and pipeline orchestration in `main.go`, plus build-tagged service files for Windows, macOS, Linux, and unsupported platforms. Top-level deploy wrappers call the CLI directly, and `wheelmaker-updater` calls the CLI's `bootstrap-update` command instead of refresh scripts.

**Tech Stack:** Go 1.26, PowerShell source checks, Bash source checks, Windows Services through PowerShell/sc.exe, macOS LaunchAgents through `launchctl`, Linux `systemd --user`.

---

## File Map

- Create `server/cmd/wheelmaker-deploy/main.go`: command parsing, mode defaults, command runner abstraction, path resolution, pull/build/npm/web/install/config/wrapper/manifest pipeline, reserved commands.
- Create `server/cmd/wheelmaker-deploy/service_windows.go`: Windows elevation check, service install/status/start/stop/restart helpers, service argument construction.
- Create `server/cmd/wheelmaker-deploy/service_darwin.go`: LaunchAgent plist generation, launchctl status/start/stop/restart helpers.
- Create `server/cmd/wheelmaker-deploy/service_linux.go`: systemd user unit generation, lingering checks, systemctl status/start/stop/restart helpers, `systemd.env` generation.
- Create `server/cmd/wheelmaker-deploy/service_other.go`: unsupported service implementation for other GOOS targets.
- Create `server/cmd/wheelmaker-deploy/main_test.go`: platform-neutral tests for option parsing, mode defaults, pipeline ordering, config generation, wrappers, manifest writing, install rules, bootstrap delegation, reserved commands.
- Modify `server/cmd/wheelmaker-updater/updater.go`: replace refresh script invocation with `wheelmaker-deploy bootstrap-update`.
- Modify `server/cmd/wheelmaker-updater/updater_test.go`: update command expectations for Windows/macOS/Linux and missing deploy CLI behavior.
- Modify `deploy.bat`: top-level Windows wrapper builds `wheelmaker-deploy.exe` when missing and calls it directly.
- Modify `deploy.sh`: top-level macOS/Linux wrapper builds `wheelmaker-deploy` when missing and calls it directly.
- Modify `scripts/test_deploy_bat.ps1`: assert direct CLI invocation and no `scripts\refresh_server.ps1` dependency.
- Modify `scripts/test_deploy_sh.ps1`: assert direct CLI invocation and no refresh script dispatch.
- Modify `server/config.example.json`: align default semantic with local `WheelMaker` project and local registry listener.
- Modify `README.md`: document `wheelmaker-deploy`, new deploy wrappers, Linux lingering requirement, old refresh scripts as compatibility path.
- Add or update source checks only where they protect the new behavior; do not modify `scripts/refresh_server*.sh` or `scripts/refresh_server.ps1`.

## Task 1: CLI Skeleton and Mode Defaults

**Files:**
- Create: `server/cmd/wheelmaker-deploy/main.go`
- Create: `server/cmd/wheelmaker-deploy/main_test.go`
- Create: `server/cmd/wheelmaker-deploy/service_other.go`

- [ ] **Step 1: Write failing tests for parsing and mode defaults**

Add tests in `server/cmd/wheelmaker-deploy/main_test.go` covering:

```go
func TestParseDeployDefaults(t *testing.T) {
	cfg := parseDeployArgsForTest(t, []string{"deploy", "--repo", "C:/repo", "--bin", "C:/bin", "--time", "04:30"})
	if cfg.Mode != modeDeploy || cfg.RepoRoot != "C:/repo" || cfg.InstallDir != "C:/bin" || cfg.UpdaterTime != "04:30" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.NoPull || cfg.NoNPM || cfg.NoBuild || cfg.NoInstall || cfg.NoRestart || cfg.NoConfig || cfg.NoWeb || cfg.NoUpdater {
		t.Fatalf("deploy defaults disabled work: %+v", cfg)
	}
}

func TestParseUpdateDefaults(t *testing.T) {
	cfg := parseDeployArgsForTest(t, []string{"update"})
	if cfg.Mode != modeUpdate {
		t.Fatalf("mode=%v", cfg.Mode)
	}
	if !cfg.NoPull || !cfg.NoConfig || !cfg.NoUpdater {
		t.Fatalf("update should imply no pull/config/updater: %+v", cfg)
	}
}

func TestReservedCommandsReturnNotImplemented(t *testing.T) {
	for _, args := range [][]string{{"upgrade-updater"}, {"service", "uninstall"}} {
		err := runDeployCLIForTest(t, args)
		if err == nil || !strings.Contains(err.Error(), "not implemented") {
			t.Fatalf("%v err=%v, want not implemented", args, err)
		}
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-deploy
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement minimal command skeleton**

Create `main.go` with:

```go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
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
	Mode        runMode
	ServiceAction string
	RepoRoot    string
	InstallDir  string
	UpdaterTime string
	NoPull      bool
	NoNPM       bool
	NoBuild     bool
	NoInstall   bool
	NoRestart   bool
	NoConfig    bool
	NoWeb       bool
	NoUpdater   bool
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
```

Implement `parseArgs` with `flag.NewFlagSet`, short option names from the spec, and mode-specific defaults:

- `update` implies `NoPull=true`, `NoConfig=true`, `NoUpdater=true`.
- `bootstrap-update` implies `NoNPM=true`, `NoInstall=true`, `NoRestart=true`, `NoConfig=true`, `NoWeb=true`, `NoUpdater=true`.
- `--no-web` also implies skipping npm sync during pipeline execution.

Create `service_other.go`:

```go
//go:build !windows && !darwin && !linux

package main

import (
	"context"
	"errors"
)

type serviceManager struct{}

func newServiceManager(deployConfig) serviceManager { return serviceManager{} }
func (serviceManager) CheckDeployPrerequisites(context.Context) error { return errors.New("services are unsupported on this platform") }
func (serviceManager) Configure(context.Context) error { return errors.New("services are unsupported on this platform") }
func (serviceManager) Start(context.Context, bool) error { return errors.New("services are unsupported on this platform") }
func (serviceManager) Stop(context.Context, bool) error { return errors.New("services are unsupported on this platform") }
func (serviceManager) Restart(context.Context, bool) error { return errors.New("services are unsupported on this platform") }
func (serviceManager) Status(context.Context) error { return errors.New("services are unsupported on this platform") }
```

- [ ] **Step 4: Run tests and verify skeleton passes**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-deploy
```

Expected: PASS for parser and reserved-command tests.

- [ ] **Step 5: Commit**

```powershell
git add server/cmd/wheelmaker-deploy
git commit -m "feat: add deploy cli skeleton"
```

## Task 2: Core Pipeline Without Platform Services

**Files:**
- Modify: `server/cmd/wheelmaker-deploy/main.go`
- Modify: `server/cmd/wheelmaker-deploy/main_test.go`

- [ ] **Step 1: Write failing pipeline ordering tests**

Add tests using a fake command runner and temp directories:

```go
func TestDeployPipelineOrder(t *testing.T) {
	h := newDeployHarness(t)
	h.cfg.Mode = modeDeploy
	if err := runDeployWithDeps(context.Background(), h.cfg, h.deps); err != nil {
		t.Fatalf("runDeployWithDeps: %v", err)
	}
	want := []string{
		"git pull --ff-only origin main",
		"npm ci --include=dev",
		"go build wheelmaker",
		"go build wheelmaker-monitor",
		"go build wheelmaker-updater",
		"go build wheelmaker-deploy",
		"npm run build:web:release",
		"service stop hub-monitor",
		"install wheelmaker",
		"install wheelmaker-monitor",
		"install wheelmaker-updater",
		"install wheelmaker-deploy",
		"write config",
		"write wrappers",
		"service configure",
		"write release",
		"service start all",
	}
	if diff := cmpStringSlices(h.events, want); diff != "" {
		t.Fatal(diff)
	}
}

func TestUpdatePipelineSkipsUpdaterAndConfig(t *testing.T) {
	h := newDeployHarness(t)
	h.cfg.Mode = modeUpdate
	h.cfg.NoPull = true
	h.cfg.NoConfig = true
	h.cfg.NoUpdater = true
	if err := runUpdateWithDeps(context.Background(), h.cfg, h.deps); err != nil {
		t.Fatalf("runUpdateWithDeps: %v", err)
	}
	assertEventsDoNotContain(t, h.events, "wheelmaker-updater")
	assertEventsDoNotContain(t, h.events, "service configure")
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-deploy -run "Pipeline|UpdatePipeline"
```

Expected: FAIL because the pipeline helpers do not exist.

- [ ] **Step 3: Implement command runner and pipeline helpers**

Add to `main.go`:

```go
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

type deployDeps struct {
	Runner commandRunner
	Services serviceManager
	Now func() time.Time
}
```

Implement:

- `pullLatest(ctx, cfg, deps)` using current branch and `git pull --ff-only origin <branch>`.
- `syncAppNPM(ctx, cfg, deps)` using `npm ci --include=dev` in `app` only when Web is enabled and `NoNPM` is false.
- `buildBinaries(ctx, cfg, deps, includeUpdater bool)` using `go build` from `server`.
- `publishWeb(ctx, cfg, deps)` using `npm run build:web:release`.
- `installBuiltBinaries(ctx, cfg, includeUpdater bool)` with platform suffix selection.
- `writeReleaseManifest(cfg, now)`.
- `runDeployWithDeps`, `runUpdateWithDeps`.

Keep external command and filesystem helpers small and directly in `main.go`.

- [ ] **Step 4: Run pipeline tests**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-deploy -run "Pipeline|UpdatePipeline"
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add server/cmd/wheelmaker-deploy
git commit -m "feat: add deploy pipeline"
```

## Task 3: Config, Wrappers, Manifest, Bootstrap

**Files:**
- Modify: `server/cmd/wheelmaker-deploy/main.go`
- Modify: `server/cmd/wheelmaker-deploy/main_test.go`
- Modify: `server/config.example.json`

- [ ] **Step 1: Write failing tests for generated config and wrappers**

Add tests:

```go
func TestEnsureConfigWritesRunnableWheelMakerDefault(t *testing.T) {
	h := newDeployHarness(t)
	if err := ensureConfig(h.cfg); err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(h.home, ".wheelmaker", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(raw)
	for _, needle := range []string{
		`"name": "WheelMaker"`,
		`"listen": true`,
		`"server": "127.0.0.1"`,
		`"token": "wheelmaker-local-token"`,
		`"hubId": "local-hub"`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("config missing %s: %s", needle, text)
		}
	}
}

func TestWriteHelperWrappers(t *testing.T) {
	h := newDeployHarness(t)
	if err := writeHelperWrappers(h.cfg); err != nil {
		t.Fatalf("writeHelperWrappers: %v", err)
	}
	assertFileContains(t, filepath.Join(h.home, ".wheelmaker", "start.bat"), "wheelmaker-deploy.exe service start")
	assertFileContains(t, filepath.Join(h.home, ".wheelmaker", "stop.bat"), "wheelmaker-deploy.exe service stop")
}
```

Add bootstrap test:

```go
func TestBootstrapBuildsTempDeployAndExecsUpdate(t *testing.T) {
	h := newDeployHarness(t)
	h.cfg.Mode = modeBootstrapUpdate
	if err := runBootstrapUpdateWithDeps(context.Background(), h.cfg, h.deps); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	assertEventsContainInOrder(t, h.events,
		"git pull --ff-only origin main",
		"go build wheelmaker-deploy-next",
		"exec wheelmaker-deploy-next update --no-pull --no-config --no-updater",
	)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-deploy -run "Config|Wrappers|Bootstrap"
```

Expected: FAIL.

- [ ] **Step 3: Implement config, wrappers, manifest, bootstrap**

Implement in `main.go`:

- `ensureConfig(cfg deployConfig) (created bool, err error)` writing the local WheelMaker config when missing.
- `writeHelperWrappers(cfg deployConfig)` generating Windows `.bat` and Unix `.sh` wrappers.
- `writeReleaseManifest(cfg deployConfig, now time.Time)` with schema version 1 and Git branch/SHA.
- `runBootstrapUpdateWithDeps` that pulls, builds `wheelmaker-deploy-next`, then executes `update --no-pull --no-config --no-updater`.

Modify `server/config.example.json` to:

```json
{
  "projects": [
    {
      "name": "WheelMaker",
      "path": "/path/to/WheelMaker"
    }
  ],
  "registry": {
    "listen": true,
    "port": 9630,
    "server": "127.0.0.1",
    "token": "wheelmaker-local-token",
    "hubId": "local-hub"
  },
  "monitor": {
    "port": 9631
  },
  "log": {
    "level": "warn"
  }
}
```

- [ ] **Step 4: Run tests**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-deploy -run "Config|Wrappers|Bootstrap"
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add server/cmd/wheelmaker-deploy server/config.example.json
git commit -m "feat: add deploy config and bootstrap"
```

## Task 4: Platform Service Implementations

**Files:**
- Create: `server/cmd/wheelmaker-deploy/service_windows.go`
- Create: `server/cmd/wheelmaker-deploy/service_darwin.go`
- Create: `server/cmd/wheelmaker-deploy/service_linux.go`
- Modify: `server/cmd/wheelmaker-deploy/main_test.go`

- [ ] **Step 1: Write failing service generation tests**

Add tests for service file content generation through pure helper functions:

```go
func TestLinuxUnitContentRequiresRestartAlways(t *testing.T) {
	unit := linuxUnitContent("WheelMaker Hub", "/repo", "/home/user/.wheelmaker/systemd.env", "/home/user/.wheelmaker/bin/wheelmaker", "-d")
	for _, needle := range []string{"Restart=always", "EnvironmentFile=", "ExecStart=", "WantedBy=default.target"} {
		if !strings.Contains(unit, needle) {
			t.Fatalf("unit missing %s:\n%s", needle, unit)
		}
	}
}

func TestMacOSPlistContent(t *testing.T) {
	plist := launchAgentPlistContent("com.wheelmaker.hub", "/repo", "/Users/me/.wheelmaker/bin/wheelmaker", []string{"-d"})
	for _, needle := range []string{"com.wheelmaker.hub", "<key>ProgramArguments</key>", "<string>-d</string>", "<key>KeepAlive</key>"} {
		if !strings.Contains(plist, needle) {
			t.Fatalf("plist missing %s:\n%s", needle, plist)
		}
	}
}

func TestWindowsUpdaterServiceArgumentsUseDeployCLIPathFlags(t *testing.T) {
	got := windowsUpdaterArgs(`C:\repo`, `C:\Users\me\.wheelmaker\bin`, "03:00")
	for _, needle := range []string{`--repo "C:\repo"`, `--install-dir "C:\Users\me\.wheelmaker\bin"`, `--time "03:00"`} {
		if !strings.Contains(got, needle) {
			t.Fatalf("args=%s missing %s", got, needle)
		}
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-deploy -run "LinuxUnit|MacOSPlist|WindowsUpdater"
```

Expected: FAIL.

- [ ] **Step 3: Implement platform service files**

Implement:

- Windows:
  - `isElevated() bool`
  - service create/delete/start/stop/status via PowerShell/sc.exe
  - `windowsUpdaterArgs(repo, bin, time string) string`
  - deploy config fails without elevation when service config is enabled
- macOS:
  - plist content helpers
  - write LaunchAgents
  - `launchctl bootout/bootstrap/kickstart/print`
- Linux:
  - `systemd.env` writer
  - unit content helpers
  - lingering check using `loginctl show-user <user> -p Linger`
  - `systemctl --user daemon-reload/enable/start/stop/restart/show`

Keep platform command execution behind the shared `commandRunner` so tests can assert commands without requiring host service managers.

- [ ] **Step 4: Run service tests and cross-compile**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-deploy
$env:GOOS='linux'; $env:GOARCH='amd64'; go build -o $env:TEMP\wheelmaker-deploy-linux-amd64 ./cmd/wheelmaker-deploy; Remove-Item Env:GOOS,Env:GOARCH -ErrorAction SilentlyContinue
$env:GOOS='darwin'; $env:GOARCH='arm64'; go build -o $env:TEMP\wheelmaker-deploy-darwin-arm64 ./cmd/wheelmaker-deploy; Remove-Item Env:GOOS,Env:GOARCH -ErrorAction SilentlyContinue
```

Expected: PASS and both cross-builds succeed.

- [ ] **Step 5: Commit**

```powershell
git add server/cmd/wheelmaker-deploy
git commit -m "feat: add deploy service backends"
```

## Task 5: Top-Level Deploy Wrappers

**Files:**
- Modify: `deploy.bat`
- Modify: `deploy.sh`
- Modify: `scripts/test_deploy_bat.ps1`
- Modify: `scripts/test_deploy_sh.ps1`

- [ ] **Step 1: Write failing source tests**

Update script tests to assert:

```powershell
Assert-Contains "wheelmaker-deploy"
Assert-Contains "go build"
Assert-Contains " deploy "
Assert-NotContains "scripts\refresh_server.ps1"
Assert-NotContains "scripts/refresh_server.sh"
Assert-NotContains "scripts/refresh_server_linux.sh"
```

For `deploy.sh`, assert the wrapper preserves macOS/Linux support and does not dispatch to refresh scripts.

- [ ] **Step 2: Run tests and verify they fail**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_bat.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_sh.ps1
```

Expected: FAIL because wrappers still call refresh scripts.

- [ ] **Step 3: Rewrite wrappers**

`deploy.bat` behavior:

- Prefer `pwsh` only for elevation/bootstrap shell needs.
- Build `server/cmd/wheelmaker-deploy` into `~\.wheelmaker\bin\wheelmaker-deploy.exe` when missing.
- Call:

```bat
"%USERPROFILE%\.wheelmaker\bin\wheelmaker-deploy.exe" deploy --repo "%~dp0" %*
```

`deploy.sh` behavior:

- Support Darwin and Linux.
- Build `server/cmd/wheelmaker-deploy` into `~/.wheelmaker/bin/wheelmaker-deploy` when missing.
- Call:

```bash
"${HOME}/.wheelmaker/bin/wheelmaker-deploy" deploy --repo "$repo_root" "$@"
```

Do not modify refresh scripts.

- [ ] **Step 4: Run script tests and syntax checks**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_bat.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_sh.ps1
bash -n deploy.sh
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add deploy.bat deploy.sh scripts/test_deploy_bat.ps1 scripts/test_deploy_sh.ps1
git commit -m "feat: route deploy wrappers to deploy cli"
```

## Task 6: Updater Uses Deploy CLI

**Files:**
- Modify: `server/cmd/wheelmaker-updater/updater.go`
- Modify: `server/cmd/wheelmaker-updater/updater_test.go`

- [ ] **Step 1: Write failing updater tests**

Update tests so Windows/macOS/Linux expect:

```text
<install-dir>/wheelmaker-deploy bootstrap-update --repo <repo> --bin <install-dir> --time <HH:mm>
```

For skip-web-publish signal, expect `--no-web`.

Add missing CLI test:

```go
func TestUpdaterFailsWhenDeployCLIMissing(t *testing.T) {
	cfg := testUpdaterConfig(t)
	err := runUpdateRoundWithOptions(context.Background(), cfg, fakeRunner{}, updateRoundOptions{})
	if err == nil || !strings.Contains(err.Error(), "wheelmaker-deploy") {
		t.Fatalf("err=%v, want missing deploy cli", err)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-updater -run "Invocation|DeployCLI|RequiredCommands|Skip"
```

Expected: FAIL because updater still calls refresh scripts.

- [ ] **Step 3: Implement updater invocation**

Replace refresh invocation selection with deploy CLI invocation:

- Resolve binary name:
  - Windows: `wheelmaker-deploy.exe`
  - Unix: `wheelmaker-deploy`
- Command path: `filepath.Join(cfg.InstallDir, binaryName)`.
- Args:

```go
[]string{"bootstrap-update", "--repo", cfg.RepoDir, "--bin", cfg.InstallDir, "--time", cfg.DailyTime}
```

- Append `--no-web` for `skip-web-publish`.
- Required commands now only validate the deploy CLI exists plus platform basics needed by updater itself.
- Do not fall back to refresh scripts.

- [ ] **Step 4: Run updater tests**

Run from `server`:

```powershell
go test ./cmd/wheelmaker-updater
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add server/cmd/wheelmaker-updater
git commit -m "feat: invoke deploy cli from updater"
```

## Task 7: Documentation and Full Verification

**Files:**
- Modify: `README.md`
- Modify as needed: `docs/superpowers/specs/2026-05-30-wheelmaker-deploy-cli-design.md` only if implementation discovers a spec correction.

- [ ] **Step 1: Update README**

Document:

- `deploy.bat` / `deploy.sh` now call `wheelmaker-deploy`.
- `wheelmaker-deploy deploy`, `update`, `bootstrap-update`, `service`, `doctor`.
- `refresh_server.*` are compatibility scripts.
- Linux requires lingering:

```bash
sudo loginctl enable-linger "$USER"
```

- `update-publish.*` remains signal-only.

- [ ] **Step 2: Run source checks**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_bat.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_sh.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_update_publish_bat.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_update_publish_sh.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_ps1.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_sh.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_linux_sh.ps1
```

Expected: PASS. Refresh script tests should still pass because the files remain unchanged.

- [ ] **Step 3: Run Go tests**

Run from `server`:

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Cross-compile deploy CLI and existing binaries**

Run from `server`:

```powershell
$env:GOOS='windows'; $env:GOARCH='amd64'; go build -o $env:TEMP\wheelmaker-deploy.exe ./cmd/wheelmaker-deploy; Remove-Item Env:GOOS,Env:GOARCH -ErrorAction SilentlyContinue
$env:GOOS='linux'; $env:GOARCH='amd64'; go build -o $env:TEMP\wheelmaker-deploy-linux-amd64 ./cmd/wheelmaker-deploy; Remove-Item Env:GOOS,Env:GOARCH -ErrorAction SilentlyContinue
$env:GOOS='darwin'; $env:GOARCH='arm64'; go build -o $env:TEMP\wheelmaker-deploy-darwin-arm64 ./cmd/wheelmaker-deploy; Remove-Item Env:GOOS,Env:GOARCH -ErrorAction SilentlyContinue
go build -o $env:TEMP\wheelmaker.exe ./cmd/wheelmaker
go build -o $env:TEMP\wheelmaker-monitor.exe ./cmd/wheelmaker-monitor
go build -o $env:TEMP\wheelmaker-updater.exe ./cmd/wheelmaker-updater
```

Expected: all builds succeed.

- [ ] **Step 5: Commit and push**

Run from repo root:

```powershell
git add -A
git commit -m "feat: add wheelmaker deploy cli"
git push origin main
```

Expected: commit and push succeed.

## Self-Review

Spec coverage:

- New CLI and command surface: Tasks 1-4.
- Top-level wrappers call deploy CLI directly: Task 5.
- Refresh scripts remain unchanged: Task 5 and Task 7 source checks.
- Updater invokes deploy CLI: Task 6.
- `update-publish.*` signal-only: Task 7 source checks.
- No updater changes during update/bootstrap: Tasks 2 and 6 tests.
- Linux systemd user plus lingering: Task 4.
- macOS LaunchAgent and Windows Service support: Task 4.
- Default runnable config: Task 3.
- Helper wrappers: Task 3.
- Manifest success-only behavior: Task 2 and Task 3.
- No ACP global dependency handling: Task 2 pipeline excludes it, Task 7 source checks guard old scripts separately.

Placeholder scan: no unresolved placeholders are intended in this plan; reserved commands have explicit not-implemented behavior.
