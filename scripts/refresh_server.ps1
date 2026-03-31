param(
  [string]$InstallDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\bin"),
  [string]$OutputPath = "",
  [string]$SourceExe = "",
  [switch]$SkipGitPull,
  [switch]$SkipDeps,
  [switch]$SkipBuild,
  [switch]$SkipInstall,
  [switch]$SkipRestart,
  [switch]$WhatIf
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Step {
  param([string]$Text)
  Write-Host ("==> {0}" -f $Text)
}

function Write-Warn {
  param([string]$Text)
  Write-Warning $Text
}

function Assert-Command {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [string]$Hint = ""
  )

  if (Get-Command $Name -ErrorAction SilentlyContinue) {
    return
  }

  if ([string]::IsNullOrWhiteSpace($Hint)) {
    throw ("required command not found in PATH: {0}" -f $Name)
  }
  throw ("required command not found in PATH: {0}. {1}" -f $Name, $Hint)
}

function Invoke-Checked {
  param(
    [Parameter(Mandatory = $true)]
    [string]$FilePath,
    [string[]]$Arguments = @(),
    [string]$FailureMessage = ""
  )

  & $FilePath @Arguments
  if ($LASTEXITCODE -ne 0) {
    if ([string]::IsNullOrWhiteSpace($FailureMessage)) {
      throw ("command failed: {0} {1} (exit={2})" -f $FilePath, ($Arguments -join " "), $LASTEXITCODE)
    }
    throw ("{0} (exit={1})" -f $FailureMessage, $LASTEXITCODE)
  }
}

function Get-ResolvedPathOrDefault {
  param(
    [string]$Path,
    [string]$Default
  )

  $target = $Path
  if ([string]::IsNullOrWhiteSpace($target)) {
    $target = $Default
  }
  return $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($target)
}

function Get-GitCurrentBranch {
  Push-Location $script:RepoRoot
  try {
    $branch = ((& git branch --show-current) | Select-Object -First 1)
    if ($LASTEXITCODE -ne 0) {
      throw ("git branch --show-current failed (exit={0})" -f $LASTEXITCODE)
    }
    $branch = [string]$branch
    $branch = $branch.Trim()
    if ($branch -eq "") {
      throw "repository is in detached HEAD state; cannot pull latest automatically"
    }
    return $branch
  }
  finally {
    Pop-Location
  }
}

function Assert-CleanGitWorktree {
  Push-Location $script:RepoRoot
  try {
    $status = @(& git status --porcelain)
    if ($LASTEXITCODE -ne 0) {
      throw ("git status failed (exit={0})" -f $LASTEXITCODE)
    }
    if (@($status | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }).Count -gt 0) {
      throw "git worktree has local changes; commit, stash, or revert them before running refresh_server.ps1"
    }
  }
  finally {
    Pop-Location
  }
}

function Pull-Latest {
  if ($SkipGitPull) {
    Write-Step "skip git pull"
    return
  }

  Assert-Command -Name "git" -Hint "Install Git and ensure git.exe is available."
  Assert-CleanGitWorktree
  $branch = Get-GitCurrentBranch

  Write-Step ("git pull --ff-only origin {0}" -f $branch)
  if ($WhatIf) {
    Write-Host ("[whatif] git pull --ff-only origin {0}" -f $branch)
    return
  }

  Push-Location $script:RepoRoot
  try {
    Invoke-Checked -FilePath "git" -Arguments @("pull", "--ff-only", "origin", $branch) -FailureMessage "git pull failed"
  }
  finally {
    Pop-Location
  }
}

function Ensure-AcpDependencies {
  if ($SkipDeps) {
    Write-Step "skip dependency install/check (-SkipDeps)"
    return
  }

  Assert-Command -Name "npm" -Hint "Install Node.js 22+."
  $missing = @()
  if (-not (Get-Command codex-acp -ErrorAction SilentlyContinue)) {
    $missing += "@zed-industries/codex-acp"
  }
  if (-not (Get-Command claude-agent-acp -ErrorAction SilentlyContinue)) {
    $missing += "@zed-industries/claude-agent-acp"
  }

  if ($missing.Count -eq 0) {
    Write-Step "ACP dependencies already installed"
    return
  }

  Write-Step ("install ACP dependencies: {0}" -f ($missing -join ", "))
  if ($WhatIf) {
    Write-Host ("[whatif] npm install -g {0}" -f ($missing -join " "))
    return
  }

  Invoke-Checked -FilePath "npm" -Arguments (@("install", "-g") + $missing) -FailureMessage "npm install failed"
}

