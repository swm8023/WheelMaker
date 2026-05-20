param(
  [switch]$Worker,
  [int]$DelaySeconds = 30,
  [string]$SignalPath = (Join-Path -Path $HOME -ChildPath ".wheelmaker\update-now.signal"),
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

if (-not $Worker) {
  $workerArgs = @(
    "-NoProfile",
    "-ExecutionPolicy", "Bypass",
    "-File", $PSCommandPath,
    "-Worker",
    "-DelaySeconds", "$DelaySeconds",
    "-SignalPath", $SignalPath
  )
  if ($SkipWebPublish) {
    $workerArgs += "-SkipWebPublish"
  }

  Start-Process -FilePath "powershell" -ArgumentList $workerArgs -WindowStyle Hidden | Out-Null

  Write-Host ("==> local refresh signal accepted (delay={0}s, signal={1})" -f $DelaySeconds, $SignalPath) -ForegroundColor Green
  exit 0
}

if ($DelaySeconds -gt 0) {
  Start-Sleep -Seconds $DelaySeconds
}

Ensure-ParentDirectory -Path $SignalPath
$payload = @()
if ($SkipWebPublish) {
  $payload += "skip-web-publish"
}
$payload += (Get-Date -Format o)
Set-Content -Path $SignalPath -Value $payload -Encoding UTF8
