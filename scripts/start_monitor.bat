@echo off
set "exe=%~dp0bin\wheelmaker-monitor.exe"
if not exist "%exe%" (
  echo wheelmaker-monitor.exe not found: %exe%
  exit /b 1
)
tasklist /FI "IMAGENAME eq wheelmaker-monitor.exe" 2>nul | find /I "wheelmaker-monitor.exe" >nul
if %errorlevel%==0 (
  echo wheelmaker-monitor already running
  exit /b 0
)
powershell -NoProfile -Command "Start-Process '%exe%' -WindowStyle Hidden"
timeout /t 2 /nobreak >nul
tasklist /FI "IMAGENAME eq wheelmaker-monitor.exe" 2>nul | find /I "wheelmaker-monitor.exe" >nul
if %errorlevel%==0 (
  echo wheelmaker-monitor started — http://localhost:9631
) else (
  echo wheelmaker-monitor failed to start
  exit /b 1
)
