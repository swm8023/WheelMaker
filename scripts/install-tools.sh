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

install_claude_agent_acp() {
    echo "Installing claude-agent-acp..."

    # Try GitHub Releases first
    LATEST=$(curl -s https://api.github.com/repos/zed-industries/claude-agent-acp/releases/latest | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -n "$LATEST" ]; then
        BINARY_NAME="claude-agent-acp-${GOOS}-${GOARCH}"
        URL="https://github.com/zed-industries/claude-agent-acp/releases/download/${LATEST}/${BINARY_NAME}"
        if curl -fsSL -o "$DEST/claude-agent-acp" "$URL"; then
            chmod +x "$DEST/claude-agent-acp"
            echo "✓ claude-agent-acp installed from GitHub Releases ($LATEST)"
            return
        fi
    fi

    # Fallback: npm install
    if command -v npm &>/dev/null; then
        echo "Falling back to npm install -g @zed-industries/claude-agent-acp..."
        npm install -g @zed-industries/claude-agent-acp >/dev/null 2>&1 || true

        # Search candidate locations
        NPM_ROOT=$(npm root -g 2>/dev/null || true)
        CANDIDATES=()
        if [ -n "$NPM_ROOT" ]; then
            CANDIDATES+=(
                "$NPM_ROOT/@zed-industries/claude-agent-acp/claude-agent-acp"
                "$NPM_ROOT/@zed-industries/claude-agent-acp/node_modules/@zed-industries/claude-agent-acp-${GOOS}-${GOARCH/amd64/x64}/bin/claude-agent-acp"
            )
        fi

        for c in "${CANDIDATES[@]}"; do
            if [ -f "$c" ]; then
                cp "$c" "$DEST/claude-agent-acp"
                chmod +x "$DEST/claude-agent-acp"
                echo "✓ claude-agent-acp installed via npm (from $c)"
                return
            fi
        done
    fi

    echo "⚠ Could not install claude-agent-acp automatically."
    echo "  Please download manually from https://github.com/zed-industries/claude-agent-acp/releases"
    echo "  or run: npm install -g @zed-industries/claude-agent-acp"
    echo "  and place the binary at: $DEST/claude-agent-acp"
}

install_codex_acp
install_claude_agent_acp
echo "Done."
