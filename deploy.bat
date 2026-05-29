@echo off
setlocal
title WheelMaker Deploy

set "_REPO=%~dp0"
if "%_REPO:~-1%"=="\" set "_REPO=%_REPO:~0,-1%"
set "_BIN=%USERPROFILE%\.wheelmaker\bin"
set "_DEPLOY_EXE=%_BIN%\wheelmaker-deploy.exe"

echo ============================================
echo   WheelMaker All-in-One Deploy
echo ============================================
echo.
echo   wheelmaker-deploy deploy: update + build + install + configure + publish web
echo.
echo ============================================
echo.

if not exist "%_DEPLOY_EXE%" (
  echo [INFO] wheelmaker-deploy.exe not found. Building bootstrap CLI...
  where go >nul 2>&1
  if errorlevel 1 (
    echo [FAILED] Go is required to build wheelmaker-deploy.exe
    exit /b 1
  )
  if not exist "%_BIN%" mkdir "%_BIN%"
  pushd "%_REPO%\server"
  go build -o "%_DEPLOY_EXE%" .\cmd\wheelmaker-deploy
  set "_BUILD_EXIT=%errorlevel%"
  popd
  if not "%_BUILD_EXIT%"=="0" (
    echo [FAILED] go build wheelmaker-deploy.exe exited with code %_BUILD_EXIT%
    exit /b %_BUILD_EXIT%
  )
)

"%_DEPLOY_EXE%" deploy --repo "%_REPO%" %*
set "_EXIT=%errorlevel%"
if not "%_EXIT%"=="0" (
  echo.
  echo [FAILED] deploy exited with code %_EXIT%
  exit /b %_EXIT%
)

echo.
echo [OK] deploy complete
exit /b 0
