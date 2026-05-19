# Agent NPM Update Settings Design

Date: 2026-05-19
Status: Approved

## Goal

Add a Settings `Update` detail page that shows and manages WheelMaker agent-related global npm packages across online hubs.

The tool should show installed and latest versions, install missing supported packages, update stale supported packages, and uninstall known deprecated packages. It must stay a controlled maintenance surface, not a remote shell.

## Context

The Web app already has shared Settings detail pages for desktop and mobile. Registry requests currently support project-level forwarding through `projectId`, and session utilities such as token stats already use the App -> registry -> hub path.

Agent package state is different from project state. Global npm packages are hub-level resources, so this design adds a minimal hub-level forwarding path for a single controlled command method: `cmd.npm`.

The current `codex` and `claude` providers default to `npx --yes <package>`. The new Settings tool intentionally manages global npm packages only, so provider launch must be aligned with that global-binary contract.

## Confirmed Scope

- Add a Web Settings `Update` detail page.
- Move Settings information architecture from `Storage` to `More`.
- Move `Token Stats` and `CC Switch` from `Chat` to `More`.
- Add desktop-only activity bar shortcuts for `Update` and `Token Stats`.
- Add registry support for hub-level `cmd.npm` forwarding by `hubId`.
- Add hub-side `cmd.npm` actions for scan, install, uninstall, and query.
- Hard-code supported and deprecated package allowlists in server code.
- Change `codex` and `claude` provider launch to use global binaries.
- Align macOS deployment ACP npm dependency checks with Windows.
- Update tests across server and app.

## Non-Goals

- Do not add generic command execution.
- Do not accept raw command, raw args, cwd, or env from the client.
- Do not add config-driven package allowlists in the first version.
- Do not let registry persist npm task state or aggregate scan results.
- Do not hot-refresh the hub provider registry after package installation.
- Do not add mobile quick shortcuts outside Settings.
- Do not manage PATH binaries, npx cache, Homebrew, Scoop, or non-npm package sources.
- Do not force-install `@openai/codex` from deployment scripts.
- Do not provide WheelMaker server update controls in this first slice.

## Protocol

Add one hub-level controlled command method:

- `cmd.npm`

The method is available to `client` role through an explicit allowlist. Registry must not allow `cmd.*` by prefix. Unknown `cmd.*` methods remain forbidden or unsupported.

`cmd.npm` is not project-scoped. Each request carries a `hubId` in the payload. Registry validates that the target hub is online, then forwards the request to that hub. Registry does not fan out, does not aggregate results, does not generate task IDs, and does not store task state.

### Payload

```json
{
  "action": "scan|install|uninstall|query",
  "hubId": "dev-hub",
  "packageName": "@openai/codex",
  "version": "latest"
}
```

Rules:

- `scan` requires `action` and `hubId`.
- `query` requires `action` and `hubId`.
- `install` requires `action`, `hubId`, `packageName`, and optional `version`.
- `uninstall` requires `action`, `hubId`, and `packageName`.
- `version` only allows empty or `latest` in the first version.
- `packageName` must match a server hard-coded allowlist.

### Scan Response

`scan` returns a complete hub snapshot that the UI can render directly:

```json
{
  "ok": true,
  "updatedAt": "2026-05-19T10:00:00Z",
  "hub": {
    "hubId": "dev-hub",
    "online": true,
    "nodeVersion": "v22.11.0",
    "npmVersion": "11.12.1",
    "npmPrefix": "C:\\Users\\you\\scoop\\apps\\nodejs\\current",
    "warning": "",
    "error": "",
    "packages": [
      {
        "packageName": "@openai/codex",
        "displayName": "Codex CLI",
        "agentTypes": ["codexapp"],
        "kind": "runtime",
        "installed": true,
        "installedVersion": "0.129.0",
        "latestVersion": "0.130.0",
        "status": "update_available",
        "error": "",
        "canInstall": true,
        "canUpdate": true,
        "canUninstall": false
      }
    ]
  },
  "task": null
}
```

