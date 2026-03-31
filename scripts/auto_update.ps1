<#
.SYNOPSIS
  WheelMaker updater service manager.

.PARAMETER Setup
  Create or update WheelMakerUpdater Windows service.

.PARAMETER Uninstall
  Stop and remove WheelMakerUpdater service.

.PARAMETER Time
  Daily update time in HH:mm, default 03:00.

.PARAMETER RepoDir
  WheelMaker repository path, default parent of scripts directory.

.PARAMETER InstallDir
  WheelMaker install directory, default ~/.wheelmaker/bin.

.PARAMETER Once
  Run updater once immediately and exit.
#>
param(
  [switch]$Setup,
  [switch]$Uninstall,
  [switch]$Once,
  [string]$Time = "03:00",
  [string]$RepoDir,
  [string]$InstallDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\bin")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $RepoDir) {
  $RepoDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
}

$baseDir = Join-Path -Path $HOME -ChildPath ".wheelmaker"
$logPath = Join-Path -Path $baseDir -ChildPath "auto_update.log"
$serviceName = "WheelMakerUpdater"
$updaterExe = Join-Path $InstallDir "wheelmaker-updater.exe"

if (-not (Test-Path $baseDir)) {
  New-Item -ItemType Directory -Path $baseDir -Force | Out-Null
}

function Write-Log {
  param([string]$Message)
  Add-Content -Path $logPath -Value ("[{0}] {1}" -f (Get-Date -Format o), $Message)
}

function Invoke-Checked {
  param([string]$FilePath, [string[]]$Arguments, [string]$Fail)
  & $FilePath @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw ("{0} (exit={1})" -f $Fail, $LASTEXITCODE)
  }
}

function Service-Exists {
  return $null -ne (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)
}

function Ensure-Service {
  if (-not (Test-Path $updaterExe)) {
    throw ("updater executable not found: {0}. Run scripts\refresh_server.ps1 first." -f $updaterExe)
  }

  $binPath = ('"{0}" --repo "{1}" --install-dir "{2}" --time {3}' -f $updaterExe, $RepoDir, $InstallDir, $Time)
  if (Service-Exists) {
    Invoke-Checked -FilePath "sc.exe" -Arguments @("config", $serviceName, "binPath=", $binPath, "start=", "auto") -Fail "service update failed"
  } else {
    Invoke-Checked -FilePath "sc.exe" -Arguments @("create", $serviceName, "binPath=", $binPath, "start=", "auto") -Fail "service create failed"
  }
}

if ($Uninstall) {
  if (Service-Exists) {
    Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
    Invoke-Checked -FilePath "sc.exe" -Arguments @("delete", $serviceName) -Fail "service delete failed"
    Write-Host "Removed service: $serviceName"
    Write-Log "service removed"
  } else {
    Write-Host "Service '$serviceName' not found"
  }
  exit 0
}

if ($Setup) {
  Ensure-Service
  Start-Service -Name $serviceName -ErrorAction SilentlyContinue
  Write-Host "Service ready: $serviceName (daily at $Time)"
  Write-Host "  RepoDir:    $RepoDir"
  Write-Host "  InstallDir: $InstallDir"
  Write-Host "  Log:        $logPath"
  Write-Log ("service setup completed repo={0} time={1}" -f $RepoDir, $Time)
  exit 0
}

if ($Once) {
  if (-not (Test-Path $updaterExe)) {
    throw ("updater executable not found: {0}" -f $updaterExe)
  }
  Write-Log "manual one-shot update begin"
  & $updaterExe --repo $RepoDir --install-dir $InstallDir --time $Time --once
  if ($LASTEXITCODE -ne 0) {
    Write-Log ("manual one-shot update failed exit={0}" -f $LASTEXITCODE)
    exit $LASTEXITCODE
  }
  Write-Log "manual one-shot update completed"
  Write-Host "One-shot update finished"
  exit 0
}

if (Service-Exists) {
  $svc = Get-Service -Name $serviceName
  Write-Host ("{0}: {1}" -f $serviceName, $svc.Status)
} else {
  Write-Host ("{0}: not installed" -f $serviceName)
}
Write-Host "Use -Setup to install/update service, -Once to run one-shot update, -Uninstall to remove service."
