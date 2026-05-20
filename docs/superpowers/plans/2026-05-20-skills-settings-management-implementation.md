# Skills Settings Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Settings `Skills` detail page and controlled `cmd.skills` Hub command for managing Hub and Project skills.

**Architecture:** Registry exposes `cmd.skills` as an explicitly allowlisted hub-level command and forwards requests by `hubId`. Hub owns a synchronous command handler that invokes the upstream `skills` CLI through a controlled runner and returns structured summaries. The Web app scans hubs, renders installed Hub and Project skills grouped by category, and uses shared confirmation flows for install, uninstall, and update.

**Tech Stack:** Go Registry/Hub server, upstream `skills` npm CLI, React/TypeScript Web app, Jest source-structure tests, Go unit tests.

---

## File Structure

- `docs/superpowers/specs/2026-05-20-skills-settings-management-design.md` records the approved design.
- `docs/adr/2026-05-20-cmd-skills-controlled-skill-management-command.md` records the controlled-command architectural decision.
- `server/internal/registry/server.go` allowlists and routes `cmd.skills`.
- `server/internal/registry/server_test.go` covers registry forwarding and method allowlisting.
- `server/internal/hub/skills_command.go` contains payload/response types, source validation, CLI runner abstraction, category lock readers, ANSI stripping, source-list parsing, and action handlers.
- `server/internal/hub/skills_command_test.go` covers Hub command behavior with a fake runner and temp project/global skill roots.
- `server/internal/hub/reporter.go` dispatches registry `cmd.skills` requests to `SkillsCommand`.
- `server/internal/hub/reporter_test.go` covers reporter dispatch and project lookup.
- `server/internal/protocol/registry.go` adds protocol structs only if shared types are needed by multiple packages; otherwise keep command-private structs in `skills_command.go`.
- `docs/registry-protocol.md` documents `cmd.skills`.
- `app/web/src/types/registry.ts` adds `RegistrySkill*` types.
- `app/web/src/services/registryRepository.ts` adds controlled `cmd.skills` request methods.
- `app/web/src/services/registryWorkspaceService.ts` exposes those methods to UI code.
- `app/web/src/skillManagementView.ts` adds pure helpers for sorting hubs/projects, grouping skills, labels, and response normalization helpers.
- `app/web/src/main.tsx` adds Skills detail state, actions, rendering, confirmation targets, and desktop shortcut.
- `app/web/src/styles.css` adds compact Skills page styles using existing Settings detail visual language.
- `app/__tests__/web-skill-management-service.test.ts` covers repository/service payloads and timeouts.
- `app/__tests__/web-skill-management-view.test.ts` covers pure view helpers.
- `app/__tests__/web-skill-management-settings.test.ts` covers Settings IA, shortcuts, and source-structure hooks.

---

### Task 1: Registry `cmd.skills` Forwarding

**Files:**
- Modify: `server/internal/registry/server.go`
- Modify: `server/internal/registry/server_test.go`
- Modify: `docs/registry-protocol.md`

- [ ] **Step 1: Write failing registry allowlist tests**

Add tests to `server/internal/registry/server_test.go` near the existing `cmd.npm` and `cmd.update` tests:

```go
func TestCmdSkillsForwardsByHubIDWithoutProjectID(t *testing.T) {
	s := NewServer(Config{Token: "tok"})
	hubPeer, hubState := newTestPeer(t, "hub", "hub-a")
	clientPeer, clientState := newTestPeer(t, "client", "")
	s.hubPeers["hub-a"] = hubPeer
	s.hubs["hub-a"] = rp.HubSnapshot{HubID: "hub-a"}

	req := envelope{
		RequestID: 11,
		Type:      "request",
		Method:    "cmd.skills",
		Payload:   rp.MustRaw(map[string]any{"action": "scan", "hubId": "hub-a"}),
	}
	go s.handleRequest(clientPeer, clientState, req)

	forwarded := readTestEnvelope(t, hubPeer)
	if forwarded.Type != "request" || forwarded.Method != "cmd.skills" {
		t.Fatalf("forwarded=%#v, want cmd.skills request", forwarded)
	}
	if forwarded.ProjectID != "" {
		t.Fatalf("forwarded projectId=%q, want empty", forwarded.ProjectID)
	}
	writeTestEnvelope(t, hubPeer, envelope{
		RequestID: forwarded.RequestID,
		Type:      "response",
		Method:    "cmd.skills",
		Payload:   rp.MustRaw(map[string]any{"ok": true, "hubId": "hub-a"}),
	})

	resp := readTestEnvelope(t, clientPeer)
	if resp.Type != "response" || resp.Method != "cmd.skills" {
		t.Fatalf("client response=%#v, want cmd.skills response", resp)
	}
	_ = hubState
}

func TestUnknownCmdMethodStillForbiddenAfterCmdSkills(t *testing.T) {
	if methodAllowed("client", "cmd.shell") {
		t.Fatal("cmd.shell should not be allowed by prefix")
	}
	if !methodAllowed("client", "cmd.skills") {
		t.Fatal("cmd.skills should be explicitly allowed")
	}
}
```

