import type {RegistryFsEntry, RegistryGitCommit, RegistryGitCommitFile} from '../types/registry';
import {
  DEFAULT_CODE_FONT,
  DEFAULT_CODE_FONT_SIZE,
  DEFAULT_CODE_LINE_HEIGHT,
  DEFAULT_CODE_TAB_SIZE,
  DEFAULT_CODE_THEME,
  isCodeFontId,
  isCodeThemeId,
  type CodeFontId,
  type CodeThemeId,
} from './shikiRenderer';

export type PersistedTab = 'chat' | 'file' | 'git';
export type PersistedThemeMode = 'dark' | 'light';

type DiffCacheEntry = {
  diff: string;
  isBinary: boolean;
  truncated: boolean;
  updatedAt: number;
};

export type PersistedProjectState = {
  dirEntries: Record<string, RegistryFsEntry[]>;
  expandedDirs: string[];
  selectedFile: string;
  pinnedFiles: string[];
  gitCurrentBranch: string;
  commits: RegistryGitCommit[];
  selectedCommit: string;
  commitFilesBySha: Record<string, RegistryGitCommitFile[]>;
  selectedDiff: string;
  diffCacheByKey: Record<string, DiffCacheEntry>;
};

export type PersistedGlobalState = {
  address: string;
  token: string;
  themeMode: PersistedThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
  wrapLines: boolean;
  showLineNumbers: boolean;
  tab: PersistedTab;
  selectedProjectId: string;
};

type PersistedWorkspaceState = {
  version: 1;
  global: PersistedGlobalState;
  projects: Record<string, PersistedProjectState>;
};

const STORAGE_KEY = 'wheelmaker.workspace.state.v1';
const DIFF_CACHE_LIMIT = 120;

function defaultGlobalState(): PersistedGlobalState {
  return {
    address: '',
    token: '',
    themeMode: 'dark',
    codeTheme: DEFAULT_CODE_THEME,
    codeFont: DEFAULT_CODE_FONT,
    codeFontSize: DEFAULT_CODE_FONT_SIZE,
    codeLineHeight: DEFAULT_CODE_LINE_HEIGHT,
    codeTabSize: DEFAULT_CODE_TAB_SIZE,
    wrapLines: false,
    showLineNumbers: true,
    tab: 'file',
    selectedProjectId: '',
  };
}

function defaultProjectState(): PersistedProjectState {
  return {
    dirEntries: {'.': []},
    expandedDirs: ['.'],
    selectedFile: '',
    pinnedFiles: [],
    gitCurrentBranch: '',
    commits: [],
    selectedCommit: '',
    commitFilesBySha: {},
    selectedDiff: '',
    diffCacheByKey: {},
  };
}

function defaultWorkspaceState(): PersistedWorkspaceState {
  return {
    version: 1,
    global: defaultGlobalState(),
    projects: {},
  };
}

function sanitizeProjectState(input: Partial<PersistedProjectState> | undefined): PersistedProjectState {
  const base = defaultProjectState();
  if (!input) return base;
  return {
    dirEntries: typeof input.dirEntries === 'object' && input.dirEntries ? input.dirEntries : base.dirEntries,
    expandedDirs: Array.isArray(input.expandedDirs) && input.expandedDirs.length > 0 ? input.expandedDirs : base.expandedDirs,
    selectedFile: typeof input.selectedFile === 'string' ? input.selectedFile : base.selectedFile,
    pinnedFiles: Array.isArray(input.pinnedFiles) ? input.pinnedFiles.filter(item => typeof item === 'string') : base.pinnedFiles,
    gitCurrentBranch: typeof input.gitCurrentBranch === 'string' ? input.gitCurrentBranch : base.gitCurrentBranch,
    commits: Array.isArray(input.commits) ? input.commits : base.commits,
    selectedCommit: typeof input.selectedCommit === 'string' ? input.selectedCommit : base.selectedCommit,
    commitFilesBySha: typeof input.commitFilesBySha === 'object' && input.commitFilesBySha ? input.commitFilesBySha : base.commitFilesBySha,
    selectedDiff: typeof input.selectedDiff === 'string' ? input.selectedDiff : base.selectedDiff,
    diffCacheByKey: typeof input.diffCacheByKey === 'object' && input.diffCacheByKey ? input.diffCacheByKey : base.diffCacheByKey,
  };
}

