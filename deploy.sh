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

case "$(uname -s)" in
  Darwin)
    refresh_script="scripts/refresh_server.sh"
    ;;
  Linux)
    refresh_script="scripts/refresh_server_linux.sh"
    ;;
  *)
    echo "[FAILED] deploy.sh supports macOS and Linux. Use deploy.bat on Windows." >&2
    exit 1
    ;;
esac

if [[ ! -x "app/node_modules/.bin/webpack" ]]; then
  cat >&2 <<'MESSAGE'
[FAILED] app web dependencies are not installed.

Run this once, then rerun deploy.sh:
  (cd app && npm ci --include=dev)
MESSAGE
  exit 1
fi

if [[ ! -x "$refresh_script" ]]; then
  chmod +x "$refresh_script"
fi

bash "$refresh_script" "$@"

echo
echo "[OK] deploy complete"
