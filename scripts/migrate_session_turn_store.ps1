param(
  [string]$DbPath = "",
  [string]$SessionRoot = "",
  [switch]$NoBackup,
  [switch]$SkipProcessCheck
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$serverRoot = Join-Path $repoRoot "server"
$wheelmakerRoot = Join-Path $env:USERPROFILE ".wheelmaker"
if ([string]::IsNullOrWhiteSpace($DbPath)) {
  $DbPath = Join-Path $wheelmakerRoot "db\client.sqlite3"
}
if ([string]::IsNullOrWhiteSpace($SessionRoot)) {
  $SessionRoot = Join-Path $wheelmakerRoot "session"
}

if (-not $SkipProcessCheck) {
  $running = Get-Process -ErrorAction SilentlyContinue |
    Where-Object { $_.ProcessName -like "wheelmaker*" } |
    Select-Object -ExpandProperty ProcessName -Unique
  if ($running.Count -gt 0) {
    throw "wheelmaker processes are still running: $($running -join ', '). Stop them before migrating."
  }
}

if (-not (Test-Path -LiteralPath $DbPath)) {
  throw "DB not found: $DbPath"
}
if (-not (Test-Path -LiteralPath $SessionRoot)) {
  New-Item -ItemType Directory -Force -Path $SessionRoot | Out-Null
}

$argsList = @("run", "./cmd/session-turn-migrator", "-db", $DbPath, "-session-root", $SessionRoot)
if ($NoBackup) {
  $argsList += "-no-backup"
}

Push-Location $serverRoot
try {
  & go @argsList
  if ($LASTEXITCODE -ne 0) {
    throw "session turn migrator exited with code $LASTEXITCODE"
  }
} finally {
  Pop-Location
}
