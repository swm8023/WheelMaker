<#
.SYNOPSIS
  Registers or removes the WheelMaker nightly auto-update scheduled task.

.PARAMETER Uninstall
  If specified, removes the scheduled task instead of creating it.

.PARAMETER Time
  The time to run the task daily. Defaults to "03:00" (3 AM).

.PARAMETER RepoDir
  Path to the WheelMaker repository root. Defaults to the parent of this script's directory.
#>
param(
  [switch]$Uninstall,
  [string]$Time = "03:00",
  [string]$RepoDir
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$taskName = "WheelMaker-AutoUpdate"

if ($Uninstall) {
  $existing = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  if ($existing) {
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
    Write-Host "Removed scheduled task: $taskName"
  } else {
    Write-Host "Task '$taskName' not found, nothing to remove."
  }
  exit 0
}

if (-not $RepoDir) {
  $RepoDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
}

$scriptPath = Join-Path $PSScriptRoot "auto_update.ps1"
if (-not (Test-Path $scriptPath)) {
  Write-Error "auto_update.ps1 not found at: $scriptPath"
  exit 1
}

$action = New-ScheduledTaskAction `
  -Execute "powershell.exe" `
  -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$scriptPath`" -RepoDir `"$RepoDir`""

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

Write-Host "  Script:  $scriptPath"
Write-Host "  RepoDir: $RepoDir"
Write-Host "  Log:     $HOME\.wheelmaker\auto_update.log"
Write-Host ""
Write-Host "To remove: powershell -File `"$PSCommandPath`" -Uninstall"
