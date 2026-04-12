# Feishu Project Config Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Configure the local WheelMaker runtime so both `wheelmaker` and `BillBoard` run as YOLO-enabled Feishu projects with tool updates filtered from IM output.

**Architecture:** This change is operational rather than application-code heavy. We will update the single local runtime config at `C:\Users\fjxyy\.wheelmaker\config.json`, then reload the existing user-mode WheelMaker processes so the hub recreates its project clients from the new config. Verification focuses on JSON validity, process health, and project registration rather than unit tests.

**Tech Stack:** JSON config, PowerShell, WheelMaker user-mode scripts, Feishu runtime already built into `server/internal/im/feishu`

---

## File Map

- Modify: `C:\Users\fjxyy\.wheelmaker\config.json`
- Create: `C:\Users\fjxyy\.wheelmaker\config.backup.2026-04-12.json`
- Use: `C:\Users\fjxyy\.wheelmaker\stop.bat`
- Use: `C:\Users\fjxyy\.wheelmaker\start.bat`
- Inspect: `C:\Users\fjxyy\.wheelmaker\log\`

### Task 1: Update Local Runtime Config

**Files:**
- Modify: `C:\Users\fjxyy\.wheelmaker\config.json`
- Create: `C:\Users\fjxyy\.wheelmaker\config.backup.2026-04-12.json`

- [ ] **Step 1: Back up the current runtime config**

Run:

```powershell
$wmHome = Join-Path $env:USERPROFILE ".wheelmaker"
$configPath = Join-Path $wmHome "config.json"
$backupPath = Join-Path $wmHome "config.backup.2026-04-12.json"
Copy-Item -LiteralPath $configPath -Destination $backupPath -Force
Get-Item $backupPath | Select-Object FullName, Length, LastWriteTime
```

Expected: one backup file exists at `C:\Users\fjxyy\.wheelmaker\config.backup.2026-04-12.json`.

- [ ] **Step 2: Replace the config with the final two-project JSON**

Use an interactive PowerShell write so the secrets come from the current operator session and never get written into repo-tracked docs:

```powershell
$wheelmakerSecret = Read-Host "WheelMaker App Secret"
$billboardSecret = Read-Host "BillBoard App Secret"

$config = [ordered]@{
  registry = [ordered]@{
    listen = $true
    port = 9630
    server = "127.0.0.1"
    hubId = "LITTLECLAW"
    token = ""
  }
  projects = @(
    [ordered]@{
      path = "D:\GithubRepos\WheelMaker"
      name = "wheelmaker"
      yolo = $true
      feishu = [ordered]@{
        app_id = "cli_a951a49417f9dbd2"
        app_secret = $wheelmakerSecret
      }
      imFilter = [ordered]@{
        block = @("tool", "tool_call")
      }
    },
    [ordered]@{
      path = "D:\GithubRepos\BillBoard"
      name = "BillBoard"
      yolo = $true
      feishu = [ordered]@{
        app_id = "cli_a951abb004fddbc9"
        app_secret = $billboardSecret
      }
      imFilter = [ordered]@{
        block = @("tool", "tool_call")
      }
    }
  )
  monitor = [ordered]@{
    port = 9631
  }
  log = [ordered]@{
    level = "warn"
  }
}

