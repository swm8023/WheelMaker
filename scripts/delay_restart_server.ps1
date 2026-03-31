param(
  [switch]$Worker,
  [int]$DelaySeconds = 30,
  [string]$InstallDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\bin")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Continue"

$baseDir = Join-Path -Path $HOME -ChildPath ".wheelmaker"
$logPath = Join-Path -Path $baseDir -ChildPath "delay_restart_server.log"
$refreshScript = Join-Path -Path $PSScriptRoot -ChildPath "refresh_server.ps1"

if (-not (Test-Path $baseDir)) {
  New-Item -ItemType Directory -Path $baseDir -Force | Out-Null
}

function Write-Log {
  param([string]$Message)
  Add-Content -Path $logPath -Value ("[{0}] {1}" -f (Get-Date -Format o), $Message)
}

if (-not $Worker) {
  Start-Process -FilePath "powershell" -ArgumentList @(
    "-NoProfile",
    "-ExecutionPolicy", "Bypass",
    "-File", $PSCommandPath,
    "-Worker",
    "-DelaySeconds", "$DelaySeconds",
    "-InstallDir", $InstallDir
  ) -WindowStyle Hidden | Out-Null
  Write-Log ("scheduled restart worker delay={0}s" -f $DelaySeconds)
  exit 0
}

Write-Log ("restart worker begin delay={0}s" -f $DelaySeconds)
Start-Sleep -Seconds $DelaySeconds

if (-not (Test-Path $refreshScript)) {
  Write-Log ("refresh script missing: {0}" -f $refreshScript)
  exit 1
}

powershell -NoProfile -ExecutionPolicy Bypass -File $refreshScript -SkipGitPull -InstallDir $InstallDir
if ($LASTEXITCODE -ne 0) {
  Write-Log ("refresh failed exit={0}" -f $LASTEXITCODE)
  exit $LASTEXITCODE
}
Write-Log "refresh complete"

try {
  Restart-Service -Name "WheelMaker" -Force -ErrorAction Stop
  Write-Log "restarted service WheelMaker"
} catch {
  Write-Log ("restart service WheelMaker failed: {0}" -f $_.Exception.Message)
  exit 1
}

try {
  Restart-Service -Name "WheelMakerMonitor" -Force -ErrorAction Stop
  Write-Log "restarted service WheelMakerMonitor"
} catch {
  Write-Log ("restart service WheelMakerMonitor failed: {0}" -f $_.Exception.Message)
  exit 1
}

Write-Log "delay restart finished"
