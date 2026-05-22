$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\publish_desktop.ps1"
$batPath = Join-Path $repoRoot "publish-desktop.bat"

if (-not (Test-Path $scriptPath)) { throw "publish_desktop.ps1 is missing" }
if (-not (Test-Path $batPath)) { throw "publish-desktop.bat is missing" }

$script = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8
$bat = Get-Content -LiteralPath $batPath -Raw -Encoding UTF8

function Assert-Contains {
  param([string]$Label, [string]$Text, [string]$Needle)
  if (-not $Text.Contains($Needle)) {
    throw "$Label does not contain expected text: $Needle"
  }
}

function Assert-NotContains {
  param([string]$Label, [string]$Text, [string]$Needle)
  if ($Text.Contains($Needle)) {
    throw "$Label should not contain text: $Needle"
  }
}

Assert-Contains "publish_desktop.ps1" $script "WHEELMAKER_WEB_TARGET"
Assert-Contains "publish_desktop.ps1" $script "server\cmd\wheelmaker-desktop\winres\icon.png"
Assert-Contains "publish_desktop.ps1" $script "New-DesktopWebBuildRoot"
Assert-Contains "publish_desktop.ps1" $script "New-DesktopWebOverlay"
Assert-Contains "publish_desktop.ps1" $script "Get-DesktopRelativePath"
Assert-Contains "publish_desktop.ps1" $script "Remove-DesktopWebBuildRoot"
Assert-Contains "publish_desktop.ps1" $script "-overlay"
Assert-Contains "publish_desktop.ps1" $script "go-winres@v0.3.3"
Assert-Contains "publish_desktop.ps1" $script "--icon"
Assert-Contains "publish_desktop.ps1" $script "desktop_windows.syso"
Assert-Contains "publish_desktop.ps1" $script "npm run build:web"
Assert-Contains "publish_desktop.ps1" $script "node scripts/export_web_release.js"
Assert-Contains "publish_desktop.ps1" $script "go build"
Assert-Contains "publish_desktop.ps1" $script "WheelMakerDesktop.exe"
Assert-Contains "publish_desktop.ps1" $script '$shortcut.IconLocation = $script:DesktopExe'
Assert-Contains "publish_desktop.ps1" $script "desktop-release.json"
Assert-Contains "publish_desktop.ps1" $script "CreateShortcut"
Assert-Contains "publish_desktop.ps1" $script "Desktop"
Assert-NotContains "publish_desktop.ps1" $script "Convert-DesktopIconSvgToPng"
Assert-NotContains "publish_desktop.ps1" $script "app\scripts\render_svg_icon.js"
Assert-NotContains "publish_desktop.ps1" $script "app\web\public\icons\icon.svg"
Assert-NotContains "publish_desktop.ps1" $script "Get-EdgeExecutable"
Assert-NotContains "publish_desktop.ps1" $script "Restart-Services"
Assert-NotContains "publish_desktop.ps1" $script "update-now.signal"
Assert-NotContains "publish_desktop.ps1" $script "Reset-DesktopWebRoot"
Assert-NotContains "publish_desktop.ps1" $script "Restore-DesktopWebRootPlaceholder"
Assert-NotContains "publish_desktop.ps1" $script "GetRelativePath"
Assert-NotContains "publish_desktop.ps1" $script 'WHEELMAKER_WEB_TARGET = $script:DesktopWebRoot'
Assert-Contains "publish-desktop.bat" $bat "scripts\publish_desktop.ps1"

$desktopIconPath = Join-Path $repoRoot "server\cmd\wheelmaker-desktop\winres\icon.png"
if (-not (Test-Path -LiteralPath $desktopIconPath)) { throw "desktop pre-rendered icon is missing" }
if ((Get-Item -LiteralPath $desktopIconPath).Length -le 0) { throw "desktop pre-rendered icon is empty" }

Write-Host "desktop publish script checks passed"
