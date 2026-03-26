import React, {useEffect, useMemo, useRef, useState} from 'react';

import type {ObserveFsEntry, ObserveProject} from './src/types/observe';
import {createObserveRepository, type ObserveRepository} from './src/services/observeRepository';
import {ConnectScreen, WorkspaceScreen} from './src/screens';

type SessionState = {
  projects: ObserveProject[];
  selectedProjectId: string;
  fileEntries: ObserveFsEntry[];
};

function App() {
  const repositoryRef = useRef<ObserveRepository | null>(null);
  const [session, setSession] = useState<SessionState | null>(null);

  useEffect(() => {
    return () => {
      repositoryRef.current?.close();
      repositoryRef.current = null;
    };
  }, []);

  const connect = async (wsUrl: string, token: string): Promise<void> => {
    const repository = createObserveRepository();
    try {
      await repository.initialize(wsUrl, token);
      const projects = await repository.listProjects();
      if (projects.length === 0) {
        throw new Error('no projects available');
      }
      const selectedProjectId = projects[0].projectId;
      const fileEntries = await repository.listFiles(selectedProjectId, '.');
      repositoryRef.current?.close();
      repositoryRef.current = repository;
      setSession({
        projects,
        selectedProjectId,
        fileEntries,
      });
    } catch (error) {
      repository.close();
      throw error;
    }
  };

  const readFile = async (path: string): Promise<string> => {
    if (!session || !repositoryRef.current) {
      throw new Error('session is not ready');
    }
    return repositoryRef.current.readFile(session.selectedProjectId, path);
  };

  const selectProject = async (projectId: string): Promise<void> => {
    if (!session || !repositoryRef.current) {
      return;
    }
    const fileEntries = await repositoryRef.current.listFiles(projectId, '.');
    setSession({
      ...session,
      selectedProjectId: projectId,
      fileEntries,
    });
  };

  const workspace = useMemo(() => {
    if (!session) {
      return null;
    }
    return (
      <WorkspaceScreen
        projects={session.projects}
        selectedProjectId={session.selectedProjectId}
        fileEntries={session.fileEntries}
        onSelectProject={selectProject}
        onReadFile={readFile}
      />
    );
  }, [session]);

  if (!session) {
    return <ConnectScreen onConnect={connect} />;
  }
  return workspace;
}

export default App;