function Read-ConfigJson {
  param([Parameter(Mandatory = $true)][string]$Path)

  try {
    return (Get-Content -Path $Path -Raw -Encoding UTF8 | ConvertFrom-Json)
  }
  catch {
    throw ("config is not valid JSON: {0}" -f $Path)
  }
}

function Validate-ExistingConfig {
  param([Parameter(Mandatory = $true)]$Config)

  if ($null -eq $Config.projects -or @($Config.projects).Count -eq 0) {
    Write-Warn "config has no projects configured yet"
    return
  }

  foreach ($project in @($Config.projects)) {
    $name = [string]$project.name
    if ([string]::IsNullOrWhiteSpace($name)) {
      Write-Warn "config contains a project without a name"
    }

    $path = [string]$project.path
    if ([string]::IsNullOrWhiteSpace($path)) {
      Write-Warn ("project '{0}' has an empty path" -f $name)
    } elseif ($path -match "^/path/to/" -or $path -match "\\/path\\/to\\/") {
      Write-Warn ("project '{0}' still uses the example path: {1}" -f $name, $path)
    } elseif (-not (Test-Path $path)) {
      Write-Warn ("project '{0}' path does not exist: {1}" -f $name, $path)
    }

    $agent = [string]$project.client.agent
    if ($agent -eq "copilot" -and -not (Get-Command copilot -ErrorAction SilentlyContinue)) {
      Write-Warn ("project '{0}' uses agent 'copilot' but the 'copilot' CLI was not found in PATH" -f $name)
    }
  }
}

function Ensure-Config {
  $configPath = $script:ConfigPath
  if (Test-Path $configPath) {
    Write-Step ("config already exists: {0}" -f $configPath)
    $config = Read-ConfigJson -Path $configPath
    Validate-ExistingConfig -Config $config
    return $false
  }

  if (-not (Test-Path $script:ConfigExamplePath)) {
    throw ("config example missing: {0}" -f $script:ConfigExamplePath)
  }

  Write-Step ("create config from example: {0}" -f $configPath)
  if ($WhatIf) {
    Write-Host ("[whatif] copy {0} -> {1}" -f $script:ConfigExamplePath, $configPath)
    return $true
  }

  New-Item -ItemType Directory -Path $script:WheelmakerHome -Force | Out-Null
  Copy-Item -Path $script:ConfigExamplePath -Destination $configPath -Force
  Write-Warn ("config was created from example: {0}" -f $configPath)
  Write-Warn "edit config.json before the first restart, then rerun scripts\refresh_server.ps1"
  return $true
}

function Install-ServiceScripts {
  Write-Step "install service helper scripts"

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
powershell -NoProfile -ExecutionPolicy Bypass -Command "$procs = @(Get-CimInstance Win32_Process -Filter ""Name='wheelmaker.exe'"" -ErrorAction SilentlyContinue); if ($procs.Count -eq 0) { Write-Host 'no running wheelmaker process'; exit 0 }; foreach ($p in $procs) { Stop-Process -Id $p.ProcessId -ErrorAction SilentlyContinue }; Start-Sleep -Seconds 2; $left = @(Get-CimInstance Win32_Process -Filter ""Name='wheelmaker.exe'"" -ErrorAction SilentlyContinue); foreach ($p in $left) { Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue }; Write-Host 'wheelmaker stopped'"
if %errorlevel% neq 0 exit /b %errorlevel%
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

  if ($WhatIf) {
    foreach ($name in $scripts.Keys) {
      Write-Host ("[whatif] write {0}" -f (Join-Path $script:WheelmakerHome $name))
    }
    return
  }

  New-Item -ItemType Directory -Path $script:WheelmakerHome -Force | Out-Null
  foreach ($name in $scripts.Keys) {
    Set-Content -Path (Join-Path $script:WheelmakerHome $name) -Value $scripts[$name] -Encoding UTF8
  }
}

function Stop-WheelmakerProcesses {
  $all = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue)
  if ($all.Count -eq 0) {
    Write-Step "no running wheelmaker process found"
    return
  }

  Write-Step ("stop wheelmaker pids: {0}" -f (($all | ForEach-Object { $_.ProcessId }) -join ","))
  if ($WhatIf) {
    foreach ($proc in $all) {
      Write-Host ("[whatif] Stop-Process -Id {0}" -f $proc.ProcessId)
    }
    return
  }

  foreach ($proc in $all) {
    Stop-Process -Id $proc.ProcessId -ErrorAction SilentlyContinue
  }

  Start-Sleep -Seconds 2
  foreach ($proc in @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue)) {
    Stop-Process -Id $proc.ProcessId -Force -ErrorAction SilentlyContinue
  }
}

