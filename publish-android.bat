@echo off
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\publish_android.ps1" %*
exit /b %ERRORLEVEL%
