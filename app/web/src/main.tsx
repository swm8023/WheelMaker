import React, {useEffect, useMemo, useState} from 'react';
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
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/500.css';

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
};

function parseUnifiedDiff(content: string): UnifiedDiffSides {
  const lines = content.split('\n');
  const oldLines: string[] = [];
  const newLines: string[] = [];
  let inHunk = false;

  for (const raw of lines) {
    if (raw.startsWith('@@')) {
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
      fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
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
};

function PrismCodeBlock({content, language, wrap, lineNumbers}: PrismCodeBlockProps) {
  return (
    <div className="code-wrap">
      <SyntaxHighlighter
        className={`code-block prism-code ${wrap ? 'wrap' : 'nowrap'}`}
        language={language}
        style={oneDark}
        showLineNumbers={lineNumbers}
        wrapLongLines={wrap}
        wrapLines={wrap}
        codeTagProps={{style: {whiteSpace: wrap ? 'pre-wrap' : 'pre', background: 'transparent'}}}
        lineProps={{style: {background: 'transparent', whiteSpace: wrap ? 'pre-wrap' : 'pre', wordBreak: wrap ? 'break-word' : 'normal', overflowWrap: wrap ? 'anywhere' : 'normal'}}}
        customStyle={{margin: 0, minWidth: '100%', background: 'transparent', padding: '0 10px'}}
        lineNumberStyle={{color: 'var(--muted)', minWidth: '2.4em', paddingRight: '10px', borderRight: '1px solid rgba(127, 127, 127, 0.18)', marginRight: '10px', textAlign: 'right', userSelect: 'none'}}>
        {content || ' '}
      </SyntaxHighlighter>
    </div>
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

  const [projectMenuOpen, setProjectMenuOpen] = useState(false);
  const [quickSettingsOpen, setQuickSettingsOpen] = useState(false);

  const [projects, setProjects] = useState<RegistryProject[]>([]);
  const [projectId, setProjectId] = useState('');
  const [loadingProject, setLoadingProject] = useState(false);
  const [refreshingProject, setRefreshingProject] = useState(false);

  const [dirEntries, setDirEntries] = useState<DirEntries>({'.': []});
  const [expandedDirs, setExpandedDirs] = useState<string[]>(['.']);
  const [loadingDirs, setLoadingDirs] = useState<Record<string, boolean>>({});
  const [selectedFile, setSelectedFile] = useState('');
  const [fileContent, setFileContent] = useState('');
  const [fileLoading, setFileLoading] = useState(false);

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
      setQuickSettingsOpen(false);
    };
    window.addEventListener('pointerdown', onPointer);
    return () => window.removeEventListener('pointerdown', onPointer);
  }, []);

  const currentProjectName = useMemo(
    () => projects.find(item => item.projectId === projectId)?.name ?? 'Project',
    [projectId, projects],
  );

  const currentCommitFiles = useMemo(
    () => commitFilesBySha[selectedCommit] ?? [],
    [commitFilesBySha, selectedCommit],
  );

  const isExpanded = (path: string) => expandedDirs.includes(path);

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
      setFileContent('');
      setCommits([]);
      setSelectedCommit('');
      setCommitFilesBySha({});
      setSelectedDiff('');
      setDiffText('');
      setProjectMenuOpen(false);
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

  const renderSidebar = () => {
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
                if (!isWide) setDrawerOpen(false);
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

  const renderCodePane = (content: string, forceLineNumbers = false, languageHint = '') => {
    const numbersOn = forceLineNumbers || showLineNumbers;
    const language = languageHint || detectCodeLanguage(selectedFile);
    return <PrismCodeBlock content={content} language={language} wrap={wrapLines} lineNumbers={numbersOn} />;
  };

  const renderDiffPane = (content: string) => {
    if (!content) return <div className="muted block">No diff available</div>;
    const {oldText, newText, hasContent} = parseUnifiedDiff(content);
    if (!hasContent) return <div className="muted block">No diff hunks available</div>;

    return (
      <div className={`code-wrap diff-wrap ${wrapLines ? 'wrap' : 'nowrap'}`}>
        <ReactDiffViewer
          oldValue={oldText}
          newValue={newText}
          splitView={false}
          showDiffOnly={false}
          disableWordDiff={true}
          compareMethod={DiffMethod.LINES}
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
          <div className="block-title">{selectedFile || 'Select a file'}</div>
          <div className="scroll-panel">{fileLoading ? <div className="muted block">Loading file...</div> : renderCodePane(fileContent, false, detectCodeLanguage(selectedFile))}</div>
        </div>
      );
    }

    return (
      <div className="content">
        <div className="block-title">{selectedDiff || 'Select a changed file'}</div>
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
              setDrawerOpen(true);
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
            CHAT
          </button>
          <button className={`tab ${tab === 'file' ? 'active' : ''}`} onClick={() => setTab('file')}>
            <span className="codicon codicon-files tab-icon" />
            FILE
          </button>
          <button className={`tab ${tab === 'git' ? 'active' : ''}`} onClick={() => setTab('git')}>
            <span className="codicon codicon-source-control tab-icon" />
            GIT
          </button>
        </div>

        <div className="settings-wrap" onPointerDown={event => event.stopPropagation()}>
          <button className="header-btn" onClick={() => setQuickSettingsOpen(value => !value)}>
            <span className="codicon codicon-settings-gear" />
          </button>
          {quickSettingsOpen ? (
            <div className="settings-menu">
              <label className="switch-row">
                <span>Dark Mode</span>
                <input type="checkbox" checked={themeMode === 'dark'} onChange={e => setThemeMode(e.target.checked ? 'dark' : 'light')} />
              </label>
              <label className="switch-row">
                <span>Wrap Line</span>
                <input type="checkbox" checked={wrapLines} onChange={e => setWrapLines(e.target.checked)} />
              </label>
              <label className="switch-row">
                <span>Line Number</span>
                <input type="checkbox" checked={showLineNumbers} onChange={e => setShowLineNumbers(e.target.checked)} />
              </label>
            </div>
          ) : null}
        </div>
      </header>

      <div className="body">
        {isWide && !sidebarCollapsed ? <aside className="left">{renderSidebar()}</aside> : null}
        <main className="right">{renderMain()}</main>
      </div>

      {tab === 'file' ? (
        <div className="status-bar">
          {selectedFile ? (
            <span className="statusbar-item">
              <span className="codicon codicon-file" />
              {selectedFile.split('/').pop()}
            </span>
          ) : null}
          {selectedFile && fileContent.length > 0 ? (
            <span className="statusbar-muted">{fileContent.split('\n').length} lines</span>
          ) : null}
          <span className="statusbar-spacer" />
          {gitCurrentBranch ? (
            <span className="statusbar-item">
              <span className="codicon codicon-git-branch" />
              {gitCurrentBranch}
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

