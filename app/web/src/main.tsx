import React, {useEffect, useMemo, useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import setiThemeJson from '@codingame/monaco-vscode-theme-seti-default-extension/resources/vs-seti-icon-theme.json';
import setiFontUrl from '@codingame/monaco-vscode-theme-seti-default-extension/resources/seti.woff';
import '@vscode/codicons/dist/codicon.css';
import '@fontsource/ibm-plex-sans/400.css';
import '@fontsource/ibm-plex-sans/500.css';
import '@fontsource/ibm-plex-sans/600.css';
import '@fontsource/jetbrains-mono/400.css';

declare const require: (id: string) => any;

import {getDefaultRegistryAddress, toRegistryWsUrl} from './runtime';
import {RegistryWorkspaceService} from './services/registryWorkspaceService';
import {
  CODE_FONT_OPTIONS,
  CODE_THEME_OPTIONS,
  CODE_THEME_OPTION_GROUPS,
  DEFAULT_CODE_FONT,
  DEFAULT_CODE_FONT_SIZE,
  DEFAULT_CODE_LINE_HEIGHT,
  DEFAULT_CODE_TAB_SIZE,
  DEFAULT_CODE_THEME,
  isCodeFontId,
  isCodeThemeId,
  renderShikiDiffHtml,
  renderShikiHtml,
  resolveCodeFontFamily,
  type CodeFontId,
  type CodeThemeId,
  type DiffRenderLine,
} from './services/shikiRenderer';
import {WorkspaceController} from './services/workspaceController';
import {WorkspaceStore} from './services/workspaceStore';
import type {
  RegistryFsEntry,
  RegistryFsInfo,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitStatus,
  RegistryGitWorkspaceChangedPayload,
  RegistryProject,
  RegistryProjectEventPayload,
} from './types/registry';
import './styles.css';

type Tab = 'chat' | 'file' | 'git';
type ThemeMode = 'dark' | 'light';
type DirEntries = Record<string, RegistryFsEntry[]>;
type GitDiffSource = 'commit' | 'worktree';
type WorkingTreeFileEntry = {
  path: string;
  status: string;
  scope: 'staged' | 'unstaged' | 'untracked';
};
type SetiThemeSection = {
  file: string;
  fileExtensions?: Record<string, string>;
  fileNames?: Record<string, string>;
};
type SetiIconDefinition = {
  fontCharacter?: string;
  fontColor?: string;
};
type SetiTheme = {
  iconDefinitions: Record<string, SetiIconDefinition>;
  file: string;
  fileExtensions?: Record<string, string>;
  fileNames?: Record<string, string>;
  light?: SetiThemeSection;
};
type SetiResolvedIcon = {
  glyph: string;
  color: string;
};
type GitDiffChange =
  | {type: 'insert'; content: string; lineNumber: number}
  | {type: 'delete'; content: string; lineNumber: number}
  | {type: 'normal'; content: string; oldLineNumber: number; newLineNumber: number};
type GitDiffFile = {
  hunks?: Array<{
    changes?: GitDiffChange[];
  }>;
};
type GitDiffParser = {
  parse: (source: string) => GitDiffFile[];
};

const WORKING_TREE_COMMIT_ID = '__WORKING_TREE__';
const gitdiffParser = require('gitdiff-parser') as GitDiffParser;

type ThinkingBlockProps = {
  content: string;
  isStreaming: boolean;
};

function ThinkingBlock({content, isStreaming}: ThinkingBlockProps) {
  const [expanded, setExpanded] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);
  const [contentHeight, setContentHeight] = useState(0);

  useEffect(() => {
    if (contentRef.current) {
      setContentHeight(contentRef.current.scrollHeight);
    }
  }, [content, expanded]);

  // Auto-collapse when streaming finishes
  const wasStreamingRef = useRef(isStreaming);
  useEffect(() => {
    if (wasStreamingRef.current && !isStreaming) {
      setExpanded(false);
    }
    wasStreamingRef.current = isStreaming;
  }, [isStreaming]);

  const summaryText = useMemo(() => {
    if (isStreaming) return '';
    const firstLine = content.split('\n').find(l => l.trim().length > 0) || '';
    return firstLine.length > 120 ? firstLine.slice(0, 120) + '…' : firstLine;
  }, [content, isStreaming]);

  return (
    <div className={`thinking-block ${isStreaming ? 'streaming' : 'done'} ${expanded ? 'expanded' : ''}`}>
      <button
        className="thinking-header"
        onClick={() => !isStreaming && setExpanded(v => !v)}
        disabled={isStreaming}
        aria-expanded={expanded}
      >
        <span className="thinking-icon codicon codicon-sparkle" />
        {isStreaming ? (
          <span className="thinking-title streaming-text">Thinking<span className="thinking-dots"><span>.</span><span>.</span><span>.</span></span></span>
        ) : (
          <span className="thinking-title summary-text">{summaryText}</span>
        )}
        {!isStreaming && (
          <span className={`thinking-chevron codicon ${expanded ? 'codicon-chevron-up' : 'codicon-chevron-down'}`} />
        )}
      </button>
      <div
        className="thinking-body"
        style={{maxHeight: expanded ? contentHeight + 16 : 0}}
      >
        <div className="thinking-content" ref={contentRef}>{content}</div>
      </div>
    </div>
  );
}

const service = new RegistryWorkspaceService();
const workspaceStore = new WorkspaceStore();
const workspaceController = new WorkspaceController(service, workspaceStore);
const setiTheme = setiThemeJson as SetiTheme;
const VS_CODE_EDITOR_FONT_FAMILY = "Consolas, 'Courier New', monospace";
const MAX_AUTO_RENDER_DIFF_CHARS = 200000;
const CODE_FONT_SIZE_OPTIONS = [12, 13, 14, 15, 16] as const;
const CODE_LINE_HEIGHT_OPTIONS = [1.35, 1.45, 1.5, 1.6, 1.7] as const;
const CODE_TAB_SIZE_OPTIONS = [2, 4, 8] as const;

function clampCodeFontSize(value: number): number {
  return Math.min(20, Math.max(11, Number.isFinite(value) ? value : DEFAULT_CODE_FONT_SIZE));
}

function clampCodeLineHeight(value: number): number {
  return Math.min(2, Math.max(1.2, Number.isFinite(value) ? value : DEFAULT_CODE_LINE_HEIGHT));
}

function clampCodeTabSize(value: number): number {
  return Math.min(8, Math.max(1, Number.isFinite(value) ? value : DEFAULT_CODE_TAB_SIZE));
}

function sortEntries(entries: RegistryFsEntry[]): RegistryFsEntry[] {
  return [...entries].sort((a, b) => {
    if (a.kind === 'dir' && b.kind !== 'dir') return -1;
    if (a.kind !== 'dir' && b.kind === 'dir') return 1;
    return a.name.localeCompare(b.name);
  });
}

function getFileExtension(path: string): string {
  const match = /\.([a-z0-9]+)$/i.exec(path);
  return match ? match[1].toLowerCase() : '';
}

function detectCodeLanguage(path: string): string {
  const ext = getFileExtension(path);
  switch (ext) {
    case 'ts':
      return 'typescript';
    case 'tsx':
      return 'tsx';
    case 'js':
      return 'javascript';
    case 'jsx':
      return 'jsx';
    case 'json':
      return 'json';
    case 'go':
      return 'go';
    case 'c':
      return 'c';
    case 'cc':
    case 'cpp':
    case 'cxx':
    case 'h':
    case 'hh':
    case 'hpp':
      return 'cpp';
    case 'rs':
      return 'rust';
    case 'sh':
    case 'bash':
      return 'bash';
    case 'yml':
    case 'yaml':
      return 'yaml';
    case 'md':
    case 'markdown':
      return 'markdown';
    case 'diff':
    case 'patch':
      return 'diff';
    case 'html':
      return 'markup';
    default:
      return 'clike';
  }
}

function isLoopbackHost(host: string): boolean {
  const v = host.trim().toLowerCase();
  return v === '127.0.0.1' || v === 'localhost' || v === '::1' || v === '[::1]';
}

function isLoopbackAddress(address: string): boolean {
  const input = address.trim();
  if (!input) return false;
  if (/^wss?:\/\//i.test(input)) {
    try {
      const url = new URL(input);
      return isLoopbackHost(url.hostname);
    } catch {
      return false;
    }
  }
  if (/^https?:\/\//i.test(input)) {
    try {
      const url = new URL(input);
      return isLoopbackHost(url.hostname);
    } catch {
      return false;
    }
  }
  const host = input.split('/')[0].split(':')[0];
  return isLoopbackHost(host);
}

