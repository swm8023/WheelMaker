# Workspace Project Lightweight Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make File/Git use a lightweight workspace project selector that follows Chat session selection without invoking the heavy project switch path.

**Architecture:** Add a lightweight service/controller path that changes `session.selectedProjectId` without loading root files. Wire Chat selection, the existing project menu, and a new PC File/Git workspace selector through one UI sync function. Persist workspace project separately from Chat selection and lazily load File/Git only when their tab is visible.

**Tech Stack:** React 19, TypeScript, Jest, existing WheelMaker web services and persistence.

---

### Task 1: Service Lightweight Project Selection

**Files:**
- Modify: `app/web/src/services/registryWorkspaceService.ts`
- Test: `app/__tests__/web-chat-project-service.test.ts`

- [ ] **Step 1: Write the failing service tests**

Add tests that set `repository.listFiles` as a Jest mock and assign an initialized private `session`:

```ts
test('lightweight project selection updates selected project without listing files', async () => {
  const service = new RegistryWorkspaceService();
  const repository = {
    listFiles: jest.fn().mockResolvedValue({ entries: [{ name: 'root', path: 'root', kind: 'file' }] }),
  };

  Object.assign(service as unknown as { repository: unknown; session: unknown }, {
    repository,
    session: {
      projects: [
        { projectId: 'p1', name: 'One', online: true, path: '/one' },
        { projectId: 'p2', name: 'Two', online: true, path: '/two' },
      ],
      selectedProjectId: 'p1',
      fileEntries: [{ name: 'old', path: 'old', kind: 'file' }],
    },
  });

  const session = await (service as any).selectProjectLightweight('p2');

  expect(session.selectedProjectId).toBe('p2');
  expect(session.fileEntries).toEqual([{ name: 'old', path: 'old', kind: 'file' }]);
  expect(repository.listFiles).not.toHaveBeenCalled();
});

test('lightweight project selection rejects unknown projects', async () => {
  const service = new RegistryWorkspaceService();

  Object.assign(service as unknown as { repository: unknown; session: unknown }, {
    repository: { listFiles: jest.fn() },
    session: {
      projects: [{ projectId: 'p1', name: 'One', online: true, path: '/one' }],
      selectedProjectId: 'p1',
      fileEntries: [],
    },
  });

  await expect((service as any).selectProjectLightweight('missing')).rejects.toThrow(
    'Project is no longer available',
  );
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `cd app && npm test -- web-chat-project-service.test.ts --runInBand`

Expected: FAIL because `selectProjectLightweight` is not defined.

- [ ] **Step 3: Implement the service method**

Add this method near `selectProject`:

```ts
  async selectProjectLightweight(projectId: string): Promise<WorkspaceSession> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    if (!this.session.projects.some(project => project.projectId === projectId)) {
      throw new Error('Project is no longer available');
    }
    this.session = {...this.session, selectedProjectId: projectId};
    return this.session;
  }
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run: `cd app && npm test -- web-chat-project-service.test.ts --runInBand`

Expected: PASS.

### Task 2: Workspace Controller Lightweight Hydration

**Files:**
- Modify: `app/web/src/services/workspaceStore.ts`
- Modify: `app/web/src/services/workspaceController.ts`
- Test: `app/__tests__/web-workspace-project-lightweight.test.ts`

- [ ] **Step 1: Write failing controller/store tests**

Create `app/__tests__/web-workspace-project-lightweight.test.ts`:

