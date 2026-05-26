# Linux systemd User Deployment Design

Date: 2026-05-18
Status: Draft

## Goal

Support deploying WheelMaker on Linux as a complete Machine A runtime while keeping the external HTTPS reverse proxy under operator control.

The Linux runtime should publish the Web UI, run the registry and monitor services, run the local hub/agent bridge, and keep the updater capable of daily and manual full refreshes.

## Decisions

- Linux is a full Machine A, not only a reporting Machine B.
- Runtime processes run as the current user through `systemd --user`, not root-owned system units.
- The deployment does not install or configure Nginx, Caddy, certificates, or public ports.
- External reverse proxy paths stay aligned with Windows and macOS:
  - `/` serves `~/.wheelmaker/web`
  - `/ws` proxies to `127.0.0.1:9630`
  - `/monitor/` proxies to `127.0.0.1:9631`
- The first Linux slice builds only the current machine architecture.
- The updater remains enabled and performs daily full refreshes plus manual signal-triggered full refreshes.
- Runtime is split into three independent user services:
  - `wheelmaker-hub.service`
  - `wheelmaker-monitor.service`
  - `wheelmaker-updater.service`
- The monitor can show and control the three `systemd --user` services.
- Deployment scripts only check dependencies and print actionable errors. They do not install OS packages, Go, Node, npm packages, agent CLIs, reverse proxies, or certificates.
- The deployment does not automatically enable lingering. Operators who need services to start before interactive login can run `loginctl enable-linger <user>` themselves.
- Uninstall removes the user service registrations and unit files, but leaves `~/.wheelmaker` data intact.

## Non-Goals

- Do not configure a public reverse proxy.
- Do not create root-owned `systemd` system services.
- Do not auto-enable `loginctl enable-linger`.
- Do not support non-systemd Linux distributions in this slice.
- Do not produce multi-architecture binaries from one deploy run.
- Do not auto-install missing dependencies.
- Do not change the registry, monitor, or Web URL contract.
- Do not delete `~/.wheelmaker/config.json`, SQLite databases, logs, or Web assets during uninstall.

## Runtime Topology

The Linux deployment keeps the same logical topology as the Windows and macOS Machine A deployments:

```text
External HTTPS reverse proxy
  /          -> ~/.wheelmaker/web
  /ws        -> 127.0.0.1:9630
  /monitor/  -> 127.0.0.1:9631

systemd --user wheelmaker-hub.service
  ~/.wheelmaker/bin/wheelmaker -d
    -> guardian
    -> hub worker
    -> registry worker when registry.listen=true

systemd --user wheelmaker-monitor.service
  ~/.wheelmaker/bin/wheelmaker-monitor

systemd --user wheelmaker-updater.service
  ~/.wheelmaker/bin/wheelmaker-updater --repo <repo> --install-dir ~/.wheelmaker/bin --time 03:00
```

All three services run as the current user. This preserves the user `HOME`, npm global package visibility, agent CLI auth files, SSH keys, and provider-specific local state used by Codex, Claude, Copilot, and related ACP CLIs.

Because `systemd --user` does not always inherit the interactive shell `PATH`, the deployment writes an environment file under `~/.wheelmaker/systemd.env` with the current `HOME` and `PATH`. If the operator changes Node, Go, npm, or agent CLI locations, rerunning deployment regenerates this environment file.

## Deployment Script

Add `scripts/refresh_server_linux.sh` as the Linux deploy entrypoint, and update root `deploy.sh` to dispatch by platform:

- macOS: `scripts/refresh_server.sh`
- Linux: `scripts/refresh_server_linux.sh`
- Windows: keep `deploy.bat`

Responsibilities:

- Resolve repo root and install directory.
- Validate Linux-only execution.
- Check that `systemctl --user` is available and usable for the current user.
- Check required commands:
  - `bash`
  - `git`
  - `go`
  - `node`
  - `npm`
  - `systemctl`
