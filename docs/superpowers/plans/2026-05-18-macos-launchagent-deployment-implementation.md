# macOS LaunchAgent Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a macOS Machine A deployment path using current-user LaunchAgents while preserving the existing Windows service deployment.

**Architecture:** Keep platform differences at deployment and service-control boundaries. Use a cross-platform Node Web export helper, a macOS `refresh_server.sh` script for build/install/LaunchAgent lifecycle, platform-selecting updater invocation, and monitor `launchctl` branches guarded by runtime OS checks.

**Tech Stack:** Go 1.26, Bash, launchctl/LaunchAgent plist files, Node.js 22, npm/Webpack, Jest, PowerShell source checks for scripts.

---

## File Structure

- Create `app/scripts/export_web_release.js`: cross-platform Web public asset exporter.
- Modify `app/package.json`: use Node exporter for `build:web:release`.
- Create `app/__tests__/web-export-release-script.test.ts`: verifies the Node exporter against a temp target.
- Create `scripts/refresh_server.sh`: macOS build/install/LaunchAgent lifecycle entrypoint.
- Create `scripts/test_refresh_server_sh.ps1`: source-level checks for the macOS shell deploy script.
- Modify `server/cmd/wheelmaker-updater/updater.go`: choose refresh script and commands by platform.
- Modify `server/cmd/wheelmaker-updater/updater_test.go`: verify Windows compatibility and darwin command selection.
- Modify `server/cmd/wheelmaker-monitor/monitor.go`: add macOS LaunchAgent status/actions.
- Modify `server/cmd/wheelmaker-monitor/monitor_test.go`: test launchctl helper behavior without executing launchctl.
- Modify `README.md`: document macOS Machine A deployment and reverse proxy contract.

### Task 1: Cross-Platform Web Release Export

**Files:**
- Create: `app/scripts/export_web_release.js`
- Modify: `app/package.json`
- Test: `app/__tests__/web-export-release-script.test.ts`

- [ ] **Step 1: Write the failing Jest test**

Create `app/__tests__/web-export-release-script.test.ts`:

```ts
import fs from 'fs';
import os from 'os';
import path from 'path';
import { spawnSync } from 'child_process';

describe('web release exporter', () => {
  test('copies public PWA assets to the configured release target', () => {
    const appRoot = path.join(__dirname, '..');
    const target = fs.mkdtempSync(path.join(os.tmpdir(), 'wheelmaker-web-release-'));
    const script = path.join(appRoot, 'scripts', 'export_web_release.js');

    const result = spawnSync(process.execPath, [script], {
      cwd: appRoot,
      env: { ...process.env, WHEELMAKER_WEB_TARGET: target },
      encoding: 'utf8',
    });

    expect(result.status).toBe(0);
    expect(fs.existsSync(path.join(target, 'manifest.webmanifest'))).toBe(true);
    expect(fs.existsSync(path.join(target, 'service-worker.js'))).toBe(true);
    expect(fs.existsSync(path.join(target, 'icons', 'icon.svg'))).toBe(true);
  });

  test('release script is wired through npm without powershell', () => {
    const appRoot = path.join(__dirname, '..');
    const packageJson = JSON.parse(fs.readFileSync(path.join(appRoot, 'package.json'), 'utf8'));

    expect(packageJson.scripts['build:web:release']).toBe(
      'npm run build:web && node scripts/export_web_release.js',
    );
  });
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `cd app && npm test -- web-export-release-script.test.ts --runInBand`

Expected: FAIL because `app/scripts/export_web_release.js` does not exist and `package.json` still calls PowerShell.

- [ ] **Step 3: Implement the Node exporter and package script**

Create `app/scripts/export_web_release.js` with recursive copy logic:

```js
const fs = require('fs');
const os = require('os');
const path = require('path');

const root = path.resolve(__dirname, '..');
const webPublic = path.join(root, 'web', 'public');
const target = process.env.WHEELMAKER_WEB_TARGET || path.join(os.homedir(), '.wheelmaker', 'web');

function ensureDir(dir) {
  fs.mkdirSync(dir, { recursive: true });
}

