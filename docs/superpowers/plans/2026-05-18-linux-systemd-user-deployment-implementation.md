# Linux systemd User Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add a Linux Machine A deployment path using current-user `systemd --user` services while preserving the existing Windows service and macOS LaunchAgent paths.

**Architecture:** Keep platform-specific behavior at deployment and service-control boundaries. Linux gets its own refresh script and monitor/updater branches; shared Web publishing and WheelMaker runtime behavior stay unchanged.

**Tech Stack:** Go 1.26, Bash, `systemctl --user`, Node.js 22, npm/Webpack, PowerShell source checks for scripts.

---

## File Map

- Create `scripts/refresh_server_linux.sh`: Linux build/install/web-publish/systemd-user lifecycle entrypoint.
- Create `scripts/test_refresh_server_linux_sh.ps1`: source-level assertions for Linux deploy script.
- Modify `deploy.sh`: dispatch macOS to `refresh_server.sh` and Linux to `refresh_server_linux.sh`; keep dependency tip one-line copyable.
- Modify `scripts/test_deploy_sh.ps1`: assert platform dispatch and Linux dependency wording.
- Modify `server/cmd/wheelmaker-updater/updater.go`: choose Linux refresh script and Linux dependency set.
- Modify `server/cmd/wheelmaker-updater/updater_test.go`: cover Linux invocation, skip behavior, and required commands.
- Modify `server/cmd/wheelmaker-monitor/monitor.go`: add Linux `systemd --user` service status/actions.
- Modify `server/cmd/wheelmaker-monitor/monitor_test.go`: cover Linux service names, unit paths, status parsing, and process filtering.
- Modify `README.md`: document Linux Machine A deployment and reverse proxy contract.
- Track `docs/superpowers/specs/2026-05-18-linux-systemd-user-deployment-design.md` and this plan in the feature branch.

## Task 1: Linux Deploy Script Tests and Script

- [x] **Step 1: Write failing source test**

Add `scripts/test_refresh_server_linux_sh.ps1` asserting the script exists and contains:

- `refresh_server_linux.sh is Linux-only`
- `systemctl --user`
- `wheelmaker-hub.service`
- `wheelmaker-monitor.service`
- `wheelmaker-updater.service`
- `~/.config/systemd/user`
- `~/.wheelmaker/systemd.env`
- `GOOS=linux`
- `npm run build:web:release`
- `--skip-web-publish`

- [x] **Step 2: Run test and verify it fails**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_linux_sh.ps1`

Expected: fail because `scripts/refresh_server_linux.sh` is missing.

- [x] **Step 3: Implement Linux refresh script**

Create `scripts/refresh_server_linux.sh` with actions `refresh`, `start`, `stop`, `restart`, `status`, and `uninstall`; generate user units; run `systemctl --user daemon-reload`; build Linux binaries; install to `~/.wheelmaker/bin`; publish Web assets; preserve/create config.

- [x] **Step 4: Verify script source and syntax**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_linux_sh.ps1
bash -n scripts/refresh_server_linux.sh
```

Expected: both pass.

## Task 2: Root deploy.sh Platform Dispatch

- [x] **Step 1: Write failing deploy.sh source test**

Update `scripts/test_deploy_sh.ps1` to expect Linux dispatch to `scripts/refresh_server_linux.sh`, macOS dispatch to `scripts/refresh_server.sh`, and an unsupported-platform message that only points Windows users to `deploy.bat`.

- [x] **Step 2: Run test and verify it fails**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_sh.ps1`

Expected: fail because `deploy.sh` is still macOS-only.

- [x] **Step 3: Implement dispatch**

Modify `deploy.sh`:

- Accept `Darwin` and `Linux`.
- Keep dependency check before refresh script invocation.
- Choose `scripts/refresh_server.sh` for `Darwin`.
- Choose `scripts/refresh_server_linux.sh` for `Linux`.
- `chmod +x` the chosen script when needed.

- [x] **Step 4: Verify deploy.sh**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_sh.ps1
bash -n deploy.sh
```

Expected: both pass.

## Task 3: Updater Linux Invocation

- [x] **Step 1: Write failing Go tests**

Extend `server/cmd/wheelmaker-updater/updater_test.go` with tests that:

