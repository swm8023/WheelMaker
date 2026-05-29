# WheelMaker Deploy CLI Design

Date: 2026-05-30
Status: Draft for review

## Goal

Move WheelMaker deployment, update, service control, Web publish, and release manifest orchestration out of platform-specific refresh scripts and into a cross-platform Go CLI.

The transitional implementation introduces `wheelmaker-deploy` while keeping existing scripts available for old installations. The first version must support Windows, macOS, and Linux, with Linux using `systemd --user` plus required lingering so services survive user logout.

## Decisions

- Add a new command under `server/cmd/wheelmaker-deploy`.
- Keep deployment code in that command package for the transitional version.
- Allow platform-specific files in the command package:
  - `main.go`
  - `service_windows.go`
  - `service_darwin.go`
  - `service_linux.go`
  - `service_other.go`
- Do not add `internal/deploy` in this phase.
- Do not refactor `wheelmaker-monitor` service-control code in this phase.
- Top-level `deploy.bat` and `deploy.sh` call `wheelmaker-deploy deploy` directly.
- Existing `scripts/refresh_server.ps1`, `scripts/refresh_server.sh`, and `scripts/refresh_server_linux.sh` remain unchanged for old-flow compatibility.
- `update-publish.bat` and `update-publish.sh` continue to only write the `full-update` signal.
- New `wheelmaker-updater` invokes installed `wheelmaker-deploy bootstrap-update`; it does not fall back to refresh scripts.
- `deploy` may install and configure `wheelmaker-updater`.
- `update` and `bootstrap-update` must not build, install, stop, start, replace, or reconfigure `wheelmaker-updater`.
- Reserve an updater self-upgrade command for a later phase; it is not implemented or invoked in the transitional version.
- No ACP or agent global package installation is performed by `wheelmaker-deploy`.
- `app` Web build dependencies are synced with `npm ci --include=dev` only when Web publish is enabled.
- No automatic backup or rollback is performed.
- Release manifests are written only after successful build, Web publish, and install.
- First deploy creates a runnable local config and still starts services.

## Non-Goals

- Do not delete the existing refresh scripts.
- Do not make `update-publish.*` run updates synchronously.
- Do not replace the updater service during updater-triggered updates.
- Do not support non-systemd Linux deployments.
- Do not auto-enable Linux lingering.
- Do not add automatic rollback or backup.
- Do not install ACP or agent global dependencies.
- Do not implement updater self-upgrade in the transitional version.
- Do not fully implement uninstall in the transitional CLI.

## Command Surface

```text
wheelmaker-deploy deploy [options]
wheelmaker-deploy bootstrap-update [options]
wheelmaker-deploy update [options]
wheelmaker-deploy upgrade-updater [options]
wheelmaker-deploy service start|stop|restart|status
wheelmaker-deploy service uninstall
wheelmaker-deploy doctor [options]
```

`upgrade-updater` and `service uninstall` are reserved in the interface but return clear not-implemented errors in the transitional version. Existing uninstall scripts remain available.

## Options

```text
--repo PATH
--bin PATH
--time HH:mm
--no-pull
--no-npm
--no-build
--no-install
--no-restart
--no-config
--no-web
--no-updater
```

Defaults:

- `--repo`: current repository root when not provided.
- `--bin`: `~/.wheelmaker/bin`.
- `--time`: `03:00`, used when configuring updater services.
- `--no-web`: disables both Web publish and the `app` `npm ci --include=dev` step.
- `--no-npm`: skips `app` `npm ci --include=dev` while still allowing Web publish to attempt a build with existing dependencies.

## Mode Defaults

| Mode | Pull | npm ci | Build | Install | Web | Config | Restart | Updater |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `deploy` | yes | yes | yes | yes | yes | yes | yes | yes |
| `bootstrap-update` | yes | no | deploy CLI temp only | no | no | no | no | no |
| `update` | no | yes | yes | yes | yes | no | yes | no |
| `upgrade-updater` | no | no | no | no | no | no | no | reserved |
| `service start` | no | no | no | no | no | no | start | no |
| `service stop` | no | no | no | no | no | no | stop | no |
| `service restart` | no | no | no | no | no | no | restart | no |
| `service status` | no | no | no | no | no | no | status | no |
| `doctor` | check | check | check | no | no | check | no | check |

`bootstrap-update` executes `update` with `--no-pull --no-config --no-updater`.

## Deploy Flow

`wheelmaker-deploy deploy` performs:

1. Resolve repository, home, install directory, build directory, and platform.
2. Validate required commands:
   - `git`
   - `go`
   - `node`
   - `npm`
   - platform service tooling
