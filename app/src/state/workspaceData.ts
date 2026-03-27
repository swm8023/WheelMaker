import {useEffect, useMemo, useState} from 'react';

import type {
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryProject,
} from '../types/observe';

export type WorkspaceTab = 'chat' | 'file' | 'git';

export type FileNode = {
  name: string;
  path: string;
  isDir: boolean;
  loaded: boolean;
  children: FileNode[];
};

export type GitCommitView = RegistryGitCommit;

export type GitCommitFileView = RegistryGitCommitFile & {
  diff: string;
  isBinary: boolean;
  truncated: boolean;
  loadingDiff: boolean;
};

export const CHAT_SESSIONS = ['General', 'WheelMaker App', 'Go Service', 'Review'];

export const CHAT_MESSAGES = [
  {role: 'system', text: 'Connected to registry workspace.'},
  {role: 'agent', text: 'Workspace theme now supports VS Code style modes.'},
];

type ProjectWorkspaceState = {
  fileTree: FileNode;
  expandedPaths: Set<string>;
  selectedFilePath: string | null;
  selectedFileContent: string;
  loadingFilePath: string;
  loadingDirs: Set<string>;
  chatSessionIndex: number;
  chatInput: string;
  gitLoading: boolean;
  gitError: string;
  gitCommits: GitCommitView[];
  gitFilesBySha: Record<string, GitCommitFileView[]>;
  selectedCommitSha: string;
  selectedDiffFilePath: string;
};

type UseWorkspaceDataArgs = {
  projects: RegistryProject[];
  selectedProjectId: string;
  fileEntries: RegistryFsEntry[];
  onListDirectory: (path: string) => Promise<RegistryFsEntry[]>;
  onReadFile: (path: string) => Promise<string>;
  onListGitCommits: (ref?: string) => Promise<RegistryGitCommit[]>;
  onListGitCommitFiles: (sha: string) => Promise<RegistryGitCommitFile[]>;
  onReadGitFileDiff: (sha: string, path: string) => Promise<RegistryGitFileDiff>;
};

type UseWorkspaceDataResult = {
  projectState: ProjectWorkspaceState;
  setChatInput: (value: string) => void;
  refreshProject: () => Promise<void>;
  selectFile: (path: string | null) => void;
  toggleDirectory: (path: string) => Promise<void>;
  selectCommit: (index: number) => void;
  selectDiffFile: (path: string) => void;
};

function initialProjectState(projectName: string, entries: RegistryFsEntry[]): ProjectWorkspaceState {
  const root: FileNode = {
    name: projectName || 'Project',
    path: '.',
    isDir: true,
    loaded: true,
    children: entries.map(entryToNode).sort(sortFileNode),
  };
  const firstFile = findFirstFile(root);
  return {
    fileTree: root,
    expandedPaths: new Set(['.']),
    selectedFilePath: firstFile?.path ?? null,
    selectedFileContent: '',
    loadingFilePath: '',
    loadingDirs: new Set(),
    chatSessionIndex: 0,
    chatInput: '',
    gitLoading: false,
    gitError: '',
    gitCommits: [],
    gitFilesBySha: {},
    selectedCommitSha: '',
    selectedDiffFilePath: '',
  };
}

