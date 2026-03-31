param(
  [switch]$Worker,
  [int]$DelaySeconds = 30,
  [string]$InstallDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\bin")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Continue"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$baseDir = Join-Path -Path $HOME -ChildPath ".wheelmaker"
$logPath = Join-Path -Path $baseDir -ChildPath "delay_restart_server.log"
$refreshScript = Join-Path -Path $PSScriptRoot -ChildPath "refresh_server.ps1"

if (-not (Test-Path $baseDir)) {
  New-Item -ItemType Directory -Path $baseDir -Force | Out-Null
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
  Add-Content -Path $logPath -Value ("[{0}] scheduled restart worker delay={1}s" -f (Get-Date -Format o), $DelaySeconds)
  exit 0
}

Add-Content -Path $logPath -Value ("[{0}] restart job begin delay={1}s" -f (Get-Date -Format o), $DelaySeconds)
Start-Sleep -Seconds $DelaySeconds

if (-not (Test-Path $refreshScript)) {
  Add-Content -Path $logPath -Value ("[{0}] refresh script missing: {1}" -f (Get-Date -Format o), $refreshScript)
  exit 1
}

powershell -NoProfile -ExecutionPolicy Bypass -File $refreshScript -SkipGitPull -InstallDir $InstallDir
if ($LASTEXITCODE -ne 0) {
  Add-Content -Path $logPath -Value ("[{0}] refresh failed exit={1}" -f (Get-Date -Format o), $LASTEXITCODE)
  exit $LASTEXITCODE
}
Add-Content -Path $logPath -Value ("[{0}] refresh completed" -f (Get-Date -Format o))

$installedExe = Join-Path -Path $InstallDir -ChildPath "wheelmaker.exe"
if (-not (Test-Path $installedExe)) {
  Add-Content -Path $logPath -Value ("[{0}] installed exe missing: {1}" -f (Get-Date -Format o), $installedExe)
  exit 1
}

$p = Start-Process -WorkingDirectory $repoRoot -FilePath $installedExe -ArgumentList "-d" -WindowStyle Hidden -PassThru
Add-Content -Path $logPath -Value ("[{0}] started guardian pid={1} path={2}" -f (Get-Date -Format o), $p.Id, $installedExe)

Start-Sleep -Seconds 4
$wm = Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue
Add-Content -Path $logPath -Value ("[{0}] wheelmaker count after restart={1}" -f (Get-Date -Format o), @($wm).Count)

$guardian = 0
$hubWorker = 0
$registryWorker = 0
foreach ($proc in @($wm)) {
  $cmd = [string]$proc.CommandLine
  if ($cmd -match "(^|\s)-d(\s|$)") {
    $guardian++
  }
  if ($cmd -match "(^|\s)--hub-worker(\s|$)") {
    $hubWorker++
  }
  if ($cmd -match "(^|\s)--registry-worker(\s|$)") {
    $registryWorker++
  }
}
Add-Content -Path $logPath -Value ("[{0}] wheelmaker roles guardian={1} hub={2} registry={3}" -f (Get-Date -Format o), $guardian, $hubWorker, $registryWorker)
if ($guardian -lt 1 -or $hubWorker -lt 1 -or $registryWorker -lt 1) {
  Add-Content -Path $logPath -Value ("[{0}] warning: expected at least 1 guardian + 1 hub + 1 registry process" -f (Get-Date -Format o))
}

# Restart monitor if binary exists
$monitorExe = Join-Path -Path $InstallDir -ChildPath "wheelmaker-monitor.exe"
if (Test-Path $monitorExe) {
  $monProcs = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker-monitor.exe'" -ErrorAction SilentlyContinue)
  foreach ($mp in $monProcs) {
    Stop-Process -Id $mp.ProcessId -Force -ErrorAction SilentlyContinue
  }
  $mp = Start-Process -FilePath $monitorExe -WindowStyle Hidden -PassThru
  Add-Content -Path $logPath -Value ("[{0}] started monitor pid={1}" -f (Get-Date -Format o), $mp.Id)
} else {
  Add-Content -Path $logPath -Value ("[{0}] monitor exe not found, skipping" -f (Get-Date -Format o))
}
