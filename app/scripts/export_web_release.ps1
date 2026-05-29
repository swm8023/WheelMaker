$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$webPublic = Join-Path $root "web\public"
$stateRoot = Join-Path $HOME ".wheelmaker"
$target = Join-Path $stateRoot "web"

if (-not (Test-Path $stateRoot)) {
  New-Item -ItemType Directory -Path $stateRoot -Force | Out-Null
}
if (-not (Test-Path $target)) {
  New-Item -ItemType Directory -Path $target -Force | Out-Null
}

if (Test-Path (Join-Path $webPublic "manifest.webmanifest")) {
  Copy-Item -Path (Join-Path $webPublic "manifest.webmanifest") -Destination (Join-Path $target "manifest.webmanifest") -Force
}
if (Test-Path (Join-Path $webPublic "service-worker.js")) {
  Copy-Item -Path (Join-Path $webPublic "service-worker.js") -Destination (Join-Path $target "service-worker.js") -Force
}
if (Test-Path (Join-Path $webPublic "icons")) {
  $targetIcons = Join-Path $target "icons"
  if (-not (Test-Path $targetIcons)) {
    New-Item -ItemType Directory -Path $targetIcons -Force | Out-Null
  }
  Copy-Item -Path (Join-Path $webPublic "icons\*") -Destination $targetIcons -Recurse -Force
}

$sha = $env:WHEELMAKER_WEB_BUILD_SHA
if ([string]::IsNullOrWhiteSpace($sha)) {
  try {
    Push-Location (Join-Path $root "..")
    $sha = ((& git rev-parse HEAD) | Select-Object -First 1)
  } catch {
    $sha = ""
  } finally {
    Pop-Location
  }
}
$builtAt = $env:WHEELMAKER_WEB_BUILD_TIME
if ([string]::IsNullOrWhiteSpace($builtAt)) {
  $builtAt = (Get-Date).ToUniversalTime().ToString("o")
}
$assets = [ordered]@{}
Get-ChildItem -LiteralPath $target -File -ErrorAction SilentlyContinue |
  Where-Object { $_.Name -match '^bundle\..+\.(js|css)$' } |
  ForEach-Object {
    $assets[$_.Name] = "sha256:" + ((Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant())
  }
$manifest = [ordered]@{
  schemaVersion = 1
  sha = [string]$sha
  builtAt = [string]$builtAt
  assets = $assets
}
$manifestPath = Join-Path $target "web-build.json"
$json = $manifest | ConvertTo-Json -Depth 4
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($manifestPath, $json + "`n", $utf8NoBom)

Write-Host "Exported web release to $target"
