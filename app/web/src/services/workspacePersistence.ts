import type {RegistryChatSession, RegistryGitCommit, RegistryGitCommitFile, RegistrySessionTurn} from '../types/registry';
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
import {
  DEFAULT_CHAT_FONT,
  isChatFontId,
  type ChatFontId,
} from '../chat/chatTypography';
import {
  normalizePortRelayListenPort,
  normalizePortRelayTarget,
  normalizePortRelayTargets,
  type PortRelayTarget,
} from '../portRelayTargets';

export type PersistedTab = 'chat' | 'file' | 'git';
export type PersistedThemeMode = 'dark' | 'light';
export type PersistedFloatingControlSlot =
  | 'upper'
  | 'upper-middle'
  | 'center'
  | 'lower-middle';

export type DiffCacheEntry = {
  diff: string;
  isBinary: boolean;
  truncated: boolean;
  updatedAt: number;
};

export type FileCacheEntry = {
  hash: string;
  value: string;
  updatedAt: number;
};

export type PersistedProjectState = {
  expandedDirs: string[];
  selectedFile: string;
  pinnedFiles: string[];
  gitCurrentBranch: string;
  selectedCommit: string;
  selectedDiff: string;
  selectedChatSessionId: string;
};

export type PersistedProjectCommitsState = {
  commits: RegistryGitCommit[];
  commitFilesBySha: Record<string, RegistryGitCommitFile[]>;
};

export type PersistedGlobalState = {
  address: string;
  token: string;
  deepseekApiKey: string;
  themeMode: PersistedThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
  chatFont: ChatFontId;
  wrapLines: boolean;
  showLineNumbers: boolean;
  hideToolCalls: boolean;
  registryDebug: boolean;
  disableFileCache: boolean;
  localHubReadEnabled: boolean;
  gestureNavigation: boolean;
  tab: PersistedTab;
  selectedProjectId: string;
  selectedChatProjectId: string;
  selectedChatSessionId: string;
  floatingControlSlot: PersistedFloatingControlSlot;
  desktopSidebarWidth: number;
  collapsedProjectIds: string[];
  desktopCollapsedProjectIds: string[];
  pinnedProjectIds: string[];
  portRelayTargets: PortRelayTarget[];
  selectedPortRelayTarget: PortRelayTarget | null;
  portRelayListenPort: number;
};

export type PersistedChatCursor = {
  turnIndex: number;
};

export type PersistedChatSessionEntry = {
  session: RegistryChatSession;
  cursor: PersistedChatCursor;
};

export type PersistedChatSessionContent = {
  turns: RegistrySessionTurn[];
};

type PersistedWorkspaceState = {
  global: PersistedGlobalState;
  projects: Record<string, PersistedProjectState>;
};

export type WorkspaceDatabaseDump = {
  global: Array<{k: string; v: string; updatedAt: number}>;
  projects: Array<{projectId: string; stateJson: string; updatedAt: number}>;
  projectCommits: Array<{projectId: string; commitsJson: string; commitFilesByShaJson: string; updatedAt: number}>;
  chatSessionIndex: Array<{k: string; projectId: string; sessionId: string; sessionJson: string; cursorJson: string; updatedAt: number}>;
  chatSessionContent: Array<{k: string; projectId: string; sessionId: string; turnsJson: string; updatedAt: number}>;
  fileCache: Array<{k: string; hash: string; v: string; updatedAt: number}>;
  diffCache: Array<{k: string; v: string; updatedAt: number}>;
  meta: Array<{k: string; v: string; updatedAt: number}>;
  localStorage: {address: string; token: string};
};

const LOCAL_ADDRESS_KEY = 'wheelmaker.workspace.address';
const LOCAL_TOKEN_KEY = 'wheelmaker.workspace.token';
const WORKSPACE_DB_NAME = 'wheelmaker.workspace.db';
const WORKSPACE_DB_VERSION = 6;
const TABLE_GLOBAL_KV = 'wm_global_kv';
const TABLE_PROJECT_STATE = 'wm_project_state';
const TABLE_PROJECT_COMMITS = 'wm_project_commits';
const TABLE_CHAT_SESSION_INDEX = 'wm_chat_session_index';
const TABLE_CHAT_SESSION_CONTENT = 'wm_chat_session_content';
const TABLE_FILE_CACHE = 'wm_file_cache';
const TABLE_DIFF_CACHE = 'wm_diff_cache';
const TABLE_META = 'wm_meta';
const DIFF_CACHE_LIMIT = 120;

const GLOBAL_KEYS = {
  deepseekApiKey: 'deepseekApiKey',
  themeMode: 'themeMode',
  codeTheme: 'codeTheme',
  codeFont: 'codeFont',
  codeFontSize: 'codeFontSize',
  codeLineHeight: 'codeLineHeight',
  codeTabSize: 'codeTabSize',
  chatFont: 'chatFont',
  wrapLines: 'wrapLines',
  showLineNumbers: 'showLineNumbers',
  hideToolCalls: 'hideToolCalls',
  registryDebug: 'registryDebug',
  disableFileCache: 'disableFileCache',
  localHubReadEnabled: 'localHubReadEnabled',
  gestureNavigation: 'gestureNavigation',
  tab: 'tab',
  selectedProjectId: 'selectedProjectId',
  selectedChatProjectId: 'selectedChatProjectId',
  selectedChatSessionId: 'selectedChatSessionId',
  floatingControlSlot: 'floatingControlSlot',
  desktopSidebarWidth: 'desktopSidebarWidth',
  collapsedProjectIds: 'collapsedProjectIds',
  desktopCollapsedProjectIds: 'desktopCollapsedProjectIds',
  pinnedProjectIds: 'pinnedProjectIds',
  portRelayTargets: 'portRelayTargets',
  selectedPortRelayTarget: 'selectedPortRelayTarget',
  portRelayListenPort: 'portRelayListenPort',
} as const;

function defaultGlobalState(): PersistedGlobalState {
  return {
    address: '',
    token: '',
    deepseekApiKey: '',
    themeMode: 'dark',
    codeTheme: DEFAULT_CODE_THEME,
    codeFont: DEFAULT_CODE_FONT,
    codeFontSize: DEFAULT_CODE_FONT_SIZE,
    codeLineHeight: DEFAULT_CODE_LINE_HEIGHT,
    codeTabSize: DEFAULT_CODE_TAB_SIZE,
    chatFont: DEFAULT_CHAT_FONT,
    wrapLines: false,
    showLineNumbers: true,
    hideToolCalls: true,
    registryDebug: false,
    disableFileCache: false,
    localHubReadEnabled: true,
    gestureNavigation: false,
    tab: 'file',
    selectedProjectId: '',
    selectedChatProjectId: '',
    selectedChatSessionId: '',
    floatingControlSlot: 'upper-middle',
    desktopSidebarWidth: 380,
    collapsedProjectIds: [],
    desktopCollapsedProjectIds: [],
    pinnedProjectIds: [],
    portRelayTargets: [],
    selectedPortRelayTarget: null,
    portRelayListenPort: 28810,
  };
}

