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
Assert-Contains "npm ci --include=dev"
Assert-Contains "ensure_acp_dependencies"
Assert-Contains "ensure_app_dependencies"
Assert-Contains "write_release_manifest"
Assert-Contains "release.json"
Assert-Contains '"schemaVersion": 1'
Assert-Contains "--skip-web-publish"
Assert-Contains "daemon-reload"
Assert-Contains "EnvironmentFile="
Assert-Contains 'tmp="$(mktemp "${dest_dir}/.${dest_name}.tmp.XXXXXX")"'
Assert-Contains 'cp "$source" "$tmp"'
Assert-Contains 'mv -f "$tmp" "$dest"'
Assert-Contains "git stash push -u -m"
Assert-Contains "wheelmaker deploy auto-stash before pull"
Assert-NotContains 'cp "$source" "$dest"'
Assert-NotContains "skip git pull and continue"
Assert-NotContains "require_command npx"

$pullIndex = $text.IndexOf("pull_latest", [StringComparison]::Ordinal)
$appDepsIndex = $text.LastIndexOf("ensure_app_dependencies", [StringComparison]::Ordinal)
$buildIndex = $text.IndexOf("build_binary `"wheelmaker`"", [StringComparison]::Ordinal)
$publishIndex = $text.LastIndexOf("publish_web", [StringComparison]::Ordinal)
$manifestIndex = $text.LastIndexOf("write_release_manifest", [StringComparison]::Ordinal)
$startIndex = $text.LastIndexOf("start_services", [StringComparison]::Ordinal)

if ($pullIndex -lt 0 -or $appDepsIndex -lt 0 -or $buildIndex -lt 0 -or $publishIndex -lt 0 -or $manifestIndex -lt 0 -or $startIndex -lt 0) {
  throw "refresh_server_linux.sh missing expected update publish call order markers"
}
if ($pullIndex -ge $appDepsIndex) {
  throw "refresh_server_linux.sh should sync app dependencies after pull_latest"
}
if ($appDepsIndex -ge $buildIndex) {
  throw "refresh_server_linux.sh should sync app dependencies before builds"
}
if ($buildIndex -ge $publishIndex) {
  throw "refresh_server_linux.sh should publish web after building binaries"
}
if ($publishIndex -ge $manifestIndex) {
  throw "refresh_server_linux.sh should write release manifest after web publish"
}
if ($manifestIndex -ge $startIndex) {
  throw "refresh_server_linux.sh should write release manifest before start"
}

Write-Host "refresh_server_linux.sh source checks passed"
