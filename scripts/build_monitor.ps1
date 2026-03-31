<#
.SYNOPSIS
  Build wheelmaker-monitor.exe
#>
param(
  [string]$InstallDir = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$serverDir = Join-Path $PSScriptRoot ".." "server"
$serverDir = (Resolve-Path $serverDir).Path

Push-Location $serverDir
try {
  $home = $env:USERPROFILE
  if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $home ".wheelmaker" "bin"
  }

  if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  }

  $outExe = Join-Path $InstallDir "wheelmaker-monitor.exe"
  Write-Host "Building wheelmaker-monitor..." -ForegroundColor Cyan
  & go build -o $outExe ./cmd/wheelmaker-monitor/
  if ($LASTEXITCODE -ne 0) {
    throw "go build failed (exit=$LASTEXITCODE)"
  }
  Write-Host "Built: $outExe" -ForegroundColor Green
}
finally {
  Pop-Location
}