function resolveInitialRegistryAddress(savedAddress: string, defaultAddress: string): string {
  const pageHost = window.location.hostname;
  if (!isLoopbackHost(pageHost) && isLoopbackAddress(savedAddress)) {
    return defaultAddress;
  }
  return savedAddress || defaultAddress;
}

type UnifiedDiffRow = {
  kind: 'context' | 'added' | 'removed';
  oldLineNumber: number | null;
  newLineNumber: number | null;
  text: string;
};

function parseUnifiedDiffRows(content: string): UnifiedDiffRow[] {
  const rows: UnifiedDiffRow[] = [];
  try {
    const files = gitdiffParser.parse(content);
    for (const file of files) {
      for (const hunk of file.hunks || []) {
        for (const change of hunk.changes || []) {
          if (change.type === 'insert') {
            rows.push({
              kind: 'added',
              oldLineNumber: null,
              newLineNumber: change.lineNumber,
              text: change.content,
            });
            continue;
          }
          if (change.type === 'delete') {
            rows.push({
              kind: 'removed',
              oldLineNumber: change.lineNumber,
              newLineNumber: null,
              text: change.content,
            });
            continue;
          }
          rows.push({
            kind: 'context',
            oldLineNumber: change.oldLineNumber,
            newLineNumber: change.newLineNumber,
            text: change.content,
          });
        }
      }
    }
  } catch {
    // fall through to fallback parser for non-standard diff snippets
  }

  if (rows.length > 0) {
    return rows;
  }

  // Fallback for non-standard patches: keep a readable inline rendering.
  const lines = content.split('\n');
  for (const raw of lines) {
    if (raw.startsWith('+')) {
      rows.push({
        kind: 'added',
        oldLineNumber: null,
        newLineNumber: null,
        text: raw.slice(1),
      });
      continue;
    }
    if (raw.startsWith('-')) {
      rows.push({
        kind: 'removed',
        oldLineNumber: null,
        newLineNumber: null,
        text: raw.slice(1),
      });
      continue;
    }
    rows.push({
      kind: 'context',
      oldLineNumber: null,
      newLineNumber: null,
      text: raw.startsWith(' ') ? raw.slice(1) : raw,
    });
  }
  return rows;
}

function buildInlineDiffRenderLines(rows: UnifiedDiffRow[]): DiffRenderLine[] {
  return rows.map(row => ({
    code: row.text,
    lineNumber: row.newLineNumber ?? row.oldLineNumber,
    kind: row.kind,
  }));
}
function toSetiGlyph(fontCharacter?: string): string {
  if (!fontCharacter) return '?';
  const hex = fontCharacter.replace('\\', '');
  const code = Number.parseInt(hex, 16);
  if (Number.isNaN(code)) return '?';
  return String.fromCodePoint(code);
}

function resolveSetiIcon(name: string, mode: ThemeMode): SetiResolvedIcon {
  const section: SetiThemeSection = mode === 'light' && setiTheme.light
    ? {file: setiTheme.light.file, fileExtensions: setiTheme.light.fileExtensions, fileNames: setiTheme.light.fileNames}
    : {file: setiTheme.file, fileExtensions: setiTheme.fileExtensions, fileNames: setiTheme.fileNames};

  const lowerName = name.toLowerCase();
  let iconId = section.file;

  if (section.fileNames?.[lowerName]) {
    iconId = section.fileNames[lowerName];
  } else if (section.fileExtensions) {
    const parts = lowerName.split('.');
    for (let i = 0; i < parts.length; i += 1) {
      const candidate = parts.slice(i).join('.');
      if (section.fileExtensions[candidate]) {
        iconId = section.fileExtensions[candidate];
        break;
      }
    }
  }

  const definition = setiTheme.iconDefinitions[iconId] ?? setiTheme.iconDefinitions[section.file] ?? {};
  return {
    glyph: toSetiGlyph(definition.fontCharacter),
    color: definition.fontColor ?? '#d4d7d6',
  };
}

function setiFontFaceCss(): string {
  return `@font-face { font-family: 'wm-seti'; src: url('${setiFontUrl}') format('woff'); font-weight: normal; font-style: normal; }`;
}

function buildWorkingTreeFiles(status: RegistryGitStatus): WorkingTreeFileEntry[] {
  const rows: WorkingTreeFileEntry[] = [];
  for (const item of status.unstaged ?? []) {
    if (!item.path) continue;
    rows.push({path: item.path, status: item.status, scope: 'unstaged'});
  }
  for (const item of status.staged ?? []) {
    if (!item.path) continue;
    rows.push({path: item.path, status: item.status, scope: 'staged'});
  }
  for (const item of status.untracked ?? []) {
    if (!item.path) continue;
    rows.push({path: item.path, status: item.status || 'U', scope: 'untracked'});
  }
  return rows;
}

function isHeavyGeneratedDiffPath(path: string): boolean {
  const normalized = (path || '').replace(/\\/g, '/').toLowerCase();
  return (
    normalized.endsWith('/dist/bundle.js') ||
    normalized.endsWith('/dist/bundle.js.map') ||
    normalized.endsWith('/dist/index.html')
  );
}

function pickPreferredPath<T extends {path: string}>(items: T[]): string {
  if (items.length === 0) return '';
  const preferred = items.find(item => !isHeavyGeneratedDiffPath(item.path));
  return (preferred ?? items[0]).path;
}

type PrismCodeBlockProps = {
  content: string;
  language: string;
  wrap: boolean;
  lineNumbers: boolean;
  themeMode: ThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
};

function PrismCodeBlock({
  content,
  language,
  wrap,
  lineNumbers,
  themeMode,
  codeTheme,
  codeFont,
  codeFontSize,
  codeLineHeight,
  codeTabSize,
}: PrismCodeBlockProps) {
  const [html, setHtml] = useState('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const nextHtml = await renderShikiHtml({
        code: content,
        language,
        themeMode,
        codeTheme,
        codeFont,
        codeFontSize,
        codeLineHeight,
        codeTabSize,
        wrap,
        lineNumbers,
        mode: 'block',
      });
      if (!cancelled) {
        setHtml(nextHtml);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [content, language, themeMode, codeTheme, codeFont, codeFontSize, codeLineHeight, codeTabSize, wrap, lineNumbers]);

  return (
    <div
      className={`code-wrap ${wrap ? 'wrap' : 'nowrap'}`}
      dangerouslySetInnerHTML={{__html: html || '<pre><code> </code></pre>'}}
    />
  );
}

type PrismDiffPaneProps = {
  content: string;
  language: string;
  wrap: boolean;
  lineNumbers: boolean;
  themeMode: ThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontFamily: string;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
};