Use the helper names already present in `server_test.go`; if their names differ, adapt the test body to the existing `cmd.npm` test pattern rather than creating duplicate helpers.

- [ ] **Step 2: Run registry tests and verify failure**

Run:

```powershell
cd server
go test ./internal/registry -run "CmdSkills|UnknownCmd" -count=1
```

Expected: FAIL because `cmd.skills` is not allowlisted or routed.

- [ ] **Step 3: Implement registry forwarding**

In `server/internal/registry/server.go`, update the main request switch:

```go
case "cmd.npm", "cmd.update", "cmd.skills":
	s.handleHubCommandForwardRequest(state.peer, state, in)
```

Update `methodAllowed`:

```go
case "client":
	return method == "project.list" || method == "project.syncCheck" || method == "batch" ||
		method == "cmd.npm" || method == "cmd.update" || method == "cmd.skills" ||
		method == "chat.send" || strings.HasPrefix(method, "session.") ||
		strings.HasPrefix(method, "fs.") || strings.HasPrefix(method, "git.")
```

Keep `executeHubCommandRequest` generic. Its existing `scan` timeout applies to `cmd.skills` `scan`; this is acceptable. If source `list` needs longer than the default timeout, add:

```go
if in.Method == "cmd.skills" && (strings.TrimSpace(payload.Action) == "scan" || strings.TrimSpace(payload.Action) == "list") {
	timeout = 60 * time.Second
}
```

- [ ] **Step 4: Document protocol**

Add `cmd.skills` to the client role row and create a new `5.12.3 cmd.skills` subsection in `docs/registry-protocol.md` with the payload and response shapes from `docs/superpowers/specs/2026-05-20-skills-settings-management-design.md`.

- [ ] **Step 5: Verify registry**

Run:

```powershell
cd server
go test ./internal/registry -run "CmdSkills|UnknownCmd|HubCommand" -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add server/internal/registry/server.go server/internal/registry/server_test.go docs/registry-protocol.md
git commit -m "feat: forward controlled skills command"
```

---

### Task 2: Hub Skills Command Handler

**Files:**
- Create: `server/internal/hub/skills_command.go`
- Create: `server/internal/hub/skills_command_test.go`
- Modify: `server/internal/hub/reporter.go`
- Modify: `server/internal/hub/reporter_test.go`

- [ ] **Step 1: Write failing Hub command tests**

Create `server/internal/hub/skills_command_test.go` with a fake runner:

```go
package hub

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeSkillsRunner struct {
	calls   []skillsCommandCall
	results map[string]skillsCommandResult
}

func (f *fakeSkillsRunner) set(dir string, name string, args []string, result skillsCommandResult) {
	if f.results == nil {
		f.results = map[string]skillsCommandResult{}
	}
	f.results[skillsCallKey(dir, name, args)] = result
}

func (f *fakeSkillsRunner) Run(_ context.Context, dir string, name string, args ...string) skillsCommandResult {
	call := skillsCommandCall{Dir: dir, Name: name, Args: append([]string(nil), args...)}
	f.calls = append(f.calls, call)
	if f.results != nil {
		if result, ok := f.results[skillsCallKey(dir, name, args)]; ok {
			return result
		}
	}
	return skillsCommandResult{ExitCode: 0}
}

func skillsCallKey(dir string, name string, args []string) string {
	return strings.Join(append([]string{dir, name}, args...), "\x00")
}

func TestSkillsCommandScanReturnsHubAndProjectSkillsWithCategories(t *testing.T) {
	home := t.TempDir()
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".skill-lock.json"), []byte(`{"version":3,"skills":{"tdd":{"pluginName":"mattpocock-skills"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "skills-lock.json"), []byte(`{"version":1,"skills":{"diagnose":{"pluginName":"mattpocock-skills"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeSkillsRunner{}
	runner.set("", "skills", []string{"list", "-g", "--json"}, skillsCommandResult{
		Stdout: `[{"name":"tdd","path":"` + filepath.ToSlash(filepath.Join(home, "skills", "tdd")) + `","scope":"global","agents":["Codex"]}]`,
	})
	runner.set(projectRoot, "skills", []string{"list", "--json"}, skillsCommandResult{
		Stdout: `[{"name":"diagnose","path":"` + filepath.ToSlash(filepath.Join(projectRoot, ".agents", "skills", "diagnose")) + `","scope":"project","agents":["Codex","Claude Code"]}]`,
	})
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{
		HubID:          "hub-a",
		GlobalLockPath: filepath.Join(home, ".skill-lock.json"),
		Projects: []ProjectInfo{{Name: "WheelMaker", Path: projectRoot, Online: true}},
	})

	respAny, cmdErr := cmd.Handle(context.Background(), rawSkillsPayload(t, map[string]any{
		"action": "scan",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle scan err=%v", cmdErr)
	}
	resp := respAny.(skillsCommandResponse)
	if !resp.OK || len(resp.HubSkills.Skills) != 1 || len(resp.Projects) != 1 {
		t.Fatalf("scan response=%#v", resp)
	}
	if resp.HubSkills.Skills[0].Category != "Mattpocock Skills" || resp.HubSkills.Skills[0].CategoryKey != "mattpocock-skills" {
		t.Fatalf("hub category=%#v", resp.HubSkills.Skills[0])
	}
	if resp.Projects[0].Skills[0].Name != "diagnose" {
		t.Fatalf("project skills=%#v", resp.Projects[0].Skills)
	}
}
```

Add focused tests for:

- `list` parses grouped source output and returns candidates.
- `install` uses fixed agents and no `--copy`.
- `uninstall` uses `--agent '*'`.
- `update` uses `-g` for Hub and `-p` for Project.
- source validation rejects `../local`, `git@github.com:a/b.git`, and `https://example.com/repo.git`.
- CLI failure returns `ok:false` with `errorSummary` no longer than 500 characters.
- unknown Project scope returns `NOT_FOUND`.

- [ ] **Step 2: Run Hub command tests and verify failure**

Run:

```powershell
cd server
go test ./internal/hub -run "SkillsCommand|CmdSkills" -count=1
```

Expected: FAIL because `SkillsCommand` does not exist.

- [ ] **Step 3: Implement command types and runner**

Create `server/internal/hub/skills_command.go` with these core types:

```go
type skillsCommandCall struct {
	Dir  string
	Name string
	Args []string
}

type skillsCommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

type skillsCommandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) skillsCommandResult
}

type execSkillsCommandRunner struct{}
```

`execSkillsCommandRunner.Run` should use `exec.CommandContext`, set `cmd.Dir = dir` when non-empty, capture stdout/stderr, and map `exec.ExitError.ExitCode()` exactly like `npm_command.go`.

Add fallback execution in a helper:

```go
func (c *SkillsCommand) runSkills(ctx context.Context, dir string, args ...string) skillsCommandResult {
	result := c.runner.Run(ctx, dir, "skills", args...)
	if !commandUnavailable(result) {
		return result
	}
	npxArgs := append([]string{"--yes", "skills"}, args...)
	return c.runner.Run(ctx, dir, "npx", npxArgs...)
}
```

`commandUnavailable` should treat `exec.ErrNotFound` and exit code `-1` with an executable lookup error as unavailable; fake tests can assert only that the first command is `skills`.

- [ ] **Step 4: Implement payload and response structs**

Use command-private structs:

```go
type skillsCommandPayload struct {
	Action      string   `json:"action"`
	HubID       string   `json:"hubId"`
	Scope       string   `json:"scope,omitempty"`
	ProjectName string   `json:"projectName,omitempty"`
	Source      string   `json:"source,omitempty"`
	Skills      []string `json:"skills,omitempty"`
}

type skillsCommandResponse struct {
	OK           bool                  `json:"ok"`
	HubID        string                `json:"hubId"`
	UpdatedAt    string                `json:"updatedAt,omitempty"`
	Source       string                `json:"source,omitempty"`
	Scope        string                `json:"scope,omitempty"`
	ProjectName  string                `json:"projectName,omitempty"`
	HubSkills    skillsScopeSnapshot   `json:"hubSkills,omitempty"`
	Projects     []skillsProjectSnapshot `json:"projects,omitempty"`
	Skills       []skillsSkillSnapshot `json:"skills,omitempty"`
	Candidates   []skillsSourceCandidate `json:"candidates,omitempty"`
	Message      string                `json:"message,omitempty"`
	ErrorSummary string                `json:"errorSummary,omitempty"`
}
```

If using `skills` for both installed skill snapshots and source candidates feels confusing, keep `Skills` for installed scope snapshots and `Candidates` for source list responses.

- [ ] **Step 5: Implement validation**

Implement:

```go
func validateRemoteSkillSource(source string) (string, *skillsCommandError)
```

Accept:

- `owner/repo` with `^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`
- `https://github.com/owner/repo` and optional `.git`
- `https://.../.well-known/agent-skills/...` or `https://.../.well-known/skills/...`

Reject local/relative/absolute paths, `git@`, `ssh://`, `file://`, and non-GitHub HTTPS Git URLs.

- [ ] **Step 6: Implement scan**

For Hub scope:

```go
result := c.runSkills(ctx, "", "list", "-g", "--json")
```

For each Project:

```go
result := c.runSkills(ctx, project.Path, "list", "--json")
```

Parse JSON:

```go
type skillsCLIListItem struct {
	Name   string   `json:"name"`
	Path   string   `json:"path"`
	Scope  string   `json:"scope"`
	Agents []string `json:"agents"`
}
```

Read categories from lock files:

- global: `GlobalLockPath` when set, otherwise `$XDG_STATE_HOME/skills/.skill-lock.json` or `~/.agents/.skill-lock.json`
- project: `<project.Path>/skills-lock.json`

Lock shape:

```go
type skillsLockFile struct {
	Skills map[string]struct {
		PluginName string `json:"pluginName"`
	} `json:"skills"`
}
```

Convert `mattpocock-skills` to `Mattpocock Skills`. Missing pluginName becomes `General` / `general`.

- [ ] **Step 7: Implement source list parser**

Run:

```go
result := c.runSkills(ctx, "", "add", source, "--list")
```

Strip ANSI and spinner control sequences with a helper based on `regexp`:

```go
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
```

Parse the cleaned output line-by-line:

- ignore `Available Skills`, `Source:`, `Found`, and blank lines
- a non-indented title line before skill rows is the current category
- skill lines are names without spaces, followed by description lines
- default category is `General`

Return `ok:false` when no candidates are parsed from a successful command.

- [ ] **Step 8: Implement write actions**

Install args:

```go
args := []string{"add", source}
if scope == "hub" {
	args = append(args, "-g")
}
args = append(args,
	"--agent", "codex", "claude-code", "opencode", "github-copilot",
	"--skill",
)
args = append(args, payload.Skills...)
args = append(args, "-y")
```

Uninstall args:

```go
args := []string{"remove"}
if scope == "hub" {
	args = append(args, "-g")
}
args = append(args, "--skill")
args = append(args, payload.Skills...)
args = append(args, "--agent", "*", "-y")
```

Update args:

```go
args := []string{"update", "-y"}
if scope == "hub" {
	args = append(args, "-g")
} else {
	args = append(args, "-p")
}
```

After successful write, rescan only the changed scope and return that scope snapshot.

- [ ] **Step 9: Wire Reporter dispatch**

In `Reporter` add:

```go
skillsCommand *SkillsCommand
```

Initialize in `NewReporter`:

```go
skillsCommand: NewSkillsCommand(skillsCommandConfig{
	HubID: cfg.HubID,
	Projects: cp,
}),
```

When `UpdateProject` changes projects, refresh the command's project list or have `replyCmdSkills` construct/update the handler config from `r.projects`.

Add to `handleRegistryRequest`:

```go
case "cmd.skills":
	r.replyCmdSkills(conn, in)
```

Add `replyCmdSkills` mirroring `replyCmdNPM`.

- [ ] **Step 10: Verify Hub tests**

Run:

```powershell
cd server
go test ./internal/hub -run "SkillsCommand|CmdSkills|ReporterRespondsToCmdSkills" -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit**

```powershell
git add server/internal/hub/skills_command.go server/internal/hub/skills_command_test.go server/internal/hub/reporter.go server/internal/hub/reporter_test.go
git commit -m "feat: add controlled skills hub command"
```

---

### Task 3: App Types, Repository, And Pure Helpers

**Files:**
- Modify: `app/web/src/types/registry.ts`
- Modify: `app/web/src/services/registryRepository.ts`
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Create: `app/web/src/skillManagementView.ts`
- Create: `app/__tests__/web-skill-management-service.test.ts`
- Create: `app/__tests__/web-skill-management-view.test.ts`

- [ ] **Step 1: Write failing service tests**

Create `app/__tests__/web-skill-management-service.test.ts`:

```ts
import {RegistryRepository} from '../web/src/services/registryRepository';
import type {RegistryClient} from '../web/src/services/registryClient';

describe('skill management registry service', () => {
  test('sends cmd.skills scan with hubId and bounded timeout', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, hubId: 'hub-a', hubSkills: {scope: 'hub', skills: []}, projects: []},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.scanSkills('hub-a');

    expect(client.request).toHaveBeenCalledWith({
      method: 'cmd.skills',
      payload: {action: 'scan', hubId: 'hub-a'},
      timeoutMs: 60000,
    });
  });

  test('sends cmd.skills source list with controlled source payload', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, hubId: 'hub-a', source: 'mattpocock/skills', candidates: []},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.listSkillsSource('hub-a', 'mattpocock/skills');

    expect(client.request).toHaveBeenCalledWith({
      method: 'cmd.skills',
      payload: {action: 'list', hubId: 'hub-a', source: 'mattpocock/skills'},
      timeoutMs: 60000,
    });
  });

  test('sends install uninstall and update without paths or raw args', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({type: 'response', payload: {ok: true, hubId: 'hub-a', skills: []}}),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.installSkills({hubId: 'hub-a', scope: 'project', projectName: 'WheelMaker', source: 'mattpocock/skills', skills: ['tdd']});
    await repository.uninstallSkills({hubId: 'hub-a', scope: 'hub', skills: ['tdd']});
    await repository.updateSkills({hubId: 'hub-a', scope: 'project', projectName: 'WheelMaker'});

    expect(client.request).toHaveBeenNthCalledWith(1, {
      method: 'cmd.skills',
      payload: {action: 'install', hubId: 'hub-a', scope: 'project', projectName: 'WheelMaker', source: 'mattpocock/skills', skills: ['tdd']},
      timeoutMs: 60000,
    });
    expect(client.request).toHaveBeenNthCalledWith(2, {
      method: 'cmd.skills',
      payload: {action: 'uninstall', hubId: 'hub-a', scope: 'hub', skills: ['tdd']},
      timeoutMs: 60000,
    });
    expect(client.request).toHaveBeenNthCalledWith(3, {
      method: 'cmd.skills',
      payload: {action: 'update', hubId: 'hub-a', scope: 'project', projectName: 'WheelMaker'},
      timeoutMs: 60000,
    });
  });
});
```

- [ ] **Step 2: Write failing helper tests**

Create `app/__tests__/web-skill-management-view.test.ts`:

```ts
import {
  deriveSkillHubIds,
  groupSkillsByCategory,
  skillScopeLabel,
  sortSkillProjects,
} from '../web/src/skillManagementView';