```ts
jest.mock('../web/src/services/shikiRenderer', () => ({
  DEFAULT_CODE_FONT: 'jetbrains',
  DEFAULT_CODE_FONT_SIZE: 13,
  DEFAULT_CODE_LINE_HEIGHT: 1.5,
  DEFAULT_CODE_TAB_SIZE: 2,
  DEFAULT_CODE_THEME: 'dark-plus',
  isCodeFontId: () => true,
  isCodeThemeId: () => true,
}));

import { WorkspaceController } from '../web/src/services/workspaceController';
import { WorkspaceStore } from '../web/src/services/workspaceStore';

describe('workspace lightweight project switching', () => {
  test('hydrates cached workspace state without loading root files', async () => {
    const service = {
      selectProjectLightweight: jest.fn().mockResolvedValue({
        projects: [
          { projectId: 'p1', name: 'One', online: true, path: '/one' },
          { projectId: 'p2', name: 'Two', online: true, path: '/two' },
        ],
        selectedProjectId: 'p2',
        fileEntries: [{ name: 'stale', path: 'stale', kind: 'file' }],
      }),
      listDirectory: jest.fn(),
    };
    const store = new WorkspaceStore({
      getProjectState: jest.fn((projectId: string) => ({
        expandedDirs: ['.'],
        selectedFile: projectId === 'p2' ? 'cached.ts' : '',
        pinnedFiles: [],
        gitCurrentBranch: '',
        selectedCommit: '',
        selectedDiff: '',
        selectedChatSessionId: '',
      })),
      getProjectCommitsState: jest.fn(() => ({ commits: [], commitFilesBySha: {} })),
      getCachedFile: jest.fn(() => null),
      getProjectDiff: jest.fn(() => null),
      patchProjectState: jest.fn(),
      patchProjectCommitsState: jest.fn(),
    } as any);
    const controller = new WorkspaceController(service as any, store);

    const result = await controller.switchProjectLightweight('p2');

    expect(service.selectProjectLightweight).toHaveBeenCalledWith('p2');
    expect(service.listDirectory).not.toHaveBeenCalled();
    expect(result.hydrated.projectId).toBe('p2');
    expect(result.hydrated.selectedFile).toBe('cached.ts');
    expect(result.rootEntries).toEqual([]);
  });

  test('rememberGlobalState preserves chat selection while persisting workspace project', () => {
    const globalState = {
      selectedProjectId: 'p1',
      selectedChatProjectId: 'chat-project',
      selectedChatSessionId: 'chat-session',
    };
    const persistence = {
      getGlobalState: jest.fn(() => globalState),
      patchGlobalState: jest.fn((patch: Record<string, unknown>) => Object.assign(globalState, patch)),
    };
    const store = new WorkspaceStore(persistence as any);

    store.rememberGlobalState({ selectedProjectId: 'p2' } as any);

    expect(persistence.patchGlobalState).toHaveBeenCalledWith({ selectedProjectId: 'p2' });
    expect(globalState.selectedChatProjectId).toBe('chat-project');
    expect(globalState.selectedChatSessionId).toBe('chat-session');
  });
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `cd app && npm test -- web-workspace-project-lightweight.test.ts --runInBand`

Expected: FAIL because `switchProjectLightweight` is not defined.

- [ ] **Step 3: Add cache-only hydrate helper**

In `WorkspaceStore`, add:

```ts
  hydrateCachedProject(projectId: string): HydratedProjectState {
    return this.hydrateProject(projectId, []);
  }
```

- [ ] **Step 4: Add controller lightweight switch**

In `WorkspaceController`, add:

```ts
  async switchProjectLightweight(projectId: string): Promise<ProjectLoadResult> {
    const session = await this.service.selectProjectLightweight(projectId);
    return {
      projects: session.projects,
      rootEntries: [],
      hydrated: this.store.hydrateCachedProject(session.selectedProjectId),
    };
  }
```

- [ ] **Step 5: Run the focused test and verify GREEN**

Run: `cd app && npm test -- web-workspace-project-lightweight.test.ts --runInBand`

Expected: PASS.

### Task 3: Main Workspace Sync Wiring

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-chat-main-composite-key.test.ts`
- Test: `app/__tests__/web-workspace-project-lightweight-ui.test.ts`

- [ ] **Step 1: Write failing UI wiring tests**

Create `app/__tests__/web-workspace-project-lightweight-ui.test.ts` with source-level assertions:

```ts
import fs from 'fs';
import path from 'path';

function readMain(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
}

function extractFunctionBody(source: string, functionName: string): string {
  const marker = `const ${functionName} = async`;
  const start = source.indexOf(marker);
  expect(start).toBeGreaterThanOrEqual(0);
  const arrowStart = source.indexOf(') => {', start);
  expect(arrowStart).toBeGreaterThanOrEqual(0);
  const bodyStart = source.indexOf('{', arrowStart);
  let depth = 0;
  for (let index = bodyStart; index < source.length; index += 1) {
    const char = source[index];
    if (char === '{') depth += 1;
    if (char === '}') {
      depth -= 1;
      if (depth === 0) return source.slice(bodyStart, index + 1);
    }
  }
  throw new Error(`Unable to extract ${functionName}`);
}

describe('workspace project lightweight UI wiring', () => {
  test('user project switching uses lightweight sync instead of switchProject', () => {
    const main = readMain();
    const syncBody = extractFunctionBody(main, 'syncWorkspaceProject');
    const projectMenuBlock = main.slice(main.indexOf('const projectMenu ='), main.indexOf('const refreshButtonContent'));

    expect(syncBody).toContain('workspaceController.switchProjectLightweight');
    expect(syncBody).toContain('workspaceStore.rememberGlobalState({');
    expect(syncBody).toContain('selectedProjectId: nextProjectId');
    expect(projectMenuBlock).toContain('syncWorkspaceProject(projectItem.projectId');
    expect(projectMenuBlock).not.toContain('switchProject(projectItem.projectId)');
  });

  test('chat session selection syncs workspace project without waiting for file/git loads', () => {
    const main = readMain();
    const body = extractFunctionBody(main, 'selectProjectChatSession');

    expect(body).toContain('syncWorkspaceProject(targetProjectId');
    expect(body).toContain("reason: 'chat'");
    expect(body).not.toContain('switchProject(');
  });

  test('pc file and git sidebars render the workspace selector above section titles', () => {
    const main = readMain();

    expect(main).toContain('const renderWorkspaceProjectSelector = () =>');
    expect(main).toContain('{isWide ? renderWorkspaceProjectSelector() : null}');
    expect(main).toContain('<div className="workspace-project-label">WORKSPACE</div>');
    expect(main).toContain('workspace-project-menu');
  });
});
```

Update `web-chat-main-composite-key.test.ts` to expect the Chat selection body to contain `syncWorkspaceProject`.

- [ ] **Step 2: Run the focused UI tests and verify RED**

Run: `cd app && npm test -- web-workspace-project-lightweight-ui.test.ts web-chat-main-composite-key.test.ts --runInBand`

Expected: FAIL because `syncWorkspaceProject` and selector rendering do not exist yet.

- [ ] **Step 3: Add workspace selector state and sync function**

In `main.tsx`, add state:

```ts
const [workspaceProjectMenuOpen, setWorkspaceProjectMenuOpen] = useState(false);
```

Add `syncWorkspaceProject` near `switchProject`:

```ts
  const syncWorkspaceProject = async (
    nextProjectId: string,
    options?: {reason?: 'chat' | 'manual'},
  ) => {
    if (!nextProjectId || nextProjectId === projectIdRef.current) {
      setWorkspaceProjectMenuOpen(false);
      return;
    }
    if (!projectsRef.current.some(item => item.projectId === nextProjectId)) {
      if (options?.reason !== 'chat') {
        setError('Project is no longer available');
      }
      setWorkspaceProjectMenuOpen(false);
      return;
    }
    captureSelectedFileScrollPosition();
    const previousProjectId = projectIdRef.current;
    if (previousProjectId) {
      workspaceStore.rememberProjectSnapshot(previousProjectId, {
        expandedDirs: expandedDirsRef.current,
        selectedFile: selectedFileRef.current,
        pinnedFiles,
        gitCurrentBranch,
        commits,
        selectedCommit,
        commitFilesBySha,
        selectedDiff,
      });
    }
    const result = await workspaceController.switchProjectLightweight(nextProjectId);
    setProjects(result.projects);
    workspaceStore.rememberGlobalState({ selectedProjectId: nextProjectId } as any);
    applyHydratedProjectState(result.hydrated);
    setWorkspaceProjectMenuOpen(false);
    if (tabRef.current === 'file') {
      loadDirectory('.').catch(err => setError(err instanceof Error ? err.message : String(err)));
    } else if (tabRef.current === 'git') {
      loadGit().catch(err => setGitError(err instanceof Error ? err.message : String(err)));
    }
  };
```

- [ ] **Step 4: Wire Chat selection and existing project menu**

In `selectProjectChatSession`, call:

```ts
    syncWorkspaceProject(targetProjectId, {reason: 'chat'}).catch(() => undefined);
```

In the existing `projectMenu`, replace:

```ts
switchProject(projectItem.projectId).catch(() => undefined)
```

with:

```ts
syncWorkspaceProject(projectItem.projectId, {reason: 'manual'}).catch(() => undefined)
```

- [ ] **Step 5: Add PC File/Git selector render helper**

Add:

