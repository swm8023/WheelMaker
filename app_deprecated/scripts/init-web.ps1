# init-web.ps1 - Initialize Flutter web platform files for WheelMaker app.
#
# Run from repo root:
#   pwsh app/scripts/init-web.ps1
#
# What it does:
#   1. Verifies Flutter is installed.
#   2. Generates (or updates) web platform scaffolding in app/.
#   3. Runs flutter pub get.

$ErrorActionPreference = "Stop"

$appDir = Split-Path -Parent $PSScriptRoot

Write-Host "==> WheelMaker web scaffold setup" -ForegroundColor Cyan
Write-Host "    app: $appDir"

if (-not (Get-Command flutter -ErrorAction SilentlyContinue)) {
    Write-Error @"
Flutter not found in PATH.
Install Flutter from: https://docs.flutter.dev/get-started/install
Then re-run this script.
"@
    exit 1
}

$flutterVersion = (flutter --version --machine 2>$null | ConvertFrom-Json).flutterVersion
Write-Host "    flutter: $flutterVersion" -ForegroundColor Green

Write-Host ""
Write-Host "==> Generating Web platform files..." -ForegroundColor Cyan
flutter create `
    --project-name wheelmaker `
    --org com.wheelmaker `
    --platforms web `
    $appDir

Write-Host ""
Write-Host "==> Installing Dart dependencies..." -ForegroundColor Cyan
Set-Location $appDir
flutter pub get

Write-Host ""
Write-Host "==> Done! Run in browser:" -ForegroundColor Green
Write-Host "    cd app"
Write-Host "    flutter run -d chrome"