If `npm list -g --depth=0 --json` fails, the hub scan returns `ok: false` or a hub-level `error`, and `packages` is empty. Failed `node --version`, `npm --version`, or `npm prefix -g` calls do not fail the scan if `npm list` succeeds; they only populate `warning` and leave the affected metadata blank.

### Install And Uninstall Responses

`install` and `uninstall` return immediately after the hub accepts the background task:

```json
{
  "ok": true,
  "accepted": true,
  "task": {
    "running": true,
    "action": "install",
    "packageName": "@openai/codex",
    "version": "latest",
    "status": "running",
    "startedAt": "2026-05-19T10:00:00Z",
    "finishedAt": "",
    "exitCode": null,
    "errorSummary": ""
  }
}
```

If a hub already has a running npm task, it returns `CONFLICT` with a message such as `npm task already running`.

### Query Response

`query` returns the current or most recent hub-local npm task:

```json
{
  "ok": true,
  "task": {
    "running": false,
    "action": "install",
    "packageName": "@openai/codex",
    "version": "latest",
    "status": "succeeded",
    "startedAt": "2026-05-19T10:00:00Z",
    "finishedAt": "2026-05-19T10:00:18Z",
    "exitCode": 0,
    "errorSummary": "",
    "message": "Installed @openai/codex@latest. Restart WheelMaker or start a new agent session for the change to take effect."
  }
}
```

If there is no task history:

```json
{
  "ok": true,
  "task": null
}
```

## Hub NPM Command

Hub owns the actual npm logic and keeps task state in memory only. The retained task state is the current running task or the most recent completed task. Hub restart clears this memory.

### Scan

`scan` reads global npm package state only. It does not inspect PATH binaries or npx cache.

Commands:

- `node --version`
- `npm --version`
- `npm prefix -g`
- `npm list -g --depth=0 --json`
- `npm view <package> version` for runtime allowlist packages

`npm view` calls run concurrently for all runtime allowlist packages. Deprecated packages do not query latest versions.

The hub does not add short per-command timeouts. The request layer gives `cmd.npm` scan a 60 second timeout in the App and registry path. `install`, `uninstall`, and `query` remain short request-response operations because install and uninstall return after task acceptance.

### Install And Update

Runtime packages use one action:

```text
npm install -g <package>@latest
```

This handles both missing packages and stale installed packages. The first version only supports `latest`.

### Uninstall

Deprecated packages can be removed with:

```text
npm uninstall -g <package>
```

The hub validates that the package is in the deprecated allowlist. It does not need to pre-check whether the package is installed. UI only shows `Uninstall` when scan reported the deprecated package as installed.

### Task Concurrency

Each hub allows one npm task at a time. Concurrent `install` or `uninstall` requests on the same hub return `CONFLICT`.

The App disables write buttons while any visible hub task is running as a UX guard. Registry does not enforce cross-hub task locking.

### Error Summary

Failed tasks surface an error summary instead of full npm logs:

- include exit code
- use the last non-empty stderr segment, truncated to 500 characters
- if stderr is empty, use the last non-empty stdout segment
- if both are empty, use `npm command failed with exit code X`

## Package Policy

The first version uses hard-coded server allowlists.

### Runtime Packages

| Package | Display | Agent Types |
| --- | --- | --- |
| `@zed-industries/codex-acp` | `Codex ACP` | `codex` |
| `@agentclientprotocol/claude-agent-acp` | `Claude ACP` | `claude` |
| `@anthropic-ai/claude-code` | `Claude CLI` | `claude` |
| `@openai/codex` | `Codex CLI` | `codexapp` |
| `@github/copilot` | `Copilot CLI` | `copilot` |
| `opencode-ai` | `OpenCode CLI` | `opencode` |

Runtime packages can be installed or updated. They cannot be uninstalled from this UI.

### Deprecated Packages

| Package | Display | Agent Types |
| --- | --- | --- |
| `@zed-industries/claude-agent-acp` | `Deprecated Claude ACP` | `claude` |

Deprecated packages are shown only when globally installed. They can only be uninstalled. They cannot be installed or updated.

### Status Values

