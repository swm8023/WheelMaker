$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$dist = Join-Path $root "dist"
$target = Join-Path $root "android\app\src\main\assets\wheelmaker-web"

if (-not (Test-Path $dist)) {
  throw "Missing dist directory: $dist. Run npm run build:web first."
}

if (Test-Path $target) {
  Get-ChildItem -LiteralPath $target -Force | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
}

New-Item -ItemType Directory -Path $target -Force | Out-Null
Copy-Item -Path (Join-Path $dist "*") -Destination $target -Recurse -Force
Get-ChildItem -Path $target -Filter *.map -Recurse | Remove-Item -Force

Write-Host "Synced web assets to $target"
