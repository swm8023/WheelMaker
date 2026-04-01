param(
  [string]$RepoRoot = "",
  [string]$InstallDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\bin"),
  [string]$OutputPath = "",
  [string]$SourceExe = "",
  [string]$UpdaterDailyTime = "03:00",
  [switch]$SkipUpdate,
  [switch]$SkipStop,
  [switch]$SkipDeploy,
  [switch]$SkipGitPull,
  [switch]$SkipDeps,
  [switch]$SkipBuild,
  [switch]$SkipInstall,
  [switch]$SkipUpdaterInstall,
  [switch]$SkipRestart,
  [switch]$SkipServiceConfig,
  [string]$ServiceUser = ("{0}\{1}" -f $env:USERDOMAIN, $env:USERNAME),
  [string]$ServicePassword = "",
  [switch]$WhatIf
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$script:WheelmakerService = "WheelMaker"
$script:MonitorService = "WheelMakerMonitor"
$script:UpdaterService = "WheelMakerUpdater"
$script:ServicePasswordPlain = ""

function Write-Step {
  param([string]$Text)
  Write-Host ("==> {0}" -f $Text)
}

function Write-Warn {
  param([string]$Text)
  Write-Warning $Text
}

function Test-IsAdministrator {
  try {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  } catch {
    return $false
  }
}

function Assert-ServiceAdminAccess {
  if ($WhatIf) { return }
  # Recreate/reconfigure services requires elevation. Pure update rounds
  # (for example updater service with -SkipServiceConfig) can continue and
  # rely on per-operation permission errors if they occur.
  $needsServiceControl = (-not $SkipServiceConfig)
  if (-not $needsServiceControl) { return }
  if (Test-IsAdministrator) { return }
  throw "windows service operations require elevated administrator PowerShell. Re-run deploy.bat (or refresh_server.ps1) in an Administrator terminal."
}

function Assert-Command {
  param([Parameter(Mandatory = $true)][string]$Name, [string]$Hint = "")
  if (Get-Command $Name -ErrorAction SilentlyContinue) { return }
  if ([string]::IsNullOrWhiteSpace($Hint)) { throw ("required command not found in PATH: {0}" -f $Name) }
  throw ("required command not found in PATH: {0}. {1}" -f $Name, $Hint)
}

function Invoke-Checked {
  param([Parameter(Mandatory = $true)][string]$FilePath, [string[]]$Arguments = @(), [string]$FailureMessage = "")
  & $FilePath @Arguments
  if ($LASTEXITCODE -eq 0) { return }
  if ([string]::IsNullOrWhiteSpace($FailureMessage)) {
    throw ("command failed: {0} {1} (exit={2})" -f $FilePath, ($Arguments -join " "), $LASTEXITCODE)
  }
  throw ("{0} (exit={1})" -f $FailureMessage, $LASTEXITCODE)
}

function Get-ResolvedPathOrDefault {
  param([string]$Path, [string]$Default)
  $target = $Path
  if ([string]::IsNullOrWhiteSpace($target)) { $target = $Default }
  return $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($target)
}

function Get-GitCurrentBranch {
  Push-Location $script:RepoRoot
  try {
    $branch = ((& git branch --show-current) | Select-Object -First 1)
    if ($LASTEXITCODE -ne 0) { throw ("git branch --show-current failed (exit={0})" -f $LASTEXITCODE) }
    $branch = ([string]$branch).Trim()
    if ($branch -eq "") { throw "repository is in detached HEAD state; cannot pull latest automatically" }
    return $branch
  } finally { Pop-Location }
}

function Assert-CleanGitWorktree {
  Push-Location $script:RepoRoot
  try {
    $status = @(& git status --porcelain)
    if ($LASTEXITCODE -ne 0) { throw ("git status failed (exit={0})" -f $LASTEXITCODE) }
    if (@($status | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }).Count -gt 0) {
      throw "git worktree has local changes; commit, stash, or revert them before running refresh_server.ps1"
    }
  } finally { Pop-Location }
}