describe('skill management view helpers', () => {
  test('derives sorted hub ids from project.list hubs', () => {
    expect(deriveSkillHubIds([{hubId: 'hub-b'}, {hubId: ' '}, {hubId: 'hub-a'}])).toEqual(['hub-a', 'hub-b']);
  });

  test('groups skills by upstream category and keeps General last', () => {
    const groups = groupSkillsByCategory([
      {name: 'plain', category: '', categoryKey: '', agents: []},
      {name: 'tdd', category: 'Mattpocock Skills', categoryKey: 'mattpocock-skills', agents: []},
    ]);
    expect(groups.map(group => group.category)).toEqual(['Mattpocock Skills', 'General']);
    expect(groups[0].skills[0].name).toBe('tdd');
  });

  test('sorts projects by online state then name', () => {
    expect(sortSkillProjects([
      {projectName: 'zeta', online: false, skills: []},
      {projectName: 'alpha', online: true, skills: []},
    ]).map(project => project.projectName)).toEqual(['alpha', 'zeta']);
  });

  test('formats scope labels', () => {
    expect(skillScopeLabel({scope: 'hub', hubId: 'hub-a'})).toBe('Hub: hub-a');
    expect(skillScopeLabel({scope: 'project', hubId: 'hub-a', projectName: 'WheelMaker'})).toBe('Project: WheelMaker');
  });
});
```

- [ ] **Step 3: Run app tests and verify failure**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-skill-management-service.test.ts __tests__/web-skill-management-view.test.ts --runInBand
```

