# install-tools.ps1 - Install ACP tools via standard npm global packages.
param()
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    Write-Error "npm not found. Install Node.js first: https://nodejs.org/"
}

Write-Host "Installing @zed-industries/codex-acp..."
& npm install -g @zed-industries/codex-acp

Write-Host "Installing @zed-industries/claude-agent-acp..."
& npm install -g @zed-industries/claude-agent-acp

$codex = Get-Command codex-acp -ErrorAction SilentlyContinue
if (-not $codex) {
    Write-Error "codex-acp not found in PATH after npm install."
}

$claude = Get-Command claude-agent-acp -ErrorAction SilentlyContinue
if (-not $claude) {
    Write-Error "claude-agent-acp not found in PATH after npm install."
}

Write-Host "Done."
Write-Host "codex-acp: $($codex.Source)"
Write-Host "claude-agent-acp: $($claude.Source)"
