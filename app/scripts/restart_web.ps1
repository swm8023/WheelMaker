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
& npm run web -- --port $Port