export function useWorkspaceData(args: UseWorkspaceDataArgs): UseWorkspaceDataResult {
  const {
    projects,
    selectedProjectId: selectedProjectIdArg,
    fileEntries,
    onListDirectory,
    onReadFile,
    onListGitCommits,
    onListGitCommitFiles,
    onReadGitFileDiff,
  } = args;
  const selectedProject = projects.find(item => item.projectId === selectedProjectIdArg) ?? projects[0];
  const selectedProjectId = selectedProject?.projectId ?? '';
  const [projectStates, setProjectStates] = useState<Record<string, ProjectWorkspaceState>>({});

  useEffect(() => {
    if (!selectedProjectId) return;
    setProjectStates(prev => {
      if (prev[selectedProjectId]) return prev;
      return {
        ...prev,
        [selectedProjectId]: initialProjectState(selectedProject?.name ?? 'Project', fileEntries),
      };
    });
  }, [fileEntries, selectedProject?.name, selectedProjectId]);

  useEffect(() => {
    if (!selectedProjectId) return;
    setProjectStates(prev => {
      const next = {...prev};
      const before = prev[selectedProjectId] ?? initialProjectState(selectedProject?.name ?? 'Project', fileEntries);
      const refreshed = initialProjectState(selectedProject?.name ?? 'Project', fileEntries);
      next[selectedProjectId] = {
        ...refreshed,
        chatInput: before.chatInput,
        chatSessionIndex: before.chatSessionIndex,
        gitCommits: before.gitCommits,
        gitFilesBySha: before.gitFilesBySha,
        selectedCommitSha: before.selectedCommitSha,
        selectedDiffFilePath: before.selectedDiffFilePath,
      };
      return next;
    });
  }, [selectedProjectId, selectedProject?.name, fileEntries]);

  const projectState = useMemo(() => {
    if (!selectedProjectId) return initialProjectState('Project', []);
    return projectStates[selectedProjectId] ?? initialProjectState(selectedProject?.name ?? 'Project', fileEntries);
  }, [selectedProject?.name, selectedProjectId, projectStates, fileEntries]);

  useEffect(() => {
    if (!selectedProjectId || !projectState.selectedFilePath) return;
    let cancelled = false;
    const path = projectState.selectedFilePath;
    const run = async () => {
      setProjectStates(prev => {
        const current = prev[selectedProjectId];
        if (!current) return prev;
        return {
          ...prev,
          [selectedProjectId]: {
            ...current,
            loadingFilePath: path,
          },
        };
      });
      try {
        const content = await onReadFile(path);
        if (cancelled) return;
        setProjectStates(prev => {
          const current = prev[selectedProjectId];
          if (!current) return prev;
          return {
            ...prev,
            [selectedProjectId]: {
              ...current,
              selectedFileContent: content,
              loadingFilePath: '',
            },
          };
        });
      } catch {
        if (cancelled) return;
        setProjectStates(prev => {
          const current = prev[selectedProjectId];
          if (!current) return prev;
          return {
            ...prev,
            [selectedProjectId]: {
              ...current,
              loadingFilePath: '',
            },
          };
        });
      }
    };
    run().catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [onReadFile, selectedProjectId, projectState.selectedFilePath]);

  useEffect(() => {
    if (!selectedProjectId) return;
    if (projectState.gitLoading) return;
    if (projectState.gitCommits.length > 0) return;

    let cancelled = false;
    const run = async () => {
      setProjectStates(prev => {
        const current = prev[selectedProjectId];
        if (!current) return prev;
        return {
          ...prev,
          [selectedProjectId]: {
            ...current,
            gitLoading: true,
            gitError: '',
          },
        };
      });
      try {
        const commits = await onListGitCommits('HEAD');
        if (cancelled) return;
        setProjectStates(prev => {
          const current = prev[selectedProjectId];
          if (!current) return prev;
          const selectedCommitSha = current.selectedCommitSha || commits[0]?.sha || '';
          return {
            ...prev,
            [selectedProjectId]: {
              ...current,
              gitLoading: false,
              gitError: '',
              gitCommits: commits,
              selectedCommitSha,
            },
          };
        });
      } catch (error) {
        if (cancelled) return;
        setProjectStates(prev => {
          const current = prev[selectedProjectId];
          if (!current) return prev;
          return {
            ...prev,
            [selectedProjectId]: {
              ...current,
              gitLoading: false,
              gitError: error instanceof Error ? error.message : String(error),
            },
          };
        });
      }
    };
    run().catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [onListGitCommits, projectState.gitCommits.length, projectState.gitLoading, selectedProjectId]);

  useEffect(() => {
    if (!selectedProjectId || !projectState.selectedCommitSha) return;
    if (projectState.gitFilesBySha[projectState.selectedCommitSha]) {
      const files = projectState.gitFilesBySha[projectState.selectedCommitSha];
      if (!projectState.selectedDiffFilePath && files[0]) {
        setProjectStates(prev => {
          const current = prev[selectedProjectId];
          if (!current) return prev;
          return {
            ...prev,
            [selectedProjectId]: {
              ...current,
              selectedDiffFilePath: files[0].path,
            },
          };
        });
      }
      return;
    }

    let cancelled = false;
    const targetSha = projectState.selectedCommitSha;
    const run = async () => {
      try {
        const files = await onListGitCommitFiles(targetSha);
        if (cancelled) return;
        const normalized: GitCommitFileView[] = files.map(file => ({
          ...file,
          diff: '',
          isBinary: false,
          truncated: false,
          loadingDiff: false,
        }));
        setProjectStates(prev => {
          const current = prev[selectedProjectId];
          if (!current) return prev;
          const selectedDiffFilePath =
            current.selectedCommitSha === targetSha
              ? current.selectedDiffFilePath || normalized[0]?.path || ''
              : current.selectedDiffFilePath;
          return {
            ...prev,
            [selectedProjectId]: {
              ...current,
              gitFilesBySha: {
                ...current.gitFilesBySha,
                [targetSha]: normalized,
              },
              selectedDiffFilePath,
            },
          };
        });
      } catch {
        // ignore; UI can keep previous available state
      }
    };

    run().catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [onListGitCommitFiles, projectState.gitFilesBySha, projectState.selectedCommitSha, projectState.selectedDiffFilePath, selectedProjectId]);

  useEffect(() => {
    if (!selectedProjectId || !projectState.selectedCommitSha || !projectState.selectedDiffFilePath) return;
    const files = projectState.gitFilesBySha[projectState.selectedCommitSha] ?? [];
    const index = files.findIndex(file => file.path === projectState.selectedDiffFilePath);
    if (index < 0) return;
    const target = files[index];
    if (target.diff || target.loadingDiff) return;

    let cancelled = false;
    const sha = projectState.selectedCommitSha;
    const path = projectState.selectedDiffFilePath;

    setProjectStates(prev => {
      const current = prev[selectedProjectId];
      if (!current) return prev;
      const currentFiles = current.gitFilesBySha[sha] ?? [];
      const nextFiles = currentFiles.map(file =>
        file.path === path
          ? {
              ...file,
              loadingDiff: true,
            }
          : file,
      );
      return {
        ...prev,
        [selectedProjectId]: {
          ...current,
          gitFilesBySha: {
            ...current.gitFilesBySha,
            [sha]: nextFiles,
          },
        },
      };
    });

    const run = async () => {
      try {
        const diffResp = await onReadGitFileDiff(sha, path);
        if (cancelled) return;
        setProjectStates(prev => {
          const current = prev[selectedProjectId];
          if (!current) return prev;
          const currentFiles = current.gitFilesBySha[sha] ?? [];
          const nextFiles = currentFiles.map(file =>
            file.path === path
              ? {
                  ...file,
                  diff: diffResp.diff,
                  isBinary: diffResp.isBinary,
                  truncated: diffResp.truncated,
                  loadingDiff: false,
                }
              : file,
          );
          return {
            ...prev,
            [selectedProjectId]: {
              ...current,
              gitFilesBySha: {
                ...current.gitFilesBySha,
                [sha]: nextFiles,
              },
            },
          };
        });
      } catch {
        if (cancelled) return;
        setProjectStates(prev => {
          const current = prev[selectedProjectId];
          if (!current) return prev;
          const currentFiles = current.gitFilesBySha[sha] ?? [];
          const nextFiles = currentFiles.map(file =>
            file.path === path
              ? {
                  ...file,
                  loadingDiff: false,
                }
              : file,
          );
          return {
            ...prev,
            [selectedProjectId]: {
              ...current,
              gitFilesBySha: {
                ...current.gitFilesBySha,
                [sha]: nextFiles,
              },
            },
          };
        });
      }
    };

    run().catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [onReadGitFileDiff, projectState.gitFilesBySha, projectState.selectedCommitSha, projectState.selectedDiffFilePath, selectedProjectId]);

  const setChatInput = (value: string) => {
    if (!selectedProjectId) return;
    setProjectStates(prev => {
      const current = prev[selectedProjectId];
      if (!current) return prev;
      return {
        ...prev,
        [selectedProjectId]: {
          ...current,
          chatInput: value,
        },
      };
    });
  };

  const refreshProject = async (): Promise<void> => {
    if (!selectedProjectId) return;
    const snapshot =
      projectStates[selectedProjectId] ?? initialProjectState(selectedProject?.name ?? 'Project', fileEntries);
    const requestedExpanded = new Set(snapshot.expandedPaths);
    requestedExpanded.add('.');
    const existingDirs = new Set<string>(['.']);
    const existingFiles = new Set<string>();

    const buildDir = async (path: string, name: string): Promise<FileNode> => {
      const entries = await onListDirectory(path);
      const children = entries.map(entryToNode).sort(sortFileNode);
      for (const child of children) {
        if (child.isDir) {
          existingDirs.add(child.path);
        } else {
          existingFiles.add(child.path);
        }
      }

      const resolvedChildren: FileNode[] = [];
      for (const child of children) {
        if (child.isDir && requestedExpanded.has(child.path)) {
          const expandedDir = await buildDir(child.path, child.name);
          resolvedChildren.push(expandedDir);
        } else {
          resolvedChildren.push(child);
        }
      }

      return {
        name,
        path,
        isDir: true,
        loaded: true,
        children: resolvedChildren,
      };
    };

    const nextRoot = await buildDir('.', selectedProject?.name ?? 'Project');
    const nextExpanded = new Set<string>();
    for (const path of requestedExpanded) {
      if (existingDirs.has(path)) {
        nextExpanded.add(path);
      }
    }
    nextExpanded.add('.');

    const prevSelectedPath = snapshot.selectedFilePath;
    const nextSelectedPath =
      prevSelectedPath && existingFiles.has(prevSelectedPath)
        ? prevSelectedPath
        : findFirstFile(nextRoot)?.path ?? null;

    let nextGitCommits = snapshot.gitCommits;
    let nextGitError = '';
    try {
      nextGitCommits = await onListGitCommits('HEAD');
    } catch (error) {
      nextGitError = error instanceof Error ? error.message : String(error);
    }

    const commitSet = new Set(nextGitCommits.map(item => item.sha));
    let nextSelectedCommitSha = snapshot.selectedCommitSha;
    if (!nextSelectedCommitSha || !commitSet.has(nextSelectedCommitSha)) {
      nextSelectedCommitSha = nextGitCommits[0]?.sha ?? '';
    }

    setProjectStates(prev => {
      const current = prev[selectedProjectId] ?? snapshot;
      const currentFiles = current.gitFilesBySha[nextSelectedCommitSha] ?? [];
      const nextSelectedDiffFilePath =
        current.selectedDiffFilePath && currentFiles.some(file => file.path === current.selectedDiffFilePath)
          ? current.selectedDiffFilePath
          : currentFiles[0]?.path ?? '';
      return {
        ...prev,
        [selectedProjectId]: {
          ...current,
          fileTree: nextRoot,
          expandedPaths: nextExpanded,
          selectedFilePath: nextSelectedPath,
          selectedFileContent:
            nextSelectedPath && nextSelectedPath === current.selectedFilePath ? current.selectedFileContent : '',
          gitLoading: false,
          gitError: nextGitError,
          gitCommits: nextGitCommits,
          selectedCommitSha: nextSelectedCommitSha,
          selectedDiffFilePath: nextSelectedDiffFilePath,
        },
      };
    });
  };

  const selectFile = (path: string | null) => {
    if (!selectedProjectId) return;
    setProjectStates(prev => {
      const current = prev[selectedProjectId];
      if (!current) return prev;
      return {
        ...prev,
        [selectedProjectId]: {
          ...current,
          selectedFilePath: path,
        },
      };
    });
  };

  const selectCommit = (index: number) => {
    if (!selectedProjectId) return;
    setProjectStates(prev => {
      const current = prev[selectedProjectId];
      if (!current) return prev;
      const commit = current.gitCommits[index];
      if (!commit) return prev;
      const files = current.gitFilesBySha[commit.sha] ?? [];
      return {
        ...prev,
        [selectedProjectId]: {
          ...current,
          selectedCommitSha: commit.sha,
          selectedDiffFilePath: files[0]?.path ?? '',
        },
      };
    });
  };

  const selectDiffFile = (path: string) => {
    if (!selectedProjectId) return;
    setProjectStates(prev => {
      const current = prev[selectedProjectId];
      if (!current) return prev;
      return {
        ...prev,
        [selectedProjectId]: {
          ...current,
          selectedDiffFilePath: path,
        },
      };
    });
  };

  const toggleDirectory = async (path: string) => {
    if (!selectedProjectId) return;
    let shouldExpand = false;
    setProjectStates(prev => {
      const current = prev[selectedProjectId];
      if (!current) return prev;
      const nextExpanded = new Set(current.expandedPaths);
      if (nextExpanded.has(path)) {
        nextExpanded.delete(path);
      } else {
        nextExpanded.add(path);
        shouldExpand = true;
      }
      return {
        ...prev,
        [selectedProjectId]: {
          ...current,
          expandedPaths: nextExpanded,
        },
      };
    });

    if (!shouldExpand) return;
    const target = findNode(projectState.fileTree, path);
    if (!target || !target.isDir || target.loaded) return;

    setProjectStates(prev => {
      const current = prev[selectedProjectId];
      if (!current) return prev;
      const nextLoading = new Set(current.loadingDirs);
      nextLoading.add(path);
      return {
        ...prev,
        [selectedProjectId]: {
          ...current,
          loadingDirs: nextLoading,
        },
      };
    });

    try {
      const entries = await onListDirectory(path);
      const children = entries.map(entryToNode).sort(sortFileNode);
      setProjectStates(prev => {
        const current = prev[selectedProjectId];
        if (!current) return prev;
        return {
          ...prev,
          [selectedProjectId]: {
            ...current,
            fileTree: patchNode(current.fileTree, path, node => ({...node, loaded: true, children})),
          },
        };
      });
    } finally {
      setProjectStates(prev => {
        const current = prev[selectedProjectId];
        if (!current) return prev;
        const nextLoading = new Set(current.loadingDirs);
        nextLoading.delete(path);
        return {
          ...prev,
          [selectedProjectId]: {
            ...current,
            loadingDirs: nextLoading,
          },
        };
      });
    }
  };

  return {
    projectState,
    setChatInput,
    refreshProject,
    selectFile,
    toggleDirectory,
    selectCommit,
    selectDiffFile,
  };
}

function entryToNode(entry: RegistryFsEntry): FileNode {
  return {
    name: entry.name,
    path: entry.path,
    isDir: entry.kind === 'dir',
    loaded: entry.kind !== 'dir',
    children: [],
  };
}

function sortFileNode(a: FileNode, b: FileNode): number {
  if (a.isDir && !b.isDir) return -1;
  if (!a.isDir && b.isDir) return 1;
  return a.name.localeCompare(b.name);
}

function findFirstFile(root: FileNode): FileNode | null {
  const sorted = [...root.children].sort(sortFileNode);
  for (const node of sorted) {
    if (!node.isDir) return node;
  }
  return null;
}

function findNode(root: FileNode, path: string): FileNode | null {
  if (root.path === path) return root;
  for (const child of root.children) {
    const found = findNode(child, path);
    if (found) return found;
  }
  return null;
}

function patchNode(root: FileNode, path: string, updater: (node: FileNode) => FileNode): FileNode {
  if (root.path === path) return updater(root);
  return {
    ...root,
    children: root.children.map(child => patchNode(child, path, updater)),
  };
}
