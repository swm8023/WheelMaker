# Skills Settings Management Design

Date: 2026-05-20
Status: Approved

## Goal

Add a Settings `Skills` detail page that manages installed agent skills across connected WheelMaker Hubs and their Projects using the upstream `skills` CLI model.

The page should show Hub-global skills first, then each Project's installed skills, grouped by upstream skill category. It should support installing skills from a remote source, uninstalling installed skills, and updating all skills in a Hub or Project scope.

## Context

WheelMaker already reports agent-visible skills in `ProjectAgentProfile`, but that scan only describes what providers can see. Skill management needs the upstream install/list/remove/update model, including source lock data and plugin category grouping.

The upstream `skills` CLI exposes the relevant operations:

- `skills list` for installed project skills
- `skills list -g` for installed global skills
- `skills add <source> --list` for source discovery
- `skills add <source> ...` for install
- `skills remove ...` for uninstall
- `skills update` for scope update

The CLI's `--json` list output does not include category. Category comes from the CLI's `pluginName` grouping, written from `.claude-plugin/plugin.json` or `.claude-plugin/marketplace.json` during install and read from the skill lock files during list.

## Confirmed Scope

- Add a Web Settings `Skills` detail page.
- Add a desktop-only activity bar shortcut for `Skills` below `Update`.
- Add `Skills` to the Settings `More` section below `Update`.
- Add registry support for hub-level `cmd.skills` forwarding by `hubId`.
- Add hub-side `cmd.skills` actions: `scan`, `list`, `install`, `uninstall`, and `update`.
- Use upstream `skills list` semantics as the Skills Page source of truth.
- Return structured skill categories from Hub responses instead of parsing CLI text in the App.
- Show Hub Skills first, then Project Skills grouped by Project.
- Group skill rows by Skill Category within each scope.
- Use fixed install target agents: Codex, Claude Code, OpenCode, and GitHub Copilot.
- Default to upstream symlink installation behavior.
- Make skill operations synchronous: Hub waits for completion and returns the final result.

## Non-Goals

- Do not expose raw shell command, args, cwd, or env.
- Do not allow arbitrary local paths, arbitrary Git URLs, or SSH Git URLs as install sources.
- Do not add a skills.sh marketplace browser in the first version.
- Do not support per-agent install or per-agent unlink.
- Do not support category-level update.
- Do not make Registry persist task state, aggregate results, or poll operation status.
- Do not replace `ProjectAgentProfile.skills`; those remain provider visibility data for project display.
- Do not hot-refresh agent provider registries after skill install or uninstall.

## Protocol

Add one client-role controlled hub command:

- `cmd.skills`

Registry explicitly allowlists `cmd.skills`; it must not allow `cmd.*` by prefix. Every request carries `payload.hubId`. Registry validates the target hub is known and online, then forwards the payload to that Hub. Registry does not inspect scope-specific fields beyond `hubId`.

### Payload

```json
{
  "action": "scan|list|install|uninstall|update",
  "hubId": "hub-a",
  "scope": "hub|project",
  "projectName": "WheelMaker",
  "source": "mattpocock/skills",
  "skills": ["tdd", "diagnose"]
}
```

Rules:

- `scan` requires `action` and `hubId`.
- `list` requires `action`, `hubId`, and `source`.
- `install` requires `action`, `hubId`, `scope`, `source`, and one or more `skills`.
- `uninstall` requires `action`, `hubId`, `scope`, and one or more `skills`.
- `update` requires `action`, `hubId`, and `scope`.
- `projectName` is required when `scope` is `project`.
- `scope` defaults are not inferred for write actions.
- `source` only accepts GitHub `owner/repo`, GitHub HTTPS repository URLs, or well-known HTTPS skill endpoints.
- `skills` are skill names, not paths.
- App never sends filesystem paths, raw command args, cwd, or env.

### Scan Response

`scan` returns installed inventory for one Hub:

```json
{
  "ok": true,
  "hubId": "hub-a",
  "updatedAt": "2026-05-20T10:00:00Z",
  "hubSkills": {
    "scope": "hub",
    "skills": [
      {
        "name": "tdd",
        "path": "C:/Users/me/.agents/skills/tdd",
        "category": "Mattpocock Skills",
        "categoryKey": "mattpocock-skills",
        "agents": ["Codex", "Claude Code", "GitHub Copilot", "OpenCode"]
      }
    ]
  },
  "projects": [
    {
      "projectName": "WheelMaker",
      "projectId": "hub-a:WheelMaker",
      "online": true,
      "path": "D:/Code/WheelMaker",
      "skills": [
        {
          "name": "diagnose",
          "path": "D:/Code/WheelMaker/.agents/skills/diagnose",
          "category": "Mattpocock Skills",
          "categoryKey": "mattpocock-skills",
          "agents": ["Codex", "Claude Code", "GitHub Copilot", "OpenCode"]
        }
      ],
      "error": ""
    }
  ],
  "message": ""
}
```

If one Project scan fails, the Hub scan remains `ok:true` and that Project carries `error`. If Hub-global scan fails, return `ok:false` with `errorSummary`.

### Source List Response

`list` returns installable candidates from one Remote Skill Source:

```json
{
  "ok": true,
  "hubId": "hub-a",
  "source": "mattpocock/skills",
  "skills": [
    {
      "name": "tdd",
      "description": "Test-driven development with red-green-refactor loop.",
      "category": "Mattpocock Skills",
      "categoryKey": "mattpocock-skills"
    }
  ],
  "message": ""
}
```

### Install, Uninstall, And Update Responses

Write actions are synchronous and return the changed scope snapshot:

```json
{
  "ok": true,
  "hubId": "hub-a",
  "scope": "project",
  "projectName": "WheelMaker",
  "updatedAt": "2026-05-20T10:00:00Z",
  "skills": [
    {
      "name": "tdd",
      "category": "Mattpocock Skills",
      "categoryKey": "mattpocock-skills",
      "agents": ["Codex", "Claude Code", "GitHub Copilot", "OpenCode"]
    }
  ],
  "message": "Installed 1 skill"
}
```

CLI failures return `ok:false` with `errorSummary` truncated to 500 characters. Protocol errors are reserved for invalid arguments, forbidden source shapes, unknown projects, offline hubs, and internal command execution failures.

## Hub Command Behavior

Hub owns the `cmd.skills` command handler. It runs the upstream CLI in controlled ways only.

Command mapping:

- Hub scan: `skills list -g --json`
- Project scan: `skills list --json` with process cwd set to the Project path
- Source list: `skills add <source> --list`
- Hub install: `skills add <source> -g --agent codex claude-code opencode github-copilot --skill <names...> -y`
- Project install: same command without `-g`, run in the Project cwd
- Hub uninstall: `skills remove -g --skill <names...> --agent '*' -y`
- Project uninstall: same command without `-g`, run in the Project cwd
- Hub update: `skills update -g -y`
- Project update: `skills update -p -y`, run in the Project cwd

The handler should resolve the CLI as `skills` when available, with a fallback to `npx --yes skills`. The fallback is still controlled: the App never chooses the executable or arguments.

Category derivation:

- For installed global skills, read `~/.agents/.skill-lock.json` or `$XDG_STATE_HOME/skills/.skill-lock.json`.
- For installed project skills, read `<project>/skills-lock.json`.
- Use `skills[skillName].pluginName` as `categoryKey`.
- Convert category keys to title case for `category`.
- If no `pluginName` exists, use `General` and `general`.

The source list action may use CLI text output only inside the Hub handler because upstream does not provide JSON for `add --list`. The parser must strip ANSI codes and recognize plugin group headings. If parsing fails, return `ok:false` rather than exposing raw text.

## Settings UI

Settings `More` row order becomes:

1. `Update`
2. `Skills`
3. `Token Stats`
4. `CC Switch`
5. `Database`
6. `Clear Local Cache`

Desktop activity bar secondary order becomes:

1. Refresh
2. Update
3. Skills
4. Token Stats
5. Settings

Mobile keeps Settings-only navigation and does not add shortcut buttons.

