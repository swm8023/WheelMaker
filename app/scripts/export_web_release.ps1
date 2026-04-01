$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$dist = Join-Path $root "dist"
$webPublic = Join-Path $root "web\public"
$stateRoot = Join-Path $HOME ".wheelmaker"
$target = Join-Path $stateRoot "web"

if (-not (Test-Path $dist)) {
  throw "Missing dist directory: $dist. Run npm run build:web first."
}

if (-not (Test-Path $stateRoot)) {
  New-Item -ItemType Directory -Path $stateRoot -Force | Out-Null
}

if (Test-Path $target) {
  Remove-Item -Recurse -Force $target
}

New-Item -ItemType Directory -Path $target | Out-Null
Copy-Item -Path (Join-Path $dist "*") -Destination $target -Recurse -Force

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

Write-Host "Exported web release to $target"
