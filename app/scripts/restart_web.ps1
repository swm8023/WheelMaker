param(
  [int]$Port = 8080
)

$connections = Get-NetTCPConnection -LocalPort $Port -ErrorAction SilentlyContinue
if ($connections) {
  $processIds = $connections | Select-Object -ExpandProperty OwningProcess -Unique
  foreach ($processId in $processIds) {
    try {
      Stop-Process -Id $processId -Force -ErrorAction Stop
      Write-Host "Stopped PID $processId on port $Port"
    } catch {
      Write-Host "Failed to stop PID ${processId}: $($_.Exception.Message)"
    }
  }
} else {
  Write-Host "No process is listening on port $Port"
}

Write-Host "Starting web dev server on port $Port..."
$appRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$logDir = Join-Path $HOME ".wheelmaker"
if (-not (Test-Path $logDir)) {
  New-Item -ItemType Directory -Path $logDir -Force | Out-Null
}
$stdoutLog = Join-Path $logDir "web-dev.log"
$stderrLog = Join-Path $logDir "web-dev.err.log"

$proc = Start-Process -FilePath "cmd.exe" `
  -WorkingDirectory $appRoot `
  -ArgumentList "/c", "npm run web -- --port $Port" `
  -RedirectStandardOutput $stdoutLog `
  -RedirectStandardError $stderrLog `
  -WindowStyle Hidden `
  -PassThru

Write-Host "Started web dev server PID $($proc.Id), waiting for port $Port..."

$ready = $false
for ($i = 0; $i -lt 20; $i++) {
  Start-Sleep -Milliseconds 500
  $listening = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
  if ($listening) {
    $ready = $true
    break
  }
}

if ($ready) {
  Write-Host "Web dev server is listening on port $Port"
  exit 0
}

Write-Host "Web dev server failed to listen on port $Port. Check logs:"
Write-Host "  $stdoutLog"
Write-Host "  $stderrLog"
exit 1
