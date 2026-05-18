#!/usr/bin/env bash
set -euo pipefail

ACTION="refresh"
REPO_ROOT=""
INSTALL_DIR="${HOME}/.wheelmaker/bin"
UPDATER_DAILY_TIME="03:00"
SKIP_UPDATE=0
SKIP_GIT_PULL=0
SKIP_DEPS=0
SKIP_BUILD=0
SKIP_INSTALL=0
SKIP_UPDATER_INSTALL=0
SKIP_RESTART=0
SKIP_SERVICE_CONFIG=0
SKIP_WEB_PUBLISH=0

HUB_LABEL="com.wheelmaker.hub"
MONITOR_LABEL="com.wheelmaker.monitor"
UPDATER_LABEL="com.wheelmaker.updater"

usage() {
  cat <<'USAGE'
Usage: scripts/refresh_server.sh [action] [options]

Actions:
  refresh     update, build, install, configure LaunchAgents, restart (default)
  start       start LaunchAgents
  stop        stop LaunchAgents
  restart     restart LaunchAgents
  status      print LaunchAgent status
  uninstall   unload and remove LaunchAgent plists; keep ~/.wheelmaker data

Options:
  --repo-root PATH
  --install-dir PATH
  --time HH:mm
  --skip-update
  --skip-git-pull
  --skip-deps
  --skip-build
  --skip-install
  --skip-updater-install
  --skip-restart
  --skip-service-config
  --skip-web-publish

LaunchAgent plists are written to ~/Library/LaunchAgents.
USAGE
}

die() {
  echo "ERROR: $*" >&2
  exit 1
}

step() {
  echo "==> $*"
}

warn() {
  echo "WARN: $*" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    refresh|start|stop|restart|status|uninstall)
      ACTION="$1"
      shift
      ;;
    --repo-root)
      [[ $# -ge 2 ]] || die "--repo-root requires a value"
      REPO_ROOT="$2"
      shift 2
      ;;
    --install-dir)
      [[ $# -ge 2 ]] || die "--install-dir requires a value"
      INSTALL_DIR="$2"
      shift 2
      ;;
    --time)
      [[ $# -ge 2 ]] || die "--time requires a value"
      UPDATER_DAILY_TIME="$2"
      shift 2
      ;;
    --skip-update)
      SKIP_UPDATE=1
      SKIP_GIT_PULL=1
      SKIP_DEPS=1
      shift
      ;;
    --skip-git-pull)
      SKIP_GIT_PULL=1
      shift
      ;;
    --skip-deps)
      SKIP_DEPS=1
      shift
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    --skip-install)
      SKIP_INSTALL=1
      shift
      ;;
    --skip-updater-install)
      SKIP_UPDATER_INSTALL=1
      shift
      ;;
    --skip-restart)
      SKIP_RESTART=1
      shift
      ;;
    --skip-service-config)
      SKIP_SERVICE_CONFIG=1
      shift
      ;;
    --skip-web-publish)
      SKIP_WEB_PUBLISH=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

[[ "$(uname -s)" == "Darwin" ]] || die "refresh_server.sh is macOS-only"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "$REPO_ROOT" ]]; then
  REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
else
  REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
fi

SERVER_ROOT="${REPO_ROOT}/server"
APP_ROOT="${REPO_ROOT}/app"
WHEELMAKER_HOME="${HOME}/.wheelmaker"
BUILD_CACHE_ROOT="${WHEELMAKER_HOME}/cache"
GO_BUILD_CACHE="${BUILD_CACHE_ROOT}/go-build"
GO_ARCH="$(go env GOARCH 2>/dev/null || uname -m)"
BUILD_OUTPUT_ROOT="${WHEELMAKER_HOME}/build/darwin_${GO_ARCH}"
CONFIG_PATH="${WHEELMAKER_HOME}/config.json"
CONFIG_EXAMPLE_PATH="${SERVER_ROOT}/config.example.json"
PLIST_DIR="${HOME}/Library/LaunchAgents"
LOG_DIR="${WHEELMAKER_HOME}/log"
USER_ID="$(id -u)"
LAUNCH_DOMAIN="gui/${USER_ID}"

