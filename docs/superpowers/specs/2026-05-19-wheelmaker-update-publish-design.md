# WheelMaker Update Publish Design

Date: 2026-05-19
Status: Approved

## Goal

Extend the existing Settings `Update` page so each online hub can report and trigger a full WheelMaker update publish round. The page shows one WheelMaker release version per hub, compares it with the configured remote branch, and can request updater-driven publish by writing a controlled signal.

## Scope

- Show `WheelMaker` above `Agent Packages` on each Update hub card.
- Show current published SHA, latest remote SHA, status, and behind commit count.
- Add `cmd.update` as an explicit client role registry method.
- Keep Registry as a hubId forwarder only; it does not create tasks or store update state.
- Write and read one release manifest at `~/.wheelmaker/release.json`.
- Trigger full update publish by writing `~/.wheelmaker/update-now.signal` with `full-update`.
- Keep `scripts/signal_update_now.ps1` for local AI/dev refresh; it writes a plain signal.

## Non-Goals

- No generic command execution.
- No raw shell command, args, cwd, or env from the App.
- No separate Hub, Monitor, Registry, Server, or Web version rows.
- No restart-only button in the first version.
- No updater availability detection in the App.
- No updater log parsing or running marker; pending state is only signal file existence.

## Release Manifest

Refresh scripts write `~/.wheelmaker/release.json` after successful build, install, and Web publish, before restart. First deploy writes the manifest even when config is newly created and restart is skipped.

```json
{
  "schemaVersion": 1,
  "repo": "D:/Code/WheelMaker",
  "branch": "main",
  "remote": "origin",
  "sha": "abc123",
  "publishedAt": "2026-05-19T10:00:00Z"
}
```

The manifest is not written for start, stop, restart, status, uninstall, or partial refresh flows that skip build, install, or Web publish.

## Protocol

`cmd.update` is available to `client` role through an explicit method allowlist. The App sends:

```json
{
  "action": "query",
  "hubId": "hub-a"
}
```

`query` reads the release manifest. If no manifest exists, it returns `not_published` and still allows `update-publish`. If a signal file exists, it returns `update_pending` without fetching remote state. Otherwise it runs controlled Git commands in `release.repo`:

```text
git fetch --prune <remote> <branch>
git rev-parse <remote>/<branch>
git rev-list --count <release.sha>..<remote>/<branch>
git rev-list --count <remote>/<branch>..<release.sha>
git status --porcelain
```

Git fetch or comparison failures return `ok:false` with `status:"checking_failed"` in the response payload, not a protocol error.

`update-publish` writes the signal:

```text
full-update
2026-05-19T10:00:00Z
```

Success means the signal was written. It does not mean update publish is complete.

## Updater Behavior

The updater consumes `update-now.signal`.

- Signal contains `full-update`: run pull, deps, build, install, Web publish, write release manifest, restart Hub and Monitor.
- Plain signal: skip pull/deps and refresh the current checkout, including build, install, Web publish, write release manifest, and Hub/Monitor restart.
- Updater-driven refresh passes `SkipUpdaterInstall`, so the updater does not replace or restart itself in that round.

## UI

The Update page derives online hubs from `project.list` and queries each hub with `cmd.update` and `cmd.npm`.

Each hub card renders:

1. `WheelMaker`
2. `Agent Packages`

The WheelMaker row shows:

- current published SHA
- latest remote SHA when known
- status label
- behind commit count
- one `Update+Publish` button when the hub can accept the request

Status values are:

- `not_published`
- `checking_failed`
- `update_pending`
- `update_available`
- `up_to_date`
- `ahead_of_remote`
- `diverged`

`update_pending` is determined only by `~/.wheelmaker/update-now.signal` existing on the hub.

## Deployment Flow Alignment

Windows and macOS/Linux refresh scripts now share the same core publish shape:

```text
git pull -> deps -> build -> install -> publish web -> write release.json -> restart hub/monitor
```

Windows `deploy.bat` no longer runs a separate outer Web publish step because `scripts/refresh_server.ps1` owns Web publish internally. macOS/Linux `scripts/refresh_server.sh` follows the same release manifest and Web publish rules.

`update-publish.bat` and `update-publish.sh` only write the `full-update` signal. They do not run refresh directly.

## Acceptance Criteria

- `cmd.update` is explicitly allowlisted and routed by `hubId`.
- `query` uses `release.json` as the published version source.
- `query` fetches remote state only when no update signal is pending.
- `update-publish` writes `full-update` to `update-now.signal`.
- Refresh scripts write `release.json` only after successful Web publish.
- The Update page shows WheelMaker status above agent npm packages.
- The App does not expose arbitrary command execution.