3. On Linux, require usable `systemctl --user` and enabled lingering.
4. Pull latest code unless `--no-pull`.
5. Run `npm ci --include=dev` in `app` when Web publish is enabled and `--no-npm` is not set.
6. Build:
   - `wheelmaker`
   - `wheelmaker-monitor`
   - `wheelmaker-updater`
   - `wheelmaker-deploy`
7. Publish Web with `npm run build:web:release` unless `--no-web`.
8. Stop hub and monitor if services are installed and restart is enabled.
9. Install binaries into `--bin`.
10. Create config if missing.
11. Generate helper wrappers.
12. Configure platform services unless `--no-config`.
13. Write `~/.wheelmaker/release.json`.
14. Start hub, monitor, and updater unless `--no-restart`.

If config was created during first deploy, services still start because the generated config is runnable.

## Bootstrap Update Flow

`wheelmaker-updater` invokes:

```text
<bin>/wheelmaker-deploy bootstrap-update --repo <repo> --bin <bin> --time <time>
```

`bootstrap-update` performs:

1. Pull latest code unless `--no-pull`.
2. Build a temporary deploy CLI from the updated source:
   - Windows: `~/.wheelmaker/build/bootstrap/wheelmaker-deploy-next.exe`
   - Unix: `~/.wheelmaker/build/bootstrap/wheelmaker-deploy-next`
3. Execute the temporary CLI:

```text
wheelmaker-deploy-next update --repo <repo> --bin <bin> --time <time> --no-pull --no-config --no-updater
```

The updater process remains the parent scheduler and is not stopped or replaced.

## Update Flow

`wheelmaker-deploy update` performs:

1. Resolve paths and validate required commands.
2. Pull only when explicitly requested by absence of `--no-pull`.
3. Run `npm ci --include=dev` when Web publish is enabled and `--no-npm` is not set.
4. Build:
   - `wheelmaker`
   - `wheelmaker-monitor`
   - `wheelmaker-deploy`
5. Publish Web unless `--no-web`.
6. Stop hub and monitor if restart is enabled.
7. Install hub, monitor, and deploy CLI.
8. Write `~/.wheelmaker/release.json`.
9. Start hub and monitor unless `--no-restart`.

`update` never touches updater binaries or updater service configuration.

## Install Rules

Install targets:

- Windows:
  - `wheelmaker.exe`
  - `wheelmaker-monitor.exe`
  - `wheelmaker-updater.exe` for `deploy` only
  - `wheelmaker-deploy.exe`
- macOS/Linux:
  - `wheelmaker`
  - `wheelmaker-monitor`
  - `wheelmaker-updater` for `deploy` only
  - `wheelmaker-deploy`

The running temporary deploy binary must never overwrite itself. Updates install the formal deploy binary in `~/.wheelmaker/bin`.

Windows installs use retry behavior for locked files. Unix installs use a temp file in the target directory and atomic rename.

## Config Generation

If `~/.wheelmaker/config.json` does not exist, `wheelmaker-deploy` writes a runnable local config instead of copying the old example as-is:

```json
{
  "projects": [
    {
      "name": "WheelMaker",
      "path": "<repo absolute path>"
    }
  ],
  "registry": {
    "listen": true,
    "port": 9630,
    "server": "127.0.0.1",
    "token": "wheelmaker-local-token",
    "hubId": "local-hub"
  },
  "monitor": {
    "port": 9631
  },
  "log": {
    "level": "warn"
  }
}
```

`server/config.example.json` should be aligned with this semantic. Its project path remains a placeholder because the real repository path is resolved at deploy time.

## Helper Wrappers

After install, `wheelmaker-deploy` writes shell wrappers under `~/.wheelmaker`.

Windows:

```text
start.bat   -> bin\wheelmaker-deploy.exe service start
stop.bat    -> bin\wheelmaker-deploy.exe service stop
restart.bat -> bin\wheelmaker-deploy.exe service restart
status.bat  -> bin\wheelmaker-deploy.exe service status
```

macOS/Linux:

```text
start.sh    -> bin/wheelmaker-deploy service start
stop.sh     -> bin/wheelmaker-deploy service stop
restart.sh  -> bin/wheelmaker-deploy service restart
status.sh   -> bin/wheelmaker-deploy service status
```

The new flow does not install copied refresh scripts.

## Platform Services

### Windows

`deploy` configures Windows services:

- `WheelMaker`
- `WheelMakerMonitor`
- `WheelMakerUpdater`

Updater service arguments include:

```text
--repo <repo> --install-dir <bin> --time <HH:mm>
```

The top-level `deploy.bat` remains responsible for transitional UAC elevation. `wheelmaker-deploy deploy` detects missing elevation for service configuration and returns an actionable error.

### macOS

`deploy` writes LaunchAgents:

- `~/Library/LaunchAgents/com.wheelmaker.hub.plist`
- `~/Library/LaunchAgents/com.wheelmaker.monitor.plist`
- `~/Library/LaunchAgents/com.wheelmaker.updater.plist`

