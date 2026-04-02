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

REM ---- prefer PowerShell 7 (pwsh); auto-install via winget if missing ----
where pwsh >nul 2>&1
if %errorlevel% equ 0 goto :run_script

echo [INFO] pwsh not found. Attempting install via winget...
winget install --id Microsoft.PowerShell --source winget --accept-package-agreements --accept-source-agreements
if %errorlevel% neq 0 goto :fallback_ps5

REM winget succeeded; refresh PATH from registry so pwsh is visible this session
for /f "usebackq tokens=2,*" %%A in (`reg query "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v PATH 2^>nul`) do set "_machinepath=%%B"
for /f "usebackq tokens=2,*" %%A in (`reg query "HKCU\Environment" /v PATH 2^>nul`) do set "_userpath=%%B"
if defined _userpath (set "PATH=%_machinepath%;%_userpath%") else (set "PATH=%_machinepath%")
where pwsh >nul 2>&1
if %errorlevel% equ 0 goto :run_script

:fallback_ps5
echo [WARN] Using Windows PowerShell 5.x. For best results install PowerShell 7:
echo         winget install Microsoft.PowerShell
set "_PS=powershell"
goto :do_run

:run_script
set "_PS=pwsh"

:do_run
%_PS% -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\refresh_server.ps1"

if %errorlevel% neq 0 (
  echo.
  echo [FAILED] deploy exited with code %errorlevel%
  pause
  exit /b %errorlevel%
)

echo.
echo [OK] deploy complete
pause
