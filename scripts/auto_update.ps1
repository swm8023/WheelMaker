<#
.SYNOPSIS
  WheelMaker auto-update: check git for new commits and deploy, or manage the scheduled task.

.PARAMETER Setup
  Register a Windows scheduled task to run this script daily. Combine with -Time to set the hour.

.PARAMETER Uninstall
  Remove the scheduled task.

.PARAMETER Time
  Time for the daily task (default "03:00"). Only used with -Setup.

.PARAMETER RepoDir
  Path to the WheelMaker repository root. Defaults to the parent of this script's directory.

.PARAMETER Worker
  Internal flag — runs the actual check/deploy logic in background.
#>
param(
  [switch]$Setup,
  [switch]$Uninstall,
  [string]$Time = "03:00",
  [string]$RepoDir,
  [switch]$Worker
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Continue"

if (-not $RepoDir) {
  $RepoDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
}

$baseDir  = Join-Path -Path $HOME -ChildPath ".wheelmaker"
$logPath  = Join-Path -Path $baseDir -ChildPath "auto_update.log"
$taskName = "WheelMaker-AutoUpdate"

if (-not (Test-Path $baseDir)) {
  New-Item -ItemType Directory -Path $baseDir -Force | Out-Null
}

function Write-Log {
  param([string]$Message)
  $ts = Get-Date -Format o
  Add-Content -Path $logPath -Value "[$ts] $Message"
}

# ── Uninstall scheduled task ──
if ($Uninstall) {
  $ErrorActionPreference = "Stop"
  $existing = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  if ($existing) {
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
    Write-Host "Removed scheduled task: $taskName"
  } else {
    Write-Host "Task '$taskName' not found, nothing to remove."
  }
  exit 0
}

# ── Setup scheduled task ──
if ($Setup) {
  $ErrorActionPreference = "Stop"
  $action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`" -RepoDir `"$RepoDir`""

  $trigger = New-ScheduledTaskTrigger -Daily -At $Time

  $settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -ExecutionTimeLimit (New-TimeSpan -Hours 1)

  $existing = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  if ($existing) {
    Set-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings | Out-Null
    Write-Host "Updated scheduled task: $taskName (daily at $Time)"
  } else {
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Description "WheelMaker nightly git pull and deploy" | Out-Null
    Write-Host "Created scheduled task: $taskName (daily at $Time)"
  }

  Write-Host "  RepoDir: $RepoDir"
  Write-Host "  Log:     $logPath"
  Write-Host ""
  Write-Host "To remove: powershell -File `"$PSCommandPath`" -Uninstall"
  exit 0
}

# ── Spawn hidden worker and exit immediately ──
if (-not $Worker) {
  Start-Process -FilePath "powershell" -ArgumentList @(
    "-NoProfile",
    "-ExecutionPolicy", "Bypass",
    "-File", $PSCommandPath,
    "-Worker",
    "-RepoDir", $RepoDir
  ) -WindowStyle Hidden | Out-Null
  Write-Log "scheduled auto-update worker repo=$RepoDir"
  exit 0
}

# ── Worker logic ──
Write-Log "auto-update check begin repo=$RepoDir"

Push-Location $RepoDir
try {
  # Fetch remote changes
  $fetchOut = git fetch origin 2>&1
  if ($LASTEXITCODE -ne 0) {
    Write-Log "git fetch failed: $fetchOut"
    exit 1
  }

  # Compare local HEAD with remote
  $localHead  = git rev-parse HEAD 2>&1
  $branch     = git rev-parse --abbrev-ref HEAD 2>&1
  $remoteHead = git rev-parse "origin/$branch" 2>&1

  if ($LASTEXITCODE -ne 0) {
    Write-Log "git rev-parse failed: local=$localHead remote=$remoteHead branch=$branch"
    exit 1
  }

  if ($localHead -eq $remoteHead) {
    Write-Log "already up-to-date ($branch @ $($localHead.Substring(0,8)))"
    exit 0
  }

  # Count new commits
  $newCommits = git log --oneline "$localHead..$remoteHead" 2>&1
  $count = ($newCommits | Measure-Object -Line).Lines
  Write-Log "found $count new commit(s) on $branch ($($localHead.Substring(0,8)) -> $($remoteHead.Substring(0,8)))"

  # Run deploy via refresh_server.ps1
  $refreshScript = Join-Path -Path $PSScriptRoot -ChildPath "refresh_server.ps1"
  if (-not (Test-Path $refreshScript)) {
    Write-Log "refresh script missing: $refreshScript"
    exit 1
  }

  Write-Log "starting deploy..."
  powershell -NoProfile -ExecutionPolicy Bypass -File $refreshScript
  $exitCode = $LASTEXITCODE

  if ($exitCode -ne 0) {
    Write-Log "deploy failed exit=$exitCode"
    exit $exitCode
  }

  $currentHead = git rev-parse HEAD 2>&1
  Write-Log "deploy completed ($branch @ $($currentHead.Substring(0,8)))"
} finally {
  Pop-Location
}