function copyFileIfExists(source, dest) {
  if (!fs.existsSync(source)) return;
  ensureDir(path.dirname(dest));
  fs.copyFileSync(source, dest);
}

function copyDir(source, dest) {
  if (!fs.existsSync(source)) return;
  ensureDir(dest);
  for (const entry of fs.readdirSync(source, { withFileTypes: true })) {
    const sourcePath = path.join(source, entry.name);
    const destPath = path.join(dest, entry.name);
    if (entry.isDirectory()) {
      copyDir(sourcePath, destPath);
    } else if (entry.isFile()) {
      copyFileIfExists(sourcePath, destPath);
    }
  }
}

ensureDir(target);
copyFileIfExists(path.join(webPublic, 'manifest.webmanifest'), path.join(target, 'manifest.webmanifest'));
copyFileIfExists(path.join(webPublic, 'service-worker.js'), path.join(target, 'service-worker.js'));
copyDir(path.join(webPublic, 'icons'), path.join(target, 'icons'));

console.log(`Exported web release to ${target}`);
```

Change `build:web:release` in `app/package.json` to:

```json
"build:web:release": "npm run build:web && node scripts/export_web_release.js"
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run: `cd app && npm test -- web-export-release-script.test.ts --runInBand`

Expected: PASS.

### Task 2: macOS Refresh Script

**Files:**
- Create: `scripts/refresh_server.sh`
- Create: `scripts/test_refresh_server_sh.ps1`

- [ ] **Step 1: Write the failing source test**

Create `scripts/test_refresh_server_sh.ps1`:

```powershell
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\refresh_server.sh"

if (-not (Test-Path $scriptPath)) {
  throw "refresh_server.sh is missing"
}

$text = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8

function Assert-Contains {
  param([string]$Needle)
  if (-not $text.Contains($Needle)) {
    throw "refresh_server.sh does not contain expected text: $Needle"
  }
}

Assert-Contains "com.wheelmaker.hub"
Assert-Contains "com.wheelmaker.monitor"
Assert-Contains "com.wheelmaker.updater"
Assert-Contains "launchctl bootstrap"
Assert-Contains "launchctl bootout"
Assert-Contains "launchctl kickstart"
Assert-Contains "GOOS=darwin"
Assert-Contains "npm run build:web:release"
Assert-Contains "--skip-web-publish"
Assert-Contains "Node.js 22.11.0+"
Assert-Contains "~/Library/LaunchAgents"

Write-Host "refresh_server.sh source checks passed"
```

- [ ] **Step 2: Run the script test and verify RED**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_sh.ps1`

Expected: FAIL because `scripts/refresh_server.sh` does not exist.

- [ ] **Step 3: Implement `scripts/refresh_server.sh`**

Create a macOS-only Bash script that:

- Parses `--repo-root`, `--install-dir`, `--time`, `--skip-update`, `--skip-git-pull`, `--skip-deps`, `--skip-build`, `--skip-install`, `--skip-updater-install`, `--skip-restart`, `--skip-service-config`, `--skip-web-publish`, and positional actions `start|stop|restart|status|uninstall`.
- Validates `uname -s` is `Darwin`.
- Checks `bash`, `git`, `go`, `node`, `npm`, `npx`, and `launchctl`.
- Validates Node.js 22.11.0+ with a `node -e` version check.
- Builds darwin binaries to `~/.wheelmaker/build/darwin_$(go env GOARCH)`.
- Installs binaries to `~/.wheelmaker/bin`.
- Generates plist files in `~/Library/LaunchAgents`.
- Starts/stops/restarts jobs using `launchctl bootstrap`, `launchctl bootout`, and `launchctl kickstart`.
- Publishes Web assets by running `npm run build:web:release` unless `--skip-web-publish` is set.
- Skips updater plist/control when `--skip-updater-install` is set, so the updater can safely invoke the script.

- [ ] **Step 4: Mark the script executable**

Run: `git update-index --chmod=+x scripts/refresh_server.sh`

Expected: exit 0.

- [ ] **Step 5: Run the script source test and verify GREEN**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_sh.ps1`

