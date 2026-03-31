param(
  [string]$InstallDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\bin"),
  [string]$SourceExe = "",
  [switch]$SkipDeps,
  [switch]$WhatIf
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$refreshScript = Join-Path $PSScriptRoot "refresh_server.ps1"
if (-not (Test-Path $refreshScript)) {
  throw ("refresh script not found: {0}" -f $refreshScript)
}

$args = @(
  "-NoProfile",
  "-ExecutionPolicy", "Bypass",
  "-File", $refreshScript,
  "-SkipGitPull",
  "-SkipBuild",
  "-SkipRestart",
  "-InstallDir", $InstallDir
)
if (-not [string]::IsNullOrWhiteSpace($SourceExe)) {
  $args += @("-SourceExe", $SourceExe)
}
if ($SkipDeps) {
  $args += "-SkipDeps"
}
if ($WhatIf) {
  $args += "-WhatIf"
}

& powershell @args

if ($LASTEXITCODE -ne 0) {
  throw ("refresh_server.ps1 install stage failed (exit={0})" -f $LASTEXITCODE)
}