HUB_BINARY="${INSTALL_DIR}/wheelmaker"
MONITOR_BINARY="${INSTALL_DIR}/wheelmaker-monitor"
UPDATER_BINARY="${INSTALL_DIR}/wheelmaker-updater"

require_command() {
  local name="$1"
  local hint="$2"
  if command -v "$name" >/dev/null 2>&1; then
    return 0
  fi
  die "required command not found in PATH: ${name}. ${hint}"
}

check_dependencies() {
  if [[ "$SKIP_DEPS" -eq 1 ]]; then
    step "skip dependency checks"
    return
  fi
  require_command bash "Install Bash."
  require_command git "Install Xcode Command Line Tools or Git."
  require_command go "Install Go 1.26+."
  require_command node "Install Node.js 22.11.0+."
  require_command npm "Install Node.js 22.11.0+ with npm."
  require_command npx "Install npm/npx."
  require_command launchctl "launchctl should be available on macOS."
  node -e "const [maj,min]=process.versions.node.split('.').map(Number); process.exit(maj > 22 || (maj === 22 && min >= 11) ? 0 : 1)" \
    || die "Node.js 22.11.0+ is required"
}

xml_escape() {
  local value="$1"
  value="${value//&/&amp;}"
  value="${value//</&lt;}"
  value="${value//>/&gt;}"
  value="${value//\"/&quot;}"
  printf '%s' "$value"
}

plist_path() {
  local label="$1"
  printf '%s/%s.plist' "$PLIST_DIR" "$label"
}

launch_target() {
  local label="$1"
  printf '%s/%s' "$LAUNCH_DOMAIN" "$label"
}

managed_labels() {
  printf '%s\n' "$HUB_LABEL"
  if [[ "$SKIP_UPDATER_INSTALL" -eq 0 ]]; then
    printf '%s\n' "$UPDATER_LABEL"
  fi
}

all_labels() {
  printf '%s\n' "$HUB_LABEL" "$MONITOR_LABEL"
  if [[ "$SKIP_UPDATER_INSTALL" -eq 0 ]]; then
    printf '%s\n' "$UPDATER_LABEL"
  fi
}

pull_latest() {
  if [[ "$SKIP_GIT_PULL" -eq 1 ]]; then
    step "skip git pull"
    return
  fi
  require_command git "Install Git."
  pushd "$REPO_ROOT" >/dev/null
  local status
  status="$(git status --porcelain)"
  if [[ -n "$status" ]]; then
    warn "git worktree has local changes; skip git pull and continue"
    popd >/dev/null
    return
  fi
  local branch
  branch="$(git branch --show-current)"
  [[ -n "$branch" ]] || die "repository is in detached HEAD state; cannot pull latest automatically"
  step "git pull --ff-only origin ${branch}"
  git pull --ff-only origin "$branch"
  popd >/dev/null
}

ensure_config() {
  if [[ -f "$CONFIG_PATH" ]]; then
    step "config already exists: ${CONFIG_PATH}"
    return 1
  fi
  [[ -f "$CONFIG_EXAMPLE_PATH" ]] || die "config example missing: ${CONFIG_EXAMPLE_PATH}"
  step "create config from example: ${CONFIG_PATH}"
  mkdir -p "$WHEELMAKER_HOME"
  cp "$CONFIG_EXAMPLE_PATH" "$CONFIG_PATH"
  warn "config was created from example: ${CONFIG_PATH}"
  warn "edit config.json before the first restart, then rerun scripts/refresh_server.sh or run scripts/refresh_server.sh start"
  return 0
}

build_binary() {
  local label="$1"
  local pkg="$2"
  local out="$3"
  if [[ "$SKIP_BUILD" -eq 1 ]]; then
    step "skip build: ${label}"
    return
  fi
  step "build ${label}: ${out}"
  mkdir -p "$(dirname "$out")" "$GO_BUILD_CACHE"
  pushd "$SERVER_ROOT" >/dev/null
  GOCACHE="$GO_BUILD_CACHE" GOOS=darwin GOARCH="$GO_ARCH" go build -o "$out" "$pkg"
  popd >/dev/null
}

