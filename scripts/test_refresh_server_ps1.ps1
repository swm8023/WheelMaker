$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\refresh_server.ps1"

if (-not (Test-Path $scriptPath)) {
  throw "refresh_server.ps1 is missing"
}

$text = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8

function Assert-Contains {
  param([string]$Needle)
  if (-not $text.Contains($Needle)) {
    throw "refresh_server.ps1 does not contain expected text: $Needle"
  }
}

function Assert-NotContains {
  param([string]$Needle)
  if ($text.Contains($Needle)) {
    throw "refresh_server.ps1 should not contain text: $Needle"
  }
}

Assert-Contains "function Ensure-AppDependencies"
Assert-Contains "npm ci --include=dev"
Assert-Contains "Ensure-AppDependencies"
Assert-Contains "function Publish-Web"
Assert-Contains "function Write-ReleaseManifest"
Assert-Contains "npm run build:web:release"
Assert-Contains "release.json"
Assert-Contains '"schemaVersion" = 1'
Assert-Contains 'System.Text.UTF8Encoding($false)'
Assert-Contains "[System.IO.File]::WriteAllText"
Assert-Contains "Pull-Latest"
Assert-Contains "Ensure-AcpDependencies"
Assert-Contains "git stash push -u -m"
Assert-Contains "wheelmaker deploy auto-stash before pull"
Assert-NotContains "skip git pull and continue"
Assert-NotContains "publish_desktop.ps1"
Assert-NotContains "publish-desktop.bat"

$pullIndex = $text.IndexOf("Pull-Latest", [StringComparison]::Ordinal)
$appDepsIndex = $text.LastIndexOf("Ensure-AppDependencies", [StringComparison]::Ordinal)
$buildIndex = $text.IndexOf("Build-Binary -Out", [StringComparison]::Ordinal)
$publishIndex = $text.LastIndexOf("Publish-Web", [StringComparison]::Ordinal)
$manifestIndex = $text.LastIndexOf("Write-ReleaseManifest", [StringComparison]::Ordinal)
$restartIndex = $text.LastIndexOf("Restart-Services", [StringComparison]::Ordinal)

if ($pullIndex -lt 0 -or $appDepsIndex -lt 0 -or $buildIndex -lt 0 -or $publishIndex -lt 0 -or $manifestIndex -lt 0 -or $restartIndex -lt 0) {
  throw "refresh_server.ps1 missing expected call order markers"
}
if ($pullIndex -ge $appDepsIndex) {
  throw "refresh_server.ps1 should sync app dependencies after Pull-Latest"
}
if ($appDepsIndex -ge $buildIndex) {
  throw "refresh_server.ps1 should sync app dependencies before builds"
}
if ($buildIndex -ge $publishIndex) {
  throw "refresh_server.ps1 should publish web after building binaries"
}
if ($publishIndex -ge $manifestIndex) {
  throw "refresh_server.ps1 should write release manifest after web publish"
}
if ($manifestIndex -ge $restartIndex) {
  throw "refresh_server.ps1 should write release manifest before restart"
}

Write-Host "refresh_server.ps1 app dependency checks passed"
