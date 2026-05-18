$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "deploy.sh"

if (-not (Test-Path $scriptPath)) {
  throw "deploy.sh is missing"
}

$text = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8

function Assert-Contains {
  param([string]$Needle)
  if (-not $text.Contains($Needle)) {
    throw "deploy.sh does not contain expected text: $Needle"
  }
}

Assert-Contains "WheelMaker All-in-One Deploy"
Assert-Contains "scripts/refresh_server.sh"
Assert-Contains "scripts/refresh_server_linux.sh"
Assert-Contains "app/node_modules/.bin/webpack"
Assert-Contains "(cd app && npm ci --include=dev)"
Assert-Contains 'refresh_script="scripts/refresh_server.sh"'
Assert-Contains 'refresh_script="scripts/refresh_server_linux.sh"'
Assert-Contains 'bash "$refresh_script" "$@"'
Assert-Contains "publish web"
Assert-Contains "deploy.sh supports macOS and Linux"
Assert-Contains "deploy.bat on Windows"

Write-Host "deploy.sh source checks passed"
