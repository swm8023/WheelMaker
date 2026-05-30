Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$manifestPath = Join-Path $repoRoot "mobile\android\app\src\main\AndroidManifest.xml"
$resRoot = Join-Path $repoRoot "mobile\android\app\src\main\res"
$stringsPath = Join-Path $resRoot "values\strings.xml"

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

function Assert-FileExists {
  param([Parameter(Mandatory = $true)][string]$Path)
  if (-not (Test-Path -LiteralPath $Path)) {
    throw "missing expected file: $Path"
  }
}

if (-not (Test-Path -LiteralPath $manifestPath)) {
  throw "AndroidManifest.xml is missing"
}

$manifest = Get-Content -LiteralPath $manifestPath -Raw
Assert-Contains -Label "AndroidManifest.xml" -Text $manifest -Needle 'android:icon="@mipmap/ic_launcher"'
Assert-Contains -Label "AndroidManifest.xml" -Text $manifest -Needle 'android:roundIcon="@mipmap/ic_launcher_round"'
Assert-Contains -Label "AndroidManifest.xml" -Text $manifest -Needle 'android:label="@string/app_name"'

$strings = Get-Content -LiteralPath $stringsPath -Raw
Assert-Contains -Label "strings.xml" -Text $strings -Needle '<string name="app_name">Wheel Maker</string>'

Assert-FileExists -Path (Join-Path $resRoot "mipmap-anydpi\ic_launcher.xml")
Assert-FileExists -Path (Join-Path $resRoot "mipmap-anydpi\ic_launcher_round.xml")
Assert-FileExists -Path (Join-Path $resRoot "mipmap-anydpi-v26\ic_launcher.xml")
Assert-FileExists -Path (Join-Path $resRoot "mipmap-anydpi-v26\ic_launcher_round.xml")
Assert-FileExists -Path (Join-Path $resRoot "drawable\ic_launcher_background.xml")
Assert-FileExists -Path (Join-Path $resRoot "drawable\ic_launcher_foreground.xml")

$foreground = Get-Content -LiteralPath (Join-Path $resRoot "drawable\ic_launcher_foreground.xml") -Raw
Assert-Contains -Label "ic_launcher_foreground.xml" -Text $foreground -Needle "M325,462"
Assert-Contains -Label "ic_launcher_foreground.xml" -Text $foreground -Needle "M1209,462"

Write-Host "android project checks passed"
