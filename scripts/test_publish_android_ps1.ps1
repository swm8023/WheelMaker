Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\publish_android.ps1"
$batPath = Join-Path $repoRoot "publish-android.bat"

function Assert-Contains {
  param(
    [Parameter(Mandatory = $true)][string]$Label,
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$Needle
  )
  if (-not $Text.Contains($Needle)) {
    throw "$Label missing expected text: $Needle"
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

if (-not (Test-Path -LiteralPath $scriptPath)) {
  throw "publish_android.ps1 is missing"
}
if (-not (Test-Path -LiteralPath $batPath)) {
  throw "publish-android.bat is missing"
}

$script = Get-Content -LiteralPath $scriptPath -Raw
$bat = Get-Content -LiteralPath $batPath -Raw

Assert-Contains -Label "publish_android.ps1" -Text $script -Needle ".wheelmaker"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "build\mobile\android"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "mobile\android"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "WHEELMAKER_WEB_TARGET"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "wheelmakerWebAssetsDir"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "wheelmakerBuildRoot"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "--project-cache-dir"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "gradle-home"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "android-release.json"
Assert-NotContains -Label "publish_android.ps1" -Text $script -Needle ".wheelmaker\web\mobile\android"
Assert-NotContains -Label "publish_android.ps1" -Text $script -Needle "mobile\android\app\src\main\assets\wheelmaker-web"

Assert-Contains -Label "publish-android.bat" -Text $bat -Needle "scripts\publish_android.ps1"
Assert-Contains -Label "publish-android.bat" -Text $bat -Needle "powershell"

Write-Host "publish_android.ps1 checks passed"
