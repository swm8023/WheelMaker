$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$batPath = Join-Path $repoRoot "update-publish.bat"
$claudePath = Join-Path $repoRoot "CLAUDE.md"

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

if (-not (Test-Path $batPath)) {
  throw "update-publish.bat is missing"
}

$bat = Get-Content -LiteralPath $batPath -Raw -Encoding UTF8
$claude = Get-Content -LiteralPath $claudePath -Raw -Encoding UTF8
$updatePublishProjectPhrase = -join ([char[]](0x66F4, 0x65B0, 0x53D1, 0x5E03, 0x5DE5, 0x7A0B))

Assert-Contains -Label "update-publish.bat" -Text $bat -Needle "WheelMaker Update Publish"
Assert-Contains -Label "update-publish.bat" -Text $bat -Needle ".wheelmaker\update-now.signal"
Assert-Contains -Label "update-publish.bat" -Text $bat -Needle "full-update"
Assert-Contains -Label "update-publish.bat" -Text $bat -Needle "updater trigger accepted"
Assert-NotContains -Label "update-publish.bat" -Text $bat -Needle "deploy.bat"
Assert-NotContains -Label "update-publish.bat" -Text $bat -Needle "refresh_server.ps1"
Assert-NotContains -Label "update-publish.bat" -Text $bat -Needle "build:web:release"
Assert-NotContains -Label "update-publish.bat" -Text $bat -Needle "publish_desktop.ps1"
Assert-NotContains -Label "update-publish.bat" -Text $bat -Needle "publish-desktop.bat"

Assert-Contains -Label "CLAUDE.md" -Text $claude -Needle $updatePublishProjectPhrase
Assert-Contains -Label "CLAUDE.md" -Text $claude -Needle "update-publish.bat"

Write-Host "update-publish.bat checks passed"
