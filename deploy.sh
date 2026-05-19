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
  Darwin|Linux)
    ;;
  MINGW*|MSYS*|CYGWIN*)
    echo "[FAILED] deploy.sh supports macOS and Linux. Use deploy.bat on Windows." >&2
    exit 1
    ;;
  *)
    echo "[FAILED] deploy.sh supports macOS and Linux only." >&2
    exit 1
    ;;
esac

if [[ ! -x "scripts/refresh_server.sh" ]]; then
  chmod +x "scripts/refresh_server.sh"
fi

bash "scripts/refresh_server.sh" "$@"

echo
echo "[OK] deploy complete"