Expected: FAIL because types and methods do not exist.

- [ ] **Step 4: Add registry types**

In `app/web/src/types/registry.ts`, add:

```ts
export type RegistrySkillScope = 'hub' | 'project';

export interface RegistrySkillSnapshot {
  name: string;
  path?: string;
  category: string;
  categoryKey: string;
  agents: string[];
}

export interface RegistrySkillScopeSnapshot {
  scope: RegistrySkillScope;
  skills: RegistrySkillSnapshot[];
}

export interface RegistrySkillProjectSnapshot {
  projectName: string;
  projectId?: string;
  online: boolean;
  path?: string;
  skills: RegistrySkillSnapshot[];
  error?: string;
}

export interface RegistrySkillCommandResponse {
  ok: boolean;
  hubId: string;
  updatedAt?: string;
  source?: string;
  scope?: RegistrySkillScope;
  projectName?: string;
  hubSkills?: RegistrySkillScopeSnapshot;
  projects?: RegistrySkillProjectSnapshot[];
  skills?: RegistrySkillSnapshot[];
  candidates?: RegistrySkillSourceCandidate[];
  message?: string;
  errorSummary?: string;
}

export interface RegistrySkillSourceCandidate {
  name: string;
  description: string;
  category: string;
  categoryKey: string;
}

export interface RegistrySkillInstallPayload {
  hubId: string;
  scope: RegistrySkillScope;
  projectName?: string;
  source: string;
  skills: string[];
}

export interface RegistrySkillScopePayload {
  hubId: string;
  scope: RegistrySkillScope;
  projectName?: string;
  skills?: string[];
}
```

- [ ] **Step 5: Add repository and service methods**

In `registryRepository.ts`, import the new types and add:

```ts
async scanSkills(hubId: string): Promise<RegistrySkillCommandResponse> {
  const resp = await this.client.request({
    method: 'cmd.skills',
    payload: {action: 'scan', hubId},
    timeoutMs: 60000,
  });
  return (resp.payload ?? {}) as RegistrySkillCommandResponse;
}

async listSkillsSource(hubId: string, source: string): Promise<RegistrySkillCommandResponse> {
  const resp = await this.client.request({
    method: 'cmd.skills',
    payload: {action: 'list', hubId, source},
    timeoutMs: 60000,
  });
  return (resp.payload ?? {}) as RegistrySkillCommandResponse;
}

async installSkills(payload: RegistrySkillInstallPayload): Promise<RegistrySkillCommandResponse> {
  const resp = await this.client.request({
    method: 'cmd.skills',
    payload: {action: 'install', ...payload},
    timeoutMs: 60000,
  });
  return (resp.payload ?? {}) as RegistrySkillCommandResponse;
}

async uninstallSkills(payload: RegistrySkillScopePayload): Promise<RegistrySkillCommandResponse> {
  const resp = await this.client.request({
    method: 'cmd.skills',
    payload: {action: 'uninstall', ...payload},
    timeoutMs: 60000,
  });
  return (resp.payload ?? {}) as RegistrySkillCommandResponse;
}

async updateSkills(payload: RegistrySkillScopePayload): Promise<RegistrySkillCommandResponse> {
  const resp = await this.client.request({
    method: 'cmd.skills',
    payload: {action: 'update', ...payload},
    timeoutMs: 60000,
  });
  return (resp.payload ?? {}) as RegistrySkillCommandResponse;
}
```