function defaultProjectState(): PersistedProjectState {
  return {
    expandedDirs: ['.'],
    selectedFile: '',
    pinnedFiles: [],
    gitCurrentBranch: '',
    selectedCommit: '',
    selectedDiff: '',
    selectedChatSessionId: '',
  };
}

function defaultProjectCommitsState(): PersistedProjectCommitsState {
  return {
    commits: [],
    commitFilesBySha: {},
  };
}

function defaultWorkspaceState(): PersistedWorkspaceState {
  return {
    global: defaultGlobalState(),
    projects: {},
  };
}

function defaultChatCursor(): PersistedChatCursor {
  return {
    turnIndex: 0,
  };
}

function sanitizeChatCursor(input: Partial<PersistedChatCursor> | undefined): PersistedChatCursor {
  const base = defaultChatCursor();
  if (!input) return base;
  const turnIndex = Number.isFinite(input.turnIndex) ? Math.max(0, Math.floor(Number(input.turnIndex))) : base.turnIndex;
  return {turnIndex};
}

function sanitizePersistedTurns(input: unknown): RegistrySessionTurn[] {
  if (!Array.isArray(input)) return [];
  const byIndex = new Map<number, RegistrySessionTurn>();
  for (const raw of input) {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) continue;
    const item = raw as Partial<RegistrySessionTurn>;
    const turnIndex = Number.isFinite(item.turnIndex)
      ? Math.trunc(Number(item.turnIndex))
      : 0;
    if (turnIndex <= 0 || typeof item.content !== 'string' || item.content === '') continue;
    byIndex.set(turnIndex, {
      turnIndex,
      content: item.content,
      finished: item.finished === true,
    });
  }
  return Array.from(byIndex.values()).sort((a, b) => a.turnIndex - b.turnIndex);
}

function persistedFinishedPrefixTurnIndex(turns: RegistrySessionTurn[]): number {
  const finished = new Set<number>();
  for (const turn of turns) {
    if (turn.finished === true && turn.turnIndex > 0) {
      finished.add(Math.trunc(turn.turnIndex));
    }
  }
  let turnIndex = 0;
  while (finished.has(turnIndex + 1)) {
    turnIndex += 1;
  }
  return turnIndex;
}

function persistedTurnPrefix(turns: RegistrySessionTurn[], cursor: PersistedChatCursor): RegistrySessionTurn[] {
  const maxTurnIndex = Math.max(0, Math.trunc(cursor.turnIndex ?? 0));
  const byIndex = new Map<number, RegistrySessionTurn>();
  for (const turn of sanitizePersistedTurns(turns)) {
    if (turn.finished === true) {
      byIndex.set(turn.turnIndex, turn);
    }
  }
  const out: RegistrySessionTurn[] = [];
  for (let turnIndex = 1; turnIndex <= maxTurnIndex; turnIndex += 1) {
    const turn = byIndex.get(turnIndex);
    if (!turn) break;
    out.push({...turn});
  }
  return out;
}

function sessionLatestTurnIndex(session: RegistryChatSession): number | null {
  if (!Number.isFinite(session.latestTurnIndex)) {
    return null;
  }
  return Math.max(0, Math.trunc(Number(session.latestTurnIndex)));
}

function samePersistedTurns(left: RegistrySessionTurn[], right: RegistrySessionTurn[]): boolean {
  if (left.length !== right.length) return false;
  for (let index = 0; index < left.length; index += 1) {
    const a = left[index];
    const b = right[index];
    if (
      a.turnIndex !== b.turnIndex ||
      a.content !== b.content ||
      a.finished !== b.finished
    ) {
      return false;
    }
  }
  return true;
}

export function reconcilePersistedChatSessionCache(
  session: RegistryChatSession,
  cursor: Partial<PersistedChatCursor> | undefined,
  turns: RegistrySessionTurn[],
): {cursor: PersistedChatCursor; turns: RegistrySessionTurn[]; stale: boolean} {
  const safeCursor = sanitizeChatCursor(cursor);
  const sanitizedTurns = sanitizePersistedTurns(turns);
  const latest = sessionLatestTurnIndex(session);
  if (latest != null && latest < safeCursor.turnIndex) {
    return {cursor: defaultChatCursor(), turns: [], stale: true};
  }
  const actualPrefix = persistedFinishedPrefixTurnIndex(sanitizedTurns);
  const repairedCursor = {turnIndex: Math.min(safeCursor.turnIndex, actualPrefix)};
  return {
    cursor: repairedCursor,
    turns: persistedTurnPrefix(sanitizedTurns, repairedCursor),
    stale: false,
  };
}

function sanitizeProjectState(input: Partial<PersistedProjectState> | undefined): PersistedProjectState {
  const base = defaultProjectState();
  if (!input) return base;
  return {
    expandedDirs: Array.isArray(input.expandedDirs) && input.expandedDirs.length > 0 ? input.expandedDirs : base.expandedDirs,
    selectedFile: typeof input.selectedFile === 'string' ? input.selectedFile : base.selectedFile,
    pinnedFiles: Array.isArray(input.pinnedFiles) ? input.pinnedFiles.filter(item => typeof item === 'string') : base.pinnedFiles,
    gitCurrentBranch: typeof input.gitCurrentBranch === 'string' ? input.gitCurrentBranch : base.gitCurrentBranch,
    selectedCommit: typeof input.selectedCommit === 'string' ? input.selectedCommit : base.selectedCommit,
    selectedDiff: typeof input.selectedDiff === 'string' ? input.selectedDiff : base.selectedDiff,
    selectedChatSessionId: typeof input.selectedChatSessionId === 'string' ? input.selectedChatSessionId : base.selectedChatSessionId,
  };
}

function sanitizeProjectCommitsState(input: Partial<PersistedProjectCommitsState> | undefined): PersistedProjectCommitsState {
  const base = defaultProjectCommitsState();
  if (!input) return base;
  return {
    commits: Array.isArray(input.commits) ? input.commits : base.commits,
    commitFilesBySha: typeof input.commitFilesBySha === 'object' && input.commitFilesBySha ? input.commitFilesBySha : base.commitFilesBySha,
  };
}