Expected: PASS.

### Task 3: Updater Platform Selection

**Files:**
- Modify: `server/cmd/wheelmaker-updater/updater.go`
- Modify: `server/cmd/wheelmaker-updater/updater_test.go`

- [ ] **Step 1: Write failing updater tests**

Add tests to `updater_test.go`:

```go
func TestRunUpdateRound_DarwinRunsRefreshShell(t *testing.T) {
  old := runtimeGOOS
  runtimeGOOS = "darwin"
  defer func() { runtimeGOOS = old }()

  repoDir := t.TempDir()
  scriptsDir := filepath.Join(repoDir, "scripts")
  if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
    t.Fatalf("mkdir scripts: %v", err)
  }
  refreshPath := filepath.Join(scriptsDir, "refresh_server.sh")
  if err := os.WriteFile(refreshPath, []byte(""), 0o755); err != nil {
    t.Fatalf("write refresh script: %v", err)
  }

  cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: "/Users/test/.wheelmaker/bin"}
  f := &fakeRunner{results: map[string]fakeResult{}}

  if err := runUpdateRound(context.Background(), cfg, f, false); err != nil {
    t.Fatalf("runUpdateRound: %v", err)
  }

  if len(f.calls) != 1 {
    t.Fatalf("expected one darwin refresh call, got %d", len(f.calls))
  }
  call := f.calls[0]
  if call.name != "bash" {
    t.Fatalf("command=%q want bash", call.name)
  }
  args := strings.Join(call.args, " ")
  if !strings.Contains(args, refreshPath) || !strings.Contains(args, "--install-dir /Users/test/.wheelmaker/bin") {
    t.Fatalf("unexpected args: %s", args)
  }
  if strings.Contains(args, "--skip-web-publish") {
    t.Fatalf("full update should publish web through refresh_server.sh: %s", args)
  }
}

func TestRunUpdateRound_DarwinManualSignalSkipsUpdateAndWebPublish(t *testing.T) {
  old := runtimeGOOS
  runtimeGOOS = "darwin"
  defer func() { runtimeGOOS = old }()

  repoDir := t.TempDir()
  scriptsDir := filepath.Join(repoDir, "scripts")
  if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
    t.Fatalf("mkdir scripts: %v", err)
  }
  refreshPath := filepath.Join(scriptsDir, "refresh_server.sh")
  if err := os.WriteFile(refreshPath, []byte(""), 0o755); err != nil {
    t.Fatalf("write refresh script: %v", err)
  }

  cfg := UpdaterConfig{RepoDir: repoDir, InstallDir: "/Users/test/.wheelmaker/bin"}
  f := &fakeRunner{results: map[string]fakeResult{}}

  if err := runUpdateRound(context.Background(), cfg, f, true); err != nil {
    t.Fatalf("runUpdateRound: %v", err)
  }

  args := strings.Join(f.calls[0].args, " ")
  if !strings.Contains(args, "--skip-update") || !strings.Contains(args, "--skip-web-publish") {
    t.Fatalf("manual signal should skip update and web publish, got: %s", args)
  }
}

func TestRequiredCommandsForOS(t *testing.T) {
  if got := requiredCommandsForOS("windows"); !reflect.DeepEqual(got, []string{"powershell"}) {
    t.Fatalf("windows commands=%#v", got)
  }
  darwin := requiredCommandsForOS("darwin")
  for _, want := range []string{"bash", "git", "go", "node", "npm", "npx", "launchctl"} {
    found := false
    for _, got := range darwin {
      if got == want {
        found = true
      }
    }
    if !found {
      t.Fatalf("darwin commands missing %s: %#v", want, darwin)
    }
  }
}
```

- [ ] **Step 2: Run updater tests and verify RED**

Run: `cd server && go test ./cmd/wheelmaker-updater`

Expected: FAIL because `runtimeGOOS` and darwin shell selection do not exist.

- [ ] **Step 3: Implement platform-specific updater invocation**

In `updater.go`:

- Add `runtimeGOOS = runtime.GOOS`.
- Add `requiredCommandsForOS(goos string) []string`.
- Change `validateCommands` to iterate over `requiredCommandsForOS(runtimeGOOS)`.
- Add a helper that returns the script path, command name, args, and whether refresh publishes Web:
  - Windows: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/refresh_server.ps1 -InstallDir ... -SkipUpdaterInstall -SkipServiceConfig`, `refreshPublishesWeb=false`.
  - Darwin: `bash scripts/refresh_server.sh --install-dir ... --skip-updater-install --skip-service-config`, `refreshPublishesWeb=true`.
  - Other OS: return unsupported platform error.
- Append `-SkipUpdate` on Windows and `--skip-update --skip-web-publish` on Darwin for plain manual signal mode.
- Keep the existing extra `npm run build:web:release` step only when `refreshPublishesWeb` is false and `skipUpdate` is false.

- [ ] **Step 4: Run updater tests and verify GREEN**

Run: `cd server && go test ./cmd/wheelmaker-updater`

Expected: PASS.

### Task 4: Monitor LaunchAgent Operations

**Files:**
- Modify: `server/cmd/wheelmaker-monitor/monitor.go`
- Modify: `server/cmd/wheelmaker-monitor/monitor_test.go`

- [ ] **Step 1: Write failing monitor helper tests**

Add tests:

```go
func TestLaunchAgentLabels(t *testing.T) {
  all := allLaunchAgentLabels()
  want := []string{launchAgentHubLabel, launchAgentMonitorLabel, launchAgentUpdaterLabel}
  if strings.Join(all, ",") != strings.Join(want, ",") {
    t.Fatalf("labels=%#v want %#v", all, want)
  }
  managed := managedLaunchAgentLabels()
  if strings.Join(managed, ",") != strings.Join([]string{launchAgentHubLabel, launchAgentUpdaterLabel}, ",") {
    t.Fatalf("managed labels=%#v", managed)
  }
}

func TestLaunchAgentPlistPath(t *testing.T) {
  home := filepath.Join("Users", "me")
  got := launchAgentPlistPath(home, launchAgentHubLabel)
  want := filepath.Join(home, "Library", "LaunchAgents", launchAgentHubLabel+".plist")
  if got != want {
    t.Fatalf("plist path=%q want %q", got, want)
  }
}

