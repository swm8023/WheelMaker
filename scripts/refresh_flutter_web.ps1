param(
  [string]$Device = "edge",
  [switch]$SkipPubGet
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$appRoot = Join-Path $repoRoot "app"

if (-not (Test-Path $appRoot)) {
  throw ("app directory not found: {0}" -f $appRoot)
}
if (-not (Get-Command flutter -ErrorAction SilentlyContinue)) {
  throw "flutter not found in PATH"
}

Write-Host ("==> app root: {0}" -f $appRoot)
Push-Location $appRoot
try {
  if (-not $SkipPubGet) {
    Write-Host "==> flutter pub get"
    & flutter pub get
    if ($LASTEXITCODE -ne 0) {
      throw ("flutter pub get failed (exit={0})" -f $LASTEXITCODE)
    }
  }

  Write-Host ("==> flutter run -d {0}" -f $Device)
  & flutter run -d $Device
  if ($LASTEXITCODE -ne 0) {
    throw ("flutter run failed (exit={0})" -f $LASTEXITCODE)
  }
}
finally {
  Pop-Location
}