function sanitizeGlobalState(input: Partial<PersistedGlobalState> | undefined): PersistedGlobalState {
  const base = defaultGlobalState();
  if (!input) return base;
  return {
    address: typeof input.address === 'string' ? input.address : base.address,
    token: typeof input.token === 'string' ? input.token : base.token,
    themeMode: input.themeMode === 'light' ? 'light' : 'dark',
    codeTheme: typeof input.codeTheme === 'string' && isCodeThemeId(input.codeTheme) ? input.codeTheme : base.codeTheme,
    codeFont: typeof input.codeFont === 'string' && isCodeFontId(input.codeFont) ? input.codeFont : base.codeFont,
    codeFontSize: typeof input.codeFontSize === 'number' && Number.isFinite(input.codeFontSize) ? input.codeFontSize : base.codeFontSize,
    codeLineHeight: typeof input.codeLineHeight === 'number' && Number.isFinite(input.codeLineHeight) ? input.codeLineHeight : base.codeLineHeight,
    codeTabSize: typeof input.codeTabSize === 'number' && Number.isFinite(input.codeTabSize) ? input.codeTabSize : base.codeTabSize,
    wrapLines: typeof input.wrapLines === 'boolean' ? input.wrapLines : base.wrapLines,
    showLineNumbers: typeof input.showLineNumbers === 'boolean' ? input.showLineNumbers : base.showLineNumbers,
    tab: input.tab === 'chat' || input.tab === 'git' ? input.tab : 'file',
    selectedProjectId: typeof input.selectedProjectId === 'string' ? input.selectedProjectId : base.selectedProjectId,
  };
}

function cloneState<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

export class WorkspacePersistenceRepository {
  private state: PersistedWorkspaceState;

  constructor() {
    this.state = this.load();
  }

  private load(): PersistedWorkspaceState {
    if (typeof window === 'undefined') return defaultWorkspaceState();
    try {
      const raw = window.localStorage.getItem(STORAGE_KEY);
      if (!raw) return defaultWorkspaceState();
      const parsed = JSON.parse(raw) as Partial<PersistedWorkspaceState>;
      const next: PersistedWorkspaceState = {
        version: 1,
        global: sanitizeGlobalState(parsed.global),
        projects: {},
      };
      const projects = parsed.projects ?? {};
      for (const [projectId, projectState] of Object.entries(projects)) {
        if (!projectId) continue;
        next.projects[projectId] = sanitizeProjectState(projectState);
      }
      return next;
    } catch {
      return defaultWorkspaceState();
    }
  }

  private save(): void {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(this.state));
    } catch {
      // keep UI resilient when storage is full or unavailable
    }
  }

  private ensureProject(projectId: string): PersistedProjectState {
    if (!this.state.projects[projectId]) {
      this.state.projects[projectId] = defaultProjectState();
    }
    return this.state.projects[projectId];
  }

  getGlobalState(): PersistedGlobalState {
    return cloneState(this.state.global);
  }

  getProjectState(projectId: string): PersistedProjectState {
    return cloneState(this.state.projects[projectId] ?? defaultProjectState());
  }

  patchGlobalState(patch: Partial<PersistedGlobalState>): void {
    this.state.global = sanitizeGlobalState({...this.state.global, ...patch});
    this.save();
  }

  patchProjectState(projectId: string, patch: Partial<PersistedProjectState>): void {
    if (!projectId) return;
    const current = this.ensureProject(projectId);
    this.state.projects[projectId] = sanitizeProjectState({...current, ...patch});
    this.save();
  }

  getProjectDiff(projectId: string, key: string): DiffCacheEntry | null {
    if (!projectId || !key) return null;
    const project = this.state.projects[projectId];
    if (!project) return null;
    const entry = project.diffCacheByKey[key];
    return entry ? cloneState(entry) : null;
  }

  putProjectDiff(projectId: string, key: string, entry: Omit<DiffCacheEntry, 'updatedAt'>): void {
    if (!projectId || !key) return;
    const project = this.ensureProject(projectId);
    const nextCache: Record<string, DiffCacheEntry> = {...project.diffCacheByKey};
    nextCache[key] = {
      ...entry,
      updatedAt: Date.now(),
    };

    const keysByNewest = Object.entries(nextCache)
      .sort((a, b) => b[1].updatedAt - a[1].updatedAt)
      .slice(0, DIFF_CACHE_LIMIT)
      .map(item => item[0]);
    const trimmed: Record<string, DiffCacheEntry> = {};
    for (const cacheKey of keysByNewest) {
      trimmed[cacheKey] = nextCache[cacheKey];
    }
    project.diffCacheByKey = trimmed;
    this.save();
  }
}
