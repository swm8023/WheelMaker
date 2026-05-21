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

HUB_SERVICE="wheelmaker-hub.service"
MONITOR_SERVICE="wheelmaker-monitor.service"
UPDATER_SERVICE="wheelmaker-updater.service"

usage() {
  cat <<'USAGE'
Usage: scripts/refresh_server_linux.sh [action] [options]

Actions:
  refresh     update, build, install, configure systemd user services, restart (default)
  start       start systemd user services
  stop        stop systemd user services
  restart     restart systemd user services
  status      print systemd user service status
  uninstall   stop, disable, and remove systemd user unit files; keep ~/.wheelmaker data

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

systemd user unit files are written to ~/.config/systemd/user.
The systemd environment file is written to ~/.wheelmaker/systemd.env.
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

[[ "$(uname -s)" == "Linux" ]] || die "refresh_server_linux.sh is Linux-only"

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
BUILD_OUTPUT_ROOT="${WHEELMAKER_HOME}/build/linux_${GO_ARCH}"
CONFIG_PATH="${WHEELMAKER_HOME}/config.json"
CONFIG_EXAMPLE_PATH="${SERVER_ROOT}/config.example.json"
SYSTEMD_USER_DIR="${HOME}/.config/systemd/user"
SYSTEMD_ENV_FILE="${WHEELMAKER_HOME}/systemd.env"
LOG_DIR="${WHEELMAKER_HOME}/log"

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

check_systemd_user() {
  [[ -n "${XDG_RUNTIME_DIR:-}" ]] || die "XDG_RUNTIME_DIR is not set; run from a real user session or configure the systemd user manager."
  systemctl --user show-environment >/dev/null 2>&1 \
    || die "systemctl --user is not available for this session. Start a user session or configure the user manager."
}

check_dependencies() {
  if [[ "$SKIP_DEPS" -eq 1 ]]; then
    step "skip dependency checks"
    return
  fi
  require_command bash "Install Bash."
  require_command git "Install Git."
  require_command go "Install Go 1.26+."
  require_command node "Install Node.js 22.11.0+."
  require_command npm "Install Node.js 22.11.0+ with npm."
  require_command npx "Install npm/npx."
  require_command systemctl "Install systemd."
  check_systemd_user
  node -e "const [maj,min]=process.versions.node.split('.').map(Number); process.exit(maj > 22 || (maj === 22 && min >= 11) ? 0 : 1)" \
    || die "Node.js 22.11.0+ is required"
}

service_unit_path() {
  local service="$1"
  printf '%s/%s' "$SYSTEMD_USER_DIR" "$service"
}

managed_services() {
  printf '%s\n' "$HUB_SERVICE"
  if [[ "$SKIP_UPDATER_INSTALL" -eq 0 ]]; then
    printf '%s\n' "$UPDATER_SERVICE"
  fi
}

all_services() {
  printf '%s\n' "$HUB_SERVICE" "$MONITOR_SERVICE"
  if [[ "$SKIP_UPDATER_INSTALL" -eq 0 ]]; then
    printf '%s\n' "$UPDATER_SERVICE"
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
  warn "edit config.json before the first restart, then rerun scripts/refresh_server_linux.sh or run scripts/refresh_server_linux.sh start"
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
  GOCACHE="$GO_BUILD_CACHE" GOOS=linux GOARCH="$GO_ARCH" go build -o "$out" "$pkg"
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
  local dest_dir
  local dest_name
  local tmp
  dest_dir="$(dirname "$dest")"
  dest_name="$(basename "$dest")"
  mkdir -p "$dest_dir"
  tmp="$(mktemp "${dest_dir}/.${dest_name}.tmp.XXXXXX")"
  if ! cp "$source" "$tmp"; then
    rm -f "$tmp"
    return 1
  fi
  if ! chmod 0755 "$tmp"; then
    rm -f "$tmp"
    return 1
  fi
  if ! mv -f "$tmp" "$dest"; then
    rm -f "$tmp"
    return 1
  fi
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

write_systemd_env() {
  step "write systemd environment: ${SYSTEMD_ENV_FILE}"
  mkdir -p "$(dirname "$SYSTEMD_ENV_FILE")"
  {
    printf 'HOME=%q\n' "$HOME"
    printf 'PATH=%q\n' "$PATH"
  } > "$SYSTEMD_ENV_FILE"
  chmod 0600 "$SYSTEMD_ENV_FILE"
}

write_hub_unit() {
  local unit
  unit="$(service_unit_path "$HUB_SERVICE")"
  cat > "$unit" <<EOF
[Unit]
Description=WheelMaker Hub

[Service]
Type=simple
WorkingDirectory=${REPO_ROOT}
EnvironmentFile=${SYSTEMD_ENV_FILE}
ExecStart=${HUB_BINARY} -d
Restart=always
RestartSec=5
StartLimitIntervalSec=300
StartLimitBurst=5

[Install]
WantedBy=default.target
EOF
}

write_monitor_unit() {
  local unit
  unit="$(service_unit_path "$MONITOR_SERVICE")"
  cat > "$unit" <<EOF
[Unit]
Description=WheelMaker Monitor

[Service]
Type=simple
WorkingDirectory=${REPO_ROOT}
EnvironmentFile=${SYSTEMD_ENV_FILE}
ExecStart=${MONITOR_BINARY}
Restart=always
RestartSec=5
StartLimitIntervalSec=300
StartLimitBurst=5

[Install]
WantedBy=default.target
EOF
}

write_updater_unit() {
  local unit
  unit="$(service_unit_path "$UPDATER_SERVICE")"
  cat > "$unit" <<EOF
[Unit]
Description=WheelMaker Updater

[Service]
Type=simple
WorkingDirectory=${REPO_ROOT}
EnvironmentFile=${SYSTEMD_ENV_FILE}
ExecStart=${UPDATER_BINARY} --repo ${REPO_ROOT} --install-dir ${INSTALL_DIR} --time ${UPDATER_DAILY_TIME}
Restart=always
RestartSec=5
StartLimitIntervalSec=300
StartLimitBurst=5

[Install]
WantedBy=default.target
EOF
}

configure_systemd_user_services() {
  if [[ "$SKIP_SERVICE_CONFIG" -eq 1 ]]; then
    step "skip systemd user service configuration"
    return
  fi
  step "write systemd user units"
  mkdir -p "$SYSTEMD_USER_DIR" "$LOG_DIR"
  write_systemd_env
  write_hub_unit
  write_monitor_unit
  if [[ "$SKIP_UPDATER_INSTALL" -eq 0 ]]; then
    write_updater_unit
  else
    step "skip updater systemd user unit configuration"
  fi
  systemctl --user daemon-reload
  warn "services start with the user session unless lingering is enabled: loginctl enable-linger $(id -un)"
}

stop_service() {
  local service="$1"
  step "systemctl --user stop ${service}"
  systemctl --user stop "$service" >/dev/null 2>&1 || true
}

start_service() {
  local service="$1"
  local unit
  unit="$(service_unit_path "$service")"
  [[ -f "$unit" ]] || die "systemd user unit not found: ${unit}"
  step "systemctl --user enable ${service}"
  systemctl --user enable "$service"
  step "systemctl --user start ${service}"
  systemctl --user start "$service"
}

restart_service() {
  local service="$1"
  local unit
  unit="$(service_unit_path "$service")"
  [[ -f "$unit" ]] || die "systemd user unit not found: ${unit}"
  step "systemctl --user enable ${service}"
  systemctl --user enable "$service"
  step "systemctl --user restart ${service}"
  systemctl --user restart "$service"
}

stop_services() {
  while IFS= read -r service; do
    [[ -n "$service" ]] || continue
    stop_service "$service"
  done < <(all_services)
}

start_services() {
  systemctl --user daemon-reload
  while IFS= read -r service; do
    [[ -n "$service" ]] || continue
    start_service "$service"
  done < <(all_services)
}

restart_services() {
  systemctl --user daemon-reload
  while IFS= read -r service; do
    [[ -n "$service" ]] || continue
    restart_service "$service"
  done < <(all_services)
}

status_services() {
  while IFS= read -r service; do
    [[ -n "$service" ]] || continue
    if [[ ! -f "$(service_unit_path "$service")" ]]; then
      echo "${service}: not installed"
    elif systemctl --user show "$service" --property=LoadState,ActiveState,UnitFileState >/dev/null 2>&1; then
      local active
      active="$(systemctl --user show "$service" --property=ActiveState --value 2>/dev/null || true)"
      local unit_state
      unit_state="$(systemctl --user show "$service" --property=UnitFileState --value 2>/dev/null || true)"
      echo "${service}: ${active:-unknown} ${unit_state:-unknown}"
    else
      echo "${service}: installed, not loaded"
    fi
  done < <(all_services)
}

uninstall_services() {
  local failed=0
  while IFS= read -r service; do
    [[ -n "$service" ]] || continue
    stop_service "$service" || failed=1
    step "systemctl --user disable ${service}"
    systemctl --user disable "$service" >/dev/null 2>&1 || true
    local unit
    unit="$(service_unit_path "$service")"
    if [[ -f "$unit" ]]; then
      step "remove ${unit}"
      rm -f "$unit" || failed=1
    fi
  done < <(all_services)
  systemctl --user daemon-reload || failed=1
  [[ "$failed" -eq 0 ]] || die "one or more systemd user service uninstall operations failed"
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
    stop_services
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

  configure_systemd_user_services
  publish_web

  if [[ "$config_created" -eq 0 && "$SKIP_RESTART" -eq 0 ]]; then
    warn "config was created from example at ${CONFIG_PATH}; edit it first, then rerun scripts/refresh_server_linux.sh"
    step "skip restart because config is newly created"
    return
  fi

  if [[ "$SKIP_RESTART" -eq 1 ]]; then
    step "skip restart"
    return
  fi
  start_services
  step "refresh complete"
}

case "$ACTION" in
  refresh)
    refresh
    ;;
  start)
    check_systemd_user
    start_services
    ;;
  stop)
    check_systemd_user
    stop_services
    ;;
  restart)
    check_systemd_user
    restart_services
    ;;
  status)
    check_systemd_user
    status_services
    ;;
  uninstall)
    check_systemd_user
    uninstall_services
    ;;
  *)
    die "unsupported action: ${ACTION}"
    ;;
esac
