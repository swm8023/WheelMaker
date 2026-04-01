$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$dist = Join-Path $root "dist"
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

Write-Host "Exported web release to $target"
