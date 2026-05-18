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

function Assert-Ordered {
  param(
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$First,
    [Parameter(Mandatory = $true)][string]$Second
  )

  $firstIndex = $Text.IndexOf($First, [StringComparison]::OrdinalIgnoreCase)
  $secondIndex = $Text.IndexOf($Second, [StringComparison]::OrdinalIgnoreCase)

  if ($firstIndex -lt 0) {
    throw "deploy.bat does not contain expected text: $First"
  }
  if ($secondIndex -lt 0) {
    throw "deploy.bat does not contain expected text: $Second"
  }
  if ($firstIndex -ge $secondIndex) {
    throw "deploy.bat should run '$First' before '$Second'"
  }
}

Assert-Contains -Text $deployBat -Needle "update + build + stop + deploy + start + publish web"
Assert-Contains -Text $deployBat -Needle 'app\node_modules\.bin\webpack.cmd'
Assert-Contains -Text $deployBat -Needle 'cd /d "%~dp0app" ^&^& npm ci --include=dev'
Assert-Contains -Text $deployBat -Needle 'pushd "%~dp0app"'
Assert-Contains -Text $deployBat -Needle "npm run build:web:release"
Assert-Contains -Text $deployBat -Needle "[FAILED] web publish exited with code"
Assert-Ordered -Text $deployBat -First "app\node_modules\.bin\webpack.cmd" -Second "scripts\refresh_server.ps1"
Assert-Ordered -Text $deployBat -First "scripts\refresh_server.ps1" -Second "npm run build:web:release"

Write-Host "deploy.bat web publish checks passed"