Mirror these methods in `RegistryWorkspaceService`.

- [ ] **Step 6: Add pure helpers**

Create `app/web/src/skillManagementView.ts`:

```ts
import type {
  RegistryHub,
  RegistrySkillProjectSnapshot,
  RegistrySkillScope,
  RegistrySkillSnapshot,
} from './types/registry';

export interface SkillCategoryGroup {
  category: string;
  categoryKey: string;
  skills: RegistrySkillSnapshot[];
}

export function deriveSkillHubIds(hubs: RegistryHub[]): string[] {
  return Array.from(new Set(
    hubs.map(hub => (hub.hubId || '').trim()).filter(Boolean),
  )).sort((a, b) => a.localeCompare(b));
}

export function groupSkillsByCategory(skills: RegistrySkillSnapshot[]): SkillCategoryGroup[] {
  const groups = new Map<string, SkillCategoryGroup>();
  skills.forEach(skill => {
    const categoryKey = (skill.categoryKey || '').trim() || 'general';
    const category = (skill.category || '').trim() || 'General';
    const existing = groups.get(categoryKey) ?? {category, categoryKey, skills: []};
    existing.skills.push(skill);
    groups.set(categoryKey, existing);
  });
  return Array.from(groups.values())
    .map(group => ({
      ...group,
      skills: [...group.skills].sort((a, b) => a.name.localeCompare(b.name)),
    }))
    .sort((a, b) => {
      if (a.categoryKey === 'general') return 1;
      if (b.categoryKey === 'general') return -1;
      return a.category.localeCompare(b.category);
    });
}

export function sortSkillProjects(projects: RegistrySkillProjectSnapshot[]): RegistrySkillProjectSnapshot[] {
  return [...projects].sort((a, b) => {
    if (a.online !== b.online) return a.online ? -1 : 1;
    return a.projectName.localeCompare(b.projectName);
  });
}

export function skillScopeLabel(input: {scope: RegistrySkillScope; hubId: string; projectName?: string}): string {
  if (input.scope === 'project') return `Project: ${input.projectName || ''}`.trim();
  return `Hub: ${input.hubId}`;
}
```

- [ ] **Step 7: Verify app helper/service tests**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-skill-management-service.test.ts __tests__/web-skill-management-view.test.ts --runInBand
```

Expected: PASS.

- [ ] **Step 8: Commit**

```powershell
git add app/web/src/types/registry.ts app/web/src/services/registryRepository.ts app/web/src/services/registryWorkspaceService.ts app/web/src/skillManagementView.ts app/__tests__/web-skill-management-service.test.ts app/__tests__/web-skill-management-view.test.ts
git commit -m "feat: add skills command app data layer"
```

---

### Task 4: Settings Skills UI

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/web/src/styles.css`
- Create: `app/__tests__/web-skill-management-settings.test.ts`
- Modify: `app/__tests__/web-chat-ui.test.ts`

- [ ] **Step 1: Write failing Settings tests**

Create `app/__tests__/web-skill-management-settings.test.ts`:

```ts
import fs from 'fs';
import path from 'path';

const root = path.resolve(__dirname, '..');
const mainTsx = fs.readFileSync(path.join(root, 'web/src/main.tsx'), 'utf8');
const stylesCss = fs.readFileSync(path.join(root, 'web/src/styles.css'), 'utf8');

describe('skill management settings UI source structure', () => {
  test('adds Skills as a settings detail and More row after Update', () => {
    expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | null;");
    expect(mainTsx).toContain("settingsDetailView === 'skills'");
    expect(mainTsx).toContain('renderSkillsSettingsDetail()');
    const moreStart = mainTsx.indexOf("renderSettingsSection('More'");
    const moreEnd = mainTsx.indexOf("renderSettingsSection(", moreStart + 1);
    const moreSection = mainTsx.slice(moreStart, moreEnd > moreStart ? moreEnd : undefined);
    expect(moreSection.indexOf("setSettingsDetailView('update')")).toBeLessThan(moreSection.indexOf("setSettingsDetailView('skills')"));
    expect(moreSection.indexOf("setSettingsDetailView('skills')")).toBeLessThan(moreSection.indexOf("setSettingsDetailView('tokenStats')"));
  });

  test('adds desktop Skills shortcut between Update and Token Stats', () => {
    const activityBarStart = mainTsx.indexOf('const desktopActivityBar = isWide ? (');
    const activityBarEnd = mainTsx.indexOf('const floatingControlStack = !isWide ? (', activityBarStart);
    const activityBar = mainTsx.slice(activityBarStart, activityBarEnd);
    expect(activityBar).toContain('codicon-symbol-method');
    expect(activityBar).toContain("openSettingsDetail('skills')");
    expect(activityBar.indexOf('title="Update"')).toBeLessThan(activityBar.indexOf('title="Skills"'));
    expect(activityBar.indexOf('title="Skills"')).toBeLessThan(activityBar.indexOf('title="Token Stats"'));
    expect(activityBar).toContain("settingsDetailView === 'skills'");
  });

  test('renders Skills detail with controlled command hooks and confirmations', () => {
    expect(mainTsx).toContain('const renderSkillsSettingsDetail = () =>');
    expect(mainTsx).toContain('refreshSkillManagement');
    expect(mainTsx).toContain('service.scanSkills');
    expect(mainTsx).toContain('service.listSkillsSource');
    expect(mainTsx).toContain('service.installSkills');
    expect(mainTsx).toContain('service.uninstallSkills');
    expect(mainTsx).toContain('service.updateSkills');
    expect(mainTsx).toContain("kind: 'skillInstall'");
    expect(mainTsx).toContain("kind: 'skillUninstall'");
    expect(mainTsx).toContain("kind: 'skillUpdate'");
  });

  test('uses compact settings skill styles', () => {
    expect(stylesCss).toContain('.settings-skills-hub');
    expect(stylesCss).toContain('.settings-skill-row');
    expect(stylesCss).toContain('.settings-skill-category');
  });
});
```

Update `app/__tests__/web-chat-ui.test.ts` assertions for `SettingsDetailView` to include `'skills'` and for the desktop shortcut order to account for the new button.

- [ ] **Step 2: Run Settings tests and verify failure**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-skill-management-settings.test.ts __tests__/web-chat-ui.test.ts --runInBand
```

Expected: FAIL because the Skills detail is not implemented.

- [ ] **Step 3: Add state and confirm target variants**

In `main.tsx`, update:

```ts
type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | null;
```

Add view state:

```ts
type SkillHubView = {
  hubId: string;
  loading: boolean;
  data: RegistrySkillCommandResponse | null;
  error: string;
};

const [skillHubs, setSkillHubs] = useState<Record<string, SkillHubView>>({});
const [skillsLoading, setSkillsLoading] = useState(false);
const [skillsError, setSkillsError] = useState('');
const [skillsPendingKey, setSkillsPendingKey] = useState('');
```

Extend `ConfirmTarget` with:

```ts
| {kind: 'skillInstall'; hubId: string; scope: RegistrySkillScope; projectName?: string; source: string; skills: string[]}
| {kind: 'skillUninstall'; hubId: string; scope: RegistrySkillScope; projectName?: string; skillName: string}
| {kind: 'skillUpdate'; hubId: string; scope: RegistrySkillScope; projectName?: string}
```

- [ ] **Step 4: Add refresh and action handlers**

Add callbacks:

```ts
const refreshSkillManagement = useCallback(async () => {
  setSkillsLoading(true);
  setSkillsError('');
  try {
    const snapshot = await service.listProjectSnapshot();
    const hubIds = deriveSkillHubIds(snapshot.hubs);
    const results = await Promise.all(hubIds.map(async hubId => {
      try {
        return {hubId, result: await service.scanSkills(hubId), error: ''};
      } catch (error) {
        return {hubId, result: null, error: error instanceof Error ? error.message : 'Skills scan failed.'};
      }
    }));
    setSkillHubs(Object.fromEntries(results.map(item => [item.hubId, {
      hubId: item.hubId,
      loading: false,
      data: item.result,
      error: item.error || (item.result?.ok === false ? item.result.errorSummary || 'Skills scan failed.' : ''),
    }])));
  } catch (error) {
    setSkillsError(error instanceof Error ? error.message : 'Skills scan failed.');
  } finally {
    setSkillsLoading(false);
  }
}, [service]);
```

Add handlers for confirmed install/uninstall/update that call the service, set `skillsPendingKey`, and then refresh the target Hub.

- [ ] **Step 5: Render the Skills detail**

Add `renderSkillsSettingsDetail` using the existing shared detail shell:

```tsx
const renderSkillsSettingsDetail = () =>
  renderSettingsDetailShell(
    'Skills',
    <>
      {skillsLoading && Object.keys(skillHubs).length === 0 ? <div className="muted block">Loading skills...</div> : null}
      {skillsError ? <div className="muted block settings-metadata-error">{skillsError}</div> : null}
      {Object.values(skillHubs).sort((a, b) => a.hubId.localeCompare(b.hubId)).map(hub => (
        <section className="settings-skills-hub" key={hub.hubId}>
          <div className="settings-skills-hub-header">
            <h3>{hub.hubId}</h3>
            <button type="button" className="settings-detail-action-btn" onClick={() => requestSkillUpdate({hubId: hub.hubId, scope: 'hub'})}>Update</button>
          </div>
          {renderSkillScopeRows(hub.hubId, 'Hub Skills', hub.data?.hubSkills?.skills ?? [], {scope: 'hub'})}
          {sortSkillProjects(hub.data?.projects ?? []).map(project => (
            <div className="settings-skills-project" key={project.projectName}>
              {renderSkillScopeRows(hub.hubId, project.projectName, project.skills, {scope: 'project', projectName: project.projectName, disabled: !project.online})}
            </div>
          ))}
        </section>
      ))}
    </>,
    <button type="button" className="settings-detail-action-btn" onClick={() => refreshSkillManagement().catch(() => undefined)} disabled={skillsLoading}>
      {skillsLoading ? 'Refreshing...' : 'Refresh'}
    </button>,
  );
