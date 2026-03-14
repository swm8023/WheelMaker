#!/usr/bin/env bash
# install-tools.sh — 下载第三方工具二进制到 bin/{platform}/
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)
DEST="$REPO_ROOT/bin/${GOOS}_${GOARCH}"

echo "Platform: ${GOOS}_${GOARCH}"
echo "Destination: $DEST"
mkdir -p "$DEST"

install_codex_acp() {
    echo "Installing codex-acp..."

    # 尝试从 GitHub Releases 下载
    LATEST=$(curl -s https://api.github.com/repos/zed-industries/codex-acp/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -n "$LATEST" ]; then
        BINARY_NAME="codex-acp-${GOOS}-${GOARCH}"
        URL="https://github.com/zed-industries/codex-acp/releases/download/${LATEST}/${BINARY_NAME}"
        if curl -fsSL -o "$DEST/codex-acp" "$URL"; then
            chmod +x "$DEST/codex-acp"
            echo "✓ codex-acp installed from GitHub Releases ($LATEST)"
            return
        fi
    fi

    # 回退：通过 npx 安装
    if command -v npx &>/dev/null; then
        echo "Falling back to npx..."
        npx --yes @zed-industries/codex-acp --version >/dev/null 2>&1 || true
        NPXBIN=$(npx --yes which codex-acp 2>/dev/null || true)
        if [ -n "$NPXBIN" ] && [ -f "$NPXBIN" ]; then
            cp "$NPXBIN" "$DEST/codex-acp"
            chmod +x "$DEST/codex-acp"
            echo "✓ codex-acp installed via npx"
            return
        fi
    fi

    echo "⚠ Could not install codex-acp automatically."
    echo "  Please download manually from https://github.com/zed-industries/codex-acp/releases"
    echo "  and place it at: $DEST/codex-acp"
}

install_codex_acp
echo "Done."