function sanitizeGlobalState(input: Partial<PersistedGlobalState> | undefined): PersistedGlobalState {
  const base = defaultGlobalState();
  if (!input) return base;
  const collapsedProjectIds = Array.isArray(input.collapsedProjectIds)
    ? Array.from(new Set(input.collapsedProjectIds.filter(item => typeof item === 'string' && item)))
    : Array.isArray(input.desktopCollapsedProjectIds)
      ? Array.from(new Set(input.desktopCollapsedProjectIds.filter(item => typeof item === 'string' && item)))
      : base.collapsedProjectIds;
  const pinnedProjectIds = Array.isArray(input.pinnedProjectIds)
    ? Array.from(new Set(input.pinnedProjectIds.filter(item => typeof item === 'string' && item)))
    : base.pinnedProjectIds;
  const floatingControlSlot =
    input.floatingControlSlot === 'upper' ||
    input.floatingControlSlot === 'upper-middle' ||
    input.floatingControlSlot === 'center' ||
    input.floatingControlSlot === 'lower-middle'
      ? input.floatingControlSlot
      : 'upper-middle';
  const sanitizeDesktopSidebarWidth = (value: unknown, fallback: number): number => {
    const numeric = typeof value === 'number' && Number.isFinite(value) ? value : fallback;
    return Math.min(560, Math.max(320, Math.round(numeric)));
  };
  return {
    address: typeof input.address === 'string' ? input.address : base.address,
    token: typeof input.token === 'string' ? input.token : base.token,
    deepseekApiKey: typeof input.deepseekApiKey === 'string' ? input.deepseekApiKey : base.deepseekApiKey,
    themeMode: input.themeMode === 'light' ? 'light' : 'dark',
    codeTheme: typeof input.codeTheme === 'string' && isCodeThemeId(input.codeTheme) ? input.codeTheme : base.codeTheme,
    codeFont: typeof input.codeFont === 'string' && isCodeFontId(input.codeFont) ? input.codeFont : base.codeFont,
    codeFontSize: typeof input.codeFontSize === 'number' && Number.isFinite(input.codeFontSize) ? input.codeFontSize : base.codeFontSize,
    codeLineHeight: typeof input.codeLineHeight === 'number' && Number.isFinite(input.codeLineHeight) ? input.codeLineHeight : base.codeLineHeight,
    codeTabSize: typeof input.codeTabSize === 'number' && Number.isFinite(input.codeTabSize) ? input.codeTabSize : base.codeTabSize,
    chatFont: typeof input.chatFont === 'string' && isChatFontId(input.chatFont) ? input.chatFont : base.chatFont,
    wrapLines: typeof input.wrapLines === 'boolean' ? input.wrapLines : base.wrapLines,
    showLineNumbers: typeof input.showLineNumbers === 'boolean' ? input.showLineNumbers : base.showLineNumbers,
    hideToolCalls: typeof input.hideToolCalls === 'boolean' ? input.hideToolCalls : base.hideToolCalls,
    registryDebug: typeof input.registryDebug === 'boolean' ? input.registryDebug : base.registryDebug,
    disableFileCache: typeof input.disableFileCache === 'boolean' ? input.disableFileCache : base.disableFileCache,
    localHubReadEnabled: typeof input.localHubReadEnabled === 'boolean' ? input.localHubReadEnabled : base.localHubReadEnabled,
    gestureNavigation: typeof input.gestureNavigation === 'boolean' ? input.gestureNavigation : base.gestureNavigation,
    tab: input.tab === 'chat' || input.tab === 'git' ? input.tab : 'file',
    selectedProjectId: typeof input.selectedProjectId === 'string' ? input.selectedProjectId : base.selectedProjectId,
    selectedChatProjectId: typeof input.selectedChatProjectId === 'string' ? input.selectedChatProjectId : base.selectedChatProjectId,
    selectedChatSessionId: typeof input.selectedChatSessionId === 'string' ? input.selectedChatSessionId : base.selectedChatSessionId,
    floatingControlSlot,
    desktopSidebarWidth: sanitizeDesktopSidebarWidth(input.desktopSidebarWidth, base.desktopSidebarWidth),
    collapsedProjectIds,
    desktopCollapsedProjectIds: collapsedProjectIds,
    pinnedProjectIds,
    portRelayTargets: normalizePortRelayTargets(input.portRelayTargets),
    selectedPortRelayTarget: normalizePortRelayTarget(input.selectedPortRelayTarget),
    portRelayListenPort: normalizePortRelayListenPort(input.portRelayListenPort, base.portRelayListenPort),
  };
}

function cloneState<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function serialize(value: unknown): string {
  return JSON.stringify(value);
}

function tryParse<T>(value: string, fallback: T): T {
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

function fileCacheKey(projectId: string, kind: 'file' | 'dir', path: string): string {
  return `fc:${projectId}:${kind}:${path}`;
}

function diffCacheKey(projectId: string, key: string): string {
  return `dc:${projectId}:${key}`;
}

function chatSessionKey(projectId: string, sessionId: string): string {
  return `cs:${projectId}:${sessionId}`;
}

function chatProjectPrefix(projectId: string): string {
  return `cs:${projectId}:`;
}


function parseChatSessionKey(key: string): {projectId: string; sessionId: string} | null {
  if (!key.startsWith('cs:')) return null;
  const payload = key.slice(3);
  const splitAt = payload.lastIndexOf(':');
  if (splitAt <= 0 || splitAt >= payload.length - 1) {
    return null;
  }
  return {
    projectId: payload.slice(0, splitAt),
    sessionId: payload.slice(splitAt + 1),
  };
}

type RawKVRow = {
  k: string;
  v: string;
  updatedAt: number;
};

type RawProjectStateRow = {
  projectId: string;
  stateJson: string;
  updatedAt: number;
};

type RawProjectCommitsRow = {
  projectId: string;
  commitsJson: string;
  commitFilesByShaJson: string;
  updatedAt: number;
};

type RawChatSessionIndexRow = {
  k: string;
  projectId: string;
  sessionId: string;
  sessionJson: string;
  cursorJson: string;
  updatedAt: number;
};

type RawChatSessionContentRow = {
  k: string;
  projectId: string;
  sessionId: string;
  turnsJson?: string;
  updatedAt: number;
};

type RawFileCacheRow = {
  k: string;
  hash: string;
  v: string;
  updatedAt: number;
};

type RawDiffCacheRow = {
  k: string;
  v: string;
  updatedAt: number;
};

class WorkspaceDatabase {
  private openPromise: Promise<IDBDatabase> | null = null;

  private open(): Promise<IDBDatabase> {
    if (!globalThis.indexedDB) {
      return Promise.reject(new Error('IndexedDB is unavailable in this environment.'));
    }
    if (this.openPromise) {
      return this.openPromise;
    }
    this.openPromise = new Promise((resolve, reject) => {
      const req = globalThis.indexedDB.open(WORKSPACE_DB_NAME, WORKSPACE_DB_VERSION);
      req.onupgradeneeded = (event) => {
        const db = req.result;
        const tx = req.transaction;
        const oldVersion = event.oldVersion;
        if (!db.objectStoreNames.contains(TABLE_GLOBAL_KV)) {
          db.createObjectStore(TABLE_GLOBAL_KV, {keyPath: 'k'});
        }
        if (!db.objectStoreNames.contains(TABLE_PROJECT_STATE)) {
          db.createObjectStore(TABLE_PROJECT_STATE, {keyPath: 'projectId'});
        }
        if (!db.objectStoreNames.contains(TABLE_PROJECT_COMMITS)) {
          db.createObjectStore(TABLE_PROJECT_COMMITS, {keyPath: 'projectId'});
        }
        if (!db.objectStoreNames.contains(TABLE_CHAT_SESSION_INDEX)) {
          db.createObjectStore(TABLE_CHAT_SESSION_INDEX, {keyPath: 'k'});
        }
        if (!db.objectStoreNames.contains(TABLE_CHAT_SESSION_CONTENT)) {
          db.createObjectStore(TABLE_CHAT_SESSION_CONTENT, {keyPath: 'k'});
        }
        if (!db.objectStoreNames.contains(TABLE_FILE_CACHE)) {
          db.createObjectStore(TABLE_FILE_CACHE, {keyPath: 'k'});
        }
        if (!db.objectStoreNames.contains(TABLE_DIFF_CACHE)) {
          db.createObjectStore(TABLE_DIFF_CACHE, {keyPath: 'k'});
        }
        if (!db.objectStoreNames.contains(TABLE_META)) {
          db.createObjectStore(TABLE_META, {keyPath: 'k'});
        }
        if (oldVersion > 0 && oldVersion < 6 && tx) {
          for (const name of [
            TABLE_GLOBAL_KV,
            TABLE_PROJECT_STATE,
            TABLE_PROJECT_COMMITS,
            TABLE_CHAT_SESSION_INDEX,
            TABLE_CHAT_SESSION_CONTENT,
            TABLE_FILE_CACHE,
            TABLE_DIFF_CACHE,
            TABLE_META,
          ]) {
            if (db.objectStoreNames.contains(name)) {
              tx.objectStore(name).clear();
            }
          }
        }
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error ?? new Error('open workspace db failed'));
    });
    return this.openPromise;
  }

  private request<T>(req: IDBRequest<T>): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error ?? new Error('workspace db request failed'));
    });
  }

  private async run<T>(
    stores: string | string[],
    mode: IDBTransactionMode,
    action: (tx: IDBTransaction) => Promise<T>,
  ): Promise<T> {
    const db = await this.open();
    return new Promise<T>((resolve, reject) => {
      const tx = db.transaction(stores, mode);
      action(tx)
        .then(result => {
          tx.oncomplete = () => resolve(result);
          tx.onerror = () => reject(tx.error ?? new Error('workspace db transaction failed'));
          tx.onabort = () => reject(tx.error ?? new Error('workspace db transaction aborted'));
        })
        .catch(error => {
          reject(error);
          try {
            tx.abort();
          } catch {
            // ignore
          }
        });
    });
  }

  async getAllRows<T>(storeName: string): Promise<T[]> {
    return this.run(storeName, 'readonly', async tx => {
      const store = tx.objectStore(storeName);
      return this.request(store.getAll() as IDBRequest<T[]>);
    });
  }

  async putRow(storeName: string, row: unknown): Promise<void> {
    await this.run(storeName, 'readwrite', async tx => {
      const store = tx.objectStore(storeName);
      await this.request(store.put(row));
    });
  }

  async deleteRow(storeName: string, key: IDBValidKey): Promise<void> {
    await this.run(storeName, 'readwrite', async tx => {
      const store = tx.objectStore(storeName);
      await this.request(store.delete(key));
    });
  }

  async clearStores(storeNames: string[]): Promise<void> {
    await this.run(storeNames, 'readwrite', async tx => {
      for (const name of storeNames) {
        await this.request(tx.objectStore(name).clear());
      }
    });
  }
}

