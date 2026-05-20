$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\signal_update_now.ps1"

if (-not (Test-Path $scriptPath)) {
  throw "signal_update_now.ps1 is missing"
}

$tempDir = Join-Path $env:TEMP ("wm-signal-test-{0}" -f ([Guid]::NewGuid().ToString("N")))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
try {
  $skipSignal = Join-Path $tempDir "skip.signal"
  powershell -NoProfile -ExecutionPolicy Bypass -File $scriptPath -Worker -DelaySeconds 0 -SignalPath $skipSignal -SkipWebPublish
  $skipRaw = Get-Content -LiteralPath $skipSignal -Raw -Encoding UTF8
  if (-not $skipRaw.Contains("skip-web-publish")) {
    throw "SkipWebPublish signal should contain skip-web-publish marker, got: $skipRaw"
  }

  $plainSignal = Join-Path $tempDir "plain.signal"
  powershell -NoProfile -ExecutionPolicy Bypass -File $scriptPath -Worker -DelaySeconds 0 -SignalPath $plainSignal
  $plainRaw = Get-Content -LiteralPath $plainSignal -Raw -Encoding UTF8
  if ($plainRaw.Contains("skip-web-publish")) {
    throw "plain signal should not contain skip-web-publish marker, got: $plainRaw"
  }
} finally {
  Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "signal_update_now.ps1 checks passed"
