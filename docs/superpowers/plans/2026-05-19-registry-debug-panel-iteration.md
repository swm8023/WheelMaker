# Registry Debug Panel Iteration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add scope-first filtering, friendly session labels, resizable list width, collapsible JSON detail, and no automatic latest scrolling to the existing Registry Debug panel.

**Architecture:** Keep capture at `RegistryClient` and extend the existing debug record model with a normalized `scope`. Keep filtering pure in `registryDebug.ts`; keep visual state local to `RegistryDebugPanel`; derive friendly labels from existing `projects` and `projectSessionsByProjectId` in `main.tsx`.

**Tech Stack:** React 19, TypeScript, `react-virtuoso`, Jest, webpack web build.

---

### Task 1: Debug Record Scope And Two-Level Filtering

**Files:**
- Modify: `app/__tests__/web-registry-debug-records.test.ts`
- Modify: `app/web/src/debug/registryDebug.ts`

- [ ] **Step 1: Write failing helper tests**

Add tests that describe the desired helper API before production changes:

```ts
expect(records[0]).toMatchObject({method: 'session.send', scope: 'session.*'});
expect(records[1]).toMatchObject({method: 'fs.read', scope: 'fs.*'});
expect(records[2]).toMatchObject({phase: 'connect_open', scope: 'lifecycle'});

expect(filterRegistryDebugRecords(records, {
  selectedScope: 'session.*',
  selectedSessionId: 'sess-a',
  includeMultiSessionRecords: false,
}).map(record => record.id)).toEqual([sessionRecord.id]);
```

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
cd app
npm test -- web-registry-debug-records.test.ts --runInBand
```

Expected: FAIL because `scope` does not exist and `filterRegistryDebugRecords` still accepts positional arguments.

- [ ] **Step 3: Implement minimal helper changes**

Add:

```ts
export type RegistryDebugScope = string;

export type RegistryDebugFilter = {
  selectedScope: string;
  selectedSessionId: string;
  includeMultiSessionRecords: boolean;
};

export function resolveRegistryDebugScope(method: string | undefined, phase: RegistryDebugPhase): RegistryDebugScope {
  if (method) {
    const dotIndex = method.indexOf('.');
    return dotIndex > 0 ? `${method.slice(0, dotIndex)}.*` : method;
  }
  if (phase.startsWith('connect_')) return 'lifecycle';
  if (phase === 'parse_error') return 'parse_error';
  return 'unknown';
}
```

Store `scope` on every appended record and update `filterRegistryDebugRecords(records, filter)`.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run:

```powershell
cd app
npm test -- web-registry-debug-records.test.ts --runInBand
```

Expected: PASS.

### Task 2: Panel UI Contract Tests

**Files:**
- Modify: `app/__tests__/web-registry-debug-panel-ui.test.ts`
- Modify later: `app/web/src/debug/RegistryDebugPanel.tsx`
- Modify later: `app/web/src/styles.css`

- [ ] **Step 1: Write failing source tests**

Add assertions:

```ts
expect(panelTsx).toContain('selectedScope');
expect(panelTsx).toContain('onSelectedScopeChange');
expect(panelTsx).toContain('sessionLabels');
expect(panelTsx).toContain('registry-debug-splitter');
expect(panelTsx).toContain('registry-debug-detail-collapsed');
expect(panelTsx).not.toContain('followOutput');
```

For styles:

```ts
expect(stylesCss).toContain('.registry-debug-splitter');
expect(stylesCss).toContain('.registry-debug-detail-collapsed');
```

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
cd app
npm test -- web-registry-debug-panel-ui.test.ts --runInBand
```

Expected: FAIL because the panel has no scope selector, splitter, detail collapse state, or label map prop yet.

### Task 3: Panel Implementation

**Files:**
- Modify: `app/web/src/debug/RegistryDebugPanel.tsx`
- Modify: `app/web/src/styles.css`

- [ ] **Step 1: Add panel props and derived options**

Add props:

```ts
selectedScope: string;
onSelectedScopeChange: (scope: string) => void;
sessionLabels: Record<string, string>;
```

Compute:

