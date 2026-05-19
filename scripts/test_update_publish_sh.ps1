$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "update-publish.sh"
$claudePath = Join-Path $repoRoot "CLAUDE.md"
$readmePath = Join-Path $repoRoot "README.md"

function Assert-Contains {
  param(
    [Parameter(Mandatory = $true)][string]$Label,
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$Needle
  )

  if (-not $Text.Contains($Needle)) {
    throw "$Label does not contain expected text: $Needle"
  }
}

function Assert-NotContains {
  param(
    [Parameter(Mandatory = $true)][string]$Label,
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$Needle
  )

  if ($Text.Contains($Needle)) {
    throw "$Label should not contain text: $Needle"
  }
}

if (-not (Test-Path $scriptPath)) {
  throw "update-publish.sh is missing"
}

$script = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8
$claude = Get-Content -LiteralPath $claudePath -Raw -Encoding UTF8
$readme = Get-Content -LiteralPath $readmePath -Raw -Encoding UTF8

Assert-Contains -Label "update-publish.sh" -Text $script -Needle "WheelMaker Update Publish"
Assert-Contains -Label "update-publish.sh" -Text $script -Needle '${HOME}/.wheelmaker/update-now.signal'
Assert-Contains -Label "update-publish.sh" -Text $script -Needle "full-update"
Assert-Contains -Label "update-publish.sh" -Text $script -Needle "updater trigger accepted"
Assert-Contains -Label "update-publish.sh" -Text $script -Needle "Darwin|Linux"
Assert-Contains -Label "update-publish.sh" -Text $script -Needle "update-publish.bat"
Assert-NotContains -Label "update-publish.sh" -Text $script -Needle "deploy.sh"
Assert-NotContains -Label "update-publish.sh" -Text $script -Needle "refresh_server.sh"
Assert-NotContains -Label "update-publish.sh" -Text $script -Needle "build:web:release"

Assert-Contains -Label "CLAUDE.md" -Text $claude -Needle "update-publish.sh"
Assert-Contains -Label "README.md" -Text $readme -Needle "bash update-publish.sh"

Write-Host "update-publish.sh checks passed"
