import type {RegistryFsEntry, RegistryGitCommit, RegistryGitCommitFile, RegistryProject} from '../types/registry';
import {
  WorkspacePersistenceRepository,
  type PersistedGlobalState,
  type WorkspaceDatabaseDump,
} from './workspacePersistence';

type ProjectSnapshot = {
  dirEntries: Record<string, RegistryFsEntry[]>;
  expandedDirs: string[];
  selectedFile: string;
  pinnedFiles: string[];
  gitCurrentBranch: string;
  commits: RegistryGitCommit[];
  selectedCommit: string;
  commitFilesBySha: Record<string, RegistryGitCommitFile[]>;
  selectedDiff: string;
};

export type HydratedProjectState = {
  projectId: string;
  dirEntries: Record<string, RegistryFsEntry[]>;
  expandedDirs: string[];
  selectedFile: string;
  pinnedFiles: string[];
  gitCurrentBranch: string;
  commits: RegistryGitCommit[];
  selectedCommit: string;
  commitFilesBySha: Record<string, RegistryGitCommitFile[]>;
  selectedDiff: string;
  cachedDiffText: string;
};

export type CachedDirectory = {
  hash: string;
  entries: RegistryFsEntry[];
};

export type CachedFile = {
  hash: string;
  content: string;
};

function sortEntries(entries: RegistryFsEntry[]): RegistryFsEntry[] {
  return [...entries].sort((a, b) => {
    if (a.kind === 'dir' && b.kind !== 'dir') return -1;
    if (a.kind !== 'dir' && b.kind === 'dir') return 1;
    return a.name.localeCompare(b.name);
  });
}

function uniqueStrings(items: string[]): string[] {
  return [...new Set(items.filter(Boolean))];
}

function diffCacheKey(sha: string, path: string): string {
  return `${sha}::${path}`;
}

export class WorkspaceStore {
  constructor(private readonly persistence = new WorkspacePersistenceRepository()) {}

  ready(): Promise<void> {
    return this.persistence.ready();
  }

  getGlobalState(defaultAddress: string): PersistedGlobalState {
    const saved = this.persistence.getGlobalState();
    return {
      ...saved,
      address: saved.address || defaultAddress,
    };
  }

  rememberGlobalState(patch: Partial<PersistedGlobalState>): void {
    const current = this.persistence.getGlobalState();
    const nextPatch: Partial<PersistedGlobalState> = {...patch};
    if (patch.selectedProjectId !== undefined && !patch.selectedProjectId) {
      nextPatch.selectedProjectId = current.selectedProjectId;
    }
    this.persistence.patchGlobalState(nextPatch);
  }

  selectProjectOnConnect(projects: RegistryProject[], fallbackProjectId: string): string {
    const preferred = this.persistence.getGlobalState().selectedProjectId;
    if (preferred && projects.some(item => item.projectId === preferred)) {
      return preferred;
    }
    return fallbackProjectId;
  }

  hydrateProject(projectId: string, rootEntries: RegistryFsEntry[]): HydratedProjectState {
    const cached = this.persistence.getProjectState(projectId);
    const rootSorted = sortEntries(rootEntries);
    const mergedDirEntries: Record<string, RegistryFsEntry[]> = {...cached.dirEntries, '.': rootSorted};
    const expandedDirs = uniqueStrings(['.', ...cached.expandedDirs.filter(path => path === '.' || !!mergedDirEntries[path])]);
    const selectedFile = cached.selectedFile || (rootSorted.find(item => item.kind === 'file')?.path ?? '');
    const pinnedFiles = uniqueStrings(cached.pinnedFiles.filter(path => !!path));
    const cacheKey = cached.selectedCommit && cached.selectedDiff ? diffCacheKey(cached.selectedCommit, cached.selectedDiff) : '';
    const cachedDiff = cacheKey ? this.persistence.getProjectDiff(projectId, cacheKey) : null;

    return {
      projectId,
      dirEntries: mergedDirEntries,
      expandedDirs: expandedDirs.length > 0 ? expandedDirs : ['.'],
      selectedFile,
      pinnedFiles,
      gitCurrentBranch: cached.gitCurrentBranch || '',
      commits: cached.commits ?? [],
      selectedCommit: cached.selectedCommit || '',
      commitFilesBySha: cached.commitFilesBySha ?? {},
      selectedDiff: cached.selectedDiff || '',
      cachedDiffText: cachedDiff?.diff ?? '',
    };
  }

  rememberProjectSnapshot(projectId: string, snapshot: ProjectSnapshot): void {
    if (!projectId) return;
    this.persistence.patchProjectState(projectId, snapshot);
  }

  getCachedDiff(projectId: string, sha: string, path: string): string | null {
    if (!projectId || !sha || !path) return null;
    return this.persistence.getProjectDiff(projectId, diffCacheKey(sha, path))?.diff ?? null;
  }

  cacheDiff(projectId: string, sha: string, path: string, diff: string, isBinary: boolean, truncated: boolean): void {
    if (!projectId || !sha || !path) return;
    this.persistence.putProjectDiff(projectId, diffCacheKey(sha, path), {
      diff,
      isBinary,
      truncated,
    });
  }

  getCachedDirectory(projectId: string, path: string): CachedDirectory | null {
    const cached = this.persistence.getCachedFile(projectId, 'dir', path);
    if (!cached) return null;
    try {
      const parsed = JSON.parse(cached.value) as RegistryFsEntry[];
      const entries = Array.isArray(parsed) ? parsed : [];
      return {hash: cached.hash, entries};
    } catch {
      return null;
    }
  }

  cacheDirectory(projectId: string, path: string, hash: string, entries: RegistryFsEntry[]): void {
    if (!projectId || !path) return;
    this.persistence.putCachedFile(projectId, 'dir', path, hash, JSON.stringify(entries));
  }

  getCachedFile(projectId: string, path: string): CachedFile | null {
    const cached = this.persistence.getCachedFile(projectId, 'file', path);
    if (!cached) return null;
    return {
      hash: cached.hash,
      content: cached.value,
    };
  }

  cacheFile(projectId: string, path: string, hash: string, content: string): void {
    if (!projectId || !path) return;
    this.persistence.putCachedFile(projectId, 'file', path, hash, content);
  }

  clearLocalCachePreservingToken(): void {
    this.persistence.clearCachePreservingToken();
  }

  dumpDatabase(): Promise<WorkspaceDatabaseDump> {
    return this.persistence.dumpDatabase();
  }
}
