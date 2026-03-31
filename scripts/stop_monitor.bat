@echo off
tasklist /FI "IMAGENAME eq wheelmaker-monitor.exe" 2>nul | find /I "wheelmaker-monitor.exe" >nul
if %errorlevel% neq 0 (
  echo no running wheelmaker-monitor process
  exit /b 0
)
taskkill /IM wheelmaker-monitor.exe >nul 2>&1
timeout /t 2 /nobreak >nul
taskkill /IM wheelmaker-monitor.exe /F >nul 2>&1
echo wheelmaker-monitor stopped
