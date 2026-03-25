param(
  [string]$OutputPath = "",
  [switch]$WhatIf
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$serverRoot = Join-Path $repoRoot "server"
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
  $OutputPath = Join-Path $serverRoot "bin\windows_amd64\wheelmaker.exe"
}

Write-Host ("==> build output: {0}" -f $OutputPath)
if ($WhatIf) {
  Write-Host ("[whatif] go build -o {0} ./cmd/wheelmaker/" -f $OutputPath)
  exit 0
}

New-Item -ItemType Directory -Path (Split-Path $OutputPath -Parent) -Force | Out-Null
Push-Location $serverRoot
try {
  & go build -o $OutputPath ./cmd/wheelmaker/
  if ($LASTEXITCODE -ne 0) {
    throw ("go build failed (exit={0})" -f $LASTEXITCODE)
  }
}
finally {
  Pop-Location
}

Write-Host "==> build done"
