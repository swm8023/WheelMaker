import React, {useEffect, useRef, useState} from 'react';
import {StyleSheet, View} from 'react-native';

import type {
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryProject,
} from './src/types/observe';
import {
  createRegistryRepository,
  type RegistryRepository,
} from './src/services/observeRepository';
import {ConnectScreen, WorkspaceScreen} from './src/screens';
import {resolveTheme, type ThemeMode} from './src/theme';

type SessionState = {
  projects: RegistryProject[];
  selectedProjectId: string;
  fileEntries: RegistryFsEntry[];
};

function App() {
  const repositoryRef = useRef<RegistryRepository | null>(null);
  const [session, setSession] = useState<SessionState | null>(null);
  const [themeMode, setThemeMode] = useState<ThemeMode>('dark');
  const theme = resolveTheme(themeMode);
  const rootStyle = [styles.root, {backgroundColor: theme.colors.background}];

  useEffect(() => {
    return () => {
      repositoryRef.current?.close();
      repositoryRef.current = null;
    };
  }, []);

  const connect = async (wsUrl: string, token: string): Promise<void> => {
    const repository = createRegistryRepository();
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
      setSession({projects, selectedProjectId, fileEntries});
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
    setSession({...session, selectedProjectId: projectId, fileEntries});
  };

  const listDirectory = async (path: string): Promise<RegistryFsEntry[]> => {
    if (!session || !repositoryRef.current) {
      return [];
    }
    return repositoryRef.current.listFiles(session.selectedProjectId, path || '.');
  };

  const logout = (): void => {
    repositoryRef.current?.close();
    repositoryRef.current = null;
    setSession(null);
  };

  const listGitCommits = async (ref = 'HEAD'): Promise<RegistryGitCommit[]> => {
    if (!session || !repositoryRef.current) {
      return [];
    }
    return repositoryRef.current.gitLog(session.selectedProjectId, ref, '', 50);
  };

  const listGitCommitFiles = async (sha: string): Promise<RegistryGitCommitFile[]> => {
    if (!session || !repositoryRef.current) {
      return [];
    }
    return repositoryRef.current.gitCommitFiles(session.selectedProjectId, sha);
  };

  const readGitFileDiff = async (sha: string, path: string): Promise<RegistryGitFileDiff> => {
    if (!session || !repositoryRef.current) {
      return {sha, path, isBinary: false, diff: '', truncated: false};
    }
    return repositoryRef.current.gitCommitFileDiff(session.selectedProjectId, sha, path, 3);
  };

  if (!session) {
    return (
      <View style={rootStyle}>
        <ConnectScreen onConnect={connect} theme={theme} />
      </View>
    );
  }

  return (
    <View style={rootStyle}>
      <WorkspaceScreen
        projects={session.projects}
        selectedProjectId={session.selectedProjectId}
        fileEntries={session.fileEntries}
        onSelectProject={selectProject}
        onListDirectory={listDirectory}
        onReadFile={readFile}
        onListGitCommits={listGitCommits}
        onListGitCommitFiles={listGitCommitFiles}
        onReadGitFileDiff={readGitFileDiff}
        onLogout={logout}
        themeMode={themeMode}
        onThemeModeChange={setThemeMode}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
  },
});

export default App;
