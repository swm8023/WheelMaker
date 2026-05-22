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

function Get-DesktopRelativePath {
  param(
    [Parameter(Mandatory = $true)][string]$Root,
    [Parameter(Mandatory = $true)][string]$Path
  )
  $rootFull = [System.IO.Path]::GetFullPath($Root)
  if (-not $rootFull.EndsWith([System.IO.Path]::DirectorySeparatorChar)) {
    $rootFull = $rootFull + [System.IO.Path]::DirectorySeparatorChar
  }
  $pathFull = [System.IO.Path]::GetFullPath($Path)
  if (-not $pathFull.StartsWith($rootFull, [StringComparison]::OrdinalIgnoreCase)) {
    throw ("path is outside root: {0}" -f $pathFull)
  }
  return $pathFull.Substring($rootFull.Length)
}

function New-DesktopWebBuildRoot {
  $root = Join-Path ([System.IO.Path]::GetTempPath()) ("wheelmaker-desktop-webroot-{0}" -f [guid]::NewGuid().ToString("N"))
  $script:DesktopWebBuildRoot = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($root)
  $script:DesktopOverlay = Join-Path $script:DesktopWebBuildRoot "overlay.json"
  if ($WhatIf) {
    Write-Host ("[whatif] create desktop web build root {0}" -f $script:DesktopWebBuildRoot)
    return
  }
  New-Item -ItemType Directory -Path $script:DesktopWebBuildRoot -Force | Out-Null
}

function Remove-DesktopWebBuildRoot {
  if ([string]::IsNullOrWhiteSpace($script:DesktopWebBuildRoot)) { return }
  $resolved = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($script:DesktopWebBuildRoot)
  $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
  if (-not $tempRoot.EndsWith([System.IO.Path]::DirectorySeparatorChar)) {
    $tempRoot = $tempRoot + [System.IO.Path]::DirectorySeparatorChar
  }
  $leaf = Split-Path -Path $resolved -Leaf
  if (-not $resolved.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -or -not $leaf.StartsWith("wheelmaker-desktop-webroot-", [StringComparison]::OrdinalIgnoreCase)) {
    throw ("refusing to remove unexpected desktop web build root: {0}" -f $resolved)
  }
  if ($WhatIf) {
    Write-Host ("[whatif] remove desktop web build root {0}" -f $resolved)
    return
  }
  if (Test-Path -LiteralPath $resolved) {
    Remove-Item -LiteralPath $resolved -Recurse -Force
  }
}

function New-DesktopWebOverlay {
  if ([string]::IsNullOrWhiteSpace($script:DesktopWebBuildRoot)) {
    throw "desktop web build root has not been created"
  }
  if ($WhatIf) {
    Write-Host ("[whatif] write Go build overlay {0} for {1}" -f $script:DesktopOverlay, $script:DesktopVirtualWebRoot)
    return
  }
  if (-not (Test-Path -LiteralPath $script:DesktopWebBuildRoot)) {
    throw ("desktop web build root is missing: {0}" -f $script:DesktopWebBuildRoot)
  }
  $files = @(Get-ChildItem -LiteralPath $script:DesktopWebBuildRoot -File -Recurse)
  if ($files.Count -eq 0) {
    throw ("desktop web build root is empty: {0}" -f $script:DesktopWebBuildRoot)
  }
  $replace = [ordered]@{}
  foreach ($file in $files) {
    $relative = Get-DesktopRelativePath -Root $script:DesktopWebBuildRoot -Path $file.FullName
    $virtualPath = Join-Path $script:DesktopVirtualWebRoot $relative
    $replace[$virtualPath] = $file.FullName
  }
  $overlay = [ordered]@{
    "Replace" = $replace
  }
  $json = $overlay | ConvertTo-Json -Depth 100
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($script:DesktopOverlay, $json, $utf8NoBom)
}

