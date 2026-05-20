@echo off
setlocal
title WheelMaker Desktop Publish

where pwsh >nul 2>&1
if %errorlevel% equ 0 (
  set "_PS=pwsh"
) else (
  set "_PS=powershell"
)

%_PS% -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\publish_desktop.ps1"
set "_EXIT=%errorlevel%"
if not "%_EXIT%"=="0" (
  echo.
  echo [FAILED] desktop publish exited with code %_EXIT%
  pause
  exit /b %_EXIT%
)

echo.
echo [OK] WheelMaker Desktop publish complete
pause
exit /b 0
