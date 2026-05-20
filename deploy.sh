#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

cat <<'BANNER'
============================================
  WheelMaker All-in-One Deploy
============================================

  update + build + stop + deploy + start + publish web
  supports macOS and Linux

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

if [[ ! -x "$refresh_script" ]]; then
  chmod +x "$refresh_script"
fi

bash "$refresh_script" "$@"

echo
echo "[OK] deploy complete"
