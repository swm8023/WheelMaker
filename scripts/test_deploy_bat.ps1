$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$deployBatPath = Join-Path $repoRoot "deploy.bat"
$deployBat = Get-Content -LiteralPath $deployBatPath -Raw

function Assert-Contains {
  param(
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$Needle
  )

  if (-not $Text.Contains($Needle)) {
    throw "deploy.bat does not contain expected text: $Needle"
  }
}

function Assert-NotContains {
  param(
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$Needle
  )

  if ($Text.Contains($Needle)) {
    throw "deploy.bat should not contain text: $Needle"
  }
}

Assert-Contains -Text $deployBat -Needle "WheelMaker All-in-One Deploy"
Assert-Contains -Text $deployBat -Needle "wheelmaker-deploy.exe"
Assert-Contains -Text $deployBat -Needle "go build"
Assert-Contains -Text $deployBat -Needle " deploy "
Assert-NotContains -Text $deployBat -Needle 'scripts\refresh_server.ps1'
Assert-NotContains -Text $deployBat -Needle 'pushd "%~dp0app"'
Assert-NotContains -Text $deployBat -Needle "npm run build:web:release"
Assert-NotContains -Text $deployBat -Needle "[FAILED] web publish exited with code"
Assert-NotContains -Text $deployBat -Needle "call npm ci --include=dev"
Assert-NotContains -Text $deployBat -Needle "syncing app Web dependencies"
Assert-NotContains -Text $deployBat -Needle "publish_desktop.ps1"
Assert-NotContains -Text $deployBat -Needle "publish-desktop.bat"

Write-Host "deploy.bat deploy-cli wrapper checks passed"
