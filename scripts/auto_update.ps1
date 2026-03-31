<#
.SYNOPSIS
  Checks for git updates and runs deploy if new commits are found.
  Designed to be called by Windows Task Scheduler (e.g. daily at 03:00).

.PARAMETER RepoDir
  Path to the WheelMaker repository root. Defaults to the parent of this script's directory.

.PARAMETER Worker
  Internal flag — runs the actual check/deploy logic. Without it the script
  spawns a hidden worker process so the Task Scheduler job returns immediately.
#>
param(
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

if (-not (Test-Path $baseDir)) {
  New-Item -ItemType Directory -Path $baseDir -Force | Out-Null
}

function Write-Log {
  param([string]$Message)
  $ts = Get-Date -Format o
  Add-Content -Path $logPath -Value "[$ts] $Message"
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
