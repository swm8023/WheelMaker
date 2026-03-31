# Windows Service Mode Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate WheelMaker runtime and auto-update to full Windows Service mode with boot auto-start and retained daily update capability.

**Architecture:** Add a dedicated `wheelmaker-updater` Go service for daily update scheduling, and convert operational scripts to service registration/control rather than process/scheduled-task management. Keep existing `wheelmaker` guardian and `wheelmaker-monitor` as service-aware binaries and align deployment around SCM operations.

**Tech Stack:** Go 1.22+, Windows SCM (`New-Service`, `Set-Service`, `sc.exe`), PowerShell scripts, existing WheelMaker logger/config modules.

---

## Chunk 1: Add WheelMakerUpdater Service Binary

### Task 1: Add updater command entrypoint and scheduling core

**Files:**
- Create: `server/cmd/wheelmaker-updater/main.go`
- Create: `server/cmd/wheelmaker-updater/service_windows.go`
- Create: `server/cmd/wheelmaker-updater/service_other.go`
- Create: `server/cmd/wheelmaker-updater/updater.go`
- Create: `server/cmd/wheelmaker-updater/updater_test.go`

- [ ] **Step 1: Write failing tests for time parsing and next-run calculation**

```go
func TestParseDailyTime(t *testing.T) {
    _, err := parseDailyTime("03:00")
    if err != nil { t.Fatalf("unexpected: %v", err) }
    if _, err := parseDailyTime("3:00"); err == nil { t.Fatalf("expected error") }
}

func TestNextRunAfterNow(t *testing.T) {
    now := time.Date(2026, 4, 1, 4, 0, 0, 0, time.Local)
    next := nextRunTime(now, 3, 0)
    if !next.After(now) { t.Fatalf("next=%v now=%v", next, now) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/wheelmaker-updater -run "ParseDailyTime|NextRun" -count=1`
Expected: FAIL (missing implementation)

- [ ] **Step 3: Implement minimal scheduler/time helpers**

- `parseDailyTime("HH:mm") -> (hour, minute, error)`
- `nextRunTime(now, hour, minute) time.Time`
- loop using `time.NewTimer` and context cancellation

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/wheelmaker-updater -run "ParseDailyTime|NextRun" -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/cmd/wheelmaker-updater
git commit -m "feat(updater): add daily scheduler and service entrypoint"
```

### Task 2: Implement update round executor

**Files:**
- Modify: `server/cmd/wheelmaker-updater/updater.go`
- Modify: `server/cmd/wheelmaker-updater/updater_test.go`

- [ ] **Step 1: Write failing tests for update decision flow**

Cover:
- up-to-date path (no deploy)
- new commit path (pull then deploy)
- command failure propagation

- [ ] **Step 2: Run targeted tests to observe failure**

Run: `go test ./cmd/wheelmaker-updater -run "UpdateRound" -count=1`
Expected: FAIL

- [ ] **Step 3: Implement command pipeline with dependency injection**

- Wrap external commands in interface for testability.
- Execute: fetch -> branch -> local head -> remote head -> pull (if needed) -> refresh script.
- Add structured log lines.

- [ ] **Step 4: Re-run tests**

Run: `go test ./cmd/wheelmaker-updater -run "UpdateRound" -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/cmd/wheelmaker-updater
git commit -m "feat(updater): implement git-aware update round execution"
```

## Chunk 2: Convert Deployment/Restart Scripts to Service Operations

### Task 3: Refactor `refresh_server.ps1` for three-service deployment

**Files:**
- Modify: `scripts/refresh_server.ps1`

- [ ] **Step 1: Add updater build/install paths and variables**

- add `wheelmaker-updater.exe` output/install path variables
- build updater alongside existing binaries

- [ ] **Step 2: Replace process lifecycle functions with service lifecycle functions**

- add `Ensure-Service`, `Start-ServiceSafe`, `Stop-ServiceSafe`, `Restart-ServiceSafe`
- enforce startup type `Automatic`
- use explicit service names: `WheelMaker`, `WheelMakerMonitor`, `WheelMakerUpdater`

- [ ] **Step 3: Update helper script generation to service wrappers**

- `start.bat` => starts services
- `stop.bat` => stops services
- `restart.bat` => restarts services

- [ ] **Step 4: Run static script sanity check**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/refresh_server.ps1 -WhatIf -SkipGitPull`
Expected: Script completes without parse/runtime errors.

- [ ] **Step 5: Commit**

```bash
git add scripts/refresh_server.ps1
git commit -m "refactor(scripts): deploy wheelmaker stack via windows services"
```

### Task 4: Refactor delayed restart script to service restart

**Files:**
- Modify: `scripts/delay_restart_server.ps1`

- [ ] **Step 1: Replace process control with service control**

- after delayed refresh, restart `WheelMaker` + `WheelMakerMonitor`
- keep log output format

- [ ] **Step 2: Smoke run worker path**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/delay_restart_server.ps1 -Worker -DelaySeconds 1`
Expected: logs contain refresh + service restart records.

- [ ] **Step 3: Commit**

```bash
git add scripts/delay_restart_server.ps1
git commit -m "refactor(scripts): delay restart through service operations"
```

### Task 5: Convert auto-update script from Scheduled Task manager to service manager

**Files:**
- Modify: `scripts/auto_update.ps1`

- [ ] **Step 1: Remove scheduled task setup/uninstall code paths**
- [ ] **Step 2: Add updater service setup/uninstall/status/one-shot paths**
- [ ] **Step 3: Validate help + setup path**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/auto_update.ps1 -Setup -Time 03:00`
Expected: updater service created/updated and set to automatic.

- [ ] **Step 4: Commit**

```bash
git add scripts/auto_update.ps1
git commit -m "refactor(scripts): manage updater as windows service"
```

## Chunk 3: Wiring, Verification, and Documentation

### Task 6: Build + tests + script smoke verification

**Files:**
- Modify: `server/go.mod` (if needed)
- Modify: `server/go.sum` (if needed)

- [ ] **Step 1: Run updater unit tests**

Run: `go test ./cmd/wheelmaker-updater -count=1`
Expected: PASS

- [ ] **Step 2: Run core server tests**

Run: `go test ./...`
Expected: PASS (or only known pre-existing failures)

- [ ] **Step 3: Run refresh script in WhatIf mode**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/refresh_server.ps1 -WhatIf -SkipGitPull`
Expected: PASS

### Task 7: Update docs and operator notes

**Files:**
- Modify: `server/CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Document service-first deployment and updater service behavior**
- [ ] **Step 2: Document removal of scheduled-task mode for auto update**
- [ ] **Step 3: Commit docs**

```bash
git add server/CLAUDE.md README.md
git commit -m "docs: describe windows service deployment and updater service"
```

### Task 8: Final integration commit

**Files:**
- Modify: all touched files above

- [ ] **Step 1: Final verification rerun**

Run:
- `go test ./cmd/wheelmaker-updater -count=1`
- `go test ./...`

- [ ] **Step 2: Final commit**

```bash
git add -A
git commit -m "feat(windows): run wheelmaker, monitor, and updater as auto-start services"
```

- [ ] **Step 3: Push**

```bash
git push origin <branch>
```
