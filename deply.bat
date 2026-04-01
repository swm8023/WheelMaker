@echo off
setlocal
title WheelMaker Deploy

echo ============================================
echo   WheelMaker All-in-One Deploy
echo ============================================
echo.
echo   update + build + stop + deploy + start
echo.
echo ============================================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\refresh_server.ps1"

if %errorlevel% neq 0 (
  echo.
  echo [FAILED] deploy exited with code %errorlevel%
  pause
  exit /b %errorlevel%
)

echo.
echo [OK] deploy complete
pause