```tsx
  const renderWorkspaceProjectSelector = () => {
    const current = projects.find(item => item.projectId === projectId);
    return (
      <div className="workspace-project-selector">
        <div className="workspace-project-label">WORKSPACE</div>
        <div className="workspace-project-control">
          <button
            type="button"
            className="workspace-project-button"
            onClick={() => setWorkspaceProjectMenuOpen(prev => !prev)}
            title={current?.path || currentProjectName}
          >
            <span className="workspace-project-name">{current?.name || currentProjectName}</span>
            <span className="codicon codicon-chevron-down" />
          </button>
          {workspaceProjectMenuOpen ? (
            <div className="workspace-project-menu">
              {sortedProjectItems.map(projectItem => (
                <button
                  key={`workspace:${projectItem.projectId}`}
                  type="button"
                  className={`workspace-project-menu-item ${projectItem.projectId === projectId ? 'selected' : ''}`}
                  onClick={() => syncWorkspaceProject(projectItem.projectId, {reason: 'manual'}).catch(() => undefined)}
                >
                  <span className="workspace-project-menu-name">{projectItem.name}</span>
                  <span className="workspace-project-menu-path">{projectItem.path || projectItem.hubId || projectItem.projectId}</span>
                </button>
              ))}
            </div>
          ) : null}
        </div>
      </div>
    );
  };
```

Call `{isWide ? renderWorkspaceProjectSelector() : null}` above the `EXPLORER` section title and above the Git `GRAPH` section title.

- [ ] **Step 6: Run focused UI tests and verify GREEN**

Run: `cd app && npm test -- web-workspace-project-lightweight-ui.test.ts web-chat-main-composite-key.test.ts --runInBand`

Expected: PASS.

### Task 4: Styling and Full Verification

**Files:**
- Modify: `app/web/src/styles.css`
- Test: `app/__tests__/web-workspace-project-lightweight-ui.test.ts`

- [ ] **Step 1: Add failing style assertions**

Extend `web-workspace-project-lightweight-ui.test.ts` to read `styles.css` and assert:

```ts
function readStyles(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');
}

test('workspace project selector has dedicated compact sidebar styles', () => {
  const styles = readStyles();

  expect(styles).toContain('.workspace-project-selector');
  expect(styles).toContain('.workspace-project-button');
  expect(styles).toContain('.workspace-project-menu');
  expect(styles).toContain('.workspace-project-menu-item.selected');
});
```

- [ ] **Step 2: Run focused style test and verify RED**

Run: `cd app && npm test -- web-workspace-project-lightweight-ui.test.ts --runInBand`

Expected: FAIL because selector styles do not exist.

- [ ] **Step 3: Add compact sidebar styles**

Add styles near the existing project/sidebar styles:

```css
.workspace-project-selector {
  position: relative;
  padding: 8px 10px 6px;
  border-bottom: 1px solid var(--border);
}

.workspace-project-label {
  margin-bottom: 4px;
  font-size: 10px;
  line-height: 1;
  color: var(--muted);
  font-weight: 700;
}

.workspace-project-button {
  width: 100%;
  min-height: 30px;
  border: 1px solid var(--border);
  background: var(--panel-2);
  color: var(--text);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 8px;
  border-radius: 6px;
  cursor: pointer;
}

.workspace-project-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  font-weight: 600;
}

.workspace-project-menu {
  position: absolute;
  z-index: 60;
  top: calc(100% - 2px);
  left: 10px;
  right: 10px;
  max-height: 320px;
  overflow: auto;
  border: 1px solid var(--border);
  background: var(--panel);
  box-shadow: var(--shadow);
  border-radius: 6px;
  padding: 4px;
}

.workspace-project-menu-item {
  width: 100%;
  border: 0;
  background: transparent;
  color: var(--text);
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 7px 8px;
  border-radius: 4px;
  text-align: left;
  cursor: pointer;
}

.workspace-project-menu-item:hover,
.workspace-project-menu-item.selected {
  background: var(--hover);
}

.workspace-project-menu-name,
.workspace-project-menu-path {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workspace-project-menu-name {
  font-size: 12px;
  font-weight: 600;
}

.workspace-project-menu-path {
  font-size: 11px;
  color: var(--muted);
}
```

- [ ] **Step 4: Run focused tests**

Run: `cd app && npm test -- web-workspace-project-lightweight-ui.test.ts web-chat-project-service.test.ts web-workspace-project-lightweight.test.ts web-chat-main-composite-key.test.ts --runInBand`

Expected: PASS.

- [ ] **Step 5: Run type check**

Run: `cd app && npm run tsc:web`

Expected: PASS.

- [ ] **Step 6: Run full web tests**

Run: `cd app && npm test -- --runInBand`

Expected: PASS.
