param(
  [string]$OutputPath = "",
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
  "-SkipDeps",
  "-SkipInstall",
  "-SkipRestart"
)
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
  $args += @("-OutputPath", $OutputPath)
}
if ($WhatIf) {
  $args += "-WhatIf"
}

& powershell @args

if ($LASTEXITCODE -ne 0) {
  throw ("refresh_server.ps1 build stage failed (exit={0})" -f $LASTEXITCODE)
}
