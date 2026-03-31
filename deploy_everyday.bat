@echo off
setlocal
title WheelMaker - Setup Daily Auto-Update

echo ============================================
echo   WheelMaker Daily Auto-Update Setup
echo ============================================
echo.
echo   This will create a scheduled task that
echo   checks for git updates every day at 3 AM
echo   and auto-deploys if new commits are found.
echo.
echo   Task name: WheelMaker-AutoUpdate
echo   Log file:  %USERPROFILE%\.wheelmaker\auto_update.log
echo.
echo ============================================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\auto_update.ps1" -Setup -RepoDir "%~dp0."

if %errorlevel% neq 0 (
  echo.
  echo [FAILED] setup exited with code %errorlevel%
  pause
  exit /b %errorlevel%
)

echo.
echo [OK] Daily auto-update is now active.
pause