```ts
const scopeOptions = React.useMemo(() => Array.from(new Set(records.map(record => record.scope))).sort(), [records]);
const scopeFilteredRecords = React.useMemo(
  () => selectedScope === 'All' ? records : records.filter(record => record.scope === selectedScope),
  [records, selectedScope],
);
const sessionIds = React.useMemo(
  () => Array.from(new Set(scopeFilteredRecords.flatMap(record => record.sessionIds))).sort(),
  [scopeFilteredRecords],
);
```

- [ ] **Step 2: Update toolbar**

Render `Scope` select before `Session`. Session options use:

```tsx
<option key={sessionId} value={sessionId}>{sessionLabels[sessionId] ?? sessionId}</option>
```

- [ ] **Step 3: Remove auto-follow**

Remove `atBottom`, `followOutput`, and `atBottomStateChange`. Keep `jumpToLatest` as a manual button when filtered records exist.

- [ ] **Step 4: Add split and collapse state**

Use local state:

```ts
const [listPaneWidth, setListPaneWidth] = React.useState(520);
const [detailCollapsed, setDetailCollapsed] = React.useState(false);
```

Add a splitter pointer interaction that adjusts only `listPaneWidth`, clamped within the current panel width. When collapsed, hide detail and splitter and let the list fill the body.

- [ ] **Step 5: Update CSS**

Replace fixed grid columns with a flex body. Add stable splitter styles:

```css
.registry-debug-splitter {
  flex: 0 0 7px;
  border: 0;
  border-left: 1px solid var(--border);
  border-right: 1px solid var(--border);
  background: var(--panel);
  cursor: col-resize;
}

.registry-debug-detail-collapsed {
  display: none;
}
```

- [ ] **Step 6: Run focused tests and verify GREEN**

Run:

```powershell
cd app
npm test -- web-registry-debug-panel-ui.test.ts --runInBand
```

Expected: PASS.

### Task 4: Main App Wiring And Friendly Labels

**Files:**
- Modify: `app/web/src/main.tsx`
- Modify: `app/__tests__/web-registry-debug-panel-ui.test.ts`

- [ ] **Step 1: Write failing wiring assertions**

Add assertions:

```ts
expect(mainTsx).toContain('selectedRegistryDebugScope');
expect(mainTsx).toContain('registryDebugSessionLabels');
expect(mainTsx).toContain('selectedScope={selectedRegistryDebugScope}');
expect(mainTsx).toContain('sessionLabels={registryDebugSessionLabels}');
```

- [ ] **Step 2: Run focused test and verify RED**

Run:

```powershell
cd app
npm test -- web-registry-debug-panel-ui.test.ts --runInBand
```

Expected: FAIL because `main.tsx` does not pass scope or friendly labels yet.

- [ ] **Step 3: Implement app state and label map**

Add:

```ts
const [selectedRegistryDebugScope, setSelectedRegistryDebugScope] = useState('All');
const registryDebugSessionLabels = useMemo(() => {
  const labels: Record<string, string> = {};
  for (const projectItem of projects) {
    for (const session of projectSessionsByProjectId[projectItem.projectId] ?? []) {
      labels[session.sessionId] = `${projectItem.name} / ${session.title || session.sessionId}`;
    }
  }
  return labels;
}, [projectSessionsByProjectId, projects]);
```

Pass the new props to `RegistryDebugPanel`. Reset scope to `All` when Debug is disabled.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run:

```powershell
cd app
npm test -- web-registry-debug-panel-ui.test.ts --runInBand
```

Expected: PASS.

### Task 5: Full Verification And Release

**Files:**
- Verify all modified files.

- [ ] **Step 1: Run unit tests**

Run:

```powershell
cd app
npm test -- --runInBand
```

Expected: all Jest suites pass.

- [ ] **Step 2: Run web typecheck**

Run:

```powershell
cd app
npm run tsc:web
```

Expected: exit 0.

- [ ] **Step 3: Run production web build**

Run:

```powershell
cd app
npm run build:web
```

Expected: exit 0. Existing webpack asset-size warnings are acceptable.

- [ ] **Step 4: Completion gate**

Run from repo root:

```powershell
git add -A
git commit -m "feat: refine registry debug panel"
git push origin main
cd app
npm run build:web:release
```

Expected: commit created, pushed to `origin/main`, release assets exported to `~/.wheelmaker/web`.
