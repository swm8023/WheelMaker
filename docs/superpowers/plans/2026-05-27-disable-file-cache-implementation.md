# Disable File Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a persisted Debug setting that clears File Cache and forces File tab directory/file access to fetch fresh data while enabled.

**Architecture:** Store `disableFileCache` as a global workspace preference in `WorkspacePersistence`, expose focused `WorkspaceStore` APIs for updating the setting and clearing only file cache, and gate existing file/directory cache reads and writes from `main.tsx` and `WorkspaceController`. Keep the UI as one additional row in the existing Debug settings section.

**Tech Stack:** React 19, TypeScript, Jest, IndexedDB-backed workspace persistence.

---

### Task 1: Persist Disable File Cache Preference

**Files:**
- Modify: `app/web/src/services/workspacePersistence.ts`
- Modify: `app/web/src/services/workspaceStore.ts`
- Test: `app/__tests__/web-disable-file-cache-settings.test.ts`

- [ ] **Step 1: Write the failing persistence/store test**

Create `app/__tests__/web-disable-file-cache-settings.test.ts` with:

```ts
import fs from 'fs';
import path from 'path';

describe('web disable file cache settings', () => {
  const projectRoot = path.join(__dirname, '..');

  test('persists disable file cache as a default-off global setting', () => {
    const workspacePersistence = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'),
      'utf8',
    );
    const workspaceStore = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspaceStore.ts'),
      'utf8',
    );

    expect(workspacePersistence).toContain('disableFileCache: boolean;');
    expect(workspacePersistence).toContain("disableFileCache: 'disableFileCache',");
    expect(workspacePersistence).toContain('disableFileCache: false,');
    expect(workspacePersistence).toContain(
      "disableFileCache: typeof input.disableFileCache === 'boolean' ? input.disableFileCache : base.disableFileCache",
    );
    expect(workspacePersistence).toContain(
      '{k: GLOBAL_KEYS.disableFileCache, v: serialize(next.disableFileCache), updatedAt: now}',
    );
    expect(workspaceStore).toContain('setDisableFileCache(disableFileCache: boolean): void {');
    expect(workspaceStore).toContain('clearFileCache(): void {');
    expect(workspaceStore).toContain('this.persistence.clearFileCache();');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- web-disable-file-cache-settings.test.ts --runInBand`

Expected: FAIL because `disableFileCache`, `setDisableFileCache`, and `clearFileCache` do not exist.

- [ ] **Step 3: Implement minimal persistence/store support**

In `app/web/src/services/workspacePersistence.ts`, insert these exact lines next to the existing `registryDebug` global fields:

```ts
  disableFileCache: boolean;
```

```ts
  disableFileCache: 'disableFileCache',
```

```ts
    disableFileCache: false,
```

```ts
    disableFileCache: typeof input.disableFileCache === 'boolean' ? input.disableFileCache : base.disableFileCache,
```

Add this exact row to every global-state row batch in `saveAllState()` and `patchGlobalState()`:

```ts
      {k: GLOBAL_KEYS.disableFileCache, v: serialize(next.disableFileCache), updatedAt: now},
```

Add a targeted cache clear method:

```ts
  clearFileCache(): void {
    this.fileCache.clear();
    void this.ready().then(async () => {
      await this.db.clearStores([TABLE_FILE_CACHE]);
    }).catch(() => undefined);
  }
```

In `app/web/src/services/workspaceStore.ts`:

```ts
  setDisableFileCache(disableFileCache: boolean): void {
    this.persistence.patchGlobalState({disableFileCache});
  }

  clearFileCache(): void {
    this.persistence.clearFileCache();
  }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- web-disable-file-cache-settings.test.ts --runInBand`

Expected: PASS.

### Task 2: Gate Workspace Hydration and Expanded Directory Validation

**Files:**
- Modify: `app/web/src/services/workspaceStore.ts`
- Modify: `app/web/src/services/workspaceController.ts`
- Test: `app/__tests__/web-disable-file-cache-settings.test.ts`

- [ ] **Step 1: Add failing controller/store assertions**

Append to `app/__tests__/web-disable-file-cache-settings.test.ts`:

```ts
  test('gates cached directory hydration and validation when file cache is disabled', () => {
    const workspaceStore = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspaceStore.ts'),
      'utf8',
    );
    const workspaceController = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspaceController.ts'),
      'utf8',
    );

    expect(workspaceStore).toContain('hydrateProject(projectId: string, rootEntries: RegistryFsEntry[], options?: {disableFileCache?: boolean}): HydratedProjectState');
    expect(workspaceStore).toContain('if (!options?.disableFileCache) {');
    expect(workspaceStore).toContain('hydrateCachedProject(projectId: string, options?: {disableFileCache?: boolean}): HydratedProjectState');
    expect(workspaceController).toContain('options?: {disableFileCache?: boolean}');
    expect(workspaceController).toContain('disableFileCache: options?.disableFileCache === true');
    expect(workspaceController).toContain('const cached = disableFileCache ? null : this.store.getCachedDirectory(projectId, dirPath);');
    expect(workspaceController).toContain('const result = await this.service.listDirectory(dirPath, disableFileCache ? undefined : cached?.hash || undefined);');
    expect(workspaceController).toContain('if (!disableFileCache) {');
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- web-disable-file-cache-settings.test.ts --runInBand`

Expected: FAIL because the options and cache gates do not exist.

- [ ] **Step 3: Implement minimal cache gates**

In `workspaceStore.ts`, change the `hydrateProject` signature to:

```ts
  hydrateProject(
    projectId: string,
    rootEntries: RegistryFsEntry[],
    options?: {disableFileCache?: boolean},
  ): HydratedProjectState {
```

Wrap the existing cached expanded-directory loop in:

```ts
    if (!options?.disableFileCache) {
```

and close it immediately after the existing loop body:

```ts
    }
```

Change `hydrateCachedProject` to:

```ts
  hydrateCachedProject(projectId: string, options?: {disableFileCache?: boolean}): HydratedProjectState {
    const cachedRoot = options?.disableFileCache ? null : this.getCachedDirectory(projectId, '.');
    return this.hydrateProject(projectId, cachedRoot?.entries ?? [], options);
  }
```

In `workspaceController.ts`, pass `disableFileCache` through `connect`, `switchProject`, `switchProjectLightweight`, `validateExpandedDirectories`, and `refreshProject` options. Inside validation, add:

```ts
    const disableFileCache = options?.disableFileCache === true;
```

Guard root directory cache writes with:

```ts
    if (!disableFileCache) {
      this.store.cacheDirectory(projectId, '.', '', dirEntries['.']);
    }
```

Use these exact lines inside the expanded-directory loop:

```ts
      const cached = disableFileCache ? null : this.store.getCachedDirectory(projectId, dirPath);
      const result = await this.service.listDirectory(dirPath, disableFileCache ? undefined : cached?.hash || undefined);
```

Guard directory cache writes with:

```ts
      if (!disableFileCache) {
        this.store.cacheDirectory(projectId, dirPath, result.hash || cached?.hash || '', entries);
      }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- web-disable-file-cache-settings.test.ts --runInBand`

Expected: PASS.

