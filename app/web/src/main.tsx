import React, {useEffect, useMemo, useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import ReactDiffViewer, {DiffMethod} from 'react-diff-viewer-continued';
import {PrismLight as SyntaxHighlighter} from 'react-syntax-highlighter';
import oneDark from 'react-syntax-highlighter/dist/cjs/styles/prism/one-dark';
import prismMarkup from 'react-syntax-highlighter/dist/cjs/languages/prism/markup';
import prismClike from 'react-syntax-highlighter/dist/cjs/languages/prism/clike';
import prismJavascript from 'react-syntax-highlighter/dist/cjs/languages/prism/javascript';
import prismTypescript from 'react-syntax-highlighter/dist/cjs/languages/prism/typescript';
import prismJsx from 'react-syntax-highlighter/dist/cjs/languages/prism/jsx';
import prismTsx from 'react-syntax-highlighter/dist/cjs/languages/prism/tsx';
import prismJson from 'react-syntax-highlighter/dist/cjs/languages/prism/json';
import prismGo from 'react-syntax-highlighter/dist/cjs/languages/prism/go';
import prismC from 'react-syntax-highlighter/dist/cjs/languages/prism/c';
import prismCpp from 'react-syntax-highlighter/dist/cjs/languages/prism/cpp';
import prismRust from 'react-syntax-highlighter/dist/cjs/languages/prism/rust';
import prismBash from 'react-syntax-highlighter/dist/cjs/languages/prism/bash';
import prismYaml from 'react-syntax-highlighter/dist/cjs/languages/prism/yaml';
import prismMarkdown from 'react-syntax-highlighter/dist/cjs/languages/prism/markdown';
import setiThemeJson from '@codingame/monaco-vscode-theme-seti-default-extension/resources/vs-seti-icon-theme.json';
import setiFontUrl from '@codingame/monaco-vscode-theme-seti-default-extension/resources/seti.woff';
import '@vscode/codicons/dist/codicon.css';
import '@fontsource/ibm-plex-sans/400.css';
import '@fontsource/ibm-plex-sans/500.css';
import '@fontsource/ibm-plex-sans/600.css';

import {getDefaultRegistryAddress, toRegistryWsUrl} from './runtime';
import {RegistryWorkspaceService} from './services/registryWorkspaceService';
import type {RegistryFsEntry, RegistryGitCommit, RegistryGitCommitFile, RegistryProject} from './types/registry';
import './styles.css';

type Tab = 'chat' | 'file' | 'git';
type ThemeMode = 'dark' | 'light';
type DirEntries = Record<string, RegistryFsEntry[]>;
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

const service = new RegistryWorkspaceService();
const setiTheme = setiThemeJson as SetiTheme;
const VS_CODE_EDITOR_FONT_FAMILY = "Consolas, 'Courier New', monospace";

SyntaxHighlighter.registerLanguage('markup', prismMarkup);
SyntaxHighlighter.registerLanguage('clike', prismClike);
SyntaxHighlighter.registerLanguage('javascript', prismJavascript);
SyntaxHighlighter.registerLanguage('typescript', prismTypescript);
SyntaxHighlighter.registerLanguage('jsx', prismJsx);
SyntaxHighlighter.registerLanguage('tsx', prismTsx);
SyntaxHighlighter.registerLanguage('json', prismJson);
SyntaxHighlighter.registerLanguage('go', prismGo);
SyntaxHighlighter.registerLanguage('c', prismC);
SyntaxHighlighter.registerLanguage('cpp', prismCpp);
SyntaxHighlighter.registerLanguage('rust', prismRust);
SyntaxHighlighter.registerLanguage('bash', prismBash);
SyntaxHighlighter.registerLanguage('yaml', prismYaml);
SyntaxHighlighter.registerLanguage('markdown', prismMarkdown);

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

type UnifiedDiffSides = {
  oldText: string;
  newText: string;
  hasContent: boolean;
  oldStart: number;
  newStart: number;
};

function parseUnifiedDiff(content: string): UnifiedDiffSides {
  const lines = content.split('\n');
  const oldLines: string[] = [];
  const newLines: string[] = [];
  let inHunk = false;
  let firstOldStart = 1;
  let firstNewStart = 1;
  let hasSeenHunk = false;

  for (const raw of lines) {
    const hunkMatch = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(raw);
    if (hunkMatch) {
      const oldStart = Number.parseInt(hunkMatch[1], 10);
      const newStart = Number.parseInt(hunkMatch[2], 10);
      if (!hasSeenHunk) {
        firstOldStart = oldStart;
        firstNewStart = newStart;
        hasSeenHunk = true;
      }
      inHunk = true;
      continue;
    }
    if (!inHunk) continue;
    if (raw.startsWith('\\ No newline at end of file')) continue;
    if (raw.startsWith('+')) {
      newLines.push(raw.slice(1));
      continue;
    }
    if (raw.startsWith('-')) {
      oldLines.push(raw.slice(1));
      continue;
    }
    if (raw.startsWith(' ')) {
      const text = raw.slice(1);
      oldLines.push(text);
      newLines.push(text);
      continue;
    }
    oldLines.push(raw);
    newLines.push(raw);
  }

  return {
    oldText: oldLines.join('\n'),
    newText: newLines.join('\n'),
    hasContent: oldLines.length > 0 || newLines.length > 0,
    oldStart: firstOldStart,
    newStart: firstNewStart,
  };
}

function getDiffViewerStyles(wrap: boolean): any {
  return {
    variables: {
      dark: {
        diffViewerBackground: 'var(--panel-3)',
        gutterBackground: 'var(--panel)',
        gutterBackgroundDark: 'var(--panel-2)',
      },
      light: {
        diffViewerBackground: 'var(--panel-3)',
        gutterBackground: 'var(--panel)',
        gutterBackgroundDark: 'var(--panel-2)',
      },
    },
    diffContainer: {
      width: '100%',
      minWidth: '100%',
      tableLayout: 'auto',
      fontFamily: VS_CODE_EDITOR_FONT_FAMILY,
      fontWeight: 400,
      fontVariantLigatures: 'none',
      fontFeatureSettings: '"liga" 0, "calt" 0',
      fontSize: '13px',
      lineHeight: '1.5',
      overflowX: wrap ? 'hidden' : 'auto',
      pre: {
        whiteSpace: wrap ? 'pre-wrap' : 'pre',
        width: '100%',
      },
    },
    content: {
      width: '100%',
      overflow: 'visible',
    },
    line: {
      verticalAlign: 'top',
    },
    lineContent: {
      overflow: 'visible',
      width: 'auto',
    },
    contentText: {
      display: 'block',
      width: '100%',
      background: 'transparent',
      fontFamily: VS_CODE_EDITOR_FONT_FAMILY,
      fontWeight: 400,
      whiteSpace: wrap ? 'pre-wrap' : 'pre',
      wordBreak: wrap ? 'break-word' : 'normal',
      overflowWrap: wrap ? 'anywhere' : 'normal',
    },
    wordAdded: {
      background: 'transparent',
    },
    wordRemoved: {
      background: 'transparent',
    },
    lineNumber: {
      fontWeight: 400,
      color: 'var(--muted)',
    },
    marker: {
      userSelect: 'none',
    },
  };
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

type PrismCodeBlockProps = {
  content: string;
  language: string;
  wrap: boolean;
  lineNumbers: boolean;
  highlightLine?: number | null;
};

function PrismCodeBlock({content, language, wrap, lineNumbers, highlightLine = null}: PrismCodeBlockProps) {
  return (
    <div className="code-wrap">
      <SyntaxHighlighter
        className={`code-block prism-code ${wrap ? 'wrap' : 'nowrap'}`}
        language={language}
        style={oneDark}
        showLineNumbers={lineNumbers}
        wrapLongLines={wrap}
        wrapLines={true}
        codeTagProps={{style: {whiteSpace: wrap ? 'pre-wrap' : 'pre', background: 'transparent', fontFamily: VS_CODE_EDITOR_FONT_FAMILY, fontWeight: 400, fontVariantLigatures: 'none', fontFeatureSettings: '"liga" 0, "calt" 0'}}}
        lineProps={lineNumber => ({
          'data-line-number': String(lineNumber),
          style: {
            background: highlightLine === lineNumber ? 'rgba(0, 122, 204, 0.24)' : 'transparent',
            whiteSpace: wrap ? 'pre-wrap' : 'pre',
            wordBreak: wrap ? 'break-word' : 'normal',
            overflowWrap: wrap ? 'anywhere' : 'normal',
          },
        })}
        customStyle={{margin: 0, minWidth: '100%', background: 'transparent', padding: '0 10px', fontFamily: VS_CODE_EDITOR_FONT_FAMILY, fontWeight: 400, fontVariantLigatures: 'none', fontFeatureSettings: '"liga" 0, "calt" 0'}}
        lineNumberStyle={{fontFamily: VS_CODE_EDITOR_FONT_FAMILY, fontWeight: 400, color: 'var(--muted)', minWidth: '2.4em', paddingRight: '10px', borderRight: '1px solid rgba(127, 127, 127, 0.18)', marginRight: '10px', textAlign: 'right', userSelect: 'none'}}>
        {content || ' '}
      </SyntaxHighlighter>
    </div>
  );
}

function PrismInlineCode({content, language, wrap}: {content: string; language: string; wrap: boolean}) {
  return (
    <SyntaxHighlighter
      PreTag="span"
      CodeTag="span"
      className="diff-inline-code"
      language={language}
      style={oneDark}
      wrapLongLines={wrap}
      wrapLines={wrap}
      codeTagProps={{
        style: {
          whiteSpace: wrap ? 'pre-wrap' : 'pre',
          background: 'transparent',
          fontFamily: VS_CODE_EDITOR_FONT_FAMILY,
          fontWeight: 400,
          fontVariantLigatures: 'none',
          fontFeatureSettings: '"liga" 0, "calt" 0',
        },
      }}
      lineProps={{style: {background: 'transparent', whiteSpace: wrap ? 'pre-wrap' : 'pre', wordBreak: wrap ? 'break-word' : 'normal', overflowWrap: wrap ? 'anywhere' : 'normal'}}}
      customStyle={{
        margin: 0,
        padding: 0,
        display: 'inline',
        background: 'transparent',
        fontFamily: VS_CODE_EDITOR_FONT_FAMILY,
        fontWeight: 400,
        fontVariantLigatures: 'none',
        fontFeatureSettings: '"liga" 0, "calt" 0',
      }}>
      {content || ' '}
    </SyntaxHighlighter>
  );
}

function App() {
  const [connected, setConnected] = useState(false);
  const [address, setAddress] = useState(getDefaultRegistryAddress());
  const [token, setToken] = useState('');
  const [error, setError] = useState('');

  const [themeMode, setThemeMode] = useState<ThemeMode>('dark');
  const [wrapLines, setWrapLines] = useState(false);
  const [showLineNumbers, setShowLineNumbers] = useState(true);
  const setiFontCss = useMemo(() => setiFontFaceCss(), []);
  const resolveFileIcon = (name: string) => resolveSetiIcon(name, themeMode);

  const [windowWidth, setWindowWidth] = useState<number>(window.innerWidth);
  const isWide = windowWidth >= 900;

  const [tab, setTab] = useState<Tab>('file');
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [sidebarSettingsOpen, setSidebarSettingsOpen] = useState(false);

  const [projectMenuOpen, setProjectMenuOpen] = useState(false);

  const [projects, setProjects] = useState<RegistryProject[]>([]);
  const [projectId, setProjectId] = useState('');
  const [loadingProject, setLoadingProject] = useState(false);
  const [refreshingProject, setRefreshingProject] = useState(false);

  const [dirEntries, setDirEntries] = useState<DirEntries>({'.': []});
  const [expandedDirs, setExpandedDirs] = useState<string[]>(['.']);
  const [loadingDirs, setLoadingDirs] = useState<Record<string, boolean>>({});
  const [selectedFile, setSelectedFile] = useState('');
  const [pinnedFiles, setPinnedFiles] = useState<string[]>([]);
  const [fileContent, setFileContent] = useState('');
  const [fileLoading, setFileLoading] = useState(false);
  const [fileSearchQuery, setFileSearchQuery] = useState('');
  const [currentMatchIndex, setCurrentMatchIndex] = useState(0);
  const [gotoLineInput, setGotoLineInput] = useState('');
  const [searchToolsOpen, setSearchToolsOpen] = useState(false);
  const [gotoToolsOpen, setGotoToolsOpen] = useState(false);
  const [temporaryHighlightLine, setTemporaryHighlightLine] = useState<number | null>(null);
  const fileScrollRef = useRef<HTMLDivElement | null>(null);
  const highlightTimerRef = useRef<number | null>(null);
  const fileSideActionsRef = useRef<HTMLDivElement | null>(null);

  const [chatSessions] = useState(['General', 'WheelMaker App', 'Go Service']);
  const [chatSessionIndex, setChatSessionIndex] = useState(0);

  const [gitLoading, setGitLoading] = useState(false);
  const [gitError, setGitError] = useState('');
  const [gitCurrentBranch, setGitCurrentBranch] = useState('');
  const [commits, setCommits] = useState<RegistryGitCommit[]>([]);
  const [selectedCommit, setSelectedCommit] = useState('');
  const [commitFilesBySha, setCommitFilesBySha] = useState<Record<string, RegistryGitCommitFile[]>>({});
  const [selectedDiff, setSelectedDiff] = useState('');
  const [diffText, setDiffText] = useState('');
  const [diffLoading, setDiffLoading] = useState(false);

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

  const currentProjectName = useMemo(
    () => projects.find(item => item.projectId === projectId)?.name ?? 'Project',
    [projectId, projects],
  );

  const currentCommitFiles = useMemo(
    () => commitFilesBySha[selectedCommit] ?? [],
    [commitFilesBySha, selectedCommit],
  );

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
      scrollToFileLine(fileSearchMatches[0], true);
    });
  }, [fileSearchMatches, fileSearchQuery, searchToolsOpen]);

  useEffect(() => () => {
    if (highlightTimerRef.current !== null) {
      window.clearTimeout(highlightTimerRef.current);
    }
  }, []);

  const scrollToFileLine = (line: number, highlight = false) => {
    const container = fileScrollRef.current;
    if (!container) return;
    const lineElement = container.querySelector(`.code-wrap [data-line-number="${line}"]`) as HTMLElement | null;
    if (lineElement) {
      const containerRect = container.getBoundingClientRect();
      const lineRect = lineElement.getBoundingClientRect();
      const delta = (lineRect.top - containerRect.top) - (container.clientHeight / 2) + (lineRect.height / 2);
      container.scrollTo({top: container.scrollTop + delta, behavior: 'smooth'});
    } else {
      const codeElement = container.querySelector('.code-block code') as HTMLElement | null;
      const lineHeight = codeElement ? Number.parseFloat(window.getComputedStyle(codeElement).lineHeight) || 20 : 20;
      container.scrollTo({top: Math.max(0, (line - 1) * lineHeight), behavior: 'smooth'});
    }
    if (highlight) {
      setTemporaryHighlightLine(line);
      if (highlightTimerRef.current !== null) {
        window.clearTimeout(highlightTimerRef.current);
      }
      highlightTimerRef.current = window.setTimeout(() => setTemporaryHighlightLine(null), 2000);
    }
  };

  const navigateSearchMatch = (delta: 1 | -1) => {
    if (fileSearchMatches.length === 0) return;
    const next = (currentMatchIndex + delta + fileSearchMatches.length) % fileSearchMatches.length;
    setCurrentMatchIndex(next);
    scrollToFileLine(fileSearchMatches[next], true);
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
      scrollToFileLine(line, true);
    });
  };

  const loadDirectory = async (path: string) => {
    if (loadingDirs[path]) return;
    setLoadingDirs(prev => ({...prev, [path]: true}));
    try {
      const entries = sortEntries(await service.listDirectory(path));
      setDirEntries(prev => ({...prev, [path]: entries}));
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
    setFileLoading(true);
    try {
      const content = await service.readFile(path);
      setFileContent(content);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setFileLoading(false);
    }
  };

  useEffect(() => {
    readSelectedFile(selectedFile).catch(() => undefined);
  }, [selectedFile]);

  const loadGit = async () => {
    setGitLoading(true);
    setGitError('');
    try {
      const [branches, nextCommits] = await Promise.all([
        service.listGitBranches(),
        service.listGitCommits('HEAD'),
      ]);
      setGitCurrentBranch(branches.current || '');
      setCommits(nextCommits);
      const firstCommit = nextCommits[0]?.sha ?? '';
      setSelectedCommit(prev => prev || firstCommit);
    } catch (err) {
      setGitError(err instanceof Error ? err.message : String(err));
    } finally {
      setGitLoading(false);
    }
  };

  useEffect(() => {
    if (!connected || tab !== 'git') return;
    if (gitLoading) return;
    if (commits.length > 0 && !gitError) return;
    loadGit().catch(() => undefined);
  }, [connected, tab, commits.length, gitError, gitLoading]);

  useEffect(() => {
    const run = async () => {
      if (!selectedCommit) return;
      if (commitFilesBySha[selectedCommit]) return;
      const files = await service.listGitCommitFiles(selectedCommit);
      setCommitFilesBySha(prev => ({...prev, [selectedCommit]: files}));
      if (!selectedDiff && files[0]) setSelectedDiff(files[0].path);
    };
    run().catch(err => setGitError(err instanceof Error ? err.message : String(err)));
  }, [selectedCommit, commitFilesBySha, selectedDiff]);

  useEffect(() => {
    const run = async () => {
      if (!selectedCommit || !selectedDiff) return;
      setDiffLoading(true);
      try {
        const diff = await service.readGitFileDiff(selectedCommit, selectedDiff);
        setDiffText(diff.diff || '');
      } catch (err) {
        setGitError(err instanceof Error ? err.message : String(err));
      } finally {
        setDiffLoading(false);
      }
    };
    run().catch(() => undefined);
  }, [selectedCommit, selectedDiff]);

  const connect = async () => {
    setError('');
    try {
      const ws = toRegistryWsUrl(address);
      const session = await service.connect(ws, token.trim());
      setProjects(session.projects);
      setProjectId(session.selectedProjectId);
      setDirEntries({'.': sortEntries(session.fileEntries)});
      setExpandedDirs(['.']);
      setSelectedFile(session.fileEntries.find(item => item.kind === 'file')?.path ?? '');
      setConnected(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const switchProject = async (nextProjectId: string) => {
    setLoadingProject(true);
    try {
      const session = await service.selectProject(nextProjectId);
      setProjects(session.projects);
      setProjectId(session.selectedProjectId);
      setDirEntries({'.': sortEntries(session.fileEntries)});
      setExpandedDirs(['.']);
      setSelectedFile(session.fileEntries.find(item => item.kind === 'file')?.path ?? '');
      setPinnedFiles([]);
      setFileContent('');
      setCommits([]);
      setSelectedCommit('');
      setCommitFilesBySha({});
      setSelectedDiff('');
      setDiffText('');
      setProjectMenuOpen(false);
      setSidebarSettingsOpen(false);
      if (!isWide) setDrawerOpen(false);
    } finally {
      setLoadingProject(false);
    }
  };

  const refreshProject = async () => {
    if (!projectId) return;
    setRefreshingProject(true);
    try {
      const expandedSnapshot = [...expandedDirs];
      const session = await service.selectProject(projectId);
      const nextDirs: DirEntries = {'.': sortEntries(session.fileEntries)};
      for (const dirPath of expandedSnapshot) {
        if (dirPath === '.') continue;
        try {
          nextDirs[dirPath] = sortEntries(await service.listDirectory(dirPath));
        } catch {
          // keep refresh resilient when directory disappeared remotely
        }
      }
      setDirEntries(nextDirs);
      setExpandedDirs(expandedSnapshot.filter(path => path === '.' || !!nextDirs[path]));
      await loadGit();
    } finally {
      setRefreshingProject(false);
    }
  };

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
          {commits.map(commit => (
            <div
              key={commit.sha}
              className={`item ${selectedCommit === commit.sha ? 'selected' : ''}`}
              onClick={() => {
                setSelectedCommit(commit.sha);
              }}>
              <span className="file-dot codicon codicon-git-commit" />
              <span className="label">{commit.title || commit.sha.slice(0, 7)}</span>
            </div>
          ))}
        </div>
        <div className="section-title">CHANGED FILES</div>
        <div className="list half">
          {currentCommitFiles.map(file => (
            <div
              key={file.path}
              className={`item ${selectedDiff === file.path ? 'selected' : ''}`}
              onClick={() => {
                setSelectedDiff(file.path);
                if (!isWide) setDrawerOpen(false);
              }}>
              <span className={`status-tag status-git-${file.status}`}>{file.status}</span>
              <span className="label">{file.path}</span>
            </div>
          ))}
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
    return <PrismCodeBlock content={content} language={language} wrap={wrapLines} lineNumbers={numbersOn} highlightLine={temporaryHighlightLine} />;
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
    const {oldText, newText, hasContent, oldStart, newStart} = parseUnifiedDiff(content);
    if (!hasContent) return <div className="muted block">No diff hunks available</div>;
    const linesOffset = Math.max(0, Math.min(oldStart, newStart) - 1);
    const language = detectCodeLanguage(selectedDiff || selectedFile);

    return (
      <div className={`code-wrap diff-wrap ${wrapLines ? 'wrap' : 'nowrap'}`}>
        <ReactDiffViewer
          oldValue={oldText}
          newValue={newText}
          splitView={false}
          showDiffOnly={false}
          disableWordDiff={true}
          compareMethod={DiffMethod.LINES}
          linesOffset={linesOffset}
          renderContent={line => <PrismInlineCode content={line} language={language} wrap={wrapLines} />}
          hideLineNumbers={!showLineNumbers}
          useDarkTheme={themeMode === 'dark'}
          styles={getDiffViewerStyles(wrapLines)}
        />
      </div>
    );
  };

  const renderMain = () => {
    if (tab === 'chat') {
      return (
        <div className="content">
          <div className="block-title">CHAT - {chatSessions[chatSessionIndex]}</div>
          <div className="scroll-panel chat-block">
            <div className="empty-card">
              <div className="empty-title">Chat Panel</div>
              <div className="empty-desc">Unified split layout is ready. Chat content will render here.</div>
            </div>
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
                            (event.currentTarget as HTMLInputElement).blur();
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
                            (event.currentTarget as HTMLInputElement).blur();
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
        <div className="scroll-panel">{diffLoading ? <div className="muted block">Loading diff...</div> : renderDiffPane(diffText)}</div>
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
          <button className="button" onClick={connect}>
            Connect
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
            {(loadingProject || refreshingProject) ? <span className="muted">...</span> : null}
          </button>
          {projectMenuOpen ? (
            <div className="project-menu">
              {projects.map(project => (
                <div
                  key={project.projectId}
                  className={`item ${project.projectId === projectId ? 'selected' : ''}`}
                  onClick={() => switchProject(project.projectId).catch(() => undefined)}>
                  {project.name}
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
            <span className="statusbar-muted">{fileContent.split('\n').length} lines</span>
          ) : null}
          <span className="statusbar-spacer" />
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

createRoot(document.getElementById('root')!).render(<App />);




