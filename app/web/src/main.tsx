import React, {useEffect, useMemo, useState} from 'react';
import {createRoot} from 'react-dom/client';

import {RegistryWorkspaceService} from '../../src/services/registryWorkspaceService';
import type {RegistryGitCommitFile, RegistryProject} from '../../src/types/observe';
import {getDefaultRegistryAddress, toRegistryWsUrl} from './runtime';
import './styles.css';

type Tab = 'chat' | 'file' | 'git';

const service = new RegistryWorkspaceService();

function App() {
  const [connected, setConnected] = useState(false);
  const [address, setAddress] = useState(getDefaultRegistryAddress());
  const [token, setToken] = useState('');
  const [error, setError] = useState('');
  const [projects, setProjects] = useState<RegistryProject[]>([]);
  const [projectId, setProjectId] = useState('');
  const [tab, setTab] = useState<Tab>('file');
  const [files, setFiles] = useState<Array<{path: string; name: string; kind: string}>>([]);
  const [selectedFile, setSelectedFile] = useState('');
  const [fileContent, setFileContent] = useState('');
  const [commits, setCommits] = useState<Array<{sha: string; title: string; author: string; time: string}>>([]);
  const [selectedCommit, setSelectedCommit] = useState('');
  const [commitFiles, setCommitFiles] = useState<RegistryGitCommitFile[]>([]);
  const [selectedDiff, setSelectedDiff] = useState('');
  const [diffText, setDiffText] = useState('');

  useEffect(() => {
    const run = async () => {
      if (!connected || !projectId) return;
      const rootFiles = await service.listDirectory('.');
      setFiles(rootFiles);
    };
    run().catch(err => setError(err instanceof Error ? err.message : String(err)));
  }, [connected, projectId]);

  useEffect(() => {
    const run = async () => {
      if (!selectedFile) return;
      const content = await service.readFile(selectedFile);
      setFileContent(content);
    };
    run().catch(err => setError(err instanceof Error ? err.message : String(err)));
  }, [selectedFile]);

  useEffect(() => {
    const run = async () => {
      if (!connected) return;
      const next = await service.listGitCommits('HEAD');
      setCommits(next);
      if (next[0]) setSelectedCommit(next[0].sha);
    };
    run().catch(err => setError(err instanceof Error ? err.message : String(err)));
  }, [connected]);

  useEffect(() => {
    const run = async () => {
      if (!selectedCommit) return;
      const next = await service.listGitCommitFiles(selectedCommit);
      setCommitFiles(next);
      if (next[0]) setSelectedDiff(next[0].path);
    };
    run().catch(err => setError(err instanceof Error ? err.message : String(err)));
  }, [selectedCommit]);

  useEffect(() => {
    const run = async () => {
      if (!selectedCommit || !selectedDiff) return;
      const diff = await service.readGitFileDiff(selectedCommit, selectedDiff);
      setDiffText(diff.diff || '');
    };
    run().catch(err => setError(err instanceof Error ? err.message : String(err)));
  }, [selectedCommit, selectedDiff]);

  const connect = async () => {
    setError('');
    try {
      const ws = toRegistryWsUrl(address);
      const session = await service.connect(ws, token.trim());
      setProjects(session.projects);
      setProjectId(session.selectedProjectId);
      setConnected(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const currentProjectName = useMemo(
    () => projects.find(item => item.projectId === projectId)?.name ?? 'Project',
    [projectId, projects],
  );

  const renderLeft = () => {
    if (tab === 'chat') {
      return (
        <div className="list">
          <div className="item selected">General</div>
          <div className="item">WheelMaker App</div>
          <div className="item">Go Service</div>
        </div>
      );
    }
    if (tab === 'file') {
      return (
        <div className="list">
          {files.map(file => (
            <div
              key={file.path}
              className={`item ${selectedFile === file.path ? 'selected' : ''}`}
              onClick={() => {
                if (file.kind !== 'dir') setSelectedFile(file.path);
              }}>
              {file.kind === 'dir' ? '📁' : '📄'} {file.name}
            </div>
          ))}
        </div>
      );
    }
    return (
      <>
        <div className="section-title">COMMITS</div>
        <div className="list" style={{maxHeight: '50%'}}>
          {commits.map(item => (
            <div
              key={item.sha}
              className={`item ${selectedCommit === item.sha ? 'selected' : ''}`}
              onClick={() => setSelectedCommit(item.sha)}>
              {item.title || item.sha.slice(0, 7)}
            </div>
          ))}
        </div>
        <div className="section-title">FILES</div>
        <div className="list" style={{maxHeight: '50%'}}>
          {commitFiles.map(item => (
            <div
              key={item.path}
              className={`item ${selectedDiff === item.path ? 'selected' : ''}`}
              onClick={() => setSelectedDiff(item.path)}>
              {item.status} {item.path}
            </div>
          ))}
        </div>
      </>
    );
  };

  const renderContent = () => {
    if (tab === 'chat') {
      return <pre className="code">CHAT mode is reserved for upcoming web chat integration.</pre>;
    }
    if (tab === 'file') {
      return <pre className="code">{fileContent || 'Select a file.'}</pre>;
    }
    return <pre className="code">{diffText || 'Select a commit file.'}</pre>;
  };

  if (!connected) {
    return (
      <div className="page">
        <div className="connect">
          <h3>Connect to WheelMaker Registry</h3>
          <input
            className="input"
            value={address}
            onChange={e => setAddress(e.target.value)}
            placeholder="127.0.0.1:9630 or ws://127.0.0.1:9630/ws"
          />
          <input
            className="input"
            value={token}
            onChange={e => setToken(e.target.value)}
            placeholder="Token (optional)"
          />
          <button className="button" onClick={connect}>
            Connect
          </button>
          {error ? <div className="error">{error}</div> : null}
        </div>
      </div>
    );
  }

  return (
    <div className="workspace">
      <div className="header">
        <strong>{currentProjectName}</strong>
        <select
          className="button"
          value={projectId}
          onChange={async e => {
            const next = await service.selectProject(e.target.value);
            setProjectId(next.selectedProjectId);
            setProjects(next.projects);
            setSelectedFile('');
            setFileContent('');
            setCommits([]);
          }}>
          {projects.map(project => (
            <option key={project.projectId} value={project.projectId}>
              {project.name}
            </option>
          ))}
        </select>
        <div className="tabs">
          <button className={`tab ${tab === 'chat' ? 'active' : ''}`} onClick={() => setTab('chat')}>
            CHAT
          </button>
          <button className={`tab ${tab === 'file' ? 'active' : ''}`} onClick={() => setTab('file')}>
            FILE
          </button>
          <button className={`tab ${tab === 'git' ? 'active' : ''}`} onClick={() => setTab('git')}>
            GIT
          </button>
        </div>
      </div>
      <div className="body">
        <aside className="left">{renderLeft()}</aside>
        <main className="right">
          <div className="content">{renderContent()}</div>
        </main>
      </div>
    </div>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
