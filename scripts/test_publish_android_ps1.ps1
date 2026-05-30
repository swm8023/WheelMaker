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

function Get-ExpectedAndroidBuildRoot {
  $homeBuildRoot = [System.IO.Path]::GetFullPath((Join-Path $HOME ".wheelmaker\build\mobile\android"))
  $repoRootPath = [System.IO.Path]::GetFullPath($repoRoot)
  $homePathRoot = [System.IO.Path]::GetPathRoot($homeBuildRoot)
  $repoPathRoot = [System.IO.Path]::GetPathRoot($repoRootPath)
  if ($homePathRoot.Equals($repoPathRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
    return $homeBuildRoot
  }
  return [System.IO.Path]::GetFullPath((Join-Path $repoPathRoot ".wheelmaker\build\mobile\android"))
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
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "kotlin.project.persistent.dir"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "--project-cache-dir"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "gradle-home"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "https.protocols=TLSv1.2"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "GetPathRoot"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "android-release.json"
Assert-NotContains -Label "publish_android.ps1" -Text $script -Needle ".wheelmaker\web\mobile\android"
Assert-NotContains -Label "publish_android.ps1" -Text $script -Needle "mobile\android\app\src\main\assets\wheelmaker-web"

Assert-Contains -Label "publish-android.bat" -Text $bat -Needle "scripts\publish_android.ps1"
Assert-Contains -Label "publish-android.bat" -Text $bat -Needle "powershell"

$whatIfOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $scriptPath -WhatIf 2>&1
if ($LASTEXITCODE -ne 0) {
  throw "publish_android.ps1 -WhatIf failed: $whatIfOutput"
}
Assert-Contains -Label "publish_android.ps1 -WhatIf" -Text ($whatIfOutput -join [Environment]::NewLine) -Needle (Get-ExpectedAndroidBuildRoot)

Write-Host "publish_android.ps1 checks passed"
