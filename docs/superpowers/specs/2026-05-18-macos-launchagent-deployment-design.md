# macOS LaunchAgent Deployment Design

Date: 2026-05-18
Status: Approved

## Goal

Support deploying WheelMaker on macOS as a complete Machine A runtime while keeping the external HTTPS reverse proxy under operator control.

The macOS runtime should publish the Web UI, run the registry and monitor services, run the local hub/agent bridge, and keep the updater capable of daily and manual full refreshes.

## Decisions

- macOS is a full Machine A, not only a reporting Machine B.
- Runtime processes run as the current user through `LaunchAgent`, not system `LaunchDaemon`.
- The deployment does not install or configure Nginx, Caddy, certificates, or public ports.
- External reverse proxy paths stay aligned with Windows:
  - `/` serves `~/.wheelmaker/web`
  - `/ws` proxies to `127.0.0.1:9630`
  - `/monitor/` proxies to `127.0.0.1:9631`
- The first macOS slice builds only the current machine architecture.
- The updater remains enabled and performs daily full refreshes plus manual signal-triggered full refreshes.
- Runtime is split into three independent agents:
  - `com.wheelmaker.hub`
  - `com.wheelmaker.monitor`
  - `com.wheelmaker.updater`
- The monitor can show and control the three `LaunchAgent` jobs.
- Deployment scripts only check dependencies and print actionable errors. They do not install Homebrew, Go, Node, npm packages, agent CLIs, or reverse proxies.
- Uninstall removes the `LaunchAgent` registrations and plist files, but leaves `~/.wheelmaker` data intact.

## Non-Goals

- Do not configure a public reverse proxy.
- Do not create root-owned LaunchDaemons.
- Do not produce universal binaries.
- Do not auto-install missing dependencies.
- Do not support Linux/systemd in this slice.
- Do not change the registry, monitor, or Web URL contract.
- Do not delete `~/.wheelmaker/config.json`, SQLite databases, logs, or Web assets during uninstall.

## Runtime Topology

The macOS deployment keeps the same logical topology as the Windows Machine A deployment:

```text
External HTTPS reverse proxy
  /          -> ~/.wheelmaker/web
  /ws        -> 127.0.0.1:9630
  /monitor/  -> 127.0.0.1:9631

LaunchAgent com.wheelmaker.hub
  ~/.wheelmaker/bin/wheelmaker -d
    -> guardian
    -> hub worker
    -> registry worker when registry.listen=true

LaunchAgent com.wheelmaker.monitor
  ~/.wheelmaker/bin/wheelmaker-monitor

LaunchAgent com.wheelmaker.updater
  ~/.wheelmaker/bin/wheelmaker-updater --repo <repo> --install-dir ~/.wheelmaker/bin --time 03:00
```

All three jobs run as the logged-in user. This preserves the user `HOME`, `PATH`, npm global package visibility, agent CLI auth, and Keychain access assumptions used by Codex, Claude, Copilot, and related ACP CLIs.

## Deployment Script

Add `scripts/refresh_server.sh` as the macOS deploy entrypoint.

Responsibilities:

- Resolve repo root and install directory.
- Validate macOS-only execution.
- Check required commands:
  - `bash`
  - `git`
  - `go`
  - `node`
  - `npm`
  - `npx`
  - `launchctl`
- Validate Node version is at least the app package engine minimum.
- Preserve or create `~/.wheelmaker/config.json` from `server/config.example.json`.
- Pull latest code with `git pull --ff-only` when the worktree is clean, unless update is skipped.
- Build:
  - `wheelmaker`
  - `wheelmaker-monitor`
  - `wheelmaker-updater`
- Install binaries into `~/.wheelmaker/bin`.
- Publish Web assets to `~/.wheelmaker/web`.
- Generate plist files under `~/Library/LaunchAgents`.
- Support lifecycle actions:
  - install/default refresh
  - start
  - stop
  - restart
  - status
  - uninstall
- Use `launchctl bootstrap`, `bootout`, `kickstart`, and `print` against `gui/<uid>/<label>`.

The script should be idempotent. Re-running it updates binaries, regenerates plist files, and restarts jobs unless restart is skipped.

## Web Publish

The existing webpack config already writes production assets to `~/.wheelmaker/web`. The current npm release command still shells through PowerShell to copy public assets.

Replace the PowerShell-only Web release helper with a Node-based script:

- `app/scripts/export_web_release.js` copies `manifest.webmanifest`, `service-worker.js`, and `icons/` from `app/web/public` into `~/.wheelmaker/web`.
- `npm run build:web:release` runs `npm run build:web && node scripts/export_web_release.js`.