- Validate Node version is at least the app package engine minimum.
- Preserve or create `~/.wheelmaker/config.json` from `server/config.example.json`.
- Stash local worktree changes, then pull latest code with `git pull --ff-only`, unless update is skipped.
- Build:
  - `wheelmaker`
  - `wheelmaker-monitor`
  - `wheelmaker-updater`
- Install binaries into `~/.wheelmaker/bin`.
- Publish Web assets to `~/.wheelmaker/web`.
- Generate user unit files under `~/.config/systemd/user`.
- Write `~/.wheelmaker/systemd.env` from the current shell environment.
- Support lifecycle actions:
  - install/default refresh
  - start
  - stop
  - restart
  - status
  - uninstall
- Use `systemctl --user daemon-reload`, `enable`, `disable`, `start`, `stop`, `restart`, `status`, and `show`.

The script should be idempotent. Re-running it updates binaries, regenerates unit files, reloads the user manager, and restarts services unless restart is skipped.

## User Unit Shape

Each generated unit should use explicit paths and current-user state:

```ini
[Unit]
Description=WheelMaker Hub

[Service]
Type=simple
WorkingDirectory=<repo-root>
EnvironmentFile=%h/.wheelmaker/systemd.env
ExecStart=%h/.wheelmaker/bin/wheelmaker -d
Restart=always
RestartSec=5
StartLimitIntervalSec=300
StartLimitBurst=5

[Install]
WantedBy=default.target
```

The monitor and updater units follow the same shape with their respective `ExecStart` commands.

Application logs remain under `~/.wheelmaker/log` where existing components already write them. Service stdout and stderr can remain in the user journal; the monitor can surface existing WheelMaker log files without depending on journal access.

## Web Publish

The Linux deployment depends on the cross-platform Node Web release helper:

- `app/scripts/export_web_release.js` copies `manifest.webmanifest`, `service-worker.js`, and `icons/` from `app/web/public` into `~/.wheelmaker/web`.
- `npm run build:web:release` runs `npm run build:web && node scripts/export_web_release.js`.

No Linux-specific Web publish path is required.

## Updater

`wheelmaker-updater` remains the long-running scheduler.

Update round rules:

- On Windows, continue invoking `scripts/refresh_server.ps1`.
- On macOS, invoke `scripts/refresh_server.sh`.
- On Linux, invoke `scripts/refresh_server_linux.sh`.
- Pass install directory and skip flags consistently.
- Manual signal remains `~/.wheelmaker/update-now.signal`.
- Signal payload containing `full-update` triggers update + build + Web publish.
- Plain manual signal skips the git update stage, matching existing Windows and macOS behavior.
- The updater logs failures and continues its scheduler loop.

The updater validates platform-specific commands instead of requiring `powershell` or `launchctl` on Linux.

## Monitor Linux Operations

The monitor keeps its current Windows and macOS behavior and gains a Linux service-control branch.

On Linux:

- `GetServiceStatus` reports the three user service units:
  - `wheelmaker-hub.service`
  - `wheelmaker-monitor.service`
  - `wheelmaker-updater.service`
- `StartService` starts hub and updater through `systemctl --user`.
- `StopService` stops hub and updater through `systemctl --user`.
- `RestartService` restarts hub and updater through `systemctl --user`.
- `RestartMonitor` restarts `wheelmaker-monitor.service` when installed; otherwise it falls back to process relaunch.
- `TriggerUpdatePublish` continues to write the full-update signal file.

The monitor should parse `systemctl --user show` output rather than scraping human-oriented `status` output for service state. It should not attempt to configure reverse proxies, certificates, or lingering.

## Process Monitoring

Linux process discovery should only report real `wheelmaker` processes. It should not classify `wheelmaker-updater`, `wheelmaker-monitor`, shell commands, grep commands, or script wrappers as hub processes.

Expected roles:

- `guardian`: `wheelmaker -d`
- `hub-worker`: `wheelmaker --hub-worker`
- `registry-worker`: `wheelmaker --registry-worker`
- `unknown`: only for unexpected real `wheelmaker` command lines