function sortByKey<T extends {k: string}>(rows: T[]): T[] {
  return [...rows].sort((a, b) => a.k.localeCompare(b.k));
}

function sortByProjectId<T extends {projectId: string}>(rows: T[]): T[] {
  return [...rows].sort((a, b) => a.projectId.localeCompare(b.projectId));
}

function compareUpdatedAtDesc(a: string, b: string): number {
  const aTime = Date.parse(a || '');
  const bTime = Date.parse(b || '');
  const safeA = Number.isFinite(aTime) ? aTime : 0;
  const safeB = Number.isFinite(bTime) ? bTime : 0;
  if (safeA === safeB) return 0;
  return safeA > safeB ? -1 : 1;
}

export class WorkspacePersistenceRepository {
  private readonly db = new WorkspaceDatabase();
  private state: PersistedWorkspaceState;
  private readonly projectCommits: Record<string, PersistedProjectCommitsState> = {};
  private readonly chatSessionIndex = new Map<string, PersistedChatSessionEntry>();
  private readonly chatSessionContent = new Map<string, PersistedChatSessionContent>();
  private readonly diffCache = new Map<string, DiffCacheEntry>();
  private readonly fileCache = new Map<string, FileCacheEntry>();
  private readonly readyPromise: Promise<void>;

  constructor() {
    this.state = defaultWorkspaceState();
    this.readyPromise = this.initialize();
  }

  ready(): Promise<void> {
    return this.readyPromise;
  }

  private async initialize(): Promise<void> {
    const [globalRows, projectRows, projectCommitRows, chatIndexRows, chatContentRows, diffRows, fileRows] = await Promise.all([
      this.db.getAllRows<RawKVRow>(TABLE_GLOBAL_KV),
      this.db.getAllRows<RawProjectStateRow>(TABLE_PROJECT_STATE),
      this.db.getAllRows<RawProjectCommitsRow>(TABLE_PROJECT_COMMITS),
      this.db.getAllRows<RawChatSessionIndexRow>(TABLE_CHAT_SESSION_INDEX),
      this.db.getAllRows<RawChatSessionContentRow>(TABLE_CHAT_SESSION_CONTENT),
      this.db.getAllRows<RawDiffCacheRow>(TABLE_DIFF_CACHE),
      this.db.getAllRows<RawFileCacheRow>(TABLE_FILE_CACHE),
    ]);

    const hasPersisted =
      globalRows.length > 0 ||
      projectRows.length > 0 ||
      projectCommitRows.length > 0 ||
      chatIndexRows.length > 0 ||
      chatContentRows.length > 0 ||
      diffRows.length > 0 ||
      fileRows.length > 0;

    if (!hasPersisted) {
      this.state = defaultWorkspaceState();
      await this.saveAllStateToDb();
      return;
    }

    this.state = this.fromDbRows(globalRows, projectRows);
    this.restoreProjectCommits(projectCommitRows);
    if (this.hasIncompatibleChatContentRows(chatContentRows)) {
      await this.resetPersistentCacheAfterIncompatibleSchema();
      return;
    }

    const repairedChatCache = this.restoreChatSessions(chatIndexRows, chatContentRows);
    this.restoreDiffCache(diffRows);
    this.restoreFileCache(fileRows);
    if (repairedChatCache) {
      await this.saveAllStateToDb();
    }
  }

  private fromDbRows(globalRows: RawKVRow[], projectRows: RawProjectStateRow[]): PersistedWorkspaceState {
    const base = defaultWorkspaceState();
    const globalPatch: Partial<PersistedGlobalState> = {};
    for (const row of globalRows) {
      if (!(row.k in GLOBAL_KEYS)) continue;
      (globalPatch as Record<string, unknown>)[row.k] = tryParse(row.v, row.v);
    }

    const projects: Record<string, PersistedProjectState> = {};
    for (const row of projectRows) {
      const raw = tryParse<Partial<PersistedProjectState>>(row.stateJson, defaultProjectState());
      projects[row.projectId] = sanitizeProjectState(raw);
    }

    return {
      global: sanitizeGlobalState({...base.global, ...globalPatch, ...this.readLocalIdentityState()}),
      projects,
    };
  }