This keeps Windows behavior intact while allowing macOS to publish Web assets without PowerShell.

## Updater

`wheelmaker-updater` remains the long-running scheduler.

Update round rules:

- On Windows, continue invoking `scripts/refresh_server.ps1`.
- On macOS, invoke `scripts/refresh_server.sh`.
- Pass install directory and skip flags consistently.
- Manual signal remains `~/.wheelmaker/update-now.signal`.
- Signal payload containing `full-update` triggers update + build + Web publish.
- Plain manual signal skips the git update stage, matching existing Windows behavior.
- The updater logs failures and continues its scheduler loop.

The updater validates platform-specific commands instead of requiring `powershell` on every OS.

## Monitor macOS Operations

The monitor keeps its current Windows service behavior and gains a macOS service-control branch.

On macOS:

- `GetServiceStatus` reports the three labels:
  - `com.wheelmaker.hub`
  - `com.wheelmaker.monitor`
  - `com.wheelmaker.updater`
- `StartService` starts hub and updater jobs through `launchctl`.
- `StopService` stops hub and updater jobs through `launchctl`.
- `RestartService` restarts hub and updater jobs through `launchctl`.
- `RestartMonitor` restarts the monitor LaunchAgent when installed; otherwise it falls back to process relaunch.
- `TriggerUpdatePublish` continues to write the full-update signal file.

The monitor should not attempt to configure reverse proxies or certificates.

## Configuration

The existing `~/.wheelmaker/config.json` format remains unchanged.

For macOS Machine A:

```json
{
  "projects": [
    {
      "name": "WheelMaker",
      "path": "/Users/me/Code/WheelMaker"
    }
  ],
  "registry": {
    "listen": true,
    "port": 9630,
    "server": "127.0.0.1",
    "token": "replace-with-shared-token",
    "hubId": "mac-a"
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

The macOS deployment publishes local endpoints only. Operator-owned reverse proxy config must provide:

```text
/          static files from ~/.wheelmaker/web
/ws        websocket proxy to http://127.0.0.1:9630
/monitor/  HTTP proxy to http://127.0.0.1:9631
```

The Web app continues to expect the registry WebSocket at `/ws` unless runtime config overrides it.

## Error Handling

- Missing dependency: fail before mutating services, with the command name and installation hint.
- Dirty git worktree: skip automatic pull and continue the local build, matching Windows behavior.
- Missing config: create from example and skip restart.
- LaunchAgent bootstrap failure: surface the `launchctl` output and leave existing plist files in place.
- Build failure: stop before replacing installed binaries.
- Web publish failure: stop before claiming refresh success.
- Uninstall failure for one job: continue attempting the remaining jobs and return a combined error.

## Testing

Unit and script tests:

- Updater chooses `refresh_server.sh` on `darwin`.
- Updater no longer requires `powershell` on non-Windows.
- Updater command arguments preserve skip/full-update behavior.
- Web release copy script copies public assets into `~/.wheelmaker/web`.
- macOS deployment script source contains expected labels, `launchctl` lifecycle verbs, dependency checks, and plist generation.
- Monitor service status/action helpers produce macOS `launchctl` operations without affecting Windows branches.

Build and verification:

- `server`: `go test ./...`
- Cross-compile:
  - `GOOS=darwin GOARCH=arm64 go build ./cmd/wheelmaker`
  - `GOOS=darwin GOARCH=arm64 go build ./cmd/wheelmaker-monitor`
  - `GOOS=darwin GOARCH=arm64 go build ./cmd/wheelmaker-updater`
- `app`: `npm run tsc:web`
- `app`: `npm test -- --runInBand`
- `app`: `npm run build:web:release`

Manual macOS validation remains required before declaring the feature production-ready because `launchctl` and Keychain-dependent agent auth cannot be fully verified from Windows.

## Rollout

1. Land cross-platform Web release support.
2. Land macOS deployment script and docs.
3. Land updater platform selection.
4. Land monitor macOS LaunchAgent operations.
5. Run Windows regression tests to confirm existing service deployment still compiles and behaves the same at the code level.
6. Run a real macOS smoke deployment:
   - `scripts/refresh_server.sh`
   - verify plist files in `~/Library/LaunchAgents`
   - `launchctl print gui/$(id -u)/com.wheelmaker.hub`
   - open local monitor endpoint
   - verify external reverse proxy maps `/`, `/ws`, and `/monitor/`

