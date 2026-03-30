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

  $startBat = @'
@echo off
set "exe=%~dp0bin\wheelmaker.exe"
if not exist "%exe%" (
  echo wheelmaker.exe not found: %exe%
  exit /b 1
)
tasklist /FI "IMAGENAME eq wheelmaker.exe" 2>nul | find /I "wheelmaker.exe" >nul
if %errorlevel%==0 (
  echo wheelmaker already running
  exit /b 0
)
powershell -NoProfile -Command "Start-Process '%exe%' '-d' -WindowStyle Hidden"
timeout /t 3 /nobreak >nul
tasklist /FI "IMAGENAME eq wheelmaker.exe" 2>nul | find /I "wheelmaker.exe" >nul
if %errorlevel%==0 (
  echo wheelmaker started
) else (
  echo wheelmaker failed to start
  exit /b 1
)
'@

  $stopBat = @'
@echo off
tasklist /FI "IMAGENAME eq wheelmaker.exe" 2>nul | find /I "wheelmaker.exe" >nul
if %errorlevel% neq 0 (
  echo no running wheelmaker process
  exit /b 0
)
taskkill /IM wheelmaker.exe >nul 2>&1
timeout /t 2 /nobreak >nul
taskkill /IM wheelmaker.exe /F >nul 2>&1
echo wheelmaker stopped
'@

  $restartBat = @'
@echo off
call "%~dp0stop.bat"
call "%~dp0start.bat"
'@

  $scripts = @{
    "start.bat"   = $startBat
    "stop.bat"    = $stopBat
    "restart.bat" = $restartBat
  }

  foreach ($name in $scripts.Keys) {
    $path = Join-Path $wmDir $name
    if ($WhatIf) {
      Write-Host ("[whatif] write {0}" -f $path)
      continue
    }
    Set-Content -Path $path -Value $scripts[$name] -Encoding UTF8
  }
  Write-Step "service scripts installed (start.bat/stop.bat/restart.bat)"
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
