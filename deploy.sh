#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
repo_root="$(pwd)"
bootstrap_dir="${HOME}/.wheelmaker/build/bootstrap"
deploy_cli="${bootstrap_dir}/wheelmaker-deploy"

cat <<'BANNER'
============================================
  WheelMaker All-in-One Deploy
============================================

  wheelmaker-deploy deploy: update + build + install + configure + publish web
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
    echo "[FAILED] deploy.sh supports macOS and Linux only. Use deploy.bat on Windows." >&2
    exit 1
    ;;
esac

echo "[INFO] Building bootstrap wheelmaker-deploy..."
command -v go >/dev/null 2>&1 || {
  echo "[FAILED] Go is required to build wheelmaker-deploy" >&2
  exit 1
}
mkdir -p "$bootstrap_dir"
(cd "${repo_root}/server" && go build -o "$deploy_cli" ./cmd/wheelmaker-deploy)
chmod +x "$deploy_cli"

"$deploy_cli" deploy --repo "$repo_root" "$@"

echo
echo "[OK] deploy complete"