install_binary() {
  local source="$1"
  local dest="$2"
  if [[ "$SKIP_INSTALL" -eq 1 ]]; then
    step "skip install: $(basename "$dest")"
    return
  fi
  [[ -f "$source" ]] || die "source binary not found: ${source}"
  step "install binary: ${source} -> ${dest}"
  mkdir -p "$(dirname "$dest")"
  cp "$source" "$dest"
  chmod 0755 "$dest"
}

publish_web() {
  if [[ "$SKIP_WEB_PUBLISH" -eq 1 ]]; then
    step "skip web publish"
    return
  fi
  step "publish Web UI"
  pushd "$APP_ROOT" >/dev/null
  npm run build:web:release
  popd >/dev/null
}

write_hub_plist() {
  local plist
  plist="$(plist_path "$HUB_LABEL")"
  cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>${HUB_LABEL}</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>WorkingDirectory</key><string>$(xml_escape "$REPO_ROOT")</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>$(xml_escape "$PATH")</string>
    <key>HOME</key><string>$(xml_escape "$HOME")</string>
  </dict>
  <key>ProgramArguments</key>
  <array>
    <string>$(xml_escape "$HUB_BINARY")</string>
    <string>-d</string>
  </array>
  <key>StandardOutPath</key><string>$(xml_escape "$LOG_DIR/${HUB_LABEL}.out.log")</string>
  <key>StandardErrorPath</key><string>$(xml_escape "$LOG_DIR/${HUB_LABEL}.err.log")</string>
</dict>
</plist>
EOF
}

write_monitor_plist() {
  local plist
  plist="$(plist_path "$MONITOR_LABEL")"
  cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>${MONITOR_LABEL}</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>WorkingDirectory</key><string>$(xml_escape "$REPO_ROOT")</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>$(xml_escape "$PATH")</string>
    <key>HOME</key><string>$(xml_escape "$HOME")</string>
  </dict>
  <key>ProgramArguments</key>
  <array>
    <string>$(xml_escape "$MONITOR_BINARY")</string>
  </array>
  <key>StandardOutPath</key><string>$(xml_escape "$LOG_DIR/${MONITOR_LABEL}.out.log")</string>
  <key>StandardErrorPath</key><string>$(xml_escape "$LOG_DIR/${MONITOR_LABEL}.err.log")</string>
</dict>
</plist>
EOF
}

write_updater_plist() {
  local plist
  plist="$(plist_path "$UPDATER_LABEL")"
  cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>${UPDATER_LABEL}</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>WorkingDirectory</key><string>$(xml_escape "$REPO_ROOT")</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>$(xml_escape "$PATH")</string>
    <key>HOME</key><string>$(xml_escape "$HOME")</string>
  </dict>
  <key>ProgramArguments</key>
  <array>
    <string>$(xml_escape "$UPDATER_BINARY")</string>
    <string>--repo</string>
    <string>$(xml_escape "$REPO_ROOT")</string>
    <string>--install-dir</string>
    <string>$(xml_escape "$INSTALL_DIR")</string>
    <string>--time</string>
    <string>$(xml_escape "$UPDATER_DAILY_TIME")</string>
  </array>
  <key>StandardOutPath</key><string>$(xml_escape "$LOG_DIR/${UPDATER_LABEL}.out.log")</string>
  <key>StandardErrorPath</key><string>$(xml_escape "$LOG_DIR/${UPDATER_LABEL}.err.log")</string>
</dict>
</plist>
EOF
}

configure_launch_agents() {
  if [[ "$SKIP_SERVICE_CONFIG" -eq 1 ]]; then
    step "skip LaunchAgent configuration"
    return
  fi
  step "write LaunchAgent plists"
  mkdir -p "$PLIST_DIR" "$LOG_DIR"
  write_hub_plist
  write_monitor_plist
  if [[ "$SKIP_UPDATER_INSTALL" -eq 0 ]]; then
    write_updater_plist
  else
    step "skip updater LaunchAgent configuration"
  fi
}

stop_label() {
  local label="$1"
  step "launchctl bootout ${label}"
  launchctl bootout "$(launch_target "$label")" >/dev/null 2>&1 || true
}