```

Keep final code consistent with existing `renderUpdateSettingsDetail` style. Do not nest cards inside cards; use flat sections and compact rows.

- [ ] **Step 6: Add Settings IA and desktop shortcut**

In `renderSettingsContent`, route:

```ts
if (settingsDetailView === 'skills') {
  return renderSkillsSettingsDetail();
}
```

Add `Skills` row in `More` immediately after `Update`.

Add desktop activity button between Update and Token Stats:

```tsx
<button
  type="button"
  className={`desktop-activity-button${sidebarSettingsOpen && settingsDetailView === 'skills' ? ' active' : ''}`}
  onClick={() => openSettingsDetail('skills')}
  title="Skills"
  aria-label="Skills"
>
  <span className="codicon codicon-symbol-method" aria-hidden="true" />
</button>
```

Update `isShortcutSettingsDetailActive` to include `skills`.

- [ ] **Step 7: Add styles**

Add compact styles in `styles.css`:

```css
.settings-skills-hub {
  border-top: 1px solid var(--border-subtle);
  padding: 12px 0;
}

.settings-skills-hub-header,
.settings-skills-scope-header,
.settings-skill-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.settings-skill-category {
  color: var(--text-muted);
  font-size: 12px;
  font-weight: 600;
  padding: 10px 0 4px;
}

.settings-skill-row {
  min-height: 36px;
  border-top: 1px solid var(--border-subtle);
  padding: 6px 0;
}

.settings-skill-row-main {
  min-width: 0;
}

.settings-skill-name {
  font-weight: 600;
}

.settings-skill-meta {
  color: var(--text-muted);
  font-size: 12px;
  overflow-wrap: anywhere;
}
```

Use existing CSS variables in the file; if variable names differ, adapt to local names rather than introducing a new palette.

- [ ] **Step 8: Verify Settings tests**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-skill-management-settings.test.ts __tests__/web-chat-ui.test.ts --runInBand
```

Expected: PASS.

- [ ] **Step 9: Commit**

```powershell
git add app/web/src/main.tsx app/web/src/styles.css app/__tests__/web-skill-management-settings.test.ts app/__tests__/web-chat-ui.test.ts
git commit -m "feat: add skills settings page"
```

---

### Task 5: End-To-End Verification

**Files:**
- Verify only unless failures require fixes.

- [ ] **Step 1: Run focused server tests**

```powershell
cd server
go test ./internal/registry -run "CmdSkills|UnknownCmd|HubCommand" -count=1
go test ./internal/hub -run "SkillsCommand|CmdSkills|ReporterRespondsToCmdSkills" -count=1
```

Expected: PASS.

- [ ] **Step 2: Run focused app tests**

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-skill-management-service.test.ts __tests__/web-skill-management-view.test.ts __tests__/web-skill-management-settings.test.ts __tests__/web-chat-ui.test.ts --runInBand
```

Expected: PASS.

- [ ] **Step 3: Run broader affected suites**

```powershell
cd server
go test ./internal/registry ./internal/hub -count=1
cd ..\app
npm test -- --runInBand
```

Expected: PASS.

- [ ] **Step 4: Build Web release**

Because files under `app/` changed, run:

```powershell
cd app
npm run build:web:release
```

Expected: exit code 0 and web assets published to `~/.wheelmaker/web`.

- [ ] **Step 5: Final completion gate**

From repo root, follow the root `CLAUDE.md` completion gate:

```powershell
git add -A
git commit -m "feat: add skills settings management"
git push origin main
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
cd app
npm run build:web:release
```

Only run the server signal if server files changed. The app build is required because app files changed.

---

## Self-Review Notes

- Spec coverage: protocol, Hub handler, source validation, synchronous operations, fixed agents, symlink default, Settings IA, desktop shortcut, confirmation flows, and tests are covered.
- Placeholder scan: no task depends on undefined placeholder behavior; where helper names may differ in existing tests, the plan explicitly says to adapt to existing helper names.
- Type consistency: action names are `scan`, `list`, `install`, `uninstall`, and `update`; scope values are `hub` and `project`; App method names map directly to those actions.