function Pull-Latest {
  if ($SkipGitPull) { Write-Step "skip git pull"; return }
  Assert-Command -Name "git" -Hint "Install Git and ensure git.exe is available."
  Assert-CleanGitWorktree
  $branch = Get-GitCurrentBranch
  Write-Step ("git pull --ff-only origin {0}" -f $branch)
  if ($WhatIf) { Write-Host ("[whatif] git pull --ff-only origin {0}" -f $branch); return }
  Push-Location $script:RepoRoot
  try { Invoke-Checked -FilePath "git" -Arguments @("pull", "--ff-only", "origin", $branch) -FailureMessage "git pull failed" }
  finally { Pop-Location }
}

function Ensure-AcpDependencies {
  if ($SkipDeps) { Write-Step "skip dependency install/check (-SkipDeps)"; return }
  Assert-Command -Name "npm" -Hint "Install Node.js 22+."
  $missing = @()
  if (-not (Get-Command codex-acp -ErrorAction SilentlyContinue)) { $missing += "@zed-industries/codex-acp" }
  if (-not (Get-Command claude-agent-acp -ErrorAction SilentlyContinue)) { $missing += "@zed-industries/claude-agent-acp" }
  if ($missing.Count -eq 0) { Write-Step "ACP dependencies already installed"; return }
  Write-Step ("install ACP dependencies: {0}" -f ($missing -join ", "))
  if ($WhatIf) { Write-Host ("[whatif] npm install -g {0}" -f ($missing -join " ")); return }
  Invoke-Checked -FilePath "npm" -Arguments (@("install", "-g") + $missing) -FailureMessage "npm install failed"
}

function Read-ConfigJson {
  param([Parameter(Mandatory = $true)][string]$Path)
  try { return (Get-Content -Path $Path -Raw -Encoding UTF8 | ConvertFrom-Json) }
  catch { throw ("config is not valid JSON: {0}" -f $Path) }
}

