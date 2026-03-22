#!/usr/bin/env bash
# install-tools.sh - Install ACP tools via standard npm global packages.
set -euo pipefail

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found. Install Node.js first: https://nodejs.org/"
  exit 1
fi

echo "Installing @zed-industries/codex-acp..."
npm install -g @zed-industries/codex-acp

echo "Installing @zed-industries/claude-agent-acp..."
npm install -g @zed-industries/claude-agent-acp

echo "Verifying CLI availability..."
if ! command -v codex-acp >/dev/null 2>&1; then
  echo "codex-acp not found in PATH after npm install."
  exit 1
fi
if ! command -v claude-agent-acp >/dev/null 2>&1; then
  echo "claude-agent-acp not found in PATH after npm install."
  exit 1
fi

echo "Done."
echo "codex-acp: $(command -v codex-acp)"
echo "claude-agent-acp: $(command -v claude-agent-acp)"