The `Skills` detail page:

- loads hubs from `project.list.hubs`
- sends one `cmd.skills` `scan` request per hub
- sorts hubs by `hubId`
- renders each hub with a Hub Skills section first
- renders each Project below Hub Skills
- groups rows by Skill Category
- shows skill name, path when useful, linked agents, and row actions
- has a Refresh action
- has an Add action for Hub scope and each Project scope
- uses the shared confirmation dialog for install, uninstall, and update
- disables Project scope actions for offline Projects
- shows short `errorSummary` values, not raw stdout/stderr

Install flow:

1. User chooses target scope from the Hub or Project Add action.
2. User enters a Remote Skill Source.
3. App calls `cmd.skills` `list`.
4. User selects one or more candidates.
5. App confirms installation with source, target scope, selected skill names, and fixed target agents.
6. App calls `cmd.skills` `install`.
7. App updates the changed scope from the response, then refreshes the hub scan if needed.

Update flow:

- Hub `Update All` calls `update` for Hub scope and each online Project scope.
- Hub Skills section `Update` calls `update` with `scope:"hub"`.
- Project section `Update` calls `update` with `scope:"project"` and `projectName`.

Uninstall flow:

- Each skill row has `Uninstall`.
- Confirmation explains that the skill will be removed from all fixed Skill Target Agents in that scope.
- App calls `cmd.skills` `uninstall` with the selected skill name.

## Error Handling

- Missing `hubId`: `INVALID_ARGUMENT`.
- Offline or unknown hub: `UNAVAILABLE` or `NOT_FOUND`.
- Missing or unsupported action: `INVALID_ARGUMENT`.
- Unsupported source shape: `FORBIDDEN` or `INVALID_ARGUMENT`.
- Missing project for Project scope: `INVALID_ARGUMENT`.
- Unknown project: `NOT_FOUND`.
- Project path missing: `INTERNAL`.
- CLI executable missing and `npx` fallback unavailable: `INTERNAL` with `errorSummary`.
- CLI failure: response payload `ok:false`, preserving the command contract but not raw logs.

## Testing Strategy

### Server

- Registry allowlists `cmd.skills` for client role.
- Unknown `cmd.*` remains forbidden.
- `cmd.skills` forwards by `hubId` without `projectId`.
- Missing `hubId` returns `INVALID_ARGUMENT`.
- Unknown/offline hub returns error.
- Hub command scan combines global and project scopes.
- Hub command reads `pluginName` from lock files and returns category/categoryKey.
- Hub command list parses grouped `skills add <source> --list` output.
- Source shape validation rejects local paths, SSH, and arbitrary Git URLs.
- Install uses fixed target agents and symlink default.
- Uninstall removes from all fixed target agents.
- Update uses scope-level commands.
- CLI failures return truncated `errorSummary`.

### App

- Repository methods send controlled `cmd.skills` payloads.
- `scan` and `list` use bounded timeouts.
- Settings `More` includes `Skills` after `Update`.
- `settingsDetailView` supports `skills`.
- Desktop activity bar includes `Skills` between `Update` and `Token Stats`.
- Mobile activity controls do not include `Skills`.
- Skills detail scans hubs from `project.list.hubs`.
- Skills rows are grouped by category.
- Install, uninstall, and update use the shared confirmation dialog.
- Project actions are disabled for offline Projects.

## Acceptance Criteria

- `cmd.skills` is explicitly allowlisted and routed by `hubId`.
- Registry remains a forwarder and does not persist skill operation state.
- Hub exposes synchronous `scan`, `list`, `install`, `uninstall`, and `update` actions.
- The App never sends raw command, args, cwd, env, or filesystem paths.
- Source installation only accepts Remote Skill Sources.
- Install always targets Codex, Claude Code, OpenCode, and GitHub Copilot.
- Installed inventory is based on upstream `skills list` semantics, not provider skill scans.
- Skill categories match upstream pluginName grouping where available.
- The Settings `Skills` detail page manages Hub and Project skills separately from `Update`.
- Full stdout/stderr is not displayed in the App.
