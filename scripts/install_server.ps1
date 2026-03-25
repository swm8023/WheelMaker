param(
  [string]$InstallDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\bin"),
  [string]$SourceExe = "",
  [switch]$SkipDeps,
  [switch]$WhatIf
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Step {
  param([string]$Text)
  Write-Host ("==> {0}" -f $Text)
}

function Ensure-Dependencies {
  if ($SkipDeps) {
    Write-Step "skip dependency install/check (-SkipDeps)"
    return
  }
  if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    throw "npm not found in PATH"
  }
  $missing = @()
  if (-not (Get-Command codex-acp -ErrorAction SilentlyContinue)) {
    $missing += "@zed-industries/codex-acp"
  }
  if (-not (Get-Command claude-agent-acp -ErrorAction SilentlyContinue)) {
    $missing += "@zed-industries/claude-agent-acp"
  }
  if ($missing.Count -eq 0) {
    Write-Step "dependencies already installed"
    return
  }
  Write-Step ("install dependencies: {0}" -f ($missing -join ", "))
  if ($WhatIf) {
    Write-Host ("[whatif] npm install -g {0}" -f ($missing -join " "))
    return
  }
  & npm install -g @missing
  if ($LASTEXITCODE -ne 0) {
    throw ("npm install failed (exit={0})" -f $LASTEXITCODE)
  }
}

function Stop-WheelmakerProcesses {
  $all = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue)
  if ($all.Count -eq 0) {
    Write-Step "no running wheelmaker process found"
    return
  }
  Write-Step ("stopping wheelmaker pids: {0}" -f (($all | ForEach-Object { $_.ProcessId }) -join ","))
  foreach ($p in $all) {
    if ($WhatIf) {
      Write-Host ("[whatif] stop pid={0}" -f $p.ProcessId)
      continue
    }
    Stop-Process -Id $p.ProcessId -ErrorAction SilentlyContinue
  }
  if ($WhatIf) {
    return
  }
  Start-Sleep -Seconds 2
  $left = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue)
  foreach ($p in $left) {
    Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
  }
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$serverRoot = Join-Path $repoRoot "server"
if ([string]::IsNullOrWhiteSpace($SourceExe)) {
  $SourceExe = Join-Path $serverRoot "bin\windows_amd64\wheelmaker.exe"
}
$source = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($SourceExe)
$targetDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($InstallDir)
$targetExe = Join-Path -Path $targetDir -ChildPath "wheelmaker.exe"

if (-not (Test-Path $source)) {
  throw ("source binary not found: {0}" -f $source)
}

Ensure-Dependencies
Write-Step ("source: {0}" -f $source)
Write-Step ("target: {0}" -f $targetExe)
Stop-WheelmakerProcesses

if ($WhatIf) {
  Write-Host ("[whatif] copy {0} -> {1}" -f $source, $targetExe)
  Write-Host "[whatif] install done"
  exit 0
}

New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
Copy-Item -Path $source -Destination $targetExe -Force
Write-Step "install done"