$configPath = Join-Path $env:USERPROFILE ".wheelmaker\config.json"
$config | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $configPath -Encoding UTF8
```

- [ ] **Step 3: Validate that the edited config is valid JSON**

Run:

```powershell
$configPath = Join-Path $env:USERPROFILE ".wheelmaker\config.json"
$cfg = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
$cfg.projects | Select-Object name, path, yolo
```

Expected: `ConvertFrom-Json` succeeds and prints exactly two projects named `wheelmaker` and `BillBoard`, both with `yolo` equal to `True`.

- [ ] **Step 4: Validate the Feishu and filter fields after parse**

Run:

```powershell
$configPath = Join-Path $env:USERPROFILE ".wheelmaker\config.json"
$cfg = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
$cfg.projects | ForEach-Object {
  [PSCustomObject]@{
    name = $_.name
    app_id = $_.feishu.app_id
    has_secret = [string]::IsNullOrWhiteSpace($_.feishu.app_secret) -eq $false
    blocked = ($_.imFilter.block -join ",")
  }
}
```

Expected: both projects show the intended `app_id`, `has_secret=True`, and `blocked=tool,tool_call`.

### Task 2: Reload The User-Mode WheelMaker Runtime

**Files:**
- Use: `C:\Users\fjxyy\.wheelmaker\stop.bat`
- Use: `C:\Users\fjxyy\.wheelmaker\start.bat`

- [ ] **Step 1: Stop the current user-mode processes cleanly**

Run:

```powershell
& "$env:USERPROFILE\.wheelmaker\stop.bat"
```

Expected: output includes stop attempts for `wheelmaker-updater`, `wheelmaker-monitor`, and `wheelmaker`, with no final error.

- [ ] **Step 2: Verify the managed processes are no longer running**

Run:

```powershell
Get-Process | Where-Object { $_.ProcessName -in @('wheelmaker', 'wheelmaker-monitor', 'wheelmaker-updater') } |
  Select-Object ProcessName, Id, StartTime
```

Expected: no rows are returned.

- [ ] **Step 3: Start the runtime again with the updated config**

Run:

```powershell
& "$env:USERPROFILE\.wheelmaker\start.bat"
```

Expected: output shows `start wheelmaker`, `start wheelmaker-monitor`, and `start wheelmaker-updater`, then completes without error.

- [ ] **Step 4: Verify the expected processes are running again**

Run:

```powershell
Get-Process | Where-Object { $_.ProcessName -in @('wheelmaker', 'wheelmaker-monitor', 'wheelmaker-updater') } |
  Select-Object ProcessName, Id, StartTime, Path
```

Expected: rows are returned for all three managed executables under `C:\Users\fjxyy\.wheelmaker\bin`.

### Task 3: Verify Project Registration And Feishu Routing

**Files:**
- Inspect: `C:\Users\fjxyy\.wheelmaker\config.json`
- Inspect: `C:\Users\fjxyy\.wheelmaker\log\`

- [ ] **Step 1: Verify registry and monitor ports came back after restart**

Run:

```powershell
$ports = 9630, 9631
foreach ($port in $ports) {
  Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue |
    Select-Object LocalAddress, LocalPort, State, OwningProcess
}
```

Expected: listeners exist for ports `9630` and `9631`.

- [ ] **Step 2: Verify the runtime now sees both configured projects**

Run:

```powershell
$configPath = Join-Path $env:USERPROFILE ".wheelmaker\config.json"
$cfg = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
$cfg.projects.Count
$cfg.projects.name
```

Expected: count is `2`, and names are `wheelmaker` and `BillBoard`.

- [ ] **Step 3: Inspect recent logs for Feishu channel registration**

Run:

```powershell
$logDir = Join-Path $env:USERPROFILE ".wheelmaker\log"
Get-ChildItem $logDir -File |
  Sort-Object LastWriteTime -Descending |
  Select-Object -First 5 FullName, LastWriteTime
```

If a current wheelmaker log exists, inspect the newest one:

```powershell
$logFile = Get-ChildItem (Join-Path $env:USERPROFILE ".wheelmaker\log") -File |
  Sort-Object LastWriteTime -Descending |
  Select-Object -First 1 -ExpandProperty FullName
Get-Content -LiteralPath $logFile -Tail 200
```

Expected: recent log output includes project startup activity and Feishu registration attempts after the restart.

- [ ] **Step 4: Run the end-to-end manual chat check**

Manual verification:

```text
1. Send one plain text message to the WheelMaker bot bound to app_id cli_a951a49417f9dbd2.
2. Confirm the response comes from the D:\GithubRepos\WheelMaker project.
3. Send one plain text message to the BillBoard bot bound to app_id cli_a951abb004fddbc9.
4. Confirm the response comes from the D:\GithubRepos\BillBoard project.
5. Watch the Feishu conversation and confirm there is no standalone tool/tool_call noise.
```

Expected: both bots respond, each routes to the correct project, and tool updates stay filtered from chat output.