function Build-ServerBinary {
  if ($SkipBuild) {
    Write-Step "skip server build"
    return
  }

  Assert-Command -Name "go" -Hint "Install Go 1.22+."
  Write-Step ("build server binary: {0}" -f $script:OutputBinary)

  if ($WhatIf) {
    Write-Host ("[whatif] go build -o {0} ./cmd/wheelmaker/" -f $script:OutputBinary)
    return
  }

  New-Item -ItemType Directory -Path (Split-Path $script:OutputBinary -Parent) -Force | Out-Null
  Push-Location $script:ServerRoot
  try {
    Invoke-Checked -FilePath "go" -Arguments @("build", "-o", $script:OutputBinary, "./cmd/wheelmaker/") -FailureMessage "go build failed"
  }
  finally {
    Pop-Location
  }
}

function Install-ServerBinary {
  if ($SkipInstall) {
    Write-Step "skip install"
    return
  }

  if (-not (Test-Path $script:SourceBinary)) {
    throw ("source binary not found: {0}" -f $script:SourceBinary)
  }

  Write-Step ("install binary: {0} -> {1}" -f $script:SourceBinary, $script:InstalledBinary)
  Stop-WheelmakerProcesses

  if ($WhatIf) {
    Write-Host ("[whatif] copy {0} -> {1}" -f $script:SourceBinary, $script:InstalledBinary)
    return
  }

  New-Item -ItemType Directory -Path $script:InstallDirResolved -Force | Out-Null
  Copy-Item -Path $script:SourceBinary -Destination $script:InstalledBinary -Force
}

function Restart-Server {
  if ($SkipRestart) {
    Write-Step "skip restart"
    return
  }

  if (-not (Test-Path $script:InstalledBinary)) {
    throw ("installed binary not found: {0}" -f $script:InstalledBinary)
  }

  Write-Step ("start daemon: {0} -d" -f $script:InstalledBinary)
  if ($WhatIf) {
    Write-Host ("[whatif] Start-Process -FilePath {0} -ArgumentList -d" -f $script:InstalledBinary)
    return
  }

  $proc = Start-Process -WorkingDirectory $script:RepoRoot -FilePath $script:InstalledBinary -ArgumentList "-d" -WindowStyle Hidden -PassThru
  Write-Step ("started guardian pid={0}" -f $proc.Id)

  Start-Sleep -Seconds 4
  $running = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue)
  if ($running.Count -eq 0) {
    throw "wheelmaker did not remain running after restart"
  }

  $guardian = @($running | Where-Object { [string]$_.CommandLine -match "(^|\s)-d(\s|$)" }).Count
  if ($guardian -lt 1) {
    throw "wheelmaker restart did not leave a guardian process running"
  }

  Write-Step ("restart verified: processes={0}, guardian={1}" -f $running.Count, $guardian)
}

$script:RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$script:ServerRoot = Join-Path $script:RepoRoot "server"
$script:WheelmakerHome = Join-Path $HOME ".wheelmaker"
$script:ConfigPath = Join-Path $script:WheelmakerHome "config.json"
$script:ConfigExamplePath = Join-Path $script:ServerRoot "config.example.json"
$script:InstallDirResolved = Get-ResolvedPathOrDefault -Path $InstallDir -Default (Join-Path $HOME ".wheelmaker\bin")
$script:OutputBinary = Get-ResolvedPathOrDefault -Path $OutputPath -Default (Join-Path $script:ServerRoot "bin\windows_amd64\wheelmaker.exe")
$script:SourceBinary = Get-ResolvedPathOrDefault -Path $SourceExe -Default $script:OutputBinary
$script:InstalledBinary = Join-Path $script:InstallDirResolved "wheelmaker.exe"

if (-not (Test-Path $script:ServerRoot)) {
  throw ("server directory not found: {0}" -f $script:ServerRoot)
}

Write-Step ("repo root: {0}" -f $script:RepoRoot)
Pull-Latest
Ensure-AcpDependencies
Build-ServerBinary

$configWasCreated = $false
if (-not $SkipInstall) {
  Install-ServerBinary
  $configWasCreated = Ensure-Config
  Install-ServiceScripts
} elseif (-not $SkipRestart) {
  if (-not (Test-Path $script:ConfigPath)) {
    throw ("config not found: {0}. Run scripts\refresh_server.ps1 without -SkipInstall first." -f $script:ConfigPath)
  }
  Write-Step ("config already exists: {0}" -f $script:ConfigPath)
  $config = Read-ConfigJson -Path $script:ConfigPath
  Validate-ExistingConfig -Config $config
}

if ($configWasCreated -and -not $SkipRestart) {
  throw ("config was created from example at {0}; edit it first, then rerun scripts\refresh_server.ps1" -f $script:ConfigPath)
}
Restart-Server
Write-Step "refresh complete"
