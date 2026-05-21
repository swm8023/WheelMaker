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
Assert-Contains "write_release_manifest"
Assert-Contains "release.json"
Assert-Contains '"schemaVersion": 1'
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
Assert-Contains 'WorkingDirectory=$REPO_ROOT'
Assert-Contains 'tmp="$(mktemp "${dest_dir}/.${dest_name}.tmp.XXXXXX")"'
Assert-Contains 'cp "$source" "$tmp"'
Assert-Contains 'mv -f "$tmp" "$dest"'
Assert-Contains "git stash push -u -m"
Assert-Contains "wheelmaker deploy auto-stash before pull"
Assert-NotContains 'cp "$source" "$dest"'
Assert-NotContains "skip git pull and continue"
Assert-NotContains "require_command npx"
Assert-NotContains 'WorkingDirectory=$(systemd_quote "$REPO_ROOT")'

$publishIndex = $text.IndexOf("publish_web", [StringComparison]::Ordinal)
$manifestIndex = $text.LastIndexOf("write_release_manifest", [StringComparison]::Ordinal)
$startIndex = $text.LastIndexOf("start_agents", [StringComparison]::Ordinal)
if ($publishIndex -lt 0 -or $manifestIndex -lt 0 -or $startIndex -lt 0) {
  throw "refresh_server.sh missing expected release call order markers"
}
if ($publishIndex -ge $manifestIndex) {
  throw "refresh_server.sh should write release manifest after web publish"
}
if ($manifestIndex -ge $startIndex) {
  throw "refresh_server.sh should write release manifest before start"
}

Write-Host "refresh_server.sh source checks passed"
