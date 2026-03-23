# init-app.ps1 — First-time Flutter project scaffold setup for WheelMaker mobile app.
#
# Run once from the repo root:
#   pwsh scripts/init-app.ps1
#
# What it does:
#   1. Verifies Flutter is installed.
#   2. Runs `flutter create` inside app/ to generate the Android/iOS platform files.
#      Our existing lib/ sources and pubspec.yaml are NOT overwritten.
#   3. Runs `flutter pub get` to fetch Dart dependencies.
#
# After this script you can build an APK with:
#   cd app && flutter build apk --debug

param(
    [switch]$ReleaseAPK   # if set, also build a release APK after init
)

$ErrorActionPreference = "Stop"

# ── Locate dirs ────────────────────────────────────────────────────────────────
$appDir  = Split-Path -Parent $PSScriptRoot   # app/scripts/ → app/
$repoRoot = Split-Path -Parent $appDir        # app/ → repo root

Write-Host "==> WheelMaker mobile app setup" -ForegroundColor Cyan
Write-Host "    repo : $repoRoot"
Write-Host "    app  : $appDir"

# ── Check Flutter ──────────────────────────────────────────────────────────────
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

# ── Generate platform scaffolding (won't overwrite existing files) ──────────────
Write-Host ""
Write-Host "==> Generating Android/iOS/Desktop platform files..." -ForegroundColor Cyan
flutter create `
    --project-name wheelmaker `
    --org com.wheelmaker `
    --platforms android,ios,windows,macos,linux `
    $appDir

# ── Install Dart dependencies ──────────────────────────────────────────────────
Write-Host ""
Write-Host "==> Installing Dart dependencies..." -ForegroundColor Cyan
Set-Location $appDir
flutter pub get

# ── Optional: build debug APK right away ──────────────────────────────────────
if ($ReleaseAPK) {
    Write-Host ""
    Write-Host "==> Building release APK..." -ForegroundColor Cyan
    flutter build apk --release
    $apkPath = Join-Path $appDir "build\app\outputs\flutter-apk\app-release.apk"
    if (Test-Path $apkPath) {
        Write-Host "    APK ready: $apkPath" -ForegroundColor Green
    }
} else {
    Write-Host ""
    Write-Host "==> Done!  Next steps:" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Build a debug APK:"
    Write-Host "    cd app"
    Write-Host "    flutter build apk --debug"
    Write-Host ""
    Write-Host "  Build a release APK:"
    Write-Host "    cd app"
    Write-Host "    flutter build apk --release"
    Write-Host ""
    Write-Host "  Run on a connected device:"
    Write-Host "    cd app"
    Write-Host "    flutter run"
}