## Configuration

The existing `~/.wheelmaker/config.json` format remains unchanged.

For Linux Machine A:

```json
{
  "projects": [
    {
      "name": "WheelMaker",
      "path": "/home/me/Code/WheelMaker"
    }
  ],
  "registry": {
    "listen": true,
    "port": 9630,
    "server": "127.0.0.1",
    "token": "replace-with-shared-token",
    "hubId": "linux-a"
  },
  "monitor": {
    "port": 9631
  },
  "log": {
    "level": "warn"
  }
}
```

The first deployment may create the example config and skip restart so the operator can fill in project paths and tokens before starting services.

## Reverse Proxy Contract

The Linux deployment publishes local endpoints only. Operator-owned reverse proxy config must provide:

```text
/          static files from ~/.wheelmaker/web
/ws        websocket proxy to http://127.0.0.1:9630
/monitor/  HTTP proxy to http://127.0.0.1:9631
```

The Web app continues to expect the registry WebSocket at `/ws` unless runtime config overrides it.

## Error Handling

- Missing dependency: fail before mutating services, with the command name and installation hint.
- Missing or unusable `systemctl --user`: fail before mutating services, with a hint to run from a real user session or configure the user manager.
- Missing `XDG_RUNTIME_DIR`: fail before mutating services, because `systemctl --user` usually cannot control the user manager without it.
- Dirty git worktree: skip automatic pull and continue the local build, matching Windows and macOS behavior.
- Missing config: create from example and skip restart.
- Unit installation failure: surface `systemctl --user` output and leave generated unit files in place for inspection.
- Build failure: stop before replacing installed binaries.
- Web publish failure: stop before claiming refresh success.
- Uninstall failure for one service: continue attempting the remaining services and return a combined error.
- Linger disabled: do not fail by default; print a clear note that services start with the user session unless the operator enables lingering.

## Testing

Unit and script tests:

- Updater chooses `refresh_server_linux.sh` on `linux`.
- Updater no longer requires `powershell` or `launchctl` on Linux.
- Updater command arguments preserve skip/full-update behavior.
- Linux deployment script source contains expected service names, `systemctl --user` lifecycle verbs, dependency checks, environment file generation, and user unit generation.
- Root `deploy.sh` dispatches to the Linux refresh script on `uname -s == Linux`.
- Monitor service status/action helpers produce Linux `systemctl --user` operations without affecting Windows or macOS branches.
- Linux process parsing excludes updater, monitor, shell, grep, and wrapper processes.

Build and verification:

- `server`: `go test ./...`
- Cross-compile:
  - `GOOS=linux GOARCH=amd64 go build ./cmd/wheelmaker`
  - `GOOS=linux GOARCH=amd64 go build ./cmd/wheelmaker-monitor`
  - `GOOS=linux GOARCH=amd64 go build ./cmd/wheelmaker-updater`
- `app`: `npm run tsc:web`
- `app`: `npm test -- --runInBand`
- `app`: `npm run build:web:release`

Manual Linux validation remains required before declaring the feature production-ready because `systemd --user`, login session environment, lingering, and agent CLI auth cannot be fully verified from Windows.

## Rollout

1. Land Linux deployment design.
2. Land `scripts/refresh_server_linux.sh` and `deploy.sh` platform dispatch.
3. Land updater Linux platform selection.
4. Land monitor Linux `systemd --user` operations.
5. Run Windows and macOS regression tests to confirm existing deployment branches still compile and behave the same at the code level.
6. Run a real Linux smoke deployment:
   - `bash deploy.sh`
   - verify unit files in `~/.config/systemd/user`
   - `systemctl --user status wheelmaker-hub.service`
   - `systemctl --user status wheelmaker-monitor.service`
   - `systemctl --user status wheelmaker-updater.service`
   - open local monitor endpoint
   - verify external reverse proxy maps `/`, `/ws`, and `/monitor/`
