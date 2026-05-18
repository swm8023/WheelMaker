@echo off
setlocal
title WheelMaker Update Publish

echo ============================================
echo   WheelMaker Update Publish
echo ============================================
echo.
echo   request updater service full update + publish
echo.
echo ============================================
echo.

where pwsh >nul 2>&1
if %errorlevel% equ 0 (
  set "_PS=pwsh"
) else (
  set "_PS=powershell"
)

%_PS% -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference = 'Stop'; $signalPath = Join-Path -Path $HOME -ChildPath '.wheelmaker\update-now.signal'; $parent = Split-Path -Path $signalPath -Parent; if (-not [string]::IsNullOrWhiteSpace($parent) -and -not (Test-Path -LiteralPath $parent)) { New-Item -ItemType Directory -Path $parent -Force | Out-Null }; Set-Content -LiteralPath $signalPath -Value @('full-update', (Get-Date -Format o)) -Encoding UTF8; Write-Host ('[OK] updater trigger accepted: {0}' -f $signalPath)"
set "_EXIT=%errorlevel%"
if not "%_EXIT%"=="0" (
  echo.
  echo [FAILED] update publish trigger exited with code %_EXIT%
  exit /b %_EXIT%
)

echo.
echo [OK] update publish requested. Check ~/.wheelmaker/log/updater.log for progress.
exit /b 0