function PrismDiffPane({
  content,
  language,
  wrap,
  lineNumbers,
  themeMode,
  codeTheme,
  codeFont,
  codeFontFamily,
  codeFontSize,
  codeLineHeight,
  codeTabSize,
}: PrismDiffPaneProps) {
  const [diffHtml, setDiffHtml] = useState('');
  const rows = useMemo(() => parseUnifiedDiffRows(content), [content]);
  const lines = useMemo(() => buildInlineDiffRenderLines(rows), [rows]);

  useEffect(() => {
    let cancelled = false;
    if (rows.length === 0) {
      setDiffHtml('');
      return () => {
        cancelled = true;
      };
    }

    setDiffHtml('');
    (async () => {
      const nextDiffHtml = await renderShikiDiffHtml({
        lines,
        language,
        themeMode,
        codeTheme,
        codeFont,
        codeFontSize,
        codeLineHeight,
        codeTabSize,
        wrap,
        lineNumbers,
      });
      if (!cancelled) {
        setDiffHtml(nextDiffHtml);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [lines, rows.length, language, themeMode, codeTheme, codeFont, codeFontSize, codeLineHeight, codeTabSize, wrap, lineNumbers]);

  if (rows.length === 0) return <div className="muted block">No diff hunks available</div>;

  const diffStyle = {
    fontFamily: codeFontFamily || VS_CODE_EDITOR_FONT_FAMILY,
    fontSize: `${codeFontSize}px`,
    lineHeight: String(codeLineHeight),
    tabSize: String(codeTabSize),
  };

  return (
    <div className={`code-wrap diff-wrap ${wrap ? 'wrap' : 'nowrap'}`}>
      <div
        className={`diff-inline ${wrap ? 'wrap' : 'nowrap'}`}
        style={diffStyle}
        dangerouslySetInnerHTML={{__html: diffHtml || '<pre><code> </code></pre>'}}
      />
    </div>
  );
}
function App() {
  const defaultRegistryAddress = useMemo(() => getDefaultRegistryAddress(), []);
  const persistedGlobal = useMemo(() => workspaceStore.getGlobalState(defaultRegistryAddress), [defaultRegistryAddress]);
  const initialRegistryAddress = resolveInitialRegistryAddress(persistedGlobal.address || '', defaultRegistryAddress);
  const [connected, setConnected] = useState(false);
  const [address, setAddress] = useState(initialRegistryAddress);
  const [token, setToken] = useState(persistedGlobal.token || '');
  const [error, setError] = useState('');
  const [autoConnecting, setAutoConnecting] = useState(false);
  const autoConnectTriedRef = useRef(false);

  const [themeMode, setThemeMode] = useState<ThemeMode>(persistedGlobal.themeMode === 'light' ? 'light' : 'dark');
  const [codeTheme, setCodeTheme] = useState<CodeThemeId>(
    typeof persistedGlobal.codeTheme === 'string' && isCodeThemeId(persistedGlobal.codeTheme)
      ? persistedGlobal.codeTheme
      : DEFAULT_CODE_THEME,
  );
  const [codeFont, setCodeFont] = useState<CodeFontId>(
    typeof persistedGlobal.codeFont === 'string' && isCodeFontId(persistedGlobal.codeFont)
      ? persistedGlobal.codeFont
      : DEFAULT_CODE_FONT,
  );
  const [codeFontSize, setCodeFontSize] = useState<number>(clampCodeFontSize(Number(persistedGlobal.codeFontSize)));
  const [codeLineHeight, setCodeLineHeight] = useState<number>(clampCodeLineHeight(Number(persistedGlobal.codeLineHeight)));
  const [codeTabSize, setCodeTabSize] = useState<number>(clampCodeTabSize(Number(persistedGlobal.codeTabSize)));
  const [wrapLines, setWrapLines] = useState(!!persistedGlobal.wrapLines);
  const [showLineNumbers, setShowLineNumbers] = useState(
    typeof persistedGlobal.showLineNumbers === 'boolean' ? persistedGlobal.showLineNumbers : true,
  );
  const codeFontFamily = useMemo(() => resolveCodeFontFamily(codeFont), [codeFont]);
  const setiFontCss = useMemo(() => setiFontFaceCss(), []);
  const resolveFileIcon = (name: string) => resolveSetiIcon(name, themeMode);

  const [windowWidth, setWindowWidth] = useState<number>(window.innerWidth);
  const isWide = windowWidth >= 900;

  const [tab, setTab] = useState<Tab>(persistedGlobal.tab ?? 'file');
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [sidebarSettingsOpen, setSidebarSettingsOpen] = useState(false);

  const [projectMenuOpen, setProjectMenuOpen] = useState(false);

  const [projects, setProjects] = useState<RegistryProject[]>([]);
  const [projectId, setProjectId] = useState('');
  const projectIdRef = useRef('');
  const currentProjectRef = useRef<RegistryProject | null>(null);
  const knownProjectRevRef = useRef('');
  const knownGitRevRef = useRef('');
  const knownWorktreeRevRef = useRef('');
  const [loadingProject, setLoadingProject] = useState(false);
  const [refreshingProject, setRefreshingProject] = useState(false);

  const [dirEntries, setDirEntries] = useState<DirEntries>({'.': []});
  const [expandedDirs, setExpandedDirs] = useState<string[]>(['.']);
  const expandedDirsRef = useRef<string[]>(['.']);
  const [loadingDirs, setLoadingDirs] = useState<Record<string, boolean>>({});
  const [selectedFile, setSelectedFile] = useState('');
  const selectedFileRef = useRef('');
  const [pinnedFiles, setPinnedFiles] = useState<string[]>([]);
  const [fileContent, setFileContent] = useState('');
  const [fileInfo, setFileInfo] = useState<RegistryFsInfo | null>(null);
  const [fileLoading, setFileLoading] = useState(false);
  const [fileSearchQuery, setFileSearchQuery] = useState('');
  const [currentMatchIndex, setCurrentMatchIndex] = useState(0);
  const [gotoLineInput, setGotoLineInput] = useState('');
  const [searchToolsOpen, setSearchToolsOpen] = useState(false);
  const [gotoToolsOpen, setGotoToolsOpen] = useState(false);
  const fileScrollRef = useRef<HTMLDivElement | null>(null);
  const liveRefreshTimerRef = useRef<number | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const dirHashRef = useRef<Record<string, string>>({});
  const fileHashRef = useRef<Record<string, string>>({});
  const fileCacheRef = useRef<Record<string, string>>({});
  const fileReadSeqRef = useRef(0);
  const fileSideActionsRef = useRef<HTMLDivElement | null>(null);

  const [chatSessions] = useState(['General', 'WheelMaker App', 'Go Service']);
  const [chatSessionIndex, setChatSessionIndex] = useState(0);

  const [gitLoading, setGitLoading] = useState(false);
  const [gitError, setGitError] = useState('');
  const [gitCurrentBranch, setGitCurrentBranch] = useState('');
  const [gitDirty, setGitDirty] = useState(false);
  const [gitStatusSummary, setGitStatusSummary] = useState({staged: 0, unstaged: 0, untracked: 0});
  const [gitLoadedProjectId, setGitLoadedProjectId] = useState('');
  const [commits, setCommits] = useState<RegistryGitCommit[]>([]);
  const [selectedCommit, setSelectedCommit] = useState('');
  const [commitFilesBySha, setCommitFilesBySha] = useState<Record<string, RegistryGitCommitFile[]>>({});
  const [workingTreeFiles, setWorkingTreeFiles] = useState<WorkingTreeFileEntry[]>([]);
  const [selectedDiffSource, setSelectedDiffSource] = useState<GitDiffSource>('commit');
  const [selectedDiffScope, setSelectedDiffScope] = useState<'staged' | 'unstaged' | 'untracked'>('unstaged');
  const [selectedDiff, setSelectedDiff] = useState('');
  const [allowHeavyDiffLoad, setAllowHeavyDiffLoad] = useState(false);
  const [allowLargeDiffRender, setAllowLargeDiffRender] = useState(false);
  const [diffText, setDiffText] = useState('');
  const [diffLoading, setDiffLoading] = useState(false);

  useEffect(() => {
    projectIdRef.current = projectId;
  }, [projectId]);

  useEffect(() => {
    setAllowHeavyDiffLoad(false);
    setAllowLargeDiffRender(false);
  }, [selectedDiff, selectedCommit, selectedDiffSource, selectedDiffScope]);

  useEffect(() => {
    const onResize = () => setWindowWidth(window.innerWidth);
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  useEffect(() => {
    if (isWide) {
      setDrawerOpen(false);
    }
  }, [isWide]);

  useEffect(() => {
    const onPointer = () => {
      setProjectMenuOpen(false);
    };
    window.addEventListener('pointerdown', onPointer);
    return () => window.removeEventListener('pointerdown', onPointer);
  }, []);

  useEffect(() => {
    const onPointerDown = (event: PointerEvent) => {
      if (!searchToolsOpen && !gotoToolsOpen) return;
      const container = fileSideActionsRef.current;
      if (!container) return;
      const target = event.target as Node | null;
      if (target && container.contains(target)) return;
      setSearchToolsOpen(false);
      setGotoToolsOpen(false);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [searchToolsOpen, gotoToolsOpen]);

  useEffect(() => {
    workspaceStore.rememberGlobalState({
      address,
      token,
      themeMode,
      codeTheme,
      codeFont,
      codeFontSize,
      codeLineHeight,
      codeTabSize,
      wrapLines,
      showLineNumbers,
      tab,
      selectedProjectId: projectId,
    });
  }, [address, token, themeMode, codeTheme, codeFont, codeFontSize, codeLineHeight, codeTabSize, wrapLines, showLineNumbers, tab, projectId]);

  useEffect(() => {
    if (!projectId) return;
    workspaceStore.rememberProjectSnapshot(projectId, {
      dirEntries,
      expandedDirs,
      selectedFile,
      pinnedFiles,
      gitCurrentBranch,
      commits,
      selectedCommit,
      commitFilesBySha,
      selectedDiff,
    });
  }, [
    projectId,
    dirEntries,
    expandedDirs,
    selectedFile,
    pinnedFiles,
    gitCurrentBranch,
    commits,
    selectedCommit,
    commitFilesBySha,
    selectedDiff,
  ]);

  const currentProjectName = useMemo(
    () => projects.find(item => item.projectId === projectId)?.name ?? 'Project',
    [projectId, projects],
  );
  const currentProject = useMemo(
    () => projects.find(item => item.projectId === projectId) ?? null,
    [projectId, projects],
  );
  currentProjectRef.current = currentProject;
  expandedDirsRef.current = expandedDirs;
  selectedFileRef.current = selectedFile;

  useEffect(() => {
    knownProjectRevRef.current = currentProject?.projectRev ?? '';
    knownGitRevRef.current = currentProject?.git?.gitRev ?? '';
    if (currentProject?.git?.worktreeRev) {
      knownWorktreeRevRef.current = currentProject.git.worktreeRev;
    }
  }, [currentProject]);

  const currentCommitFiles = useMemo(
    () => commitFilesBySha[selectedCommit] ?? [],
    [commitFilesBySha, selectedCommit],
  );

  const worktreeActive = selectedDiffSource === 'worktree';

  const isExpanded = (path: string) => expandedDirs.includes(path);
  const isSelectedFilePinned = selectedFile ? pinnedFiles.includes(selectedFile) : false;
  const hasPinnedFiles = pinnedFiles.length > 0;
  const fileLines = useMemo(() => fileContent.split('\n'), [fileContent]);
  const fileSearchMatches = useMemo(() => {
    const query = fileSearchQuery.trim().toLocaleLowerCase();
    if (!query) return [] as number[];
    const matches: number[] = [];
    for (let i = 0; i < fileLines.length; i += 1) {
      if (fileLines[i].toLocaleLowerCase().includes(query)) {
        matches.push(i + 1);
      }
    }
    return matches;
  }, [fileContent, fileLines, fileSearchQuery]);

  const applyHydratedProjectState = (
    hydrated: {
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
    },
  ) => {
    fileReadSeqRef.current += 1;
    fileHashRef.current = {};
    fileCacheRef.current = {};
    expandedDirsRef.current = hydrated.expandedDirs;
    selectedFileRef.current = hydrated.selectedFile;
    setProjectId(hydrated.projectId);
    setDirEntries(hydrated.dirEntries);
    setExpandedDirs(hydrated.expandedDirs);
    setSelectedFile(hydrated.selectedFile);
    setPinnedFiles([]);
    setPinnedFiles(hydrated.pinnedFiles);
    setFileContent('');
    setFileInfo(null);
    setGitCurrentBranch(hydrated.gitCurrentBranch);
    setCommits(hydrated.commits);
    setSelectedCommit(hydrated.selectedCommit);
    setCommitFilesBySha(hydrated.commitFilesBySha);
    setSelectedDiff(hydrated.selectedDiff);
    setDiffText(hydrated.cachedDiffText);
    setWorkingTreeFiles([]);
    setGitLoadedProjectId('');
    setProjectMenuOpen(false);
    setSidebarSettingsOpen(false);
    if (!isWide) setDrawerOpen(false);
  };

  const togglePinSelectedFile = () => {
    if (!selectedFile) return;
    setPinnedFiles(prev => prev.includes(selectedFile) ? prev.filter(path => path !== selectedFile) : [...prev, selectedFile]);
  };

  useEffect(() => {
    if (fileSearchMatches.length === 0) {
      setCurrentMatchIndex(0);
      return;
    }
    setCurrentMatchIndex(prev => Math.min(prev, fileSearchMatches.length - 1));
  }, [fileSearchMatches.length]);

  useEffect(() => {
    if (!searchToolsOpen) return;
    const query = fileSearchQuery.trim();
    if (!query || fileSearchMatches.length === 0) return;
    setCurrentMatchIndex(0);
    window.requestAnimationFrame(() => {
      scrollToFileLine(fileSearchMatches[0]);
    });
  }, [fileSearchMatches, fileSearchQuery, searchToolsOpen]);

  useEffect(() => () => {
    if (liveRefreshTimerRef.current !== null) {
      window.clearTimeout(liveRefreshTimerRef.current);
    }
    if (reconnectTimerRef.current !== null) {
      window.clearTimeout(reconnectTimerRef.current);
    }
  }, []);

  const scrollToFileLine = (line: number) => {
    const container = fileScrollRef.current;
    if (!container) return;
    const lineElement = container.querySelector(`.code-wrap [data-line-number="${line}"]`) as HTMLElement | null;
    if (lineElement) {
      const containerRect = container.getBoundingClientRect();
      const lineRect = lineElement.getBoundingClientRect();
      const delta = (lineRect.top - containerRect.top) - (container.clientHeight / 2) + (lineRect.height / 2);
      container.scrollTo({top: container.scrollTop + delta, behavior: 'smooth'});
    } else {
      const codeElement = container.querySelector('.code-wrap pre code') as HTMLElement | null;
      const lineHeight = codeElement ? Number.parseFloat(window.getComputedStyle(codeElement).lineHeight) || 20 : 20;
      container.scrollTo({top: Math.max(0, (line - 1) * lineHeight), behavior: 'smooth'});
    }
  };

  const navigateSearchMatch = (delta: 1 | -1) => {
    if (fileSearchMatches.length === 0) return;
    const next = (currentMatchIndex + delta + fileSearchMatches.length) % fileSearchMatches.length;
    setCurrentMatchIndex(next);
    scrollToFileLine(fileSearchMatches[next]);
  };

  const triggerGoToLine = () => {
    if (!selectedFile || fileLoading || !fileLines.length) return;
    const raw = gotoLineInput.trim();
    if (!raw) return;
    if (!/^\d+$/.test(raw)) {
      return;
    }
    const parsed = Number.parseInt(raw, 10);
    if (!Number.isFinite(parsed) || parsed < 1) {
      return;
    }
    const line = Math.max(1, Math.min(fileLines.length, parsed));
    setGotoLineInput(String(line));
    window.requestAnimationFrame(() => {
      scrollToFileLine(line);
    });
  };

  const loadDirectory = async (path: string) => {
    if (loadingDirs[path]) return;
    setLoadingDirs(prev => ({...prev, [path]: true}));
    try {
      const result = await service.listDirectory(path, dirHashRef.current[path]);
      if (!result.notModified) {
        const entries = sortEntries(result.entries);
        setDirEntries(prev => ({...prev, [path]: entries}));
        if (result.hash) {
          dirHashRef.current[path] = result.hash;
        }
      }
    } finally {
      setLoadingDirs(prev => {
        const next = {...prev};
        delete next[path];
        return next;
      });
    }
  };

  const toggleDirectory = async (path: string) => {
    if (isExpanded(path)) {
      setExpandedDirs(prev => prev.filter(item => item !== path));
      return;
    }
    setExpandedDirs(prev => [...prev, path]);
    if (!dirEntries[path]) {
      await loadDirectory(path);
    }
  };

  const readSelectedFile = async (path: string) => {
    if (!path) return;
    const requestSeq = fileReadSeqRef.current + 1;
    fileReadSeqRef.current = requestSeq;
    setFileLoading(true);
    try {
      const info = await service.getFileInfo(path);
      if (requestSeq !== fileReadSeqRef.current) return;
      setFileInfo(info);
      const result = await service.readFile(path, {
        knownHash: fileHashRef.current[path],
        offset: info.isBinary ? 0 : 1,
        count: info.isBinary ? 65536 : Math.max(1, info.totalLines ?? 500),
      });
      if (requestSeq !== fileReadSeqRef.current) return;
      if (result.notModified) {
        const cachedContent = fileCacheRef.current[path];
        setFileContent(typeof cachedContent === 'string' ? cachedContent : '');
        if (result.hash) {
          fileHashRef.current[path] = result.hash;
        }
        return;
      }
      setFileContent(result.content);
      fileCacheRef.current[path] = result.content;
      if (result.hash) {
        fileHashRef.current[path] = result.hash;
      }
    } catch (err) {
      if (requestSeq !== fileReadSeqRef.current) return;
      setFileInfo(null);
      setFileContent('');
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (requestSeq === fileReadSeqRef.current) {
        setFileLoading(false);
      }
    }
  };

  useEffect(() => {
    if (!selectedFile) {
      fileReadSeqRef.current += 1;
      setFileLoading(false);
      setFileInfo(null);
      setFileContent('');
      return;
    }
    readSelectedFile(selectedFile).catch(() => undefined);
  }, [projectId, selectedFile]);

  const loadGit = async () => {
    const targetProjectId = projectId;
    if (!targetProjectId) return;
    setGitLoading(true);
    setGitError('');
    try {
      const [branchData, commitData, statusData] = await Promise.all([
        service.listGitBranches(),
        service.listGitCommits('HEAD'),
        service.getGitStatus(),
      ]);
      setGitCurrentBranch(branchData.current || '');
      setGitDirty(statusData.dirty);
      setGitStatusSummary({
        staged: statusData.staged.length,
        unstaged: statusData.unstaged.length,
        untracked: statusData.untracked.length,
      });
      const working = buildWorkingTreeFiles(statusData);
      setWorkingTreeFiles(working);
      knownWorktreeRevRef.current = statusData.worktreeRev ?? '';
      setCommits(commitData);
      setGitLoadedProjectId(targetProjectId);
      const firstCommit = commitData[0]?.sha ?? '';
      setSelectedCommit(prev => prev || firstCommit);
      if (!selectedDiff) {
        if (working[0]) {
          const preferredPath = pickPreferredPath(working);
          const preferredFile = working.find(item => item.path === preferredPath) ?? working[0];
          setSelectedDiff(preferredFile.path);
          setSelectedDiffSource('worktree');
          setSelectedDiffScope(preferredFile.scope);
        } else if (firstCommit) {
          setSelectedDiffSource('commit');
        }
      }
    } catch (err) {
      setGitError(err instanceof Error ? err.message : String(err));
    } finally {
      setGitLoading(false);
    }
  };

  const refreshGitStatusOnly = async () => {
    try {
      const statusData = await service.getGitStatus();
      setGitDirty(statusData.dirty);
      setGitStatusSummary({
        staged: statusData.staged.length,
        unstaged: statusData.unstaged.length,
        untracked: statusData.untracked.length,
      });
      setWorkingTreeFiles(buildWorkingTreeFiles(statusData));
      knownWorktreeRevRef.current = statusData.worktreeRev ?? '';
    } catch {
      // Keep existing UI state on transient status fetch failure.
    }
  };

  useEffect(() => {
    if (!connected || tab !== 'git') return;
    if (!projectId) return;
    if (gitLoading) return;
    if (gitLoadedProjectId === projectId && !gitError) return;
    loadGit().catch(() => undefined);
  }, [connected, tab, projectId, gitError, gitLoading, gitLoadedProjectId]);

  useEffect(() => {
    const run = async () => {
      if (!selectedCommit) return;
      if (commitFilesBySha[selectedCommit]) return;
      const files = await service.listGitCommitFiles(selectedCommit);
      setCommitFilesBySha(prev => ({...prev, [selectedCommit]: files}));
      if (!selectedDiff && files[0]) {
        setSelectedDiff(pickPreferredPath(files));
        setSelectedDiffSource('commit');
      }
    };
    run().catch(err => setGitError(err instanceof Error ? err.message : String(err)));
  }, [selectedCommit, commitFilesBySha, selectedDiff]);

  useEffect(() => {
    const run = async () => {
      if (!projectId || !selectedDiff) return;
      if (isHeavyGeneratedDiffPath(selectedDiff) && !allowHeavyDiffLoad) {
        setDiffText('');
        return;
      }
      const cacheScope = selectedDiffSource === 'worktree' ? `WORKTREE:${selectedDiffScope}` : selectedCommit;
      if (!cacheScope) return;
      const cachedDiff = workspaceStore.getCachedDiff(projectId, cacheScope, selectedDiff);
      if (cachedDiff !== null) {
        setDiffText(cachedDiff);
        return;
      }
      setDiffLoading(true);
      try {
        const diff = selectedDiffSource === 'worktree'
          ? await service.readWorkingTreeFileDiff(selectedDiff, selectedDiffScope)
          : await service.readGitFileDiff(selectedCommit, selectedDiff);
        setDiffText(diff.diff || '');
        workspaceStore.cacheDiff(projectId, cacheScope, selectedDiff, diff.diff || '', !!diff.isBinary, !!diff.truncated);
      } catch (err) {
        setGitError(err instanceof Error ? err.message : String(err));
      } finally {
        setDiffLoading(false);
      }
    };
    run().catch(() => undefined);
  }, [projectId, selectedCommit, selectedDiff, selectedDiffSource, selectedDiffScope, allowHeavyDiffLoad]);

  const connect = async () => {
    setError('');
    if (reconnectTimerRef.current !== null) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    try {
      const ws = toRegistryWsUrl(address);
      const result = await workspaceController.connect(ws, token.trim());
      setProjects(result.projects);
      dirHashRef.current = {};
      fileHashRef.current = {};
      fileCacheRef.current = {};
      applyHydratedProjectState(result.hydrated);
      setGitDirty(Boolean(result.projects.find(item => item.projectId === result.hydrated.projectId)?.git?.dirty));
      setConnected(true);
      workspaceController.validateExpandedDirectories(result.rootEntries, result.hydrated.expandedDirs).then(validated => {
        if (projectIdRef.current !== result.hydrated.projectId) return;
        setDirEntries(validated.dirEntries);
        setExpandedDirs(validated.expandedDirs);
      }).catch(() => undefined);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      if (projects.length > 0) {
        reconnectTimerRef.current = window.setTimeout(() => {
          setAutoConnecting(true);
          connect().catch(() => undefined);
        }, 1000);
      }
    } finally {
      setAutoConnecting(false);
    }
  };

  useEffect(() => {
    if (connected || autoConnecting) return;
    if (autoConnectTriedRef.current) return;
    if (!address.trim()) return;
    autoConnectTriedRef.current = true;
    setAutoConnecting(true);
    connect().catch(() => {
      setAutoConnecting(false);
    });
  }, [address, autoConnecting, connected, projects.length]);

  const switchProject = async (nextProjectId: string) => {
    setLoadingProject(true);
    try {
      const result = await workspaceController.switchProject(nextProjectId);
      setProjects(result.projects);
      applyHydratedProjectState(result.hydrated);
      workspaceController.validateExpandedDirectories(result.rootEntries, result.hydrated.expandedDirs).then(validated => {
        if (projectIdRef.current !== result.hydrated.projectId) return;
        setDirEntries(validated.dirEntries);
        setExpandedDirs(validated.expandedDirs);
      }).catch(() => undefined);
    } finally {
      setLoadingProject(false);
    }
  };

  const refreshProject = async () => {
    if (!projectId) return;
    const latestProject = currentProjectRef.current;
    const latestExpandedDirs = expandedDirsRef.current;
    const latestSelectedFile = selectedFileRef.current;
    setRefreshingProject(true);
    try {
      const sync = await service.syncCheck({
        knownProjectRev: latestProject?.projectRev ?? '',
        knownGitRev: latestProject?.git?.gitRev ?? '',
        knownWorktreeRev: latestProject?.git?.worktreeRev ?? '',
      });
      if (sync.staleDomains.includes('project') || !latestProject) {
        setProjects(await service.listProjects());
      }
      if (sync.staleDomains.some(domain => domain === 'fs' || domain === 'project')) {
        const validated = await workspaceController.refreshProject(projectId, [...latestExpandedDirs]);
        setDirEntries(validated.dirEntries);
        setExpandedDirs(validated.expandedDirs);
        dirHashRef.current = {};
      } else {
        await Promise.all(latestExpandedDirs.map(path => loadDirectory(path)));
      }
      if (latestSelectedFile && sync.staleDomains.some(domain => domain === 'fs' || domain === 'project')) {
        await readSelectedFile(latestSelectedFile);
      }
      if (sync.staleDomains.some(domain => domain === 'git' || domain === 'worktree' || domain === 'project')) {
        await loadGit();
      }
    } finally {
      setRefreshingProject(false);
    }
  };

  useEffect(() => {
    if (!connected) return;

    const scheduleRefresh = () => {
      if (liveRefreshTimerRef.current !== null) {
        return;
      }
      liveRefreshTimerRef.current = window.setTimeout(() => {
        liveRefreshTimerRef.current = null;
        refreshProject().catch(() => undefined);
      }, 150);
    };

    const unsubscribeEvent = service.onEvent(event => {
      const eventProjectId = event.projectId ?? '';
      if (event.method === 'project.online' || event.method === 'project.offline') {
        setProjects(prev =>
          prev.map(item =>
            item.projectId === eventProjectId ? {...item, online: event.method === 'project.online'} : item,
          ),
        );
      }
      if (eventProjectId && eventProjectId !== projectIdRef.current) {
        return;
      }
      if (event.method === 'git.workspace.changed') {
        const payload = (event.payload ?? {}) as RegistryGitWorkspaceChangedPayload;
        const gitRevChanged = !!payload.gitRev && payload.gitRev !== knownGitRevRef.current;
        if (payload.gitRev) knownGitRevRef.current = payload.gitRev;
        if (!gitRevChanged && payload.worktreeRev && payload.worktreeRev === knownWorktreeRevRef.current) {
          return;
        }
        if (gitRevChanged) {
          setGitLoadedProjectId('');
        }
        refreshGitStatusOnly().catch(() => undefined);
        return;
      }
      if (event.method === 'project.changed') {
        const payload = (event.payload ?? {}) as RegistryProjectEventPayload;
        const changedDomains = Array.isArray(payload.changedDomains)
          ? payload.changedDomains.filter(item => typeof item === 'string')
          : [];
        if (payload.projectRev) {
          knownProjectRevRef.current = payload.projectRev;
        }
        if (payload.gitRev) {
          if (payload.gitRev !== knownGitRevRef.current) {
            setGitLoadedProjectId('');
          }
          knownGitRevRef.current = payload.gitRev;
        }
        if (changedDomains.includes('git')) {
          setGitLoadedProjectId('');
          refreshGitStatusOnly().catch(() => undefined);
          return;
        }
        if (changedDomains.includes('worktree')) {
          refreshGitStatusOnly().catch(() => undefined);
          return;
        }
        if (changedDomains.length > 0 && !changedDomains.includes('project') && !changedDomains.includes('fs')) {
          return;
        }
      }
      if (
        event.method === 'project.changed' ||
        event.method === 'project.online' ||
        event.method === 'project.offline'
      ) {
        scheduleRefresh();
      }
    });

    const unsubscribeClose = service.onClose(() => {
      setConnected(false);
      setError('Registry connection closed. Reconnect to resume live updates.');
      if (reconnectTimerRef.current !== null) {
        window.clearTimeout(reconnectTimerRef.current);
      }
      reconnectTimerRef.current = window.setTimeout(() => {
        setAutoConnecting(true);
        connect().catch(() => undefined);
      }, 1000);
    });

    return () => {
      unsubscribeEvent();
      unsubscribeClose();
    };
  }, [connected, projectId]);

  const renderFileTree = (path: string, depth: number): React.ReactNode => {
    const entries = dirEntries[path] ?? [];
    return entries.map(entry => {
      if (entry.kind === 'dir') {
        const expanded = isExpanded(entry.path);
        return (
          <div key={entry.path}>
            <div
              className="item"
              style={{paddingLeft: 10 + depth * 14}}
              onClick={() => {
                toggleDirectory(entry.path).catch(() => undefined);
              }}>
              <span className={`caret codicon ${expanded ? 'codicon-chevron-down' : 'codicon-chevron-right'}`} />
              <span className={`node-icon codicon ${expanded ? 'codicon-folder-opened' : 'codicon-folder'}`} />
              <span className="label">{entry.name}</span>
              {loadingDirs[entry.path] ? <span className="muted">...</span> : null}
            </div>
            {expanded ? renderFileTree(entry.path, depth + 1) : null}
          </div>
        );
      }

      const fileIcon = resolveFileIcon(entry.name);
      return (
        <div
          key={entry.path}
          className={`item ${selectedFile === entry.path ? 'selected' : ''}`}
          style={{paddingLeft: 10 + depth * 14}}
          onClick={() => {
            setSelectedFile(entry.path);
            if (!isWide) setDrawerOpen(false);
          }}>
          <span className="caret placeholder" aria-hidden="true" />
          <span
            className="node-icon seti-icon"
            style={{color: fileIcon.color}}
          >
            <span className="seti-glyph">{fileIcon.glyph}</span>
          </span>
          <span className="label">{entry.name}</span>
        </div>
      );
    });
  };

  const renderSidebarMain = () => {
    if (tab === 'chat') {
      return (
        <>
          <div className="section-title">CHAT SESSIONS</div>
          <div className="list">
            {chatSessions.map((session, index) => (
              <div
                key={session}
                className={`item ${chatSessionIndex === index ? 'selected' : ''}`}
                onClick={() => {
                  setChatSessionIndex(index);
                  if (!isWide) setDrawerOpen(false);
                }}>
                <span className="file-dot codicon codicon-comment-discussion" />
                <span className="label">{session}</span>
              </div>
            ))}
          </div>
        </>
      );
    }

    if (tab === 'file') {
      return (
        <>
          <div className="section-title">EXPLORER</div>
          <div className="list">{renderFileTree('.', 0)}</div>
        </>
      );
    }

    return (
      <>
        <div className="section-title">COMMITS {gitCurrentBranch ? `(${gitCurrentBranch})` : ''}</div>
        <div className="list half">
          {gitLoading ? <div className="muted block">Loading commits...</div> : null}
          {gitError ? <div className="error block">{gitError}</div> : null}
          {workingTreeFiles.length > 0 ? (
            <div
              className={`item ${worktreeActive ? 'selected' : ''}`}
              onClick={() => {
                setSelectedDiffSource('worktree');
                if (workingTreeFiles[0]) {
                  const preferredPath = pickPreferredPath(workingTreeFiles);
                  const preferredFile = workingTreeFiles.find(item => item.path === preferredPath) ?? workingTreeFiles[0];
                  setSelectedDiff(preferredFile.path);
                  setSelectedDiffScope(preferredFile.scope);
                }
                if (!isWide) setDrawerOpen(false);
              }}>
              <span className="file-dot codicon codicon-source-control" />
              <span className="label">Working Tree</span>
            </div>
          ) : null}
          {commits.map(commit => (
            <div
              key={commit.sha}
              className={`item ${!worktreeActive && selectedCommit === commit.sha ? 'selected' : ''}`}
              onClick={() => {
                setSelectedCommit(commit.sha);
                setSelectedDiffSource('commit');
                const files = commitFilesBySha[commit.sha] ?? [];
                if (files[0]) {
                  setSelectedDiff(pickPreferredPath(files));
                } else {
                  setSelectedDiff('');
                }
                setSelectedDiffScope('unstaged');
                if (!isWide) setDrawerOpen(false);
              }}>
              <span className="file-dot codicon codicon-git-commit" />
              <span className="label">{commit.title || commit.sha.slice(0, 7)}</span>
            </div>
          ))}
        </div>
        <div className="section-title">FILES</div>
        <div className="list half">
          {worktreeActive ? (
            workingTreeFiles.length === 0 ? (
              <div className="muted block">No local changes</div>
            ) : (
              workingTreeFiles.map(file => (
                <div
                  key={`${WORKING_TREE_COMMIT_ID}:${file.scope}:${file.path}`}
                  className={`item ${selectedDiff === file.path && selectedDiffScope === file.scope ? 'selected' : ''}`}
                  onClick={() => {
                    setSelectedDiff(file.path);
                    setSelectedDiffSource('worktree');
                    setSelectedDiffScope(file.scope);
                    if (!isWide) setDrawerOpen(false);
                  }}>
                  <span className={`status-tag status-git-${file.status}`}>{file.status}</span>
                  <span className="muted" style={{marginRight: 6}}>{file.scope}</span>
                  <span className="label">{file.path}</span>
                </div>
              ))
            )
          ) : (
            currentCommitFiles.map(file => (
              <div
                key={file.path}
                className={`item ${selectedDiffSource === 'commit' && selectedDiff === file.path ? 'selected' : ''}`}
                onClick={() => {
                  setSelectedDiff(file.path);
                  setSelectedDiffSource('commit');
                  if (!isWide) setDrawerOpen(false);
                }}>
                <span className={`status-tag status-git-${file.status}`}>{file.status}</span>
                <span className="label">{file.path}</span>
              </div>
            ))
          )}
        </div>
      </>
    );
  };

  const renderSidebar = () => (
    <>
      <div className="sidebar-scroll">
        {sidebarSettingsOpen ? (
          <>
            <div className="section-title">SETTINGS</div>
            <div className="list">
              <label className="switch-row sidebar-setting-row">
                <span>Dark Mode</span>
                <input type="checkbox" checked={themeMode === 'dark'} onChange={e => setThemeMode(e.target.checked ? 'dark' : 'light')} />
              </label>
              <label className="switch-row sidebar-setting-row">
                <span>Code Theme</span>
                <select
                  className="sidebar-setting-select"
                  value={codeTheme}
                  onChange={event => {
                    const next = event.target.value;
                    if (isCodeThemeId(next)) setCodeTheme(next);
                  }}>
                  <option key={CODE_THEME_OPTIONS[0].id} value={CODE_THEME_OPTIONS[0].id}>{CODE_THEME_OPTIONS[0].label}</option>
                  {CODE_THEME_OPTION_GROUPS.map(group => (
                    <optgroup key={group.label} label={group.label}>
                      {group.options.map(item => (
                        <option key={item.id} value={item.id}>{item.label}</option>
                      ))}
                    </optgroup>
                  ))}
                </select>
              </label>
              <label className="switch-row sidebar-setting-row">
                <span>Code Font</span>
                <select
                  className="sidebar-setting-select"
                  value={codeFont}
                  onChange={event => {
                    const next = event.target.value;
                    if (isCodeFontId(next)) setCodeFont(next);
                  }}>
                  {CODE_FONT_OPTIONS.map(item => (
                    <option key={item.id} value={item.id}>{item.label}</option>
                  ))}
                </select>
              </label>
              <label className="switch-row sidebar-setting-row">
                <span>Font Size</span>
                <select
                  className="sidebar-setting-select"
                  value={String(codeFontSize)}
                  onChange={event => {
                    setCodeFontSize(clampCodeFontSize(Number(event.target.value)));
                  }}>
                  {CODE_FONT_SIZE_OPTIONS.map(size => (
                    <option key={size} value={size}>{size}px</option>
                  ))}
                </select>
              </label>
              <label className="switch-row sidebar-setting-row">
                <span>Line Height</span>
                <select
                  className="sidebar-setting-select"
                  value={String(codeLineHeight)}
                  onChange={event => {
                    setCodeLineHeight(clampCodeLineHeight(Number(event.target.value)));
                  }}>
                  {CODE_LINE_HEIGHT_OPTIONS.map(v => (
                    <option key={v} value={v}>{v}</option>
                  ))}
                </select>
              </label>
              <label className="switch-row sidebar-setting-row">
                <span>Tab Size</span>
                <select
                  className="sidebar-setting-select"
                  value={String(codeTabSize)}
                  onChange={event => {
                    setCodeTabSize(clampCodeTabSize(Number(event.target.value)));
                  }}>
                  {CODE_TAB_SIZE_OPTIONS.map(v => (
                    <option key={v} value={v}>{v}</option>
                  ))}
                </select>
              </label>
            </div>
          </>
        ) : renderSidebarMain()}
      </div>
      <div className="sidebar-footer">
        <button
          type="button"
          className="sidebar-settings-btn"
          onClick={() => setSidebarSettingsOpen(value => !value)}
          title={sidebarSettingsOpen ? 'Back to sidebar' : 'Open settings'}>
          <span className={`codicon ${sidebarSettingsOpen ? 'codicon-arrow-left' : 'codicon-settings-gear'}`} />
          <span>{sidebarSettingsOpen ? 'Back' : 'Settings'}</span>
        </button>
      </div>
    </>
  );

  const renderCodePane = (content: string, forceLineNumbers = false, languageHint = '') => {
    const numbersOn = forceLineNumbers || showLineNumbers;
    const language = languageHint || detectCodeLanguage(selectedFile);
    return (
      <PrismCodeBlock
        content={content}
        language={language}
        wrap={wrapLines}
        lineNumbers={numbersOn}
        themeMode={themeMode}
        codeTheme={codeTheme}
        codeFont={codeFont}
        codeFontSize={codeFontSize}
        codeLineHeight={codeLineHeight}
        codeTabSize={codeTabSize}
      />
    );
  };

  const renderViewTools = () => (
    <>
      <button
        type="button"
        className={`view-tool ${wrapLines ? 'active' : ''}`}
        onClick={() => setWrapLines(value => !value)}
        title="Toggle wrap line"
        aria-label="Toggle wrap line">
        <span className="codicon codicon-word-wrap view-tool-icon" />
      </button>
      <button
        type="button"
        className={`view-tool ${showLineNumbers ? 'active' : ''}`}
        onClick={() => setShowLineNumbers(value => !value)}
        title="Toggle line number"
        aria-label="Toggle line number">
        <span className="codicon codicon-list-ordered view-tool-icon" />
      </button>
    </>
  );

  const renderDiffPane = (content: string) => {
    if (!content) return <div className="muted block">No diff available</div>;
    const shouldDelayLargeRender =
      !allowLargeDiffRender &&
      isHeavyGeneratedDiffPath(selectedDiff || '') &&
      content.length > MAX_AUTO_RENDER_DIFF_CHARS;
    if (shouldDelayLargeRender) {
      return (
        <div className="muted block">
          Large generated diff detected ({(content.length / 1024).toFixed(0)} KB). Click to render when needed.
          <div style={{marginTop: 10}}>
            <button type="button" className="button" onClick={() => setAllowLargeDiffRender(true)}>Render Diff</button>
          </div>
        </div>
      );
    }

    const language = detectCodeLanguage(selectedDiff || selectedFile);
    return (
      <PrismDiffPane
        content={content}
        language={language}
        wrap={wrapLines}
        lineNumbers={showLineNumbers}
        themeMode={themeMode}
        codeTheme={codeTheme}
        codeFont={codeFont}
        codeFontFamily={codeFontFamily}
        codeFontSize={codeFontSize}
        codeLineHeight={codeLineHeight}
        codeTabSize={codeTabSize}
      />
    );
  };
  const renderMain = () => {
    const heavyDiffDeferred = !!selectedDiff && isHeavyGeneratedDiffPath(selectedDiff) && !allowHeavyDiffLoad;

    if (tab === 'chat') {
      return (
        <div className="content">
          <div className="block-title">CHAT - {chatSessions[chatSessionIndex]}</div>
          <div className="scroll-panel chat-block">
            <ThinkingBlock
              content={"Now I have a good understanding of the codebase structure. The server uses a Go daemon with ACP bridge and IM adapters, the app is built with React/React Native for cross-platform support.\n\nThe key files are:\n- server/internal/protocol/update.go — streaming update types\n- app/web/src/main.tsx — main web entry point\n- app/web/src/styles.css — all styles\n\nI'll proceed to implement the thinking block component with the collapsible UI pattern the user requested."}
              isStreaming={false}
            />
            <ThinkingBlock
              content={"Let me analyze the protocol definitions to understand how thought chunks are structured and streamed from the agent..."}
              isStreaming={true}
            />
            <ThinkingBlock
              content={"The user wants better information density for thinking blocks. Currently the thinking block takes too much space when collapsed. I should redesign it so the title bar shows a summary of the thinking content after streaming completes, and clicking expands to show full content.\n\nKey design decisions:\n1. Use the first meaningful line as summary text\n2. Chevron icon to indicate expandability\n3. Smooth height animation for expand/collapse\n4. Distinct visual treatment for streaming vs completed states"}
              isStreaming={false}
            />
          </div>
        </div>
      );
    }

    if (tab === 'file') {
      return (
        <div className="content">
          <div className="block-title with-tools file-title-bar">
            <span className="title-text">{selectedFile || 'Select a file'}</span>
            <div className="view-tools">{renderViewTools()}</div>
          </div>
          <div className="file-pane">
            <div className="file-main-col">
              {hasPinnedFiles ? (
                <div className="pinned-strip">
                  <span className="pinned-label">Pinned</span>
                  {pinnedFiles.map(path => (
                    <div key={path} className={`pinned-entry ${selectedFile === path ? 'active' : ''}`}>
                      <button type="button" className="pinned-open" onClick={() => setSelectedFile(path)} title={path}>
                        {path.split('/').pop() || path}
                      </button>
                      <button
                        type="button"
                        className="pinned-close"
                        onClick={() => setPinnedFiles(prev => prev.filter(item => item !== path))}
                      aria-label={`Unpin ${path}`}>
                        x
                      </button>
                    </div>
                  ))}
                </div>
              ) : null}
              <div className="file-code-area">
                <div ref={fileSideActionsRef} className="file-side-actions">
                  <button
                    type="button"
                    className={`pinned-pin-toggle file-pin-floating ${isSelectedFilePinned ? 'active' : ''}`}
                    onClick={togglePinSelectedFile}
                    disabled={!selectedFile}
                    title={isSelectedFilePinned ? 'Unpin current file' : 'Pin current file'}
                    aria-label={isSelectedFilePinned ? 'Unpin current file' : 'Pin current file'}>
                    <span className="codicon codicon-pinned view-tool-icon" />
                  </button>
                  <div className="file-action-group side-action-group">
                    <button
                      type="button"
                      className={`view-tool ${gotoToolsOpen ? 'active' : ''}`}
                      onClick={() => {
                        setGotoToolsOpen(value => {
                          const next = !value;
                          if (next) setSearchToolsOpen(false);
                          return next;
                        });
                      }}
                      title="Toggle go to line"
                      aria-label="Toggle go to line">
                      <span className="codicon codicon-symbol-number view-tool-icon" />
                    </button>
                    <div className={`file-action-panel side-action-panel ${gotoToolsOpen ? 'open' : ''}`}>
                      <input
                        className="goto-input"
                        value={gotoLineInput}
                        onChange={event => setGotoLineInput(event.target.value)}
                        onKeyDown={event => {
                          if (event.key === 'Enter') {
                            event.preventDefault();
                            triggerGoToLine();
                          }
                        }}
                        inputMode="numeric"
                        placeholder="Line"
                      />
                      <button type="button" className="view-tool goto-trigger" title="Go to line" onClick={triggerGoToLine}>
                        <span className="codicon codicon-arrow-right view-tool-icon" />
                      </button>
                    </div>
                  </div>
                  <div className="file-action-group side-action-group">
                    <button
                      type="button"
                      className={`view-tool ${searchToolsOpen ? 'active' : ''}`}
                      onClick={() => {
                        setSearchToolsOpen(value => {
                          const next = !value;
                          if (next) setGotoToolsOpen(false);
                          return next;
                        });
                      }}
                      title="Toggle search"
                      aria-label="Toggle search">
                      <span className="codicon codicon-search view-tool-icon" />
                    </button>
                    <div className={`file-action-panel side-action-panel ${searchToolsOpen ? 'open' : ''}`}>
                      <input
                        className="search-input"
                        value={fileSearchQuery}
                        onChange={event => setFileSearchQuery(event.target.value)}
                        onKeyDown={event => {
                          if (event.key === 'Enter') {
                            event.preventDefault();
                            navigateSearchMatch(1);
                          }
                        }}
                        placeholder="Find in file"
                      />
                      <button type="button" className="view-tool search-nav" title="Previous match" onClick={() => navigateSearchMatch(-1)}>
                        <span className="codicon codicon-chevron-up view-tool-icon" />
                      </button>
                      <button type="button" className="view-tool search-nav" title="Next match" onClick={() => navigateSearchMatch(1)}>
                        <span className="codicon codicon-chevron-down view-tool-icon" />
                      </button>
                      <span className="search-count">{fileSearchMatches.length === 0 ? '0/0' : `${currentMatchIndex + 1}/${fileSearchMatches.length}`}</span>
                    </div>
                  </div>
                </div>
                <div ref={fileScrollRef} className="scroll-panel">
                  {fileLoading ? <div className="muted block">Loading file...</div> : renderCodePane(fileContent, false, detectCodeLanguage(selectedFile))}
                </div>
              </div>
            </div>
          </div>
        </div>
      );
    }

    return (
      <div className="content">
        <div className="block-title with-tools">
          <span className="title-text">{selectedDiff || 'Select a changed file'}</span>
          <div className="view-tools">{renderViewTools()}</div>
        </div>
        <div className="scroll-panel">
          {heavyDiffDeferred ? (
            <div className="muted block">
              Heavy generated file selected. Diff loading is paused to keep UI responsive.
              <div style={{marginTop: 10}}>
                <button type="button" className="button" onClick={() => setAllowHeavyDiffLoad(true)}>Load Diff</button>
              </div>
            </div>
          ) : diffLoading ? (
            <div className="muted block">Loading diff...</div>
          ) : (
            renderDiffPane(diffText)
          )}
        </div>
      </div>
    );
  };

  if (!connected) {
    return (
      <div className={`page theme-${themeMode}`}>
        <style>{setiFontCss}</style>
        <div className="connect">
          <h3>Connect to WheelMaker Registry</h3>
          <input
            className="input"
            value={address}
            onChange={e => setAddress(e.target.value)}
            placeholder="127.0.0.1:9630 or ws://127.0.0.1:9630/ws"
          />
          <input className="input" value={token} onChange={e => setToken(e.target.value)} placeholder="Token (optional)" />
          <button className="button" onClick={() => connect().catch(() => undefined)}>
            {autoConnecting ? 'Connecting...' : 'Connect'}
          </button>
          {error ? <div className="error">{error}</div> : null}
        </div>
      </div>
    );
  }

  return (
    <div className={`workspace theme-${themeMode}`}>
      <style>{setiFontCss}</style>
      <header className="header">
        <button
          className="header-btn"
          onClick={() => {
            if (isWide) {
              setSidebarCollapsed(value => !value);
            } else {
              setDrawerOpen(value => !value);
            }
          }}>
          <span className={`codicon ${isWide ? (sidebarCollapsed ? 'codicon-layout-sidebar-left-off' : 'codicon-layout-sidebar-left') : 'codicon-menu'}`} />
        </button>

        <div className="project-wrap" onPointerDown={event => event.stopPropagation()}>
          <button className="project-btn" onClick={() => setProjectMenuOpen(value => !value)}>
            <span className="project-arrow codicon codicon-chevron-down" />
            <span className="project-name" title={currentProjectName}>{currentProjectName}</span>
            <span className={`project-presence ${currentProject?.online ? 'online' : 'offline'}`} />
            {gitDirty ? <span className="project-dirty">dirty</span> : null}
            {(loadingProject || refreshingProject) ? <span className="muted">...</span> : null}
          </button>
          {projectMenuOpen ? (
            <div className="project-menu">
              {projects.map(project => (
                <div
                  key={project.projectId}
                  className={`item project-menu-item ${project.projectId === projectId ? 'selected' : ''}`}
                  onClick={() => switchProject(project.projectId).catch(() => undefined)}>
                  <div className="project-menu-main">
                    <span className="project-menu-name">{project.name}</span>
                    <span className="project-menu-path" title={project.path || ''}>{project.path || '-'}</span>
                  </div>
                  <span className={`project-menu-state ${project.online ? 'online' : 'offline'}`}>
                    {project.online ? 'online' : 'offline'}
                  </span>
                  <span className="project-menu-hub">{project.hubId || 'local-hub'}</span>
                </div>
              ))}
            </div>
          ) : null}
        </div>

        <button className="header-btn refresh-btn" onClick={() => refreshProject().catch(() => undefined)} title="Refresh project">
          {refreshingProject ? '...' : <span className="codicon codicon-refresh" />}
        </button>

        <div className="header-spacer" />

        <div className="tabs">
          <button className={`tab ${tab === 'chat' ? 'active' : ''}`} onClick={() => setTab('chat')}>
            <span className="codicon codicon-comment-discussion tab-icon" />
            <span className="tab-label">CHAT</span>
          </button>
          <button className={`tab ${tab === 'file' ? 'active' : ''}`} onClick={() => setTab('file')}>
            <span className="codicon codicon-files tab-icon" />
            <span className="tab-label">FILE</span>
          </button>
          <button className={`tab ${tab === 'git' ? 'active' : ''}`} onClick={() => setTab('git')}>
            <span className="codicon codicon-source-control tab-icon" />
            <span className="tab-label">GIT</span>
          </button>
        </div>
      </header>

      <div className="body">
        {isWide && !sidebarCollapsed ? <aside className="workspace-left">{renderSidebar()}</aside> : null}
        <main className="workspace-right">{renderMain()}</main>
      </div>

      {tab === 'file' ? (
        <div className="status-bar">
          {selectedFile ? (
            <span className="statusbar-item">
              <span className="codicon codicon-file" />
              <span className="statusbar-text">{selectedFile.split('/').pop()}</span>
            </span>
          ) : null}
          {selectedFile && fileContent.length > 0 ? (
            <span className="statusbar-muted">{fileInfo?.isBinary ? `${fileInfo.size ?? 0} bytes` : `${fileInfo?.totalLines ?? fileContent.split('\n').length} lines`}</span>
          ) : null}
          <span className="statusbar-spacer" />
          <span className="statusbar-muted">{gitDirty ? `dirty ${gitStatusSummary.staged}/${gitStatusSummary.unstaged}/${gitStatusSummary.untracked}` : 'clean'}</span>
          {gitCurrentBranch ? (
            <span className="statusbar-item">
              <span className="codicon codicon-git-branch" />
              <span className="statusbar-text">{gitCurrentBranch}</span>
            </span>
          ) : null}
        </div>
      ) : null}

      {!isWide ? (
        <div className={`drawer-overlay ${drawerOpen ? 'show' : ''}`} onClick={() => setDrawerOpen(false)}>
          <aside className={`drawer ${drawerOpen ? 'show' : ''}`} onClick={event => event.stopPropagation()}>
            {renderSidebar()}
          </aside>
        </div>
      ) : null}
    </div>
  );
}

if ('serviceWorker' in navigator && window.isSecureContext) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/service-worker.js').then(registration => {
      window.setTimeout(() => {
        registration.update().catch(() => undefined);
      }, 1500);

      if (registration.waiting) {
        registration.waiting.postMessage('SKIP_WAITING');
      }

      registration.addEventListener('updatefound', () => {
        const installing = registration.installing;
        if (!installing) return;
        installing.addEventListener('statechange', () => {
          if (installing.state === 'installed' && navigator.serviceWorker.controller) {
            registration.waiting?.postMessage('SKIP_WAITING');
          }
        });
      });
    }).catch(() => undefined);
  });
}

createRoot(document.getElementById('root')!).render(<App />);