function Validate-ExistingConfig {
  param([Parameter(Mandatory = $true)]$Config)
  if ($null -eq $Config.projects -or @($Config.projects).Count -eq 0) {
    Write-Warn "config has no projects configured yet"
    return
  }
  foreach ($project in @($Config.projects)) {
    $name = [string]$project.name
    if ([string]::IsNullOrWhiteSpace($name)) { Write-Warn "config contains a project without a name" }
    $path = [string]$project.path
    if ([string]::IsNullOrWhiteSpace($path)) { Write-Warn ("project '{0}' has an empty path" -f $name) }
    elseif ($path -match "^/path/to/" -or $path -match "\\/path\\/to\\/") { Write-Warn ("project '{0}' still uses the example path: {1}" -f $name, $path) }
    elseif (-not (Test-Path $path)) { Write-Warn ("project '{0}' path does not exist: {1}" -f $name, $path) }
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
  if (-not (Test-Path $script:ConfigExamplePath)) { throw ("config example missing: {0}" -f $script:ConfigExamplePath) }
  Write-Step ("create config from example: {0}" -f $configPath)
  if ($WhatIf) { Write-Host ("[whatif] copy {0} -> {1}" -f $script:ConfigExamplePath, $configPath); return $true }
  New-Item -ItemType Directory -Path $script:WheelmakerHome -Force | Out-Null
  Copy-Item -Path $script:ConfigExamplePath -Destination $configPath -Force
  Write-Warn ("config was created from example: {0}" -f $configPath)
  Write-Warn "edit config.json before the first restart, then rerun scripts\refresh_server.ps1"
  return $true
}

function Install-ServiceScripts {
  Write-Step "install service helper scripts"
  $installedRefreshScript = Join-Path $script:WheelmakerHome "refresh_server.ps1"
  $sourceRefreshScript = Join-Path $script:RepoRoot "scripts\refresh_server.ps1"
  $common = "-RepoRoot `"$($script:RepoRoot)`" -InstallDir `"$($script:InstallDirResolved)`""
  $startBody = "-SkipUpdate -SkipBuild -SkipDeploy {0}" -f $common
  $stopBody = "-SkipUpdate -SkipBuild -SkipDeploy -SkipRestart {0}" -f $common
  $startBat = @"
@echo off
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -File "$installedRefreshScript" $startBody
set "_exit=%errorlevel%"
if not "%_exit%"=="0" (
  echo.
  echo [FAILED] start exited with code %_exit%
  pause
)
exit /b %_exit%
"@
  $stopBat = @"
@echo off
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -File "$installedRefreshScript" $stopBody
set "_exit=%errorlevel%"
if not "%_exit%"=="0" (
  echo.
  echo [FAILED] stop exited with code %_exit%
  pause
)
exit /b %_exit%
"@

  $scripts = @{ "start.bat" = $startBat; "stop.bat" = $stopBat }
  if ($WhatIf) {
    Write-Host ("[whatif] copy {0} -> {1}" -f $sourceRefreshScript, $installedRefreshScript)
    foreach ($name in $scripts.Keys) { Write-Host ("[whatif] write {0}" -f (Join-Path $script:WheelmakerHome $name)) }
    return
  }
  New-Item -ItemType Directory -Path $script:WheelmakerHome -Force | Out-Null
  Copy-Item -Path $sourceRefreshScript -Destination $installedRefreshScript -Force
  foreach ($name in $scripts.Keys) { Set-Content -Path (Join-Path $script:WheelmakerHome $name) -Value $scripts[$name] -Encoding UTF8 }
}

function Backup-Logs {
  $logDir = Join-Path $script:WheelmakerHome "logs"
  $logFiles = @(Get-ChildItem -Path $script:WheelmakerHome -Filter "*.log" -File -ErrorAction SilentlyContinue)
  if ($logFiles.Count -eq 0) { Write-Step "no log files to backup"; return }
  $ts = Get-Date -Format "yyyyMMdd_HHmmss"
  $dest = Join-Path $logDir $ts
  Write-Step ("backup {0} log file(s) -> {1}" -f $logFiles.Count, $dest)
  if ($WhatIf) {
    foreach ($f in $logFiles) { Write-Host ("[whatif] copy {0} -> {1}" -f $f.FullName, $dest) }
    return
  }
  New-Item -ItemType Directory -Path $dest -Force | Out-Null
  foreach ($f in $logFiles) { Copy-Item -Path $f.FullName -Destination $dest -Force }
}

function Build-Binary {
  param([Parameter(Mandatory = $true)][string]$Out, [Parameter(Mandatory = $true)][string]$Pkg, [Parameter(Mandatory = $true)][string]$Label)
  if ($SkipBuild) { Write-Step ("skip build: {0}" -f $Label); return }
  Assert-Command -Name "go" -Hint "Install Go 1.22+."
  Write-Step ("build {0}: {1}" -f $Label, $Out)
  if ($WhatIf) { Write-Host ("[whatif] go build -o {0} {1}" -f $Out, $Pkg); return }
  New-Item -ItemType Directory -Path (Split-Path $Out -Parent) -Force | Out-Null
  Push-Location $script:ServerRoot
  try { Invoke-Checked -FilePath "go" -Arguments @("build", "-o", $Out, $Pkg) -FailureMessage ("go build failed: {0}" -f $Label) }
  finally { Pop-Location }
}

function Test-ServiceExists {
  param([Parameter(Mandatory = $true)][string]$Name)
  return $null -ne (Get-Service -Name $Name -ErrorAction SilentlyContinue)
}

function Wait-ServiceStatus {
  param([Parameter(Mandatory = $true)][string]$Name, [Parameter(Mandatory = $true)][string]$Status, [int]$TimeoutSeconds = 20)
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $svc) { return }
    if ([string]$svc.Status -eq $Status) { return }
    Start-Sleep -Milliseconds 250
  }
  throw ("service {0} failed to reach status {1}" -f $Name, $Status)
}

