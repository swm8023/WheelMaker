import React, {useEffect, useMemo, useState} from 'react';
import {createRoot} from 'react-dom/client';
import Prism from 'prismjs';
import setiThemeJson from '@codingame/monaco-vscode-theme-seti-default-extension/resources/vs-seti-icon-theme.json';
import setiFontUrl from '@codingame/monaco-vscode-theme-seti-default-extension/resources/seti.woff';
import 'prismjs/components/prism-markup';
import 'prismjs/components/prism-clike';
import 'prismjs/components/prism-javascript';
import 'prismjs/components/prism-typescript';
import 'prismjs/components/prism-jsx';
import 'prismjs/components/prism-tsx';
import 'prismjs/components/prism-json';
import 'prismjs/components/prism-go';
import 'prismjs/components/prism-c';
import 'prismjs/components/prism-cpp';
import 'prismjs/components/prism-rust';
import 'prismjs/components/prism-bash';
import 'prismjs/components/prism-yaml';
import 'prismjs/components/prism-markdown';
import 'prismjs/components/prism-diff';
import 'prismjs/themes/prism-tomorrow.css';
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

function detectPrismLanguage(path: string): string {
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
              <span className="status-tag">{file.status}</span>
              <span className="label">{file.path}</span>
            </div>
          ))}
        </div>
      </>
    );
  };

  const renderCodePane = (content: string, forceLineNumbers = false, languageHint = '') => {
    const numbersOn = forceLineNumbers || showLineNumbers;
    const language = languageHint || detectPrismLanguage(selectedFile);
    const grammar = Prism.languages[language] || Prism.languages.clike;
    const highlighted = Prism.highlight(content || '', grammar, language);
    const lines = highlighted.split('\n');

    if (!numbersOn) {
      return <pre className={`code-block prism-code ${wrapLines ? 'wrap' : 'nowrap'}`} dangerouslySetInnerHTML={{__html: highlighted || ' '}} />;
    }

    return (
      <div className={`code-grid prism-code ${wrapLines ? 'wrap' : 'nowrap'}`}>
        {lines.map((line: string, index: number) => (
          <div key={`${index}-${line.length}`} className="code-row">
            <span className="line-number">{index + 1}</span>
            <span className="line-text" dangerouslySetInnerHTML={{__html: line || ' '}} />
          </div>
        ))}
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
          <div className="scroll-panel">{fileLoading ? <div className="muted block">Loading file...</div> : renderCodePane(fileContent, false, detectPrismLanguage(selectedFile))}</div>
        </div>
      );
    }

    return (
      <div className="content">
        <div className="block-title">{selectedDiff || 'Select a changed file'}</div>
        <div className="scroll-panel">{diffLoading ? <div className="muted block">Loading diff...</div> : renderCodePane(diffText, true, 'diff')}</div>
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
          <span className={`codicon ${isWide ? (sidebarCollapsed ? 'codicon-panel-left-expand' : 'codicon-panel-left') : 'codicon-menu'}`} />
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

      {gitCurrentBranch ? (
        <div className="status-bar">
          <span className="statusbar-item">
            <span className="codicon codicon-git-branch" />
            {gitCurrentBranch}
          </span>
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
