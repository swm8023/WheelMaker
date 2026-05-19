$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\refresh_server.sh"

if (-not (Test-Path $scriptPath)) {
  throw "refresh_server.sh is missing"
}

$text = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8

function Assert-Contains {
  param([string]$Needle)
  if (-not $text.Contains($Needle)) {
    throw "refresh_server.sh does not contain expected text: $Needle"
  }
}

function Assert-NotContains {
  param([string]$Needle)
  if ($text.Contains($Needle)) {
    throw "refresh_server.sh should not contain text: $Needle"
  }
}

Assert-Contains "com.wheelmaker.hub"
Assert-Contains "com.wheelmaker.monitor"
Assert-Contains "com.wheelmaker.updater"
Assert-Contains "launchctl bootstrap"
Assert-Contains "launchctl bootout"
Assert-Contains "launchctl kickstart"
Assert-Contains "systemctl --user"
Assert-Contains "TARGET_GOOS"
Assert-Contains 'GOOS="$TARGET_GOOS"'
Assert-Contains "npm run build:web:release"
Assert-Contains "npm ci --include=dev"
Assert-Contains "--skip-web-publish"
Assert-Contains "Node.js 22.11.0+"
Assert-Contains "ensure_acp_dependencies"
Assert-Contains "@zed-industries/codex-acp"
Assert-Contains "@agentclientprotocol/claude-agent-acp"
Assert-Contains "@zed-industries/claude-agent-acp"
Assert-Contains "npm uninstall -g"
Assert-Contains "npm install -g"
Assert-Contains "~/Library/LaunchAgents"
Assert-Contains "~/.config/systemd/user"
Assert-Contains "git stash push -u -m"
Assert-Contains "wheelmaker deploy auto-stash before pull"
Assert-NotContains "skip git pull and continue"
Assert-NotContains "require_command npx"

Write-Host "refresh_server.sh source checks passed"
