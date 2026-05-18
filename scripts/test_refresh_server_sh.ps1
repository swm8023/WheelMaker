$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\refresh_server.sh"

if (-not (Test-Path $scriptPath)) {
  throw "refresh_server.sh is missing"
}

$text = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8

function Assert-Contains {
  param([string]$Needle)
  if (-not $text.Contains($Needle)) {
    throw "refresh_server.sh does not contain expected text: $Needle"
  }
}

Assert-Contains "com.wheelmaker.hub"
Assert-Contains "com.wheelmaker.monitor"
Assert-Contains "com.wheelmaker.updater"
Assert-Contains "launchctl bootstrap"
Assert-Contains "launchctl bootout"
Assert-Contains "launchctl kickstart"
Assert-Contains "GOOS=darwin"
Assert-Contains "npm run build:web:release"
Assert-Contains "--skip-web-publish"
Assert-Contains "Node.js 22.11.0+"
Assert-Contains "~/Library/LaunchAgents"

Write-Host "refresh_server.sh source checks passed"
