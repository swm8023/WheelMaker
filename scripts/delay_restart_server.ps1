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
$stdout = Join-Path -Path $baseDir -ChildPath "wheelmaker-stdout.log"
$stderr = Join-Path -Path $baseDir -ChildPath "wheelmaker-stderr.log"
$buildScript = Join-Path -Path $PSScriptRoot -ChildPath "build_server.ps1"
$installScript = Join-Path -Path $PSScriptRoot -ChildPath "install_server.ps1"

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

if (-not (Test-Path $buildScript)) {
  Add-Content -Path $logPath -Value ("[{0}] build script missing: {1}" -f (Get-Date -Format o), $buildScript)
  exit 1
}
if (-not (Test-Path $installScript)) {
  Add-Content -Path $logPath -Value ("[{0}] install script missing: {1}" -f (Get-Date -Format o), $installScript)
  exit 1
}

powershell -NoProfile -ExecutionPolicy Bypass -File $buildScript
if ($LASTEXITCODE -ne 0) {
  Add-Content -Path $logPath -Value ("[{0}] build failed exit={1}" -f (Get-Date -Format o), $LASTEXITCODE)
  exit $LASTEXITCODE
}

powershell -NoProfile -ExecutionPolicy Bypass -File $installScript -InstallDir $InstallDir -SkipDeps
if ($LASTEXITCODE -ne 0) {
  Add-Content -Path $logPath -Value ("[{0}] install failed exit={1}" -f (Get-Date -Format o), $LASTEXITCODE)
  exit $LASTEXITCODE
}
Add-Content -Path $logPath -Value ("[{0}] install completed" -f (Get-Date -Format o))

$installedExe = Join-Path -Path $InstallDir -ChildPath "wheelmaker.exe"
if (-not (Test-Path $installedExe)) {
  Add-Content -Path $logPath -Value ("[{0}] installed exe missing: {1}" -f (Get-Date -Format o), $installedExe)
  exit 1
}

$p = Start-Process -WorkingDirectory $repoRoot -FilePath $installedExe -ArgumentList "-d" -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
Add-Content -Path $logPath -Value ("[{0}] started guardian pid={1} path={2}" -f (Get-Date -Format o), $p.Id, $installedExe)

Start-Sleep -Seconds 4
$wm = Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" -ErrorAction SilentlyContinue
Add-Content -Path $logPath -Value ("[{0}] wheelmaker count after restart={1}" -f (Get-Date -Format o), @($wm).Count)