function Stop-ServiceSafe {
  param([Parameter(Mandatory = $true)][string]$Name)
  if (-not (Test-ServiceExists -Name $Name)) { return }
  Write-Step ("stop service: {0}" -f $Name)
  if ($WhatIf) { Write-Host ("[whatif] Stop-Service -Name {0} -Force" -f $Name); return }
  try {
    Stop-Service -Name $Name -Force -ErrorAction Stop
  } catch {
    $message = $_.Exception.Message
    if ($message -match "Access is denied" -or $message -match "Cannot open" -or $message -match "拒绝访问") {
      throw ("cannot stop service {0}: access denied. Run this script in an Administrator PowerShell terminal." -f $Name)
    }
    Write-Warn ("Stop-Service failed for {0}: {1}. Continue with wait/fallback." -f $Name, $message)
  }
  try {
    Wait-ServiceStatus -Name $Name -Status "Stopped" -TimeoutSeconds 30
  } catch {
    Write-Warn ("service stop timeout: {0}; force-killing bound process" -f $Name)
    try {
      & taskkill /F /FI ("SERVICES eq {0}" -f $Name) | Out-Null
    } catch {
      Write-Warn ("taskkill fallback failed for service {0}: {1}" -f $Name, $_.Exception.Message)
    }
    Start-Sleep -Seconds 1

    $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $svc -or [string]$svc.Status -eq "Stopped") {
      return
    }
    Write-Warn ("service {0} still not fully stopped; continue deploy anyway" -f $Name)
  }
}

function Start-ServiceSafe {
  param([Parameter(Mandatory = $true)][string]$Name)
  if (-not (Test-ServiceExists -Name $Name)) { Write-Warn ("service not found, skip start: {0}" -f $Name); return }
  Write-Step ("start service: {0}" -f $Name)
  if ($WhatIf) { Write-Host ("[whatif] Start-Service -Name {0}" -f $Name); return }
  try {
    Start-Service -Name $Name -ErrorAction Stop
  } catch {
    $message = $_.Exception.Message
    if ($message -match "Access is denied" -or $message -match "Cannot open" -or $message -match "拒绝访问") {
      throw ("cannot start service {0}: access denied. Run this script in an Administrator PowerShell terminal." -f $Name)
    }
    throw
  }
  Wait-ServiceStatus -Name $Name -Status "Running" -TimeoutSeconds 20
}

function Stop-LegacyProcessMode {
  if ($SkipStop) { return }
  if ($WhatIf) {
    Write-Step "stop legacy process-mode instances (whatif)"
    Write-Host "[whatif] stop process: wheelmaker.exe / wheelmaker-monitor.exe / wheelmaker-updater.exe"
    Write-Host "[whatif] remove legacy scheduled tasks if present"
    return
  }

  Write-Step "stop legacy process-mode instances (if any)"
  $legacyNames = @("wheelmaker.exe", "wheelmaker-monitor.exe", "wheelmaker-updater.exe")
  foreach ($name in $legacyNames) {
    try {
      $procs = @(Get-CimInstance Win32_Process -Filter ("Name='{0}'" -f $name) -ErrorAction SilentlyContinue)
      if ($procs.Count -eq 0) { continue }
      foreach ($proc in $procs) {
        try { Stop-Process -Id $proc.ProcessId -Force -ErrorAction SilentlyContinue }
        catch { Write-Warn ("failed to stop legacy process {0} pid={1}: {2}" -f $name, $proc.ProcessId, $_.Exception.Message) }
      }
    } catch {
      Write-Warn ("failed to enumerate legacy process {0}: {1}" -f $name, $_.Exception.Message)
    }
  }

  $legacyTasks = @("WheelMakerAutoUpdate", "WheelMakerAutoUpdater", "WheelMakerUpdater", "WheelMaker-Update")
  foreach ($task in $legacyTasks) {
    try {
      & schtasks.exe /Query /TN $task *> $null
      if ($LASTEXITCODE -eq 0) {
        Write-Step ("remove legacy scheduled task: {0}" -f $task)
        & schtasks.exe /Delete /TN $task /F *> $null
        if ($LASTEXITCODE -ne 0) { Write-Warn ("failed to delete scheduled task: {0}" -f $task) }
      }
    } catch {
      Write-Warn ("failed to query/delete scheduled task {0}: {1}" -f $task, $_.Exception.Message)
    }
  }
}

