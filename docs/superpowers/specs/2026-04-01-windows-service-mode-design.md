# Windows Service Mode for WheelMaker + Monitor + Updater Design

## Goal

Convert WheelMaker runtime and auto-update to full Windows Service mode. Keep update capability while removing Scheduled Task dependency. Services must auto-start on boot and run under current user account.

## Scope

- Keep `WheelMaker` and `WheelMakerMonitor` as always-on services.
- Introduce `WheelMakerUpdater` service to run daily update at configured time.
- Update all operational scripts to manage services, not foreground/background processes or scheduled tasks.

Out of scope:
- Linux/macOS service support.
- Replacing `go build` with prebuilt binary delivery.

## Service Topology

Three Windows services with `StartupType=Automatic`:

1. `WheelMaker`
- Binary: `~/.wheelmaker/bin/wheelmaker.exe`
- Responsibility: guardian process that ensures hub and registry workers are running.

2. `WheelMakerMonitor`
- Binary: `~/.wheelmaker/bin/wheelmaker-monitor.exe`
- Responsibility: dashboard and service operations endpoint.

3. `WheelMakerUpdater`
- Binary: `~/.wheelmaker/bin/wheelmaker-updater.exe`
- Responsibility: daily update scheduler and deployment executor.

All services run as current user account (not LocalSystem).

## Updater Runtime Design

`wheelmaker-updater` behavior:

- Parse required args:
  - `--repo` (absolute repo root)
  - `--install-dir` (wheelmaker bin install dir)
  - `--time` (`HH:mm`, default `03:00`)
  - optional `--once` for one-shot run and exit
- Validate inputs before each run:
  - repo exists and contains `.git` and `server/go.mod`
  - install dir exists or can be created
  - required commands available (`git`, `go`, `powershell`)
- Scheduler loop:
  - Compute next local trigger from configured daily time.
  - Sleep until trigger or service stop signal.
  - Execute one update round.
- Update round:
  1. `git fetch origin`
  2. detect current branch (`git rev-parse --abbrev-ref HEAD`)
  3. compare local and remote heads
  4. if same head -> log “up to date”, return success
  5. if different -> `git pull --ff-only origin <branch>`
  6. run deploy script via PowerShell:
     `scripts/refresh_server.ps1 -SkipGitPull`
- Error policy:
  - Never terminate service due to one failed update round.
  - Log the failure and continue next daily schedule.

## Script Migration Design

### `scripts/refresh_server.ps1`

Keep as primary deploy entrypoint, but switch process lifecycle actions to service actions:

- Build and install `wheelmaker.exe`, `wheelmaker-monitor.exe`, and new `wheelmaker-updater.exe`.
- Replace `start.bat/stop.bat/restart.bat` process scripts with service-oriented helper scripts (service start/stop/restart wrappers).
- Register or update Windows services for all three binaries.
- Ensure `Automatic` startup mode.
- Restart services using SCM (`Restart-Service` / `Start-Service` / `Stop-Service`) with wait+timeout checks.

### `scripts/delay_restart_server.ps1`

- Keep delayed worker structure.
- Replace direct process start/kill with service restart operations:
  - run `refresh_server.ps1 -SkipGitPull`
  - then restart `WheelMaker` and `WheelMakerMonitor` by service name.
- Logging remains in `~/.wheelmaker/delay_restart_server.log`.

### `scripts/auto_update.ps1`

- Remove Scheduled Task lifecycle.
- Repurpose as service manager for updater:
  - `-Setup` => create/update `WheelMakerUpdater` service
  - `-Uninstall` => stop/delete updater service
  - default run => trigger one-shot updater execution (`wheelmaker-updater --once ...`) or print status
- Keep existing log path semantics.

## Service Registration Rules

- Use PowerShell `New-Service` / `Set-Service` and direct SCM fallback (`sc.exe`) where needed.
- Explicit binary paths and args (no path guessing):
  - updater `--repo <repo> --install-dir <installDir> --time <HH:mm>`
- During refresh, if service exists:
  - stop service
  - replace binary
  - update binPath if changed
  - set startup type automatic
  - start service

## Observability

- Updater log file: `~/.wheelmaker/updater.log`.
- Keep deploy logs from refresh script.
- Monitor remains source of runtime service status.

## Failure Handling and Safety

- Do not mutate git worktree if dirty (existing guard retained in refresh).
- Fail fast on missing config or binaries during install/update.
- When updater update round fails after pull/build, preserve logs and return non-zero for that round, but service loop continues.
- Service stop/start functions must include explicit timeout and actionable error text.

## Testing Strategy

1. Unit tests (Go updater package):
- time parsing and next-run calculation
- command pipeline behavior when up-to-date vs new commits
- argument validation

2. Script smoke checks (manual):
- `refresh_server.ps1 -WhatIf`
- install services and verify:
  - `Get-Service WheelMaker,WheelMakerMonitor,WheelMakerUpdater`
  - startup type is `Automatic`
- updater one-shot dry run (`--once`) and log emission

3. End-to-end manual checks:
- reboot machine and verify all three services auto-start
- trigger monitor restart endpoint and ensure service-based behavior remains correct

## Rollout

- Commit scripts + updater binary source in one feature branch.
- Deploy with `scripts/refresh_server.ps1`.
- Validate services and logs.
- Keep rollback path: disable/delete `WheelMakerUpdater` service and run prior refresh script revision if needed.
