import {useEffect, useMemo, useState} from 'react';

import type {RegistryFsEntry, RegistryProject} from '../types/observe';

export type WorkspaceTab = 'chat' | 'file' | 'git';

export type FileNode = {
  name: string;
  path: string;
  isDir: boolean;
  loaded: boolean;
  children: FileNode[];
};

type GitFile = {
  path: string;
  diff: string;
};

type GitCommit = {
  hash: string;
  message: string;
  files: GitFile[];
};

export const GIT_COMMITS: GitCommit[] = [
  {
    hash: '14b16e2',
    message: 'feat(app): implement registry connect, project list, and files',
    files: [
      {
        path: 'app/lib/services/registry_ws_client.dart',
        diff: '@@ -1,3 +1,4 @@\n+class RegistryWsClient { ... }',
      },
      {
        path: 'app/lib/screens/connect_screen.dart',
        diff: '@@ -40,2 +75,20 @@\n+Future<void> _openRegistryWorkspace() async { ... }',
      },
    ],
  },
];

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
  selectedCommitIndex: number;
  selectedDiffFilePath: string;
};

type UseWorkspaceDataArgs = {
  projects: RegistryProject[];
  selectedProjectId: string;
  fileEntries: RegistryFsEntry[];
  onListDirectory: (path: string) => Promise<RegistryFsEntry[]>;
  onReadFile: (path: string) => Promise<string>;
};

type UseWorkspaceDataResult = {
  projectState: ProjectWorkspaceState;
  setChatInput: (value: string) => void;
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
    selectedCommitIndex: 0,
    selectedDiffFilePath: GIT_COMMITS[0].files[0].path,
  };
}

export function useWorkspaceData(args: UseWorkspaceDataArgs): UseWorkspaceDataResult {
  const {projects, selectedProjectId: selectedProjectIdArg, fileEntries, onListDirectory, onReadFile} = args;
  const selectedProject =
    projects.find(item => item.projectId === selectedProjectIdArg) ?? projects[0];
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
      next[selectedProjectId] = initialProjectState(selectedProject?.name ?? 'Project', fileEntries);
      return next;
    });
  }, [selectedProjectId, selectedProject?.name, fileEntries]);

  const projectState = useMemo(() => {
    if (!selectedProjectId) {
      return initialProjectState('Project', []);
    }
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
      return {
        ...prev,
        [selectedProjectId]: {
          ...current,
          selectedCommitIndex: index,
          selectedDiffFilePath: GIT_COMMITS[index]?.files[0]?.path ?? '',
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