function Build-DesktopWeb {
  Assert-Command -Name "npm" -Hint "Install Node.js 22+."
  Assert-Command -Name "node" -Hint "Install Node.js 22+."
  New-DesktopWebBuildRoot
  $previousTarget = $env:WHEELMAKER_WEB_TARGET
  $env:WHEELMAKER_WEB_TARGET = $script:DesktopWebBuildRoot
  Push-Location $script:AppRoot
  try {
    Write-Step "build embedded Workspace Web UI"
    if ($WhatIf) {
      Write-Host ("[whatif] WHEELMAKER_WEB_TARGET={0} npm run build:web" -f $script:DesktopWebBuildRoot)
      Write-Host "[whatif] node scripts/export_web_release.js"
      New-DesktopWebOverlay
      return
    }
    Invoke-Checked -FilePath "npm" -Arguments @("run", "build:web") -FailureMessage "desktop web build failed"
    Invoke-Checked -FilePath "node" -Arguments @("scripts/export_web_release.js") -FailureMessage "desktop web public asset export failed"
    New-DesktopWebOverlay
  } finally {
    if ($null -ne $previousTarget) {
      $env:WHEELMAKER_WEB_TARGET = $previousTarget
    } else {
      Remove-Item Env:WHEELMAKER_WEB_TARGET -ErrorAction SilentlyContinue
    }
    Pop-Location
  }
}

function Build-DesktopResource {
  Assert-Command -Name "go" -Hint "Install Go 1.26+."
  if (-not (Test-Path -LiteralPath $script:DesktopIconPng)) {
    throw ("desktop icon PNG is missing: {0}" -f $script:DesktopIconPng)
  }
  Write-Step "generate desktop exe icon resource"
  if ($WhatIf) {
    Write-Host ("[whatif] go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --out {0} --no-suffix --manifest gui --icon {1}" -f $script:DesktopSyso, $script:DesktopIconPng)
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
      $script:DesktopIconPng,
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
    $buildArgs = @("build", "-overlay", $script:DesktopOverlay, "-ldflags", "-H windowsgui", "-o", $script:DesktopExe, "./cmd/wheelmaker-desktop/")
    if ($WhatIf) { Write-Host ("[whatif] go build -overlay {0} -ldflags -H windowsgui -o {1} ./cmd/wheelmaker-desktop/" -f $script:DesktopOverlay, $script:DesktopExe); return }
    if (-not (Test-Path -LiteralPath $script:DesktopOverlay)) {
      throw ("desktop web overlay is missing: {0}" -f $script:DesktopOverlay)
    }
    New-Item -ItemType Directory -Path $script:OutputDir -Force | Out-Null
    Invoke-Checked -FilePath "go" -Arguments $buildArgs -FailureMessage "desktop binary build failed"
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
    "embeddedWebRoot" = $script:DesktopVirtualWebRoot
    "embeddedWebBuildMode" = "go-build-overlay"
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
  $shortcut.IconLocation = $script:DesktopExe
  $shortcut.Save()
}

$script:RepoRoot = if ([string]::IsNullOrWhiteSpace($RepoRoot)) { (Resolve-Path (Join-Path $PSScriptRoot "..")).Path } else { (Resolve-Path $RepoRoot).Path }
$script:AppRoot = Join-Path $script:RepoRoot "app"
$script:ServerRoot = Join-Path $script:RepoRoot "server"
$script:DesktopVirtualWebRoot = Join-Path $script:RepoRoot "server\cmd\wheelmaker-desktop\webroot"
$script:DesktopWebBuildRoot = ""
$script:DesktopOverlay = ""
$script:DesktopSyso = Join-Path $script:RepoRoot "server\cmd\wheelmaker-desktop\desktop_windows.syso"
$script:OutputDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputDir)
$script:DesktopIconPng = Join-Path $script:RepoRoot "server\cmd\wheelmaker-desktop\winres\icon.png"
$script:DesktopExe = Join-Path $script:OutputDir "WheelMakerDesktop.exe"
$script:ManifestPath = Join-Path $script:OutputDir "desktop-release.json"

try {
  Build-DesktopWeb
  Build-DesktopResource
  Build-DesktopBinary
  Write-DesktopReleaseManifest
  New-DesktopShortcut
  Write-Step "desktop publish complete"
} finally {
  Remove-DesktopWebBuildRoot
}
