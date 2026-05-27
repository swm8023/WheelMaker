# Disable File Cache Design

## Goal

Add a persisted Debug setting named `Disable File Cache`. When enabled, the app clears the existing file cache and stops using file or directory cache for File tab directory and file access. Each directory expansion and file open should request fresh data from the registry service.

## Scope

- Add a `Disable File Cache` checkbox to the Settings Debug section.
- Persist the setting as a global workspace preference, defaulting to `false`.
- Clear the existing file cache immediately when the setting is enabled.
- While enabled, bypass cached directory entries, cached file contents, in-memory file hashes, and `knownHash` conditional reads.
- While enabled, avoid writing refreshed directory and file data back to the file cache.
- When disabled again, restore the existing cache behavior for future visits.

Out of scope:

- Chat, git diff, registry debug, token, and other non-file caches.
- Server-side cache behavior, if any.
- Changing file tree layout or settings navigation.

## Architecture

The setting belongs in `WorkspacePersistence` global state next to existing Debug-oriented global preferences such as `registryDebug`. `WorkspaceStore` should expose focused methods to read/update this preference and to clear only file cache state.

The File tab already has two cache layers:

- IndexedDB-backed file cache in `WorkspacePersistence` / `WorkspaceStore`, storing both `file` and `dir` entries.
- Component-local refs in `main.tsx`: `dirHashRef`, `fileHashRef`, and `fileCacheRef`.

The new setting should gate both layers. Enabling it clears both layers, and read paths branch on the setting before consulting or writing cache.

## Data Flow

On startup:

1. Load persisted global state.
2. Initialize React state from `persistedGlobal.disableFileCache`, defaulting to `false`.
3. Pass the setting into cache-sensitive project hydration and file read paths.

When enabling:

1. Persist `disableFileCache: true`.
2. Clear `wm_file_cache` through a targeted file-cache clear method.
3. Clear `dirHashRef`, `fileHashRef`, and `fileCacheRef`.
4. Keep the current visible tree/content until the next explicit directory or file access refreshes it.

When accessing directories:

- If disabled, call `service.listDirectory(path)` without `knownHash`.
- Ignore persisted directory cache and do not call `cacheDirectory`.
- Existing expanded directory validation should not hydrate expanded directories from cached directory contents.

When accessing files:

- If disabled, call `service.readProjectFile(path, projectId)` without `knownHash`.
- Ignore persisted file content and in-memory file content cache.
- Treat `notModified` as unusable without a client cache; perform an unconditional read fallback if needed.
- Do not call `cacheFile`.

## Error Handling

Clearing file cache is best-effort, matching existing persistence behavior. If IndexedDB clear fails asynchronously, the UI setting still remains enabled and runtime paths continue to bypass cache, so user-visible behavior remains fresh reads.

If a backend unexpectedly returns `notModified` during disabled mode, the app should immediately retry the read without `knownHash`. That keeps the UI from rendering stale or empty content.

## Testing

Add focused Jest coverage for:

- `disableFileCache` exists in global persisted state, defaults to `false`, normalizes loaded values, and is persisted.
- Settings Debug section renders a `Disable File Cache` checkbox bound to the persisted setting.
- Enabling the setting clears file cache through `WorkspaceStore`.
- Directory loading and expanded-directory validation skip persisted directory cache, skip `knownHash`, and skip cache writes when disabled.
- File reading skips persisted file cache, skips `knownHash`, and skips cache writes when disabled.

## Acceptance Criteria

- With `Disable File Cache` off, existing file and directory cache behavior is unchanged.
- Turning it on clears stored file cache immediately.
- While on, opening files and folders always goes to the service without conditional cache keys.
- The setting survives page reloads.
- Turning it off allows future file and directory reads to repopulate cache.
