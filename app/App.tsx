import React, {useEffect, useRef, useState} from 'react';
import {StyleSheet, View} from 'react-native';

import type {
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryProject,
} from './src/types/observe';
import {RegistryWorkspaceService, type WorkspaceSession} from './src/services/registryWorkspaceService';
import {ConnectScreen, WorkspaceScreen} from './src/screens';
import {resolveTheme, type ThemeMode} from './src/theme';

type SessionState = {
  projects: RegistryProject[];
  selectedProjectId: string;
  fileEntries: RegistryFsEntry[];
};

function App() {
  const serviceRef = useRef<RegistryWorkspaceService>(new RegistryWorkspaceService());
  const [session, setSession] = useState<SessionState | null>(null);
  const [themeMode, setThemeMode] = useState<ThemeMode>('dark');
  const theme = resolveTheme(themeMode);
  const rootStyle = [styles.root, {backgroundColor: theme.colors.background}];

  useEffect(() => {
    return () => {
      serviceRef.current.close();
    };
  }, []);

  const applySession = (next: WorkspaceSession) => {
    setSession({
      projects: next.projects,
      selectedProjectId: next.selectedProjectId,
      fileEntries: next.fileEntries,
    });
  };

  const connect = async (wsUrl: string, token: string): Promise<void> => {
    const nextSession = await serviceRef.current.connect(wsUrl, token);
    applySession(nextSession);
  };

  const readFile = async (path: string): Promise<string> => {
    return serviceRef.current.readFile(path);
  };

  const selectProject = async (projectId: string): Promise<void> => {
    const nextSession = await serviceRef.current.selectProject(projectId);
    applySession(nextSession);
  };

  const listDirectory = async (path: string): Promise<RegistryFsEntry[]> => {
    return serviceRef.current.listDirectory(path);
  };

  const logout = (): void => {
    serviceRef.current.close();
    setSession(null);
  };

  const listGitCommits = async (ref = 'HEAD'): Promise<RegistryGitCommit[]> => {
    return serviceRef.current.listGitCommits(ref);
  };

  const listGitBranches = async (): Promise<{current: string; branches: string[]}> => {
    return serviceRef.current.listGitBranches();
  };

  const listGitCommitFiles = async (sha: string): Promise<RegistryGitCommitFile[]> => {
    return serviceRef.current.listGitCommitFiles(sha);
  };

  const readGitFileDiff = async (sha: string, path: string): Promise<RegistryGitFileDiff> => {
    return serviceRef.current.readGitFileDiff(sha, path);
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
        onListGitBranches={listGitBranches}
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