  private restoreProjectCommits(rows: RawProjectCommitsRow[]): void {
    for (const key of Object.keys(this.projectCommits)) {
      delete this.projectCommits[key];
    }
    for (const row of rows) {
      const commits = tryParse<RegistryGitCommit[]>(row.commitsJson, []);
      const commitFilesBySha = tryParse<Record<string, RegistryGitCommitFile[]>>(row.commitFilesByShaJson, {});
      this.projectCommits[row.projectId] = sanitizeProjectCommitsState({
        commits,
        commitFilesBySha,
      });
    }
  }

  private restoreChatSessions(indexRows: RawChatSessionIndexRow[], contentRows: RawChatSessionContentRow[]): boolean {
    this.chatSessionIndex.clear();
    this.chatSessionContent.clear();
    let repaired = false;

    for (const row of indexRows) {
      const session = tryParse<RegistryChatSession>(row.sessionJson, {
        sessionId: row.sessionId,
        title: row.sessionId,
        preview: '',
        updatedAt: '',
        messageCount: 0,
      });
      const cursor = sanitizeChatCursor(tryParse<Partial<PersistedChatCursor>>(row.cursorJson, defaultChatCursor()));
      this.chatSessionIndex.set(row.k, {
        session,
        cursor,
      });
    }

    for (const row of contentRows) {
      const turns = tryParse<RegistrySessionTurn[]>(row.turnsJson ?? '[]', []);
      this.chatSessionContent.set(row.k, {
        turns: sanitizePersistedTurns(turns),
      });
    }

    for (const [key, entry] of this.chatSessionIndex.entries()) {
      const existingContent = this.chatSessionContent.get(key) ?? {turns: []};
      const nextCache = reconcilePersistedChatSessionCache(entry.session, entry.cursor, existingContent.turns);
      if (
        nextCache.cursor.turnIndex !== entry.cursor.turnIndex ||
        !samePersistedTurns(existingContent.turns, nextCache.turns)
      ) {
        repaired = true;
      }
      this.chatSessionIndex.set(key, {
        session: entry.session,
        cursor: nextCache.cursor,
      });
      this.chatSessionContent.set(key, {
        turns: nextCache.turns,
      });
    }

    return repaired;
  }

  private hasIncompatibleChatContentRows(rows: RawChatSessionContentRow[]): boolean {
    return rows.some(row => typeof row.turnsJson !== 'string');
  }

  private async resetPersistentCacheAfterIncompatibleSchema(): Promise<void> {
    const localIdentity = this.readLocalIdentityState();
    const preservedToken = localIdentity.token || this.state.global.token;
    const preservedAddress = localIdentity.address || this.state.global.address;
    this.state = defaultWorkspaceState();
    this.state.global.token = preservedToken;
    this.state.global.address = preservedAddress;
    this.saveLocalIdentityState({address: preservedAddress, token: preservedToken});
    for (const key of Object.keys(this.projectCommits)) {
      delete this.projectCommits[key];
    }
    this.chatSessionIndex.clear();
    this.chatSessionContent.clear();
    this.diffCache.clear();
    this.fileCache.clear();
    await this.saveAllStateToDb();
  }
  private restoreDiffCache(rows: RawDiffCacheRow[]): void {
    this.diffCache.clear();
    for (const row of rows) {
      const payload = tryParse<{diff?: string; isBinary?: boolean; truncated?: boolean}>(row.v, {});
      this.diffCache.set(row.k, {
        diff: typeof payload.diff === 'string' ? payload.diff : '',
        isBinary: !!payload.isBinary,
        truncated: !!payload.truncated,
        updatedAt: row.updatedAt,
      });
    }
  }

  private restoreFileCache(rows: RawFileCacheRow[]): void {
    this.fileCache.clear();
    for (const row of rows) {
      this.fileCache.set(row.k, {
        hash: row.hash || '',
        value: row.v || '',
        updatedAt: row.updatedAt,
      });
    }
  }

  private async saveAllStateToDb(): Promise<void> {
    const now = Date.now();
    await this.db.clearStores([
      TABLE_GLOBAL_KV,
      TABLE_PROJECT_STATE,
      TABLE_PROJECT_COMMITS,
      TABLE_CHAT_SESSION_INDEX,
      TABLE_CHAT_SESSION_CONTENT,
      TABLE_DIFF_CACHE,
      TABLE_FILE_CACHE,
      TABLE_META,
    ]);

    const globalRows: Array<{k: string; v: string; updatedAt: number}> = [
      {k: GLOBAL_KEYS.deepseekApiKey, v: serialize(this.state.global.deepseekApiKey), updatedAt: now},
      {k: GLOBAL_KEYS.themeMode, v: serialize(this.state.global.themeMode), updatedAt: now},
      {k: GLOBAL_KEYS.codeTheme, v: serialize(this.state.global.codeTheme), updatedAt: now},
      {k: GLOBAL_KEYS.codeFont, v: serialize(this.state.global.codeFont), updatedAt: now},
      {k: GLOBAL_KEYS.codeFontSize, v: serialize(this.state.global.codeFontSize), updatedAt: now},
      {k: GLOBAL_KEYS.codeLineHeight, v: serialize(this.state.global.codeLineHeight), updatedAt: now},
      {k: GLOBAL_KEYS.codeTabSize, v: serialize(this.state.global.codeTabSize), updatedAt: now},
      {k: GLOBAL_KEYS.chatFont, v: serialize(this.state.global.chatFont), updatedAt: now},
      {k: GLOBAL_KEYS.wrapLines, v: serialize(this.state.global.wrapLines), updatedAt: now},
      {k: GLOBAL_KEYS.showLineNumbers, v: serialize(this.state.global.showLineNumbers), updatedAt: now},
      {k: GLOBAL_KEYS.hideToolCalls, v: serialize(this.state.global.hideToolCalls), updatedAt: now},
      {k: GLOBAL_KEYS.registryDebug, v: serialize(this.state.global.registryDebug), updatedAt: now},
      {k: GLOBAL_KEYS.disableFileCache, v: serialize(this.state.global.disableFileCache), updatedAt: now},
      {k: GLOBAL_KEYS.localHubReadEnabled, v: serialize(this.state.global.localHubReadEnabled), updatedAt: now},
      {k: GLOBAL_KEYS.gestureNavigation, v: serialize(this.state.global.gestureNavigation), updatedAt: now},
      {k: GLOBAL_KEYS.tab, v: serialize(this.state.global.tab), updatedAt: now},
      {k: GLOBAL_KEYS.selectedProjectId, v: serialize(this.state.global.selectedProjectId), updatedAt: now},
      {k: GLOBAL_KEYS.selectedChatProjectId, v: serialize(this.state.global.selectedChatProjectId), updatedAt: now},
      {k: GLOBAL_KEYS.selectedChatSessionId, v: serialize(this.state.global.selectedChatSessionId), updatedAt: now},
      {k: GLOBAL_KEYS.floatingControlSlot, v: serialize(this.state.global.floatingControlSlot), updatedAt: now},
      {k: GLOBAL_KEYS.desktopSidebarWidth, v: serialize(this.state.global.desktopSidebarWidth), updatedAt: now},
      {k: GLOBAL_KEYS.collapsedProjectIds, v: serialize(this.state.global.collapsedProjectIds), updatedAt: now},
      {k: GLOBAL_KEYS.desktopCollapsedProjectIds, v: serialize(this.state.global.desktopCollapsedProjectIds), updatedAt: now},
      {k: GLOBAL_KEYS.pinnedProjectIds, v: serialize(this.state.global.pinnedProjectIds), updatedAt: now},
      {k: GLOBAL_KEYS.portRelayTargets, v: serialize(this.state.global.portRelayTargets), updatedAt: now},
      {k: GLOBAL_KEYS.selectedPortRelayTarget, v: serialize(this.state.global.selectedPortRelayTarget), updatedAt: now},
      {k: GLOBAL_KEYS.portRelayListenPort, v: serialize(this.state.global.portRelayListenPort), updatedAt: now},
    ];

    for (const row of globalRows) {
      await this.db.putRow(TABLE_GLOBAL_KV, row);
    }

    for (const [projectId, state] of Object.entries(this.state.projects)) {
      await this.db.putRow(TABLE_PROJECT_STATE, {
        projectId,
        stateJson: serialize(state),
        updatedAt: now,
      });
    }

    for (const [projectId, state] of Object.entries(this.projectCommits)) {
      await this.db.putRow(TABLE_PROJECT_COMMITS, {
        projectId,
        commitsJson: serialize(state.commits),
        commitFilesByShaJson: serialize(state.commitFilesBySha),
        updatedAt: now,
      });
    }

    for (const [k, entry] of this.chatSessionIndex.entries()) {
      const parsed = parseChatSessionKey(k);
      const projectId = parsed?.projectId || '';
      const sessionId = parsed?.sessionId || entry.session.sessionId;
      await this.db.putRow(TABLE_CHAT_SESSION_INDEX, {
        k,
        projectId,
        sessionId,
        sessionJson: serialize(entry.session),
        cursorJson: serialize(entry.cursor),
        updatedAt: now,
      });
    }

    for (const [k, content] of this.chatSessionContent.entries()) {
      const parsed = parseChatSessionKey(k);
      const projectId = parsed?.projectId || '';
      const sessionId = parsed?.sessionId || '';
      await this.db.putRow(TABLE_CHAT_SESSION_CONTENT, {
        k,
        projectId,
        sessionId,
        turnsJson: serialize(content.turns),
        updatedAt: now,
      });
    }
    for (const [k, entry] of this.diffCache.entries()) {
      await this.db.putRow(TABLE_DIFF_CACHE, {
        k,
        v: serialize({
          diff: entry.diff,
          isBinary: entry.isBinary,
          truncated: entry.truncated,
        }),
        updatedAt: entry.updatedAt,
      });
    }

    for (const [k, entry] of this.fileCache.entries()) {
      await this.db.putRow(TABLE_FILE_CACHE, {
        k,
        hash: entry.hash,
        v: entry.value,
        updatedAt: entry.updatedAt,
      });
    }

    await this.db.putRow(TABLE_META, {
      k: 'schemaVersion',
      v: serialize(WORKSPACE_DB_VERSION),
      updatedAt: now,
    });
  }

