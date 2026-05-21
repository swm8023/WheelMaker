$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\refresh_server_linux.sh"

if (-not (Test-Path $scriptPath)) {
  throw "refresh_server_linux.sh is missing"
}

$text = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8

function Assert-Contains {
  param([string]$Needle)
  if (-not $text.Contains($Needle)) {
    throw "refresh_server_linux.sh does not contain expected text: $Needle"
  }
}

function Assert-NotContains {
  param([string]$Needle)
  if ($text.Contains($Needle)) {
    throw "refresh_server_linux.sh should not contain text: $Needle"
  }
}

Assert-Contains "refresh_server_linux.sh is Linux-only"
Assert-Contains "systemctl --user"
Assert-Contains "wheelmaker-hub.service"
Assert-Contains "wheelmaker-monitor.service"
Assert-Contains "wheelmaker-updater.service"
Assert-Contains "~/.config/systemd/user"
Assert-Contains "~/.wheelmaker/systemd.env"
Assert-Contains "GOOS=linux"
Assert-Contains "npm run build:web:release"
Assert-Contains "--skip-web-publish"
Assert-Contains "daemon-reload"
Assert-Contains "EnvironmentFile="
Assert-Contains 'tmp="$(mktemp "${dest_dir}/.${dest_name}.tmp.XXXXXX")"'
Assert-Contains 'cp "$source" "$tmp"'
Assert-Contains 'mv -f "$tmp" "$dest"'
Assert-NotContains 'cp "$source" "$dest"'

Write-Host "refresh_server_linux.sh source checks passed"