function Prepare-ServiceCredentials {
  if ($SkipServiceConfig -or $WhatIf) { return }
  if ([string]::IsNullOrWhiteSpace($ServiceUser)) { return }
  if (-not [string]::IsNullOrWhiteSpace($ServicePassword)) {
    $script:ServicePasswordPlain = $ServicePassword
    return
  }
  if ([Console]::IsInputRedirected) {
    throw ("service account password is required to configure services as {0}; pass -ServicePassword in non-interactive mode" -f $ServiceUser)
  }
  $secure = Read-Host -AsSecureString -Prompt ("Enter password for service account {0}" -f $ServiceUser)
  $bstr = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
  try {
    $plain = [System.Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
    if (-not [string]::IsNullOrWhiteSpace($plain)) {
      $script:ServicePasswordPlain = $plain
    } else {
      throw ("service account password is required to configure services as {0}" -f $ServiceUser)
    }
  } finally {
    [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
  }
}

function Wait-ServiceDeleted {
  param([Parameter(Mandatory = $true)][string]$Name, [int]$TimeoutSeconds = 20)
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $svc) { return }
    Start-Sleep -Milliseconds 250
  }
  throw ("service {0} failed to delete within timeout" -f $Name)
}

function Ensure-Service {
  param([Parameter(Mandatory = $true)][string]$Name, [Parameter(Mandatory = $true)][string]$BinaryPath, [string]$Arguments = "")
  $binPath = ('"{0}" {1}' -f $BinaryPath, $Arguments).Trim()
  if ($WhatIf) {
    Write-Host ("[whatif] reinstall service {0} binPath={1}" -f $Name, $binPath)
    return
  }
  if (Test-ServiceExists -Name $Name) {
    Stop-ServiceSafe -Name $Name
    Invoke-Checked -FilePath "sc.exe" -Arguments @("delete", $Name) -FailureMessage ("service delete failed: {0}" -f $Name)
    Wait-ServiceDeleted -Name $Name -TimeoutSeconds 20
  }
  Invoke-Checked -FilePath "sc.exe" -Arguments @("create", $Name, "binPath=", $binPath, "start=", "auto") -FailureMessage ("service create failed: {0}" -f $Name)
  if (-not [string]::IsNullOrWhiteSpace($script:ServicePasswordPlain)) {
    Invoke-Checked -FilePath "sc.exe" -Arguments @("config", $Name, "obj=", $ServiceUser, "password=", $script:ServicePasswordPlain) -FailureMessage ("service account config failed: {0}" -f $Name)
  }
}

function Resolve-AccountSid {
  param([Parameter(Mandatory = $true)][string]$Account)
  try {
    $nt = New-Object System.Security.Principal.NTAccount($Account)
    $sid = $nt.Translate([System.Security.Principal.SecurityIdentifier])
    return $sid.Value
  } catch {
    throw ("failed to resolve SID for account '{0}': {1}" -f $Account, $_.Exception.Message)
  }
}

function Get-ServiceSecurityDescriptor {
  param([Parameter(Mandatory = $true)][string]$Name)
  $output = & sc.exe sdshow $Name
  if ($LASTEXITCODE -ne 0) {
    throw ("service sdshow failed: {0} (exit={1})" -f $Name, $LASTEXITCODE)
  }
  $text = (($output | Out-String).Trim())
  $dPos = $text.IndexOf("D:")
  if ($dPos -lt 0) {
    throw ("service {0} security descriptor missing DACL" -f $Name)
  }
  return $text.Substring($dPos)
}

function Ensure-ServiceAclEntry {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][string]$Sid,
    [Parameter(Mandatory = $true)][string]$Rights
  )
  $sd = Get-ServiceSecurityDescriptor -Name $Name
  $sidPattern = [Regex]::Escape(";;;$Sid)")
  if ($sd -match $sidPattern) {
    Write-Step ("service ACL already contains account SID for {0}" -f $Name)
    return
  }

  $ace = "(A;;{0};;;{1})" -f $Rights, $Sid
  $sPos = $sd.IndexOf("S:")
  if ($sPos -ge 0) {
    $newSd = $sd.Substring(0, $sPos) + $ace + $sd.Substring($sPos)
  } else {
    $newSd = $sd + $ace
  }
  Invoke-Checked -FilePath "sc.exe" -Arguments @("sdset", $Name, $newSd) -FailureMessage ("service ACL update failed: {0}" -f $Name)
  Write-Step ("granted service control ACL: {0}" -f $Name)
}