Service operations use `launchctl bootout`, `bootstrap`, `kickstart`, and `print` against the current user's `gui/<uid>` domain.

### Linux

`deploy` writes systemd user units:

- `~/.config/systemd/user/wheelmaker-hub.service`
- `~/.config/systemd/user/wheelmaker-monitor.service`
- `~/.config/systemd/user/wheelmaker-updater.service`

It also writes:

```text
~/.wheelmaker/systemd.env
```

Linux deployment requires:

- `systemctl --user` works in the current session.
- `loginctl show-user <user> -p Linger` reports enabled lingering.

If lingering is disabled, deploy fails and tells the operator to run:

```bash
sudo loginctl enable-linger "$USER"
```

No bypass flag is provided because services may stop after logout without lingering.

## Release Manifest

`~/.wheelmaker/release.json` is written only when build, install, and Web publish complete successfully.

It is written after binary installation and before service start:

```json
{
  "schemaVersion": 1,
  "repo": "<repo absolute path>",
  "branch": "<current branch>",
  "remote": "origin",
  "sha": "<HEAD sha>",
  "publishedAt": "<UTC RFC3339>"
}
```

The manifest is skipped when `--no-build`, `--no-install`, or `--no-web` is used.

## Failure Policy

The transitional version does not create backups and does not perform automatic rollback.

Rules:

- Build, npm install, and Web publish happen before hub and monitor are stopped.
- If build, npm install, or Web publish fails, installed binaries are not touched.
- If stop or install fails, the command exits with an error.
- `release.json` is never written for failed rounds.
- Operators can rerun `deploy` or `update` after fixing the underlying error.

## Script Transition

`deploy.bat` and `deploy.sh` become thin top-level launchers:

- Build `wheelmaker-deploy` when it is missing.
- Call `wheelmaker-deploy deploy` directly.
- Do not call `scripts/refresh_server.*`.

Existing refresh scripts remain in the repository unchanged for old updater installations and manual old-flow use.

`update-publish.bat` and `update-publish.sh` remain signal-only:

```text
full-update
<timestamp>
```

## Updater Changes

`wheelmaker-updater` no longer selects refresh scripts. It invokes the installed deploy CLI:

```text
<bin>/wheelmaker-deploy bootstrap-update --repo <repo> --bin <bin> --time <HH:mm>
```

If the deploy CLI is missing, updater logs a clear error and does not fall back to refresh scripts. Running `deploy.bat` or `deploy.sh` repairs the installation.

Manual signal semantics stay unchanged:

- `full-update` triggers update and Web publish.
- `skip-web-publish` remains accepted for compatibility and maps to a deploy CLI invocation with `--no-web`.
- Plain signals are treated as `full-update`.

## Updater Self-Upgrade

Updater self-upgrade is reserved for a later phase and is not implemented in the transitional CLI.

The intended future shape is an external handoff, not in-place replacement by the updater process:

1. `wheelmaker-deploy` builds a new updater binary into a staging path.
2. It starts an independent one-shot upgrade job that is not part of the running updater service process tree.
3. The one-shot job stops the updater service, replaces the updater binary, starts the updater service, and exits.

Platform-specific future handoff mechanisms:

- Windows: detached process, temporary scheduled task, or one-shot service.
- Linux: `systemd-run --user` transient unit.
- macOS: independent LaunchAgent or detached helper process.

No automatic call to this flow is made by `deploy`, `bootstrap-update`, or `update` in the transitional version.

## Doctor

`wheelmaker-deploy doctor` checks but does not modify:

- repo validity
- required tools
- Go availability
- Node and npm availability
- app package files
- install directory
- service manager availability
- Windows elevation when service config would be needed
- Linux `systemctl --user`
- Linux lingering
- macOS `launchctl`

Linux no-linger is a failing doctor result.

## Acceptance Criteria

- `server/cmd/wheelmaker-deploy` builds on Windows, macOS, and Linux.
- `deploy.bat` and `deploy.sh` call `wheelmaker-deploy deploy` directly.
- Existing refresh scripts remain unchanged.
- `wheelmaker-updater` invokes `wheelmaker-deploy bootstrap-update`.
- `update-publish.*` remains signal-only.
- `deploy` installs and configures updater services.
- `update` and `bootstrap-update` do not touch updater services or updater binaries.
- `upgrade-updater` exists only as a reserved command and returns not implemented.
- Linux deploy fails when lingering is disabled.
- First deploy creates a runnable local config and starts services.
- Helper start, stop, restart, and status wrappers are generated.
- `release.json` is written only after successful build, Web publish, and install.
- No ACP global dependency install or uninstall remains in the new deploy CLI.