func TestParseLaunchAgentServiceInfo(t *testing.T) {
  running := parseLaunchAgentServiceInfo(launchAgentHubLabel, true, []byte("state = running\npid = 123\n"))
  if !running.Installed || running.Status != "Running" || running.StartType != "LaunchAgent" {
    t.Fatalf("running info=%#v", running)
  }
  stopped := parseLaunchAgentServiceInfo(launchAgentHubLabel, true, []byte("state = waiting\n"))
  if stopped.Status != "Stopped" {
    t.Fatalf("stopped info=%#v", stopped)
  }
  missing := parseLaunchAgentServiceInfo(launchAgentHubLabel, false, nil)
  if missing.Installed || missing.Status != "NotInstalled" {
    t.Fatalf("missing info=%#v", missing)
  }
}
```

- [ ] **Step 2: Run monitor tests and verify RED**

Run: `cd server && go test ./cmd/wheelmaker-monitor`

Expected: FAIL because launch agent helper functions do not exist.

- [ ] **Step 3: Implement LaunchAgent helpers and macOS branches**

In `monitor.go`:

- Add labels:
  - `com.wheelmaker.hub`
  - `com.wheelmaker.monitor`
  - `com.wheelmaker.updater`
- Add `monitorGOOS = runtime.GOOS`.
- Add helper functions:
  - `allLaunchAgentLabels()`
  - `managedLaunchAgentLabels()`
  - `launchAgentDomain()`
  - `launchAgentPlistPath(home, label string) string`
  - `parseLaunchAgentServiceInfo(label string, installed bool, output []byte) ServiceInfo`
  - `getLaunchAgentServiceInfo(label string) (ServiceInfo, error)`
  - `runLaunchAgentAction(label, action string) error`
  - `runManagedLaunchAgentAction(action string) error`
- Update existing runtime branches to use `monitorGOOS`.
- Make `listManagedServices` return LaunchAgent service rows on darwin.
- Make `StartService`, `StopService`, and `RestartService` control hub/updater LaunchAgents on darwin.
- Make `RestartMonitor` kickstart `com.wheelmaker.monitor` on darwin when its plist exists.

- [ ] **Step 4: Run monitor tests and verify GREEN**

Run: `cd server && go test ./cmd/wheelmaker-monitor`

Expected: PASS.

### Task 5: Documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add README source checks**

No separate test file is needed. Use `rg` after editing:

Run: `rg -n "macOS|LaunchAgent|refresh_server.sh|127.0.0.1:9630|127.0.0.1:9631|~/Library/LaunchAgents" README.md`

Expected before edit: FAIL to find the new macOS deployment section.

- [ ] **Step 2: Document the macOS path**

Add a macOS deployment section near the Windows deployment section covering:

- Requirements: Go 1.26+, Node.js 22.11+, npm, git, npx, launchctl, agent CLIs.
- Run: `bash scripts/refresh_server.sh`.
- Services: `com.wheelmaker.hub`, `com.wheelmaker.monitor`, `com.wheelmaker.updater`.
- Operations:
  - `bash scripts/refresh_server.sh status`
  - `bash scripts/refresh_server.sh start`
  - `bash scripts/refresh_server.sh stop`
  - `bash scripts/refresh_server.sh restart`
  - `bash scripts/refresh_server.sh uninstall`
- Reverse proxy contract:
  - `/` -> `~/.wheelmaker/web`
  - `/ws` -> `127.0.0.1:9630`
  - `/monitor/` -> `127.0.0.1:9631`

- [ ] **Step 3: Run README check**

Run: `rg -n "macOS|LaunchAgent|refresh_server.sh|127.0.0.1:9630|127.0.0.1:9631|~/Library/LaunchAgents" README.md`

Expected: matches in the macOS section.

### Task 6: Full Verification and Commit

**Files:**
- All files changed by prior tasks.

- [ ] **Step 1: Run script source checks**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_sh.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_bat.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_update_publish_bat.ps1
```

Expected: all PASS.

- [ ] **Step 2: Run server tests**

Run: `cd server && go test ./...`

Expected: PASS.

- [ ] **Step 3: Run darwin cross-builds**

Run:

```powershell
cd server
$env:GOOS='darwin'; $env:GOARCH='arm64'
go build -o $env:TEMP\wheelmaker-darwin-arm64 ./cmd/wheelmaker/
go build -o $env:TEMP\wheelmaker-monitor-darwin-arm64 ./cmd/wheelmaker-monitor/
go build -o $env:TEMP\wheelmaker-updater-darwin-arm64 ./cmd/wheelmaker-updater/
Remove-Item Env:\GOOS
Remove-Item Env:\GOARCH
```

Expected: all builds exit 0.

- [ ] **Step 4: Run focused app tests**

Run: `cd app && npm test -- web-export-release-script.test.ts --runInBand`

Expected: PASS.

- [ ] **Step 5: Run app typecheck and full tests**

Run:

```powershell
cd app
npm run tsc:web
npm test -- --runInBand
```

Expected: PASS.

- [ ] **Step 6: Run Web release build**

Run: `cd app && npm run build:web:release`

Expected: PASS and prints `Exported web release to ...\.wheelmaker\web`.

- [ ] **Step 7: Commit implementation**

Run:

```powershell
git add app/package.json app/scripts/export_web_release.js app/__tests__/web-export-release-script.test.ts scripts/refresh_server.sh scripts/test_refresh_server_sh.ps1 server/cmd/wheelmaker-updater/updater.go server/cmd/wheelmaker-updater/updater_test.go server/cmd/wheelmaker-monitor/monitor.go server/cmd/wheelmaker-monitor/monitor_test.go README.md docs/superpowers/plans/2026-05-18-macos-launchagent-deployment-implementation.md
git commit -m "feat: add macos launchagent deployment"
```

Expected: commit succeeds.