function Ensure-ServiceControlAclForAccount {
  if ($SkipServiceConfig) { return }
  if ([string]::IsNullOrWhiteSpace($ServiceUser)) { return }
  $sid = Resolve-AccountSid -Account $ServiceUser
  # Rights required for updater-driven service management:
  # RP(start), WP(stop), LC(query status), LO(interrogate), CR(user-defined), RC(read control)
  $rights = "RPWPLCLOCRRC"
  Ensure-ServiceAclEntry -Name $script:WheelmakerService -Sid $sid -Rights $rights
  Ensure-ServiceAclEntry -Name $script:MonitorService -Sid $sid -Rights $rights
}

function Configure-Services {
  if ($SkipServiceConfig) { Write-Step "skip service configuration"; return }
  Prepare-ServiceCredentials
  Write-Step "ensure windows services"
  Ensure-Service -Name $script:WheelmakerService -BinaryPath $script:InstalledBinary
  Ensure-Service -Name $script:MonitorService -BinaryPath $script:MonitorInstalledBinary
  if (-not $SkipUpdaterInstall) {
    Ensure-Service -Name $script:UpdaterService -BinaryPath $script:UpdaterInstalledBinary -Arguments $script:UpdaterServiceArguments
  } else {
    Write-Step "skip updater service configuration (-SkipUpdaterInstall)"
  }
  Ensure-ServiceControlAclForAccount
}

function Install-Binary {
  param([Parameter(Mandatory = $true)][string]$Source, [Parameter(Mandatory = $true)][string]$Dest, [string]$StopServiceName = "")
  if ($SkipInstall) { Write-Step ("skip install: {0}" -f (Split-Path $Dest -Leaf)); return }
  if (-not (Test-Path $Source)) { if ($WhatIf) { Write-Host ("[whatif] source binary missing (expected from build): {0}" -f $Source); return } ; throw ("source binary not found: {0}" -f $Source) }
  if (-not [string]::IsNullOrWhiteSpace($StopServiceName)) { Stop-ServiceSafe -Name $StopServiceName }
  Write-Step ("install binary: {0} -> {1}" -f $Source, $Dest)
  if ($WhatIf) { Write-Host ("[whatif] copy {0} -> {1}" -f $Source, $Dest); return }
  New-Item -ItemType Directory -Path (Split-Path $Dest -Parent) -Force | Out-Null
  Copy-Item -Path $Source -Destination $Dest -Force
}

function Restart-Services {
  if ($SkipRestart) { Write-Step "skip restart"; return }
  Start-ServiceSafe -Name $script:WheelmakerService
  Start-ServiceSafe -Name $script:MonitorService
  if (-not $SkipUpdaterInstall) {
    Start-ServiceSafe -Name $script:UpdaterService
  } else {
    Write-Step "skip updater service start (-SkipUpdaterInstall)"
  }
}

function Get-UpdaterServiceArguments {
  $repo = $script:RepoRoot.Replace('"', '\"')
  $install = $script:InstallDirResolved.Replace('"', '\"')
  $timeValue = $UpdaterDailyTime.Replace('"', '\"')
  return ('--repo "{0}" --install-dir "{1}" --time "{2}"' -f $repo, $install, $timeValue)
}

