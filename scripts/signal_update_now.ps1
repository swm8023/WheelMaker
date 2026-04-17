param(
  [switch]$Worker,
  [int]$DelaySeconds = 30,
  [string]$SignalPath = (Join-Path -Path $HOME -ChildPath ".wheelmaker\update-now.signal"),
  [string]$AppRoot = (Join-Path -Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path -ChildPath "app"),
  [switch]$SkipWebPublish
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Ensure-ParentDirectory {
  param([Parameter(Mandatory = $true)][string]$Path)
  $parent = Split-Path -Path $Path -Parent
  if (-not [string]::IsNullOrWhiteSpace($parent) -and -not (Test-Path $parent)) {
    New-Item -ItemType Directory -Path $parent -Force | Out-Null
  }
}

function Invoke-WebReleaseBuild {
  param([Parameter(Mandatory = $true)][string]$Root)

  if (-not (Test-Path $Root)) {
    throw ("app directory not found: {0}" -f $Root)
  }
  if (-not (Test-Path (Join-Path -Path $Root -ChildPath "package.json"))) {
    throw ("package.json not found in app directory: {0}" -f $Root)
  }
  if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    throw "npm not found in PATH"
  }

  Write-Host ("==> publishing web assets from: {0}" -f $Root)
  Push-Location $Root
  try {
    & npm run build:web:release
    if ($LASTEXITCODE -ne 0) {
      throw ("npm run build:web:release failed (exit={0})" -f $LASTEXITCODE)
    }
  }
  finally {
    Pop-Location
  }
}

if (-not $Worker) {
  $workerArgs = @(
    "-NoProfile",
    "-ExecutionPolicy", "Bypass",
    "-File", $PSCommandPath,
    "-Worker",
    "-DelaySeconds", "$DelaySeconds",
    "-SignalPath", $SignalPath,
    "-AppRoot", $AppRoot
  )
  if ($SkipWebPublish) {
    $workerArgs += "-SkipWebPublish"
  }

  Start-Process -FilePath "powershell" -ArgumentList $workerArgs -WindowStyle Hidden | Out-Null

  $publishHint = if ($SkipWebPublish) { "web publish skipped" } else { "web publish enabled" }
  Write-Host ("==> updater trigger accepted (delay={0}s, signal={1}, {2})" -f $DelaySeconds, $SignalPath, $publishHint) -ForegroundColor Green
  exit 0
}

if ($DelaySeconds -gt 0) {
  Start-Sleep -Seconds $DelaySeconds
}

if (-not $SkipWebPublish) {
  Invoke-WebReleaseBuild -Root $AppRoot
}

Ensure-ParentDirectory -Path $SignalPath
Set-Content -Path $SignalPath -Value (Get-Date -Format o) -Encoding UTF8