- `runtimeGOOS = "linux"` invokes `bash scripts/refresh_server_linux.sh --repo-root <repo> --install-dir <dir> --skip-updater-install --skip-service-config`.
- Manual signal appends `--skip-update --skip-web-publish`.
- `requiredCommandsForOS("linux")` includes `bash`, `git`, `go`, `node`, `npm`, `npx`, `systemctl`.

- [x] **Step 2: Run tests and verify failure**

Run: `go test ./cmd/wheelmaker-updater -run "Linux|RequiredCommands"`

Expected: fail because Linux is unsupported.

- [x] **Step 3: Implement updater branch**

Add a `linux` case to `refreshInvocationForOS` and `requiredCommandsForOS`.

- [x] **Step 4: Verify updater tests**

Run: `go test ./cmd/wheelmaker-updater`

Expected: pass.

## Task 4: Monitor Linux Service Operations

- [x] **Step 1: Write failing monitor tests**

Extend `server/cmd/wheelmaker-monitor/monitor_test.go` with tests for:

- all Linux service names.
- managed Linux service names are hub + updater.
- unit path is `~/.config/systemd/user/<service>`.
- parsing `LoadState=loaded ActiveState=active UnitFileState=enabled` returns `Running` and `systemd --user`.
- missing unit returns `NotInstalled`.
- Unix process parsing excludes updater, monitor, shell, grep, and wrapper rows.

- [x] **Step 2: Run tests and verify failure**

Run: `go test ./cmd/wheelmaker-monitor -run "Linux|Systemd|UnixWheelmaker"`

Expected: fail because Linux systemd helpers do not exist yet.

- [x] **Step 3: Implement monitor Linux branch**

Add Linux service constants and helpers. Route `listManagedServices`, `StartService`, `StopService`, `RestartService`, and `RestartMonitor` through `systemctl --user` when `monitorGOOS == "linux"` and unit files exist; otherwise preserve process fallback.

- [x] **Step 4: Verify monitor tests**

Run: `go test ./cmd/wheelmaker-monitor`

Expected: pass.

## Task 5: README Linux Documentation

- [x] **Step 1: Write failing documentation check**

Use `rg` to check README contains Linux deployment strings:

```powershell
rg -n "Linux Machine A|systemd --user|refresh_server_linux.sh|wheelmaker-hub.service|127.0.0.1:9630|127.0.0.1:9631" README.md
```

Expected before edit: fail to find the new section.

- [x] **Step 2: Document Linux deployment**

Add a Linux Machine A section next to macOS deployment covering:

- prerequisites.
- `bash deploy.sh`.
- generated user services.
- lifecycle commands.
- reverse proxy contract.
- lingering note.

- [x] **Step 3: Verify docs**

Run the same `rg` command.

Expected: all expected terms found.

## Task 6: Final Verification and Commit

- [x] **Step 1: Run full verification**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_linux_sh.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_sh.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_sh.ps1
go test ./...
```

Run from `server` for `go test ./...`.

- [x] **Step 2: Cross-compile Linux**

Run:

```powershell
$env:GOOS='linux'; $env:GOARCH='amd64'; go build -o $env:TEMP\wheelmaker-linux-amd64 ./cmd/wheelmaker/; go build -o $env:TEMP\wheelmaker-monitor-linux-amd64 ./cmd/wheelmaker-monitor/; go build -o $env:TEMP\wheelmaker-updater-linux-amd64 ./cmd/wheelmaker-updater/; Remove-Item Env:GOOS -ErrorAction SilentlyContinue; Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
```

- [x] **Step 3: Commit and push feature branch**

Stage only files changed in this worktree and commit:

```powershell
git add deploy.sh README.md scripts/refresh_server_linux.sh scripts/test_refresh_server_linux_sh.ps1 scripts/test_deploy_sh.ps1 server/cmd/wheelmaker-updater/updater.go server/cmd/wheelmaker-updater/updater_test.go server/cmd/wheelmaker-monitor/monitor.go server/cmd/wheelmaker-monitor/monitor_test.go docs/superpowers/specs/2026-05-18-linux-systemd-user-deployment-design.md docs/superpowers/plans/2026-05-18-linux-systemd-user-deployment-implementation.md
git commit -m "feat: add linux systemd user deployment"
git push origin feature/linux-systemd-user-deploy
```

Expected: branch pushed. Merge to `main` can happen after review or on explicit request.