### Task 3: Add Debug Setting UI and Runtime Cache Clear

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-disable-file-cache-settings.test.ts`

- [ ] **Step 1: Add failing UI/runtime assertions**

Append to `app/__tests__/web-disable-file-cache-settings.test.ts`:

```ts
  test('adds debug setting and clears file cache when enabled', () => {
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const [disableFileCache, setDisableFileCache] = useState(');
    expect(mainTsx).toContain("typeof persistedGlobal.disableFileCache === 'boolean'");
    expect(mainTsx).toContain('workspaceStore.setDisableFileCache(disableFileCache);');
    expect(mainTsx).toContain('if (disableFileCache) {');
    expect(mainTsx).toContain('workspaceStore.clearFileCache();');
    expect(mainTsx).toContain('dirHashRef.current = {};');
    expect(mainTsx).toContain('fileHashRef.current = {};');
    expect(mainTsx).toContain('fileCacheRef.current = {};');
    expect(mainTsx).toContain('Disable File Cache');
    expect(mainTsx).toContain('checked={disableFileCache}');
    expect(mainTsx).toContain('onChange={event => setDisableFileCache(event.target.checked)}');
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- web-disable-file-cache-settings.test.ts --runInBand`

Expected: FAIL because the React state, effect, and setting row do not exist.

- [ ] **Step 3: Implement minimal UI and effect**

In `main.tsx`, add state near other persisted global settings:

```tsx
  const [disableFileCache, setDisableFileCache] = useState(
    typeof persistedGlobal.disableFileCache === 'boolean'
      ? persistedGlobal.disableFileCache
      : false,
  );
```

Add an effect after refs are declared:

```tsx
  useEffect(() => {
    workspaceStore.setDisableFileCache(disableFileCache);
    if (disableFileCache) {
      workspaceStore.clearFileCache();
      dirHashRef.current = {};
      fileHashRef.current = {};
      fileCacheRef.current = {};
    }
  }, [disableFileCache]);
```

Add a Debug section row:

```tsx
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-files settings-row-icon" aria-hidden="true" />
            Disable File Cache
          </span>
          <input
            type="checkbox"
            checked={disableFileCache}
            onChange={event => setDisableFileCache(event.target.checked)}
          />
        </label>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- web-disable-file-cache-settings.test.ts --runInBand`

Expected: PASS.

### Task 4: Bypass File and Directory Cache in Main Runtime

**Files:**
- Modify: `app/web/src/main.tsx`
- Test: `app/__tests__/web-disable-file-cache-settings.test.ts`

- [ ] **Step 1: Add failing runtime assertions**

Append to `app/__tests__/web-disable-file-cache-settings.test.ts`:

```ts
  test('bypasses directory and file cache while disabled setting is enabled', () => {
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const fileCacheDisabled = disableFileCache === true;');
    expect(mainTsx).toContain('const persistedCache = !fileCacheDisabled && targetProjectId');
    expect(mainTsx).toContain('dirHashRef.current[path] || persistedCache?.hash ||');
    expect(mainTsx).toContain('fileCacheDisabled ? undefined : knownHash || undefined');
    expect(mainTsx).toContain('if (!fileCacheDisabled && targetProjectId) {');
    expect(mainTsx).toContain('const persistedFile = fileCacheDisabled ? null : workspaceStore.getCachedFile(targetProjectId, path);');
    expect(mainTsx).toContain('const knownHash = !fileCacheDisabled && typeof cachedContent ===');
    expect(mainTsx).toContain('fileCacheDisabled ? undefined : knownHash || undefined');
    expect(mainTsx).toContain('if (!fileCacheDisabled) {');
    expect(mainTsx).toContain('controller.connect(wsUrl, token, {disableFileCache})');
    expect(mainTsx).toContain('controller.switchProject(targetProjectId, {disableFileCache})');
    expect(mainTsx).toContain('controller.switchProjectLightweight(nextProjectId, {disableFileCache})');
    expect(mainTsx).toContain('controller.validateExpandedDirectories(projectIdRef.current, result.rootEntries, result.hydrated.expandedDirs, {disableFileCache})');
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- web-disable-file-cache-settings.test.ts --runInBand`

Expected: FAIL because `main.tsx` still always consults and writes file cache.

- [ ] **Step 3: Implement minimal runtime bypass**

In `loadDirectory`, compute:

```tsx
    const fileCacheDisabled = disableFileCache === true;
```

Then only load/write cache when `!fileCacheDisabled`, and pass `undefined` as known hash when disabled.

In `readSelectedFile`, compute:

```tsx
      const fileCacheDisabled = disableFileCache === true;
```

Then only load/write file cache and memory hashes when `!fileCacheDisabled`. If `result.notModified` while disabled, perform an unconditional `service.readProjectFile(path, targetProjectId)` and render that result without caching it.

Update controller calls to pass `{disableFileCache}` for connect, project switch, lightweight switch, refresh, and expanded-directory validation.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- web-disable-file-cache-settings.test.ts --runInBand`

Expected: PASS.

### Task 5: Verification

**Files:**
- Verify only.

- [ ] **Step 1: Run focused tests**

Run: `npm test -- web-disable-file-cache-settings.test.ts web-file-not-modified-cache.test.ts web-registry-debug-settings.test.ts web-workspace-project-lightweight.test.ts --runInBand`

Expected: PASS.

- [ ] **Step 2: Run TypeScript check**

Run: `npm run tsc:web`

Expected: PASS.

- [ ] **Step 3: Run full app tests if focused verification passes**

Run: `npm test -- --runInBand`

Expected: PASS.

---

## Self-Review

- Spec coverage: Tasks cover persisted setting, Debug UI row, immediate file-cache clear, directory/file cache bypass, expanded-directory hydration bypass, and verification.
- Placeholder scan: No placeholders or deferred implementation steps remain.
- Type consistency: `disableFileCache` is the single option/property name across persistence, store, controller, and UI.
