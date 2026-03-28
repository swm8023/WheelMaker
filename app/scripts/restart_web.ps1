param(
  [int]$Port = 8080
)

$connections = Get-NetTCPConnection -LocalPort $Port -ErrorAction SilentlyContinue
if ($connections) {
  $pids = $connections | Select-Object -ExpandProperty OwningProcess -Unique
  foreach ($pid in $pids) {
    try {
      Stop-Process -Id $pid -Force -ErrorAction Stop
      Write-Host "Stopped PID $pid on port $Port"
    } catch {
      Write-Host "Failed to stop PID ${pid}: $($_.Exception.Message)"
    }
  }
} else {
  Write-Host "No process is listening on port $Port"
}

Write-Host "Starting web dev server on port $Port..."
& npm run web -- --port $Port