start_label() {
  local label="$1"
  local plist
  plist="$(plist_path "$label")"
  [[ -f "$plist" ]] || die "LaunchAgent plist not found: ${plist}"
  step "launchctl bootstrap ${label}"
  launchctl bootout "$(launch_target "$label")" >/dev/null 2>&1 || true
  launchctl bootstrap "$LAUNCH_DOMAIN" "$plist"
  step "launchctl kickstart ${label}"
  launchctl kickstart -k "$(launch_target "$label")"
}

stop_agents() {
  while IFS= read -r label; do
    [[ -n "$label" ]] || continue
    stop_label "$label"
  done < <(all_labels)
}

start_agents() {
  while IFS= read -r label; do
    [[ -n "$label" ]] || continue
    start_label "$label"
  done < <(all_labels)
}

restart_agents() {
  stop_agents
  start_agents
}

status_agents() {
  while IFS= read -r label; do
    [[ -n "$label" ]] || continue
    if launchctl print "$(launch_target "$label")" >/dev/null 2>&1; then
      echo "${label}: loaded"
    elif [[ -f "$(plist_path "$label")" ]]; then
      echo "${label}: installed, not loaded"
    else
      echo "${label}: not installed"
    fi
  done < <(all_labels)
}

uninstall_agents() {
  local failed=0
  while IFS= read -r label; do
    [[ -n "$label" ]] || continue
    stop_label "$label" || failed=1
    local plist
    plist="$(plist_path "$label")"
    if [[ -f "$plist" ]]; then
      step "remove ${plist}"
      rm -f "$plist" || failed=1
    fi
  done < <(all_labels)
  [[ "$failed" -eq 0 ]] || die "one or more LaunchAgent uninstall operations failed"
}

refresh() {
  [[ -d "$SERVER_ROOT" ]] || die "server directory not found: ${SERVER_ROOT}"
  check_dependencies
  pull_latest

  local output_hub="${BUILD_OUTPUT_ROOT}/wheelmaker"
  local output_monitor="${BUILD_OUTPUT_ROOT}/wheelmaker-monitor"
  local output_updater="${BUILD_OUTPUT_ROOT}/wheelmaker-updater"

  build_binary "wheelmaker" "./cmd/wheelmaker/" "$output_hub"
  build_binary "wheelmaker-monitor" "./cmd/wheelmaker-monitor/" "$output_monitor"
  if [[ "$SKIP_UPDATER_INSTALL" -eq 0 ]]; then
    build_binary "wheelmaker-updater" "./cmd/wheelmaker-updater/" "$output_updater"
  else
    step "skip build: wheelmaker-updater"
  fi

  if [[ "$SKIP_INSTALL" -eq 0 && "$SKIP_RESTART" -eq 0 ]]; then
    if [[ "$SKIP_UPDATER_INSTALL" -eq 0 ]]; then
      stop_label "$UPDATER_LABEL"
    fi
    stop_label "$HUB_LABEL"
    stop_label "$MONITOR_LABEL"
  fi

  install_binary "$output_hub" "$HUB_BINARY"
  install_binary "$output_monitor" "$MONITOR_BINARY"
  if [[ "$SKIP_UPDATER_INSTALL" -eq 0 ]]; then
    install_binary "$output_updater" "$UPDATER_BINARY"
  else
    step "skip install: wheelmaker-updater"
  fi

  local config_created=1
  if ensure_config; then
    config_created=0
  fi

  configure_launch_agents
  publish_web

  if [[ "$config_created" -eq 0 && "$SKIP_RESTART" -eq 0 ]]; then
    warn "config was created from example at ${CONFIG_PATH}; edit it first, then rerun scripts/refresh_server.sh"
    step "skip restart because config is newly created"
    return
  fi

  if [[ "$SKIP_RESTART" -eq 1 ]]; then
    step "skip restart"
    return
  fi
  start_agents
  step "refresh complete"
}

case "$ACTION" in
  refresh)
    refresh
    ;;
  start)
    start_agents
    ;;
  stop)
    stop_agents
    ;;
  restart)
    restart_agents
    ;;
  status)
    status_agents
    ;;
  uninstall)
    uninstall_agents
    ;;
  *)
    die "unsupported action: ${ACTION}"
    ;;
esac
