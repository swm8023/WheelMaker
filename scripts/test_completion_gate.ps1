$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$claudePath = Join-Path $repoRoot "CLAUDE.md"

if (-not (Test-Path $claudePath)) {
  throw "CLAUDE.md is missing"
}

$text = Get-Content -LiteralPath $claudePath -Raw -Encoding UTF8
$webPublish = 'cd app && npm run build:web:release'
$serverSignal = 'scripts/signal_update_now.ps1 -DelaySeconds 30 -SkipWebPublish'

$webIndex = $text.IndexOf($webPublish, [StringComparison]::Ordinal)
$signalIndex = $text.IndexOf($serverSignal, [StringComparison]::Ordinal)

if ($webIndex -lt 0) {
  throw "CLAUDE.md completion gate should include web publish command: $webPublish"
}
if ($signalIndex -lt 0) {
  throw "CLAUDE.md completion gate should include server signal command with -SkipWebPublish: $serverSignal"
}
if ($webIndex -ge $signalIndex) {
  throw "CLAUDE.md completion gate should publish Web before signaling server refresh"
}

Write-Host "completion gate order checks passed"