  private readLocalIdentityState(): Pick<PersistedGlobalState, 'address' | 'token'> {
    if (typeof window === 'undefined') {
      return {address: '', token: ''};
    }
    let address = '';
    let token = '';
    try {
      const rawAddress = window.localStorage.getItem(LOCAL_ADDRESS_KEY);
      address = typeof rawAddress === 'string' ? rawAddress : '';
    } catch {
      // ignore
    }
    try {
      const rawToken = window.localStorage.getItem(LOCAL_TOKEN_KEY);
      token = typeof rawToken === 'string' ? rawToken : '';
    } catch {
      // ignore
    }
    return {address, token};
  }

  private saveLocalIdentityState(value: Pick<PersistedGlobalState, 'address' | 'token'>): void {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage.setItem(LOCAL_ADDRESS_KEY, value.address || '');
    } catch {
      // ignore
    }
    try {
      window.localStorage.setItem(LOCAL_TOKEN_KEY, value.token || '');
    } catch {
      // ignore
    }
  }

  private ensureProject(projectId: string): PersistedProjectState {
    if (!this.state.projects[projectId]) {
      this.state.projects[projectId] = defaultProjectState();
    }
    return this.state.projects[projectId];
  }

  private ensureProjectCommits(projectId: string): PersistedProjectCommitsState {
    if (!this.projectCommits[projectId]) {
      this.projectCommits[projectId] = defaultProjectCommitsState();
    }
    return this.projectCommits[projectId];
  }

  getGlobalState(): PersistedGlobalState {
    const localIdentity = this.readLocalIdentityState();
    this.state.global = sanitizeGlobalState({...this.state.global, ...localIdentity});
    return cloneState(this.state.global);
  }

  getProjectState(projectId: string): PersistedProjectState {
    return cloneState(this.state.projects[projectId] ?? defaultProjectState());
  }

  getProjectCommitsState(projectId: string): PersistedProjectCommitsState {
    return cloneState(this.projectCommits[projectId] ?? defaultProjectCommitsState());
  }

  getProjectChatSessions(projectId: string): PersistedChatSessionEntry[] {
    if (!projectId) return [];
    const prefix = chatProjectPrefix(projectId);
    const entries: PersistedChatSessionEntry[] = [];
    for (const [key, value] of this.chatSessionIndex.entries()) {
      if (!key.startsWith(prefix)) continue;
      entries.push(cloneState(value));
    }
    return entries.sort((a, b) => {
      const updatedAtDelta = compareUpdatedAtDesc(a.session.updatedAt || '', b.session.updatedAt || '');
      if (updatedAtDelta !== 0) return updatedAtDelta;
      return (a.session.title || '').localeCompare(b.session.title || '');
    });
  }

  getProjectChatSessionContent(projectId: string, sessionId: string): PersistedChatSessionContent | null {
    if (!projectId || !sessionId) return null;
    const entry = this.chatSessionContent.get(chatSessionKey(projectId, sessionId));
    return entry ? cloneState(entry) : null;
  }

  replaceProjectChatSessions(projectId: string, sessions: PersistedChatSessionEntry[]): void {
    if (!projectId) return;
    const prefix = chatProjectPrefix(projectId);
    const keepKeys = new Set<string>();
    const now = Date.now();

    for (const sessionEntry of sessions) {
      const sessionId = sessionEntry.session.sessionId;
      if (!sessionId) continue;
      const key = chatSessionKey(projectId, sessionId);
      keepKeys.add(key);
      const existingContent = this.chatSessionContent.get(key) ?? {turns: []};
      const nextCache = reconcilePersistedChatSessionCache(
        sessionEntry.session,
        sessionEntry.cursor,
        existingContent.turns,
      );
      const entry: PersistedChatSessionEntry = {
        session: sessionEntry.session,
        cursor: nextCache.cursor,
      };
      this.chatSessionIndex.set(key, entry);
      const contentChanged = !samePersistedTurns(existingContent.turns, nextCache.turns);
      if (contentChanged) {
        this.chatSessionContent.set(key, {turns: nextCache.turns});
      }
      void this.ready().then(() => this.db.putRow(TABLE_CHAT_SESSION_INDEX, {
        k: key,
        projectId,
        sessionId,
        sessionJson: serialize(entry.session),
        cursorJson: serialize(entry.cursor),
        updatedAt: now,
      })).catch(() => undefined);
      if (contentChanged) {
        void this.ready().then(() => this.db.putRow(TABLE_CHAT_SESSION_CONTENT, {
          k: key,
          projectId,
          sessionId,
          turnsJson: serialize(nextCache.turns),
          updatedAt: now,
        })).catch(() => undefined);
      }
    }

    const deleteKeys: string[] = [];
    for (const key of this.chatSessionIndex.keys()) {
      if (!key.startsWith(prefix)) continue;
      if (keepKeys.has(key)) continue;
      deleteKeys.push(key);
    }

    for (const key of deleteKeys) {
      this.chatSessionIndex.delete(key);
      this.chatSessionContent.delete(key);
      void this.ready().then(async () => {
        await this.db.deleteRow(TABLE_CHAT_SESSION_INDEX, key);
        await this.db.deleteRow(TABLE_CHAT_SESSION_CONTENT, key);
      }).catch(() => undefined);
    }
  }

  patchProjectChatSession(projectId: string, session: RegistryChatSession, cursor: PersistedChatCursor): void {
    const sessionId = (session.sessionId || '').trim();
    if (!projectId || !sessionId) return;
    const key = chatSessionKey(projectId, sessionId);
    const now = Date.now();
    const existingContent = this.chatSessionContent.get(key) ?? {turns: []};
    const nextCache = reconcilePersistedChatSessionCache(session, cursor, existingContent.turns);
    const entry: PersistedChatSessionEntry = {
      session,
      cursor: nextCache.cursor,
    };
    this.chatSessionIndex.set(key, entry);
    const contentChanged = !samePersistedTurns(existingContent.turns, nextCache.turns);
    if (contentChanged) {
      this.chatSessionContent.set(key, {turns: nextCache.turns});
    }
    void this.ready().then(() => this.db.putRow(TABLE_CHAT_SESSION_INDEX, {
      k: key,
      projectId,
      sessionId,
      sessionJson: serialize(entry.session),
      cursorJson: serialize(entry.cursor),
      updatedAt: now,
    })).catch(() => undefined);
    if (contentChanged) {
      void this.ready().then(() => this.db.putRow(TABLE_CHAT_SESSION_CONTENT, {
        k: key,
        projectId,
        sessionId,
        turnsJson: serialize(nextCache.turns),
        updatedAt: now,
      })).catch(() => undefined);
    }
  }

  patchProjectChatSessionContent(
    projectId: string,
    sessionId: string,
    turns: RegistrySessionTurn[],
  ): void {
    if (!projectId || !sessionId) return;
    const key = chatSessionKey(projectId, sessionId);
    const now = Date.now();
    const sanitizedTurns = sanitizePersistedTurns(turns);
    const proposedCursor = {turnIndex: persistedFinishedPrefixTurnIndex(sanitizedTurns)};
    const existingIndex = this.chatSessionIndex.get(key);
    const repairedCache = existingIndex
      ? reconcilePersistedChatSessionCache(existingIndex.session, proposedCursor, sanitizedTurns)
      : {
          cursor: proposedCursor,
          turns: persistedTurnPrefix(sanitizedTurns, proposedCursor),
          stale: false,
        };
    const payload: PersistedChatSessionContent = {
      turns: repairedCache.turns,
    };
    this.chatSessionContent.set(key, payload);
    if (existingIndex && existingIndex.cursor.turnIndex !== repairedCache.cursor.turnIndex) {
      const nextIndex = {
        session: existingIndex.session,
        cursor: repairedCache.cursor,
      };
      this.chatSessionIndex.set(key, nextIndex);
      void this.ready().then(() => this.db.putRow(TABLE_CHAT_SESSION_INDEX, {
        k: key,
        projectId,
        sessionId,
        sessionJson: serialize(nextIndex.session),
        cursorJson: serialize(nextIndex.cursor),
        updatedAt: now,
      })).catch(() => undefined);
    }
    void this.ready().then(() => this.db.putRow(TABLE_CHAT_SESSION_CONTENT, {
      k: key,
      projectId,
      sessionId,
      turnsJson: serialize(payload.turns),
      updatedAt: now,
    })).catch(() => undefined);
  }

  deleteProjectChatSession(projectId: string, sessionId: string): void {
    if (!projectId || !sessionId) return;
    const key = chatSessionKey(projectId, sessionId);
    this.chatSessionIndex.delete(key);
    this.chatSessionContent.delete(key);
    void this.ready().then(async () => {
      await this.db.deleteRow(TABLE_CHAT_SESSION_INDEX, key);
      await this.db.deleteRow(TABLE_CHAT_SESSION_CONTENT, key);
    }).catch(() => undefined);
  }
  patchGlobalState(patch: Partial<PersistedGlobalState>): void {
    this.state.global = sanitizeGlobalState({...this.state.global, ...patch});
    this.saveLocalIdentityState(this.state.global);

    const now = Date.now();
    const next = cloneState(this.state.global);
    void this.ready().then(async () => {
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.deepseekApiKey, v: serialize(next.deepseekApiKey), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.themeMode, v: serialize(next.themeMode), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.codeTheme, v: serialize(next.codeTheme), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.codeFont, v: serialize(next.codeFont), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.codeFontSize, v: serialize(next.codeFontSize), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.codeLineHeight, v: serialize(next.codeLineHeight), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.codeTabSize, v: serialize(next.codeTabSize), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.chatFont, v: serialize(next.chatFont), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.wrapLines, v: serialize(next.wrapLines), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.showLineNumbers, v: serialize(next.showLineNumbers), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.hideToolCalls, v: serialize(next.hideToolCalls), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.registryDebug, v: serialize(next.registryDebug), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.disableFileCache, v: serialize(next.disableFileCache), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.localHubReadEnabled, v: serialize(next.localHubReadEnabled), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.gestureNavigation, v: serialize(next.gestureNavigation), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.tab, v: serialize(next.tab), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.selectedProjectId, v: serialize(next.selectedProjectId), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.selectedChatProjectId, v: serialize(next.selectedChatProjectId), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.selectedChatSessionId, v: serialize(next.selectedChatSessionId), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.floatingControlSlot, v: serialize(next.floatingControlSlot), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.desktopSidebarWidth, v: serialize(next.desktopSidebarWidth), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.collapsedProjectIds, v: serialize(next.collapsedProjectIds), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.desktopCollapsedProjectIds, v: serialize(next.desktopCollapsedProjectIds), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.pinnedProjectIds, v: serialize(next.pinnedProjectIds), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.portRelayTargets, v: serialize(next.portRelayTargets), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.selectedPortRelayTarget, v: serialize(next.selectedPortRelayTarget), updatedAt: now});
      await this.db.putRow(TABLE_GLOBAL_KV, {k: GLOBAL_KEYS.portRelayListenPort, v: serialize(next.portRelayListenPort), updatedAt: now});
    }).catch(() => undefined);
  }

  patchProjectState(projectId: string, patch: Partial<PersistedProjectState>): void {
    if (!projectId) return;
    const current = this.ensureProject(projectId);
    this.state.projects[projectId] = sanitizeProjectState({...current, ...patch});
    const now = Date.now();
    const nextState = cloneState(this.state.projects[projectId]);
    void this.ready().then(() => this.db.putRow(TABLE_PROJECT_STATE, {
      projectId,
      stateJson: serialize(nextState),
      updatedAt: now,
    })).catch(() => undefined);
  }

  patchProjectCommitsState(projectId: string, patch: Partial<PersistedProjectCommitsState>): void {
    if (!projectId) return;
    const current = this.ensureProjectCommits(projectId);
    this.projectCommits[projectId] = sanitizeProjectCommitsState({...current, ...patch});
    const now = Date.now();
    const nextState = cloneState(this.projectCommits[projectId]);
    void this.ready().then(() => this.db.putRow(TABLE_PROJECT_COMMITS, {
      projectId,
      commitsJson: serialize(nextState.commits),
      commitFilesByShaJson: serialize(nextState.commitFilesBySha),
      updatedAt: now,
    })).catch(() => undefined);
  }

  getProjectDiff(projectId: string, key: string): DiffCacheEntry | null {
    if (!projectId || !key) return null;
    const entry = this.diffCache.get(diffCacheKey(projectId, key));
    return entry ? cloneState(entry) : null;
  }

  putProjectDiff(projectId: string, key: string, entry: Omit<DiffCacheEntry, 'updatedAt'>): void {
    if (!projectId || !key) return;
    const now = Date.now();
    const k = diffCacheKey(projectId, key);
    this.diffCache.set(k, {
      ...entry,
      updatedAt: now,
    });

    const prefix = `dc:${projectId}:`;
    const keysByNewest = [...this.diffCache.entries()]
      .filter(([cacheKey]) => cacheKey.startsWith(prefix))
      .sort((a, b) => b[1].updatedAt - a[1].updatedAt)
      .slice(0, DIFF_CACHE_LIMIT)
      .map(item => item[0]);

    const keepSet = new Set(keysByNewest);
    for (const cacheKey of [...this.diffCache.keys()]) {
      if (!cacheKey.startsWith(prefix)) continue;
      if (keepSet.has(cacheKey)) continue;
      this.diffCache.delete(cacheKey);
      void this.ready().then(() => this.db.deleteRow(TABLE_DIFF_CACHE, cacheKey)).catch(() => undefined);
    }

    const payload = this.diffCache.get(k);
    if (!payload) return;
    void this.ready().then(() => this.db.putRow(TABLE_DIFF_CACHE, {
      k,
      v: serialize({
        diff: payload.diff,
        isBinary: payload.isBinary,
        truncated: payload.truncated,
      }),
      updatedAt: payload.updatedAt,
    })).catch(() => undefined);
  }

  getCachedFile(projectId: string, kind: 'file' | 'dir', path: string): FileCacheEntry | null {
    if (!projectId || !path) return null;
    const entry = this.fileCache.get(fileCacheKey(projectId, kind, path));
    return entry ? cloneState(entry) : null;
  }

  putCachedFile(projectId: string, kind: 'file' | 'dir', path: string, hash: string, value: string): void {
    if (!projectId || !path) return;
    const now = Date.now();
    const k = fileCacheKey(projectId, kind, path);
    this.fileCache.set(k, {
      hash: hash || '',
      value,
      updatedAt: now,
    });
    void this.ready().then(() => this.db.putRow(TABLE_FILE_CACHE, {
      k,
      hash: hash || '',
      v: value,
      updatedAt: now,
    })).catch(() => undefined);
  }

  clearFileCache(): void {
    this.fileCache.clear();
    void this.ready().then(async () => {
      await this.db.clearStores([TABLE_FILE_CACHE]);
    }).catch(() => undefined);
  }

  clearCachePreservingToken(): void {
    const localIdentity = this.readLocalIdentityState();
    const preservedToken = localIdentity.token || this.state.global.token;
    const preservedAddress = localIdentity.address || this.state.global.address;

    this.state = defaultWorkspaceState();
    this.state.global.token = preservedToken;
    this.state.global.address = preservedAddress;
    this.saveLocalIdentityState({address: preservedAddress, token: preservedToken});

    for (const key of Object.keys(this.projectCommits)) {
      delete this.projectCommits[key];
    }
    this.chatSessionIndex.clear();
    this.chatSessionContent.clear();
    this.diffCache.clear();
    this.fileCache.clear();

    const now = Date.now();
    void this.ready().then(async () => {
      await this.db.clearStores([
        TABLE_GLOBAL_KV,
        TABLE_PROJECT_STATE,
        TABLE_PROJECT_COMMITS,
        TABLE_CHAT_SESSION_INDEX,
        TABLE_CHAT_SESSION_CONTENT,
        TABLE_DIFF_CACHE,
        TABLE_FILE_CACHE,
        TABLE_META,
      ]);
      await this.db.putRow(TABLE_META, {
        k: 'schemaVersion',
        v: serialize(WORKSPACE_DB_VERSION),
        updatedAt: now,
      });
      await this.db.putRow(TABLE_META, {
        k: 'cacheClearedAt',
        v: serialize(new Date(now).toISOString()),
        updatedAt: now,
      });
    }).catch(() => undefined);
  }

  async dumpDatabase(): Promise<WorkspaceDatabaseDump> {
    await this.ready();
    const [global, projects, projectCommits, chatSessionIndex, chatSessionContent, fileCache, diffCache, meta] = await Promise.all([
      this.db.getAllRows<{k: string; v: string; updatedAt: number}>(TABLE_GLOBAL_KV),
      this.db.getAllRows<{projectId: string; stateJson: string; updatedAt: number}>(TABLE_PROJECT_STATE),
      this.db.getAllRows<{projectId: string; commitsJson: string; commitFilesByShaJson: string; updatedAt: number}>(TABLE_PROJECT_COMMITS),
      this.db.getAllRows<{k: string; projectId: string; sessionId: string; sessionJson: string; cursorJson: string; updatedAt: number}>(TABLE_CHAT_SESSION_INDEX),
      this.db.getAllRows<{k: string; projectId: string; sessionId: string; turnsJson: string; updatedAt: number}>(TABLE_CHAT_SESSION_CONTENT),
      this.db.getAllRows<{k: string; hash: string; v: string; updatedAt: number}>(TABLE_FILE_CACHE),
      this.db.getAllRows<{k: string; v: string; updatedAt: number}>(TABLE_DIFF_CACHE),
      this.db.getAllRows<{k: string; v: string; updatedAt: number}>(TABLE_META),
    ]);
    return {
      global: sortByKey(global),
      projects: sortByProjectId(projects),
      projectCommits: sortByProjectId(projectCommits),
      chatSessionIndex: sortByKey(chatSessionIndex),
      chatSessionContent: sortByKey(chatSessionContent),
      fileCache: sortByKey(fileCache),
      diffCache: sortByKey(diffCache),
      meta: sortByKey(meta),
      localStorage: this.readLocalIdentityState(),
    };
  }
}

