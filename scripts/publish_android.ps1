param(
  [string]$RepoRoot = "",
  [string]$OutputDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\mobile\android"),
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

function New-CleanDirectory {
  param([Parameter(Mandatory = $true)][string]$Path)
  $resolved = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Path)
  $root = [System.IO.Path]::GetFullPath($script:BuildRoot)
  if (-not $root.EndsWith([System.IO.Path]::DirectorySeparatorChar)) {
    $root = $root + [System.IO.Path]::DirectorySeparatorChar
  }
  $target = [System.IO.Path]::GetFullPath($resolved)
  if (-not $target.StartsWith($root, [StringComparison]::OrdinalIgnoreCase)) {
    throw ("refusing to clean path outside Android build root: {0}" -f $target)
  }
  if ($WhatIf) {
    Write-Host ("[whatif] clean directory {0}" -f $target)
    return
  }
  if (Test-Path -LiteralPath $target) {
    Remove-Item -LiteralPath $target -Recurse -Force
  }
  New-Item -ItemType Directory -Path $target -Force | Out-Null
}

function Build-AndroidWeb {
  if (-not $WhatIf) {
    Assert-Command -Name "npm" -Hint "Install Node.js 22+."
    Assert-Command -Name "node" -Hint "Install Node.js 22+."
  }
  New-CleanDirectory -Path $script:WebRoot
  $previousTarget = $env:WHEELMAKER_WEB_TARGET
  $env:WHEELMAKER_WEB_TARGET = $script:WebRoot
  Push-Location $script:AppRoot
  try {
    Write-Step "build embedded Android Workspace Web UI"
    if ($WhatIf) {
      Write-Host ("[whatif] WHEELMAKER_WEB_TARGET={0} npm run build:web" -f $script:WebRoot)
      Write-Host "[whatif] node scripts/export_web_release.js"
      return
    }
    Invoke-Checked -FilePath "npm" -Arguments @("run", "build:web") -FailureMessage "Android Web build failed"
    Invoke-Checked -FilePath "node" -Arguments @("scripts/export_web_release.js") -FailureMessage "Android Web public asset export failed"
    if (-not (Test-Path -LiteralPath (Join-Path $script:WebRoot "index.html"))) {
      throw ("Android Web build missing index.html: {0}" -f $script:WebRoot)
    }
  } finally {
    if ($null -ne $previousTarget) {
      $env:WHEELMAKER_WEB_TARGET = $previousTarget
    } else {
      Remove-Item Env:WHEELMAKER_WEB_TARGET -ErrorAction SilentlyContinue
    }
    Pop-Location
  }
}

function Build-AndroidApk {
  if (-not $WhatIf) {
    Assert-Command -Name "gradle" -Hint "Install Gradle or use Android Studio's Gradle command in PATH."
  }
  New-CleanDirectory -Path $script:GradleBuildRoot
  New-CleanDirectory -Path $script:GradleCacheDir
  New-CleanDirectory -Path $script:GradleHomeDir

  Push-Location $script:AndroidRoot
  try {
    Write-Step "build WheelMakerAndroid APK"
    $args = @(
      "assembleRelease",
      "--project-cache-dir", $script:GradleCacheDir,
      "-g", $script:GradleHomeDir,
      "-PwheelmakerBuildRoot=$script:GradleBuildRoot",
      "-PwheelmakerWebAssetsDir=$script:WebRoot"
    )
    if ($WhatIf) {
      Write-Host ("[whatif] gradle {0}" -f ($args -join " "))
      return
    }
    New-Item -ItemType Directory -Path $script:OutputDir -Force | Out-Null
    Invoke-Checked -FilePath "gradle" -Arguments $args -FailureMessage "Android APK build failed"
  } finally {
    Pop-Location
  }
}

function Copy-AndroidOutputs {
  $apk = Get-ChildItem -LiteralPath $script:GradleBuildRoot -Recurse -Filter "*.apk" |
    Where-Object { $_.FullName -match "\\outputs\\apk\\release\\" } |
    Select-Object -First 1
  if ($null -eq $apk) {
    throw ("release APK was not produced under {0}" -f $script:GradleBuildRoot)
  }
  $target = Join-Path $script:OutputDir "WheelMakerAndroid.apk"
  Write-Step ("copy APK: {0}" -f $target)
  if ($WhatIf) {
    Write-Host ("[whatif] copy {0} -> {1}" -f $apk.FullName, $target)
    return
  }
  Copy-Item -LiteralPath $apk.FullName -Destination $target -Force
}

function Write-AndroidReleaseManifest {
  Assert-Command -Name "git" -Hint "Install Git and ensure git.exe is available."
  $apkPath = Join-Path $script:OutputDir "WheelMakerAndroid.apk"
  $apkHash = ""
  $apkSize = 0
  if (-not $WhatIf -and (Test-Path -LiteralPath $apkPath)) {
    $apkHash = (Get-FileHash -LiteralPath $apkPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $apkSize = (Get-Item -LiteralPath $apkPath).Length
  }
  $manifest = [ordered]@{
    "schemaVersion" = 1
    "platform" = "android"
    "repo" = $script:RepoRoot
    "branch" = Get-GitValue -Arguments @("branch", "--show-current")
    "sha" = Get-GitValue -Arguments @("rev-parse", "HEAD")
    "builtAt" = (Get-Date).ToUniversalTime().ToString("o")
    "apk" = [ordered]@{
      "fileName" = "WheelMakerAndroid.apk"
      "path" = $apkPath
      "sha256" = $apkHash
      "size" = $apkSize
    }
    "webRoot" = $script:WebRoot
    "gradleBuildRoot" = $script:GradleBuildRoot
  }
  if ($WhatIf) {
    Write-Host ("[whatif] write {0}" -f $script:ManifestPath)
    return
  }
  New-Item -ItemType Directory -Path $script:OutputDir -Force | Out-Null
  $json = $manifest | ConvertTo-Json -Depth 8
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($script:ManifestPath, $json, $utf8NoBom)
}

$script:RepoRoot = if ([string]::IsNullOrWhiteSpace($RepoRoot)) { (Resolve-Path (Join-Path $PSScriptRoot "..")).Path } else { (Resolve-Path $RepoRoot).Path }
$script:AppRoot = Join-Path $script:RepoRoot "app"
$script:AndroidRoot = Join-Path $script:RepoRoot "mobile\android"
$script:WheelMakerHome = Join-Path $HOME ".wheelmaker"
$script:BuildRoot = Join-Path $script:WheelMakerHome "build\mobile\android"
$script:WebRoot = Join-Path $script:BuildRoot "webroot"
$script:GradleBuildRoot = Join-Path $script:BuildRoot "gradle-build"
$script:GradleCacheDir = Join-Path $script:BuildRoot "gradle-cache"
$script:GradleHomeDir = Join-Path $script:BuildRoot "gradle-home"
$script:OutputDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputDir)
$script:ManifestPath = Join-Path $script:OutputDir "android-release.json"

Build-AndroidWeb
Build-AndroidApk
if ($WhatIf) {
  $target = Join-Path $script:OutputDir "WheelMakerAndroid.apk"
  Write-Step ("copy APK: {0}" -f $target)
  Write-Host ("[whatif] copy release APK from {0} -> {1}" -f $script:GradleBuildRoot, $target)
} else {
  Copy-AndroidOutputs
}
Write-AndroidReleaseManifest
Write-Step "Android APK publish complete"
