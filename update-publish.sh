#!/usr/bin/env bash
set -euo pipefail

cat <<'BANNER'
============================================
  WheelMaker Update Publish
============================================

  request updater service full update + publish
  supports macOS and Linux

============================================
BANNER

case "$(uname -s)" in
  Darwin|Linux)
    ;;
  MINGW*|MSYS*|CYGWIN*)
    echo "[FAILED] update-publish.sh supports macOS and Linux. Use update-publish.bat on Windows." >&2
    exit 1
    ;;
  *)
    echo "[FAILED] update-publish.sh supports macOS and Linux only." >&2
    exit 1
    ;;
esac

signal_path="${WHEELMAKER_UPDATE_SIGNAL:-${HOME}/.wheelmaker/update-now.signal}"
signal_dir="$(dirname "$signal_path")"

mkdir -p "$signal_dir"
{
  printf '%s\n' "full-update"
  date -u +"%Y-%m-%dT%H:%M:%SZ"
} > "$signal_path"

echo "[OK] updater trigger accepted: $signal_path"
echo
echo "[OK] update publish requested. Check ~/.wheelmaker/log/updater.log for progress."
