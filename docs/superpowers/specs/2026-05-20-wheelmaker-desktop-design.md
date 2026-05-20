# WheelMaker Desktop Design

Date: 2026-05-20
Status: Approved

## Goal

Add a separate Windows desktop distribution for WheelMaker. `WheelMakerDesktop.exe` opens the existing Workspace Web UI in an embedded WebView window, with the Web UI assets compiled into the executable.

The browser-served Web UI remains the primary distribution path. Desktop publishing is manual and does not run as part of deploy, update-publish, or updater automation.

## Product Language

- **Workspace Web UI** is the existing Chat, File, Git, and Settings web experience.
- **WheelMaker Desktop** is the user-facing Windows executable named `WheelMakerDesktop`.
- **Monitor Dashboard** stays separate and is not the desktop app.

## Non-Goals

- Do not replace or rename the existing `wheelmaker.exe` service binary.
- Do not embed or manage Hub, Registry, Monitor, or Updater service lifecycles in the desktop process.
- Do not bundle Microsoft Edge WebView2 Runtime in the first version.
- Do not remove or change the existing `~/.wheelmaker/web` publish flow.
- Do not add desktop publishing to `deploy.bat`, `update-publish.bat`, or `WheelMakerUpdater`.
- Do not make the first version cross-platform.

## Architecture

Add a Windows-only Go command:

```text
server/cmd/wheelmaker-desktop
```

The command uses a Go WebView2 binding to create a native window and navigate it to an internal localhost URL.

At startup, `WheelMakerDesktop`:

1. Starts an HTTP server bound to `127.0.0.1` on a random free port.
2. Serves embedded Web UI files from `embed.FS`.
3. Navigates the WebView2 window to the local server root.
4. Leaves WheelMaker runtime services untouched.

The local server uses `embed.FS` instead of `~/.wheelmaker/web`, so the desktop app can open the UI even when the external web publish directory is missing.

## Embedded Web Assets

The desktop publish script builds the Workspace Web UI into a generated embed staging directory under the desktop command package, for example:

```text
server/cmd/wheelmaker-desktop/webroot/
```

The directory is generated and should be git-ignored. The desktop command embeds that directory at compile time.

The Webpack build should support a configurable output path so desktop publishing can build into the embed staging directory without changing the normal `npm run build:web:release` output of `~/.wheelmaker/web`.

The embedded asset set includes:

- `index.html`
- `bundle.js`
- `runtime-config.js`
- source maps when produced by the release build
- `manifest.webmanifest`
- `service-worker.js`
- `icons/`
- any Webpack emitted fonts or static assets

The current frontend uses absolute root paths such as `/bundle.js` and `/service-worker.js`, so the localhost server should serve the embedded app at `/`.

## Runtime Connection

`WheelMakerDesktop` does not start Hub or Registry. The embedded Web UI uses the same Registry connection behavior as the browser app. Because the WebView navigates to `http://127.0.0.1:<desktop-port>/`, the existing runtime default resolves Registry to `ws://127.0.0.1:9630/ws`.

If Registry is not running, the Workspace Web UI should show its existing disconnected or reconnecting state. Desktop-specific service startup is out of scope.

## WebView2 Dependency

The first version may depend on the system-installed Microsoft Edge WebView2 Runtime.

If WebView2 is missing, `WheelMakerDesktop` should fail with an actionable message telling the user that Microsoft Edge WebView2 Runtime is required. The first version does not download or install it automatically.

## Manual Publish Flow

Add a manual root-level entrypoint:

```text
publish-desktop.bat
```

It calls:

```text
scripts/publish_desktop.ps1
```

The script builds only the desktop distribution. It does not stop, start, restart, install, or update any WheelMaker service.

Publish output:

```text
~/.wheelmaker/desktop/WheelMakerDesktop.exe
~/.wheelmaker/desktop/desktop-release.json
```

`desktop-release.json` records at least:

- schema version
- repo path
- branch
- git SHA
- build time
- desktop binary path
- embedded Web staging path

The script creates a current-user desktop shortcut:

```text
%USERPROFILE%\Desktop\WheelMaker Desktop.lnk
```

The shortcut target is `~/.wheelmaker/desktop/WheelMakerDesktop.exe`.

## Comparison

Recommended: Go + WebView2 + embedded Web + localhost server.

This fits the existing Go repository, keeps the desktop app as a single executable carrying Web UI resources, and does not disturb the Web-first release flow. It only requires WebView2 Runtime to exist on Windows.

Alternative: Wails.

Wails provides a complete desktop app framework, but it adds a new project structure, CLI, and build model. That is more framework than the first desktop shell needs.

Alternative: Electron.

Electron is mature and cross-platform, but it brings a much larger runtime and does not match the agreed dependency model of using system WebView2.

## Testing

Automated checks:

- `scripts/publish_desktop.ps1 -WhatIf` verifies the publish flow without building or writing the shortcut.
- Script tests assert desktop publishing is not called from `deploy.bat`, `update-publish.bat`, or updater refresh scripts.
- Go tests cover the embedded HTTP server path fallback and content types where practical.
- Go tests cover WebView2 missing-runtime error formatting behind a small injectable launcher boundary.

Manual verification:

- Run `publish-desktop.bat`.
- Confirm `~/.wheelmaker/desktop/WheelMakerDesktop.exe` exists.
- Confirm `%USERPROFILE%\Desktop\WheelMaker Desktop.lnk` exists and points to the desktop executable.
- Temporarily move `~/.wheelmaker/web` aside and confirm the desktop app still opens the UI.
- Confirm existing `deploy.bat` and `update-publish.bat` still follow their current behavior.

## Rollout

Implement in a separate desktop publishing slice:

1. Add configurable Webpack output path for desktop staging without changing normal Web release output.
2. Add `wheelmaker-desktop` Go command with embedded static server and WebView2 window launch.
3. Add manual desktop publish scripts and shortcut creation.
4. Add tests for script boundaries and local static serving.
5. Keep Web release as the default and document desktop publishing as a manual path.