Package and task statuses use snake_case:

- `not_installed`
- `up_to_date`
- `update_available`
- `latest_unknown`
- `checking_failed`
- `deprecated`
- `installing`
- `updating`
- `uninstalling`
- `running`
- `succeeded`
- `failed`

## Settings UI

### Information Architecture

Settings sections become:

1. `Appearance`
2. `Chat`
3. `Code Display`
4. `More`

`More` replaces `Storage`.

`CC Switch` and `Token Stats` move from `Chat` to `More`. Existing other Chat settings remain in Chat. The `More` row order is:

1. `Update`
2. `Token Stats`
3. `CC Switch`
4. `Database`
5. `Clear Local Cache`

`Update` is a Settings detail route. The detail page title is `Update`. The first content group title is `Agent Packages`, leaving room for future WheelMaker server update controls in the same page.

### Update Detail

The `Update` detail page:

- loads online hubs from `project.list`
- derives unique hub IDs from online projects
- sorts hub groups by stable `hubId` order
- sends one `cmd.npm` scan request per hub
- renders hub cards with `nodeVersion`, `npmVersion`, and `npmPrefix`
- renders package rows with display name, package name, agent tags, installed version, latest version, status, and action button
- includes a `Refresh` action
- does not auto-refresh on a timer

Opening the page scans all online hubs. Manual refresh scans all online hubs. When an install or uninstall task completes, the App refreshes all online hubs again.

Install, update, and uninstall actions use the existing styled confirmation modal. Confirmation copy includes hub ID, package name, installed version when known, and target action. It does not use `window.confirm`.

Accepted write actions poll `cmd.npm` with `action=query` for the target hub until the task finishes. The UI shows only success/failure state and error summary, not full stdout/stderr.

Successful install or update shows that restarting WheelMaker or starting a new agent session is required for changes to take effect. The page does not provide a restart button.

### Desktop Activity Bar

Desktop gets two shortcut buttons in the left activity bar:

- `Update`, icon `codicon-cloud-download`
- `Token Stats`, icon `codicon-pulse`

They appear in the secondary group below `Refresh` and above `Settings`.

Clicking either shortcut opens Settings and navigates directly to the matching detail route. Active state is detail-specific:

- `Update` button is active when Settings is open on `Update`.
- `Token Stats` button is active when Settings is open on `Token Stats`.
- Settings gear is active for the Settings main list and other Settings detail pages, but not when `Update` or `Token Stats` shortcut pages are active.

Mobile keeps Settings-only navigation and does not get these shortcuts.

## Provider Launch

Align agent launch with global npm package management:

- `codex` launches `codex-acp`
- `claude` launches `claude-agent-acp`
- neither provider defaults to `npx --yes <package>`

If the global binary is missing at hub startup, the provider is not registered. The project agent list hides unavailable providers as it does today.

Package install or update does not hot-refresh the provider registry. The Update page tells users to restart WheelMaker for provider availability re-detection.

`codexapp` continues to launch `codex app-server --listen stdio://` through the global `codex` binary.

## Deployment Scripts

Windows `scripts/refresh_server.ps1` already checks and installs:

- `@zed-industries/codex-acp`
- `@agentclientprotocol/claude-agent-acp`

It also removes deprecated `@zed-industries/claude-agent-acp`. Keep that behavior.

macOS `scripts/refresh_server.sh` must be aligned:

- require `npm`
- no longer require `npx` as a runtime dependency
- remove deprecated `@zed-industries/claude-agent-acp` when present
- install missing `@zed-industries/codex-acp` and `@agentclientprotocol/claude-agent-acp`

Do not force-install `@openai/codex`, `@anthropic-ai/claude-code`, `@github/copilot`, or `opencode-ai` from deployment scripts. Those are managed through the Update page.

## Error Handling