$script:RepoRoot = if ([string]::IsNullOrWhiteSpace($RepoRoot)) { (Resolve-Path (Join-Path $PSScriptRoot "..")).Path } else { (Resolve-Path $RepoRoot).Path }
$script:ServerRoot = Join-Path $script:RepoRoot "server"
$script:WheelmakerHome = Join-Path $HOME ".wheelmaker"
$script:ConfigPath = Join-Path $script:WheelmakerHome "config.json"
$script:ConfigExamplePath = Join-Path $script:ServerRoot "config.example.json"
$script:InstallDirResolved = Get-ResolvedPathOrDefault -Path $InstallDir -Default (Join-Path $HOME ".wheelmaker\bin")
$script:OutputBinary = Get-ResolvedPathOrDefault -Path $OutputPath -Default (Join-Path $script:ServerRoot "bin\windows_amd64\wheelmaker.exe")
$script:SourceBinary = Get-ResolvedPathOrDefault -Path $SourceExe -Default $script:OutputBinary
$script:InstalledBinary = Join-Path $script:InstallDirResolved "wheelmaker.exe"
$script:MonitorOutputBinary = Join-Path $script:ServerRoot "bin\windows_amd64\wheelmaker-monitor.exe"
$script:MonitorInstalledBinary = Join-Path $script:InstallDirResolved "wheelmaker-monitor.exe"
$script:UpdaterOutputBinary = Join-Path $script:ServerRoot "bin\windows_amd64\wheelmaker-updater.exe"
$script:UpdaterInstalledBinary = Join-Path $script:InstallDirResolved "wheelmaker-updater.exe"
$script:UpdaterServiceArguments = Get-UpdaterServiceArguments

if ($SkipUpdate) {
  $SkipGitPull = $true
  $SkipDeps = $true
}
if ($SkipDeploy) {
  $SkipInstall = $true
}

if (-not (Test-Path $script:ServerRoot)) { throw ("server directory not found: {0}" -f $script:ServerRoot) }

Assert-ServiceAdminAccess
Write-Step ("repo root: {0}" -f $script:RepoRoot)
Pull-Latest
Ensure-AcpDependencies
Build-Binary -Out $script:OutputBinary -Pkg "./cmd/wheelmaker/" -Label "wheelmaker"
Build-Binary -Out $script:MonitorOutputBinary -Pkg "./cmd/wheelmaker-monitor/" -Label "wheelmaker-monitor"
if (-not $SkipUpdaterInstall) {
  Build-Binary -Out $script:UpdaterOutputBinary -Pkg "./cmd/wheelmaker-updater/" -Label "wheelmaker-updater"
} else {
  Write-Step "skip build: wheelmaker-updater (-SkipUpdaterInstall)"
}

$configWasCreated = $false
if (-not $SkipStop) {
  if (-not $SkipUpdaterInstall) {
    Stop-ServiceSafe -Name $script:UpdaterService
  } else {
    Write-Step "skip stop service: WheelMakerUpdater (-SkipUpdaterInstall)"
  }
  Stop-ServiceSafe -Name $script:WheelmakerService
  Stop-ServiceSafe -Name $script:MonitorService
  Stop-LegacyProcessMode
}
if (-not $SkipInstall) {
  Backup-Logs
  Install-Binary -Source $script:SourceBinary -Dest $script:InstalledBinary -StopServiceName $script:WheelmakerService
  Install-Binary -Source $script:MonitorOutputBinary -Dest $script:MonitorInstalledBinary -StopServiceName $script:MonitorService
  if (-not $SkipUpdaterInstall) {
    Install-Binary -Source $script:UpdaterOutputBinary -Dest $script:UpdaterInstalledBinary -StopServiceName $script:UpdaterService
  } else {
    Write-Step "skip install: wheelmaker-updater.exe (-SkipUpdaterInstall)"
  }
  $configWasCreated = Ensure-Config
  Install-ServiceScripts
  Configure-Services
} elseif (-not $SkipRestart) {
  if (-not (Test-Path $script:ConfigPath)) { throw ("config not found: {0}. Run scripts\refresh_server.ps1 without -SkipInstall first." -f $script:ConfigPath) }
  Write-Step ("config already exists: {0}" -f $script:ConfigPath)
  $config = Read-ConfigJson -Path $script:ConfigPath
  Validate-ExistingConfig -Config $config
}

if ($configWasCreated -and -not $SkipRestart) {
  throw ("config was created from example at {0}; edit it first, then rerun scripts\refresh_server.ps1" -f $script:ConfigPath)
}

Restart-Services
Write-Step "refresh complete"


