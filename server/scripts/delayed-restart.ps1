param(
  [switch]$Worker
)

$ErrorActionPreference = "Continue"
$baseDir = Join-Path -Path $HOME -ChildPath ".wheelmaker"
$logPath = Join-Path -Path $baseDir -ChildPath "delayed-restart.log"
$stdout = Join-Path -Path $baseDir -ChildPath "wheelmaker-stdout.log"
$stderr = Join-Path -Path $baseDir -ChildPath "wheelmaker-stderr.log"

if (-not $Worker) {
  Start-Process -FilePath "powershell" -ArgumentList @(
    "-NoProfile",
    "-ExecutionPolicy", "Bypass",
    "-File", $PSCommandPath,
    "-Worker"
  ) -WindowStyle Hidden | Out-Null
  Add-Content -Path $logPath -Value ("[{0}] scheduled restart worker" -f (Get-Date -Format o))
  return
}

Add-Content -Path $logPath -Value ("[{0}] restart job begin" -f (Get-Date -Format o))
Start-Sleep -Seconds 30

$running = Get-Process wheelmaker -ErrorAction SilentlyContinue
if ($running) {
  Add-Content -Path $logPath -Value ("[{0}] stopping pids: {1}" -f (Get-Date -Format o), (($running | ForEach-Object { $_.Id }) -join ","))
  $running | Stop-Process -ErrorAction SilentlyContinue
  Start-Sleep -Seconds 2
  $left = Get-Process wheelmaker -ErrorAction SilentlyContinue
  if ($left) {
    Add-Content -Path $logPath -Value ("[{0}] force stopping pids: {1}" -f (Get-Date -Format o), (($left | ForEach-Object { $_.Id }) -join ","))
    $left | Stop-Process -Force -ErrorAction SilentlyContinue
  }
} else {
  Add-Content -Path $logPath -Value ("[{0}] no running wheelmaker found" -f (Get-Date -Format o))
}

$p = Start-Process -FilePath "go" -ArgumentList "run ./cmd/wheelmaker/" -WorkingDirectory "D:\Code\WheelMaker\server" -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
Add-Content -Path $logPath -Value ("[{0}] started go pid={1}" -f (Get-Date -Format o), $p.Id)

Start-Sleep -Seconds 4
$wm = Get-Process wheelmaker -ErrorAction SilentlyContinue
Add-Content -Path $logPath -Value ("[{0}] wheelmaker count after restart={1}" -f (Get-Date -Format o), @($wm).Count)
