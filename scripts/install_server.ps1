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

function Ensure-DefaultConfig {
  $wmDir = Join-Path -Path $HOME -ChildPath ".wheelmaker"
  $cfgPath = Join-Path -Path $wmDir -ChildPath "config.json"
  if (Test-Path $cfgPath) {
    Write-Step ("config already exists: {0}" -f $cfgPath)
    return
  }
  Write-Step ("generating default config: {0}" -f $cfgPath)
  if ($WhatIf) {
    Write-Host ("[whatif] write {0}" -f $cfgPath)
    return
  }
  New-Item -ItemType Directory -Path $wmDir -Force | Out-Null
  $defaultConfig = @'
{
  "projects": [
    {
      "name": "my-project",
      "path": "",
      "im": {
        "type": "feishu",
        "appID": "",
        "appSecret": ""
      },
      "client": {
        "agent": "claude"
      }
    }
  ],
  "registry": {
    "listen": true,
    "port": 9630,
    "server": "127.0.0.1",
    "token": "",
    "hubId": ""
  },
  "log": {
    "level": "warn"
  }
}
'@
  Set-Content -Path $cfgPath -Value $defaultConfig -Encoding UTF8
  Write-Step "default config written (edit before first run)"
}

function Install-ServiceScripts {
  $wmDir = Join-Path -Path $HOME -ChildPath ".wheelmaker"
  New-Item -ItemType Directory -Path $wmDir -Force | Out-Null

  $startScript = @'
# WheelMaker start script — launch daemon in background
$exe = Join-Path $PSScriptRoot "bin\wheelmaker.exe"
if (-not (Test-Path $exe)) {
  Write-Host "wheelmaker.exe not found: $exe" -ForegroundColor Red
  exit 1
}
$running = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue)
if ($running.Count -gt 0) {
  Write-Host ("wheelmaker already running (pids: {0})" -f (($running | ForEach-Object { $_.ProcessId }) -join ",")) -ForegroundColor Yellow
  exit 0
}
Start-Process -FilePath $exe -ArgumentList "-d" -WindowStyle Hidden
Start-Sleep -Seconds 3
$procs = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue)
if ($procs.Count -gt 0) {
  Write-Host ("wheelmaker started (pids: {0})" -f (($procs | ForEach-Object { $_.ProcessId }) -join ","))
} else {
  Write-Host "wheelmaker failed to start" -ForegroundColor Red
  exit 1
}
'@

  $stopScript = @'
# WheelMaker stop script — stop all wheelmaker processes
$all = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue)
if ($all.Count -eq 0) {
  Write-Host "no running wheelmaker process"
  exit 0
}
Write-Host ("stopping pids: {0}" -f (($all | ForEach-Object { $_.ProcessId }) -join ","))
foreach ($p in $all) {
  Stop-Process -Id $p.ProcessId -ErrorAction SilentlyContinue
}
Start-Sleep -Seconds 2
$left = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue)
foreach ($p in $left) {
  Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
}
Write-Host "wheelmaker stopped"
'@

  $restartScript = @'
# WheelMaker restart script — stop then start
& (Join-Path $PSScriptRoot "stop.ps1")
& (Join-Path $PSScriptRoot "start.ps1")
'@

  $scripts = @{
    "start.ps1"   = $startScript
    "stop.ps1"    = $stopScript
    "restart.ps1" = $restartScript
  }

  foreach ($name in $scripts.Keys) {
    $path = Join-Path $wmDir $name
    if ($WhatIf) {
      Write-Host ("[whatif] write {0}" -f $path)
      continue
    }
    Set-Content -Path $path -Value $scripts[$name] -Encoding UTF8
  }
  Write-Step "service scripts installed (start/stop/restart)"
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
Ensure-DefaultConfig
Install-ServiceScripts
Write-Step "install done"
