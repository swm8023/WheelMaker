@echo off
setlocal
title WheelMaker Deploy

echo ============================================
echo   WheelMaker All-in-One Deploy
echo ============================================
echo.
echo   1. git pull
echo   2. install ACP dependencies
echo   3. go build
echo   4. backup logs
echo   5. stop wheelmaker
echo   6. install binary
echo   7. start service
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
