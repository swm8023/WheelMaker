#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

cat <<'BANNER'
============================================
  WheelMaker All-in-One Deploy
============================================

  update + build + stop + deploy + start + publish web

============================================
BANNER

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "[FAILED] deploy.sh is macOS-only. Use deploy.bat on Windows." >&2
  exit 1
fi

if [[ ! -x "app/node_modules/.bin/webpack" ]]; then
  cat >&2 <<'MESSAGE'
[FAILED] app web dependencies are not installed.

Run this once, then rerun deploy.sh:
  cd app
  npm ci --include=dev
  cd ..
MESSAGE
  exit 1
fi

if [[ ! -x "scripts/refresh_server.sh" ]]; then
  chmod +x "scripts/refresh_server.sh"
fi

bash "scripts/refresh_server.sh" "$@"

echo
echo "[OK] deploy complete"