- Missing `hubId`: return `INVALID_ARGUMENT`.
- Unknown or offline hub: return `NOT_FOUND` or `UNAVAILABLE`.
- Unsupported `cmd.*` method: reject at registry allowlist.
- Unsupported `cmd.npm.action`: return `INVALID_ARGUMENT`.
- Unsupported `packageName`: return `FORBIDDEN` or `INVALID_ARGUMENT`.
- Unsupported `version`: return `INVALID_ARGUMENT`.
- Hub task already running: return `CONFLICT`.
- `npm list` failure during scan: return hub-level scan error with no package rows.
- `npm view` failure for one runtime package: keep the hub scan successful and mark that package as `latest_unknown` or `checking_failed` with an error summary.

## Testing Strategy

### Server

Update existing Go test files where practical.

Agent provider tests:

- `codex` resolves `codex-acp` and does not call `npx`.
- `claude` resolves `claude-agent-acp` and does not call `npx`.
- missing binaries make the provider unavailable.

Registry tests:

- client role may call `cmd.npm`.
- unknown `cmd.*` is not allowed by prefix.
- `cmd.npm` forwards by `hubId` without `projectId`.
- missing hub ID returns `INVALID_ARGUMENT`.
- unknown/offline hub returns an error.
- forwarded responses are returned to the original request.

Reporter and hub command tests:

- reporter dispatches `cmd.npm` to the hub npm command handler.
- scan returns runtime and deprecated package rows from fake npm data.
- `npm list` failure produces a hub-level error.
- node/npm/prefix failures become hub warnings when package data is available.
- runtime package install accepts and starts a background task.
- deprecated package uninstall accepts and starts a background task.
- unsupported package names are rejected.
- deprecated packages cannot be installed.
- runtime packages cannot be uninstalled.
- concurrent hub npm tasks return `CONFLICT`.
- query returns `task: null` before any task exists.
- failed task summaries follow the 500-character stderr/stdout fallback rule.

### App

Repository and service tests:

- `cmd.npm` scan/install/uninstall/query payloads are shaped correctly.
- scan uses a 60 second request timeout.
- install/uninstall/query use short request paths.

Settings source-structure tests:

- Settings sections include `More` instead of `Storage`.
- `More` order is `Update`, `Token Stats`, `CC Switch`, `Database`, `Clear Local Cache`.
- `Token Stats` and `CC Switch` are no longer in the Chat section.
- existing non-moved Chat options remain in Chat.
- `settingsDetailView` supports `update`.
- `Update` uses the shared Settings detail shell.
- `Update` detail title is `Update`.
- `Agent Packages` appears as the first Update content group.

Desktop activity bar tests:

- `Update` and `Token Stats` shortcuts are between Refresh and Settings.
- `Update` uses `codicon-cloud-download`.
- `Token Stats` uses `codicon-pulse`.
- shortcut click handlers open Settings detail pages.
- active-state logic keeps Settings gear inactive when the `Update` or `Token Stats` shortcut detail is active.
- mobile does not render these shortcuts.

Update page behavior tests:

- online hubs are derived from `project.list` and sorted by hub ID.
- scan requests are sent per hub.
- hub metadata shows node, npm, and npm prefix values.
- package rows show install/update/uninstall actions according to policy.
- confirmation modal is used for install/update/uninstall.
- accepted tasks trigger query polling.
- task completion triggers all-hub rescan.
- error summaries render without full logs.

## Acceptance Criteria

- Settings main list has a `More` section with the confirmed row order.
- `Update` opens a detail page titled `Update`.
- The Update page displays `Agent Packages` grouped by hub.
- All package management is based on global npm package state only.
- Runtime package names are hard-coded and cannot be supplied arbitrarily.
- Deprecated package uninstall is supported only for hard-coded deprecated packages.
- Hub allows only one npm task at a time.
- Registry remains a simple `hubId` forwarder for `cmd.npm`.
- `cmd.npm` is explicitly allowlisted; generic `cmd.*` is not exposed.
- Desktop activity bar has `Update` and `Token Stats` shortcuts below Refresh and above Settings.
- Mobile does not add shortcut buttons.
- `codex` and `claude` launch global binaries rather than `npx`.
- macOS deployment installs missing Codex/Claude ACP packages like Windows.
- Tests cover protocol, hub command policy, UI IA, shortcuts, and provider launch changes.

