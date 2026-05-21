param(
  [string]$RepoRoot = "",
  [string]$OutputDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\desktop"),
  [switch]$WhatIf
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Step {
  param([string]$Text)
  Write-Host ("==> {0}" -f $Text)
}

function Assert-Command {
  param([Parameter(Mandatory = $true)][string]$Name, [string]$Hint = "")
  if (Get-Command $Name -ErrorAction SilentlyContinue) { return }
  if ([string]::IsNullOrWhiteSpace($Hint)) { throw ("required command not found in PATH: {0}" -f $Name) }
  throw ("required command not found in PATH: {0}. {1}" -f $Name, $Hint)
}

function Invoke-Checked {
  param(
    [Parameter(Mandatory = $true)][string]$FilePath,
    [string[]]$Arguments = @(),
    [string]$FailureMessage = ""
  )
  & $FilePath @Arguments
  if ($LASTEXITCODE -eq 0) { return }
  if ([string]::IsNullOrWhiteSpace($FailureMessage)) {
    throw ("command failed: {0} {1} (exit={2})" -f $FilePath, ($Arguments -join " "), $LASTEXITCODE)
  }
  throw ("{0} (exit={1})" -f $FailureMessage, $LASTEXITCODE)
}

function Get-GitValue {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  Push-Location $script:RepoRoot
  try {
    $value = ((& git @Arguments) | Select-Object -First 1)
    if ($LASTEXITCODE -ne 0) { throw ("git {0} failed (exit={1})" -f ($Arguments -join " "), $LASTEXITCODE) }
    return ([string]$value).Trim()
  } finally {
    Pop-Location
  }
}

function Reset-DesktopWebRoot {
  $resolved = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($script:DesktopWebRoot)
  $expected = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath((Join-Path $script:RepoRoot "server\cmd\wheelmaker-desktop\webroot"))
  if (-not [String]::Equals($resolved, $expected, [StringComparison]::OrdinalIgnoreCase)) {
    throw ("refusing to clean unexpected desktop webroot: {0}" -f $resolved)
  }
  if ($WhatIf) { Write-Host ("[whatif] clean {0}" -f $resolved); return }
  New-Item -ItemType Directory -Path $resolved -Force | Out-Null
  Get-ChildItem -LiteralPath $resolved -Force | Where-Object { $_.Name -ne ".gitkeep" } | Remove-Item -Recurse -Force
}

function Restore-DesktopWebRootPlaceholder {
  if ($WhatIf) {
    Write-Host ("[whatif] restore {0}" -f (Join-Path $script:DesktopWebRoot ".gitkeep"))
    return
  }
  New-Item -ItemType Directory -Path $script:DesktopWebRoot -Force | Out-Null
  $placeholderPath = Join-Path $script:DesktopWebRoot ".gitkeep"
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($placeholderPath, "`n", $utf8NoBom)
}

function Build-DesktopWeb {
  Assert-Command -Name "npm" -Hint "Install Node.js 22+."
  Assert-Command -Name "node" -Hint "Install Node.js 22+."
  Reset-DesktopWebRoot
  $previousTarget = $env:WHEELMAKER_WEB_TARGET
  $env:WHEELMAKER_WEB_TARGET = $script:DesktopWebRoot
  Push-Location $script:AppRoot
  try {
    Write-Step "build embedded Workspace Web UI"
    if ($WhatIf) {
      Write-Host ("[whatif] WHEELMAKER_WEB_TARGET={0} npm run build:web" -f $script:DesktopWebRoot)
      Write-Host "[whatif] node scripts/export_web_release.js"
      return
    }
    Invoke-Checked -FilePath "npm" -Arguments @("run", "build:web") -FailureMessage "desktop web build failed"
    Invoke-Checked -FilePath "node" -Arguments @("scripts/export_web_release.js") -FailureMessage "desktop web public asset export failed"
    Restore-DesktopWebRootPlaceholder
  } finally {
    if ($null -ne $previousTarget) {
      $env:WHEELMAKER_WEB_TARGET = $previousTarget
    } else {
      Remove-Item Env:WHEELMAKER_WEB_TARGET -ErrorAction SilentlyContinue
    }
    Pop-Location
  }
}

function Convert-DesktopIconSvgToPng {
  $iconSource = Join-Path $script:RepoRoot "app\web\public\icons\icon.svg"
  if (-not (Test-Path -LiteralPath $iconSource)) {
    throw ("PWA icon source is missing: {0}" -f $iconSource)
  }
  $renderer = Join-Path $script:RepoRoot "app\scripts\render_svg_icon.js"
  if (-not (Test-Path -LiteralPath $renderer)) {
    throw ("SVG icon renderer is missing: {0}" -f $renderer)
  }
  if ($WhatIf) {
    Write-Host ("[whatif] node {0} {1} {2} 1024" -f $renderer, $iconSource, $script:DesktopGeneratedIconPng)
    return
  }

  New-Item -ItemType Directory -Path (Split-Path -Parent $script:DesktopGeneratedIconPng) -Force | Out-Null
  Invoke-Checked -FilePath "node" -Arguments @(
    $renderer,
    $iconSource,
    $script:DesktopGeneratedIconPng,
    "1024"
  ) -FailureMessage "desktop SVG icon rasterization failed"
}

function Build-DesktopResource {
  Assert-Command -Name "node" -Hint "Install Node.js 22+."
  Assert-Command -Name "go" -Hint "Install Go 1.26+."
  Write-Step "generate desktop exe icon resource"
  Convert-DesktopIconSvgToPng
  if ($WhatIf) {
    Write-Host ("[whatif] go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --out {0} --no-suffix --manifest gui --icon {1}" -f $script:DesktopSyso, $script:DesktopGeneratedIconPng)
    return
  }
  Push-Location (Join-Path $script:RepoRoot "server\cmd\wheelmaker-desktop")
  try {
    Invoke-Checked -FilePath "go" -Arguments @(
      "run",
      "github.com/tc-hib/go-winres@v0.3.3",
      "simply",
      "--arch",
      "amd64",
      "--out",
      $script:DesktopSyso,
      "--no-suffix",
      "--manifest",
      "gui",
      "--icon",
      $script:DesktopGeneratedIconPng,
      "--file-description",
      "WheelMaker Desktop",
      "--product-name",
      "WheelMaker Desktop",
      "--original-filename",
      "WheelMakerDesktop.exe"
    ) -FailureMessage "desktop Windows resource generation failed"
  } finally {
    Pop-Location
  }
}

function Build-DesktopBinary {
  Assert-Command -Name "go" -Hint "Install Go 1.26+."
  Push-Location $script:ServerRoot
  try {
    Write-Step ("build WheelMakerDesktop.exe: {0}" -f $script:DesktopExe)
    if ($WhatIf) { Write-Host ("[whatif] go build -ldflags -H windowsgui -o {0} ./cmd/wheelmaker-desktop/" -f $script:DesktopExe); return }
    New-Item -ItemType Directory -Path $script:OutputDir -Force | Out-Null
    Invoke-Checked -FilePath "go" -Arguments @("build", "-ldflags", "-H windowsgui", "-o", $script:DesktopExe, "./cmd/wheelmaker-desktop/") -FailureMessage "desktop binary build failed"
  } finally {
    Pop-Location
  }
}

function Write-DesktopReleaseManifest {
  Assert-Command -Name "git" -Hint "Install Git and ensure git.exe is available."
  $manifest = [ordered]@{
    "schemaVersion" = 1
    "repo" = $script:RepoRoot
    "branch" = Get-GitValue -Arguments @("branch", "--show-current")
    "sha" = Get-GitValue -Arguments @("rev-parse", "HEAD")
    "builtAt" = (Get-Date).ToUniversalTime().ToString("o")
    "desktopExe" = $script:DesktopExe
    "embeddedWebRoot" = $script:DesktopWebRoot
  }
  if ($WhatIf) { Write-Host ("[whatif] write {0}" -f $script:ManifestPath); return }
  New-Item -ItemType Directory -Path $script:OutputDir -Force | Out-Null
  $json = $manifest | ConvertTo-Json -Depth 4
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($script:ManifestPath, $json, $utf8NoBom)
}

function New-DesktopShortcut {
  $desktop = [Environment]::GetFolderPath("Desktop")
  $shortcutPath = Join-Path $desktop "WheelMaker Desktop.lnk"
  Write-Step ("create desktop shortcut: {0}" -f $shortcutPath)
  if ($WhatIf) { Write-Host ("[whatif] CreateShortcut {0} -> {1}" -f $shortcutPath, $script:DesktopExe); return }
  $shell = New-Object -ComObject WScript.Shell
  $shortcut = $shell.CreateShortcut($shortcutPath)
  $shortcut.TargetPath = $script:DesktopExe
  $shortcut.WorkingDirectory = $script:OutputDir
  $shortcut.IconLocation = $script:DesktopGeneratedIconPng
  $shortcut.Save()
}

$script:RepoRoot = if ([string]::IsNullOrWhiteSpace($RepoRoot)) { (Resolve-Path (Join-Path $PSScriptRoot "..")).Path } else { (Resolve-Path $RepoRoot).Path }
$script:AppRoot = Join-Path $script:RepoRoot "app"
$script:ServerRoot = Join-Path $script:RepoRoot "server"
$script:DesktopWebRoot = Join-Path $script:RepoRoot "server\cmd\wheelmaker-desktop\webroot"
$script:DesktopSyso = Join-Path $script:RepoRoot "server\cmd\wheelmaker-desktop\desktop_windows.syso"
$script:OutputDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputDir)
$script:DesktopGeneratedIconPng = Join-Path $script:OutputDir "winres-icon.png"
$script:DesktopExe = Join-Path $script:OutputDir "WheelMakerDesktop.exe"
$script:ManifestPath = Join-Path $script:OutputDir "desktop-release.json"

Build-DesktopWeb
Build-DesktopResource
Build-DesktopBinary
Write-DesktopReleaseManifest
New-DesktopShortcut
Write-Step "desktop publish complete"
