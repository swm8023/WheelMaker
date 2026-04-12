# Feishu Project Config Expansion Design

**Date:** 2026-04-12
**Scope:** Local WheelMaker runtime configuration in `C:\Users\fjxyy\.wheelmaker\config.json`
**Approach:** Extend the existing `projects` array so `wheelmaker` and `BillBoard` both run as YOLO-enabled Feishu projects under the same hub process

---

## Goals

1. Keep the existing local WheelMaker hub and monitor process model unchanged.
2. Add Feishu bot connectivity for the existing `wheelmaker` project.
3. Add a second managed project for `D:\GithubRepos\BillBoard`.
4. Enable `yolo: true` for both projects.
5. Suppress IM-visible tool activity by blocking `tool` and `tool_call` updates on both projects.

---

## Non-Goals

1. Do not add new server-side configuration fields or environment-variable expansion.
2. Do not change Feishu transport code, routing logic, or message rendering behavior.
3. Do not merge the two projects into one shared Feishu application.
4. Do not block additional update types such as `thought` unless explicitly requested later.

---

## Current State

The current local config contains one project:

1. `wheelmaker`
2. `path = D:\GithubRepos\WheelMaker`
3. `yolo = false`
4. no `feishu` block
5. no `imFilter` block

As a result, the hub starts successfully but only the app channel is available for that project, and `BillBoard` is not managed at all.

---

## Design Summary

Update `C:\Users\fjxyy\.wheelmaker\config.json` so the `projects` array contains two entries:

1. `wheelmaker`
2. `BillBoard`

Each project entry will explicitly define:

1. `name`
2. `path`
3. `yolo: true`
4. `feishu.app_id`
5. `feishu.app_secret`
6. `imFilter.block = ["tool", "tool_call"]`

The shared `registry`, `monitor`, and `log` sections remain unchanged.

---

## Target Config Shape

```json
{
  "registry": {
    "listen": true,
    "port": 9630,
    "server": "127.0.0.1",
    "hubId": "LITTLECLAW",
    "token": ""
  },
  "projects": [
    {
      "path": "D:\\GithubRepos\\WheelMaker",
      "name": "wheelmaker",
      "yolo": true,
      "feishu": {
        "app_id": "cli_a951a49417f9dbd2",
        "app_secret": "<wheelmaker secret>"
      },
      "imFilter": {
        "block": ["tool", "tool_call"]
      }
    },
    {
      "path": "D:\\GithubRepos\\BillBoard",
      "name": "BillBoard",
      "yolo": true,
      "feishu": {
        "app_id": "cli_a951abb004fddbc9",
        "app_secret": "<billboard secret>"
      },
      "imFilter": {
        "block": ["tool", "tool_call"]
      }
    }
  ],
  "monitor": {
    "port": 9631
  },
  "log": {
    "level": "warn"
  }
}
```

Secrets are shown as placeholders in the spec only; the actual runtime config will contain the real values provided by the user.

---

## Runtime Behavior

After the config update and process restart:

1. The hub will build one client for `wheelmaker` and one client for `BillBoard`.
2. Each client will register a Feishu channel because both projects now satisfy `HasFeishu()`.
3. Each project will receive inbound and outbound traffic through its own Feishu app credentials.
4. Tool-related IM updates will be filtered before they are emitted to Feishu.
5. Existing registry and monitor behavior will continue to work without modification.

---

## Validation Plan

Validation for this change is operational rather than code-based:

1. Confirm the local config parses as valid JSON after editing.
2. Restart WheelMaker so it reloads `config.json`.
3. Verify the process remains running after restart.
4. Inspect logs or runtime behavior to confirm two projects are registered.
5. Send a Feishu message to each bot and verify the corresponding project responds.
6. Confirm tool update noise does not appear in Feishu chats.

---

## Risks And Mitigations

1. A malformed JSON edit would prevent startup.
   Mitigation: validate the file structure before restart.
2. Incorrect Feishu credentials would register the project but fail at runtime connection or send paths.
   Mitigation: verify with a real inbound message after restart.
3. Blocking only `tool` without `tool_call` could leave partial tool noise visible.
   Mitigation: block both values together.

---

## Out Of Scope Follow-Ups

1. Move Feishu secrets from plain JSON into environment-based configuration.
2. Add first-class secret management to WheelMaker config loading.
3. Extend IM filtering defaults for `thought` or other update classes.
