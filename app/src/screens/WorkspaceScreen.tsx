import React, {useEffect, useMemo, useState} from 'react';
import {
  Modal,
  Pressable,
  ScrollView,
  StatusBar,
  StyleSheet,
  Text,
  TextInput,
  useWindowDimensions,
  View,
} from 'react-native';
import {SafeAreaProvider, SafeAreaView} from 'react-native-safe-area-context';

import {CodeView, MarkdownView} from '../components';
import {nextThemeMode, resolveTheme, type ThemeMode} from '../theme';
import type {RegistryFsEntry, RegistryProject} from '../types/observe';
import {isMarkdownPath} from '../utils/codeLanguage';
import {iconForPath} from '../utils/fileIcon';

type WorkspaceTab = 'chat' | 'file' | 'git';

type FileNode = {
  name: string;
  path: string;
  isDir: boolean;
  content?: string;
  children?: FileNode[];
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

const CHAT_SESSIONS = ['General', 'WheelMaker App', 'Go Service', 'Review'];
const CHAT_MESSAGES = [
  {role: 'system', text: 'Connected to registry workspace.'},
  {role: 'agent', text: 'Workspace theme now supports VS Code style modes.'},
];

const GIT_COMMITS: GitCommit[] = [
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

type WorkspaceScreenProps = {
  projects: RegistryProject[];
  selectedProjectId: string;
  fileEntries: RegistryFsEntry[];
  onSelectProject: (projectId: string) => Promise<void>;
  onReadFile: (path: string) => Promise<string>;
};

export function WorkspaceScreen({
  projects,
  selectedProjectId,
  fileEntries,
  onSelectProject,
  onReadFile,
}: WorkspaceScreenProps) {
  const {width} = useWindowDimensions();
  const isWide = width >= 900;
  const compact = width < 560;

  const [themeMode, setThemeMode] = useState<ThemeMode>('dark');
  const theme = resolveTheme(themeMode);

  const [tab, setTab] = useState<WorkspaceTab>('chat');
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [chatSessionIndex] = useState(0);
  const [chatInput, setChatInput] = useState('');
  const [selectedCommitIndex, setSelectedCommitIndex] = useState(0);
  const [selectedDiffFilePath, setSelectedDiffFilePath] = useState(
    GIT_COMMITS[0].files[0].path,
  );
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(
    fileEntries.find(entry => entry.kind === 'file')?.path ?? fileEntries[0]?.path ?? null,
  );
  const [selectedFileContent, setSelectedFileContent] = useState('');
  const [loadingProject, setLoadingProject] = useState(false);
  const [loadingFilePath, setLoadingFilePath] = useState('');

  useEffect(() => {
    const firstFile = fileEntries.find(entry => entry.kind === 'file')?.path;
    if (firstFile && firstFile !== selectedFilePath) {
      setSelectedFilePath(firstFile);
    }
  }, [fileEntries, selectedFilePath]);

  useEffect(() => {
    const firstFile = fileEntries.find(entry => entry.kind === 'file')?.path;
    if (!firstFile) {
      setSelectedFileContent('');
      return;
    }

    let cancelled = false;
    const load = async () => {
      setLoadingFilePath(firstFile);
      try {
        const content = await onReadFile(firstFile);
        if (!cancelled) {
          setSelectedFileContent(content);
        }
      } finally {
        if (!cancelled) {
          setLoadingFilePath('');
        }
      }
    };
    load().catch(() => undefined);

    return () => {
      cancelled = true;
    };
  }, [fileEntries, onReadFile]);

  const selectedCommit = GIT_COMMITS[selectedCommitIndex];
  const selectedDiffFile = selectedCommit.files.find(
    file => file.path === selectedDiffFilePath,
  );

  const fileTree = useMemo(() => {
    const project = projects.find(item => item.projectId === selectedProjectId);
    return buildFileTree(project, fileEntries);
  }, [projects, selectedProjectId, fileEntries]);

  const expandedPaths = useMemo(() => {
    return new Set([fileTree.path]);
  }, [fileTree.path]);

  const leftPanel = renderSidebar({
    theme,
    tab,
    projects,
    selectedProjectId,
    onProjectSelect: async projectId => {
      setLoadingProject(true);
      try {
        await onSelectProject(projectId);
      } finally {
        setLoadingProject(false);
      }
      if (!isWide) {
        setDrawerOpen(false);
      }
    },
    expandedPaths,
    selectedFilePath,
    onFileSelect: async path => {
      setSelectedFilePath(path);
      const entry = fileEntries.find(item => item.path === path);
      if (entry?.kind === 'file') {
        setLoadingFilePath(path);
        try {
          setSelectedFileContent(await onReadFile(path));
        } finally {
          setLoadingFilePath('');
        }
      }
      if (!isWide) {
        setDrawerOpen(false);
      }
    },
    selectedCommitIndex,
    onCommitSelect: index => {
      setSelectedCommitIndex(index);
      setSelectedDiffFilePath(GIT_COMMITS[index]?.files[0]?.path ?? '');
    },
    selectedDiffFilePath,
    onDiffFileSelect: path => {
      setSelectedDiffFilePath(path);
      if (!isWide) {
        setDrawerOpen(false);
      }
    },
    fileTree,
  });

  return (
    <SafeAreaProvider>
      <StatusBar barStyle={themeMode === 'dark' ? 'light-content' : 'dark-content'} />
      <SafeAreaView style={[styles.safeArea, {backgroundColor: theme.colors.background}]}> 
        <View style={[styles.header, {borderColor: theme.colors.border, backgroundColor: theme.colors.panel}]}> 
          <Pressable
            onPress={() => {
              if (isWide) {
                setSidebarCollapsed(value => !value);
              } else {
                setDrawerOpen(true);
              }
            }}
            style={[styles.headerButton, {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary}]}> 
            <Text style={{color: theme.colors.text}}>{isWide ? (sidebarCollapsed ? '>' : '<') : '='}</Text>
          </Pressable>
          <Text style={[styles.headerTitle, {color: theme.colors.text}]} numberOfLines={1}>
            WheelMaker Project
          </Text>
          <Pressable
            onPress={() => setThemeMode(mode => nextThemeMode(mode))}
            style={[styles.themeButton, {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary}]}> 
            <Text style={{color: theme.colors.text}}>{themeMode === 'dark' ? 'Dark' : 'Light'}</Text>
          </Pressable>
          <View style={[styles.segmentWrap, {borderColor: theme.colors.border}]}> 
            {(['chat', 'file', 'git'] as WorkspaceTab[]).map(item => (
              <Pressable
                key={item}
                style={[
                  styles.segmentItem,
                  {borderColor: theme.colors.border},
                  tab === item && {backgroundColor: theme.colors.rowSelected},
                ]}
                onPress={() => setTab(item)}>
                <Text style={{color: theme.colors.text}}>{compact ? item[0].toUpperCase() : item.toUpperCase()}</Text>
              </Pressable>
            ))}
          </View>
        </View>

        <View style={styles.container}>
          {isWide && !sidebarCollapsed ? (
            <View style={[styles.sidebar, {backgroundColor: theme.colors.panel}]}>{leftPanel}</View>
          ) : null}
          {isWide && !sidebarCollapsed ? (
            <View style={[styles.divider, {backgroundColor: theme.colors.border}]} />
          ) : null}

          <View style={[styles.mainPane, {backgroundColor: theme.colors.background}]}> 
            {tab === 'chat' ? (
              <View style={styles.mainBlock}>
                <Text style={[styles.blockTitle, {borderColor: theme.colors.border, color: theme.colors.text}]}> 
                  CHAT - {CHAT_SESSIONS[chatSessionIndex]}
                </Text>
                <ScrollView style={styles.scrollArea}>
                  {CHAT_MESSAGES.map((msg, idx) => (
                    <View
                      key={idx}
                      style={[
                        styles.chatBubble,
                        {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
                      ]}>
                      <Text style={{color: theme.colors.text}}>
                        [{msg.role}] {msg.text}
                      </Text>
                    </View>
                  ))}
                </ScrollView>
                <View style={[styles.inputRow, {borderColor: theme.colors.border}]}> 
                  <TextInput
                    value={chatInput}
                    onChangeText={setChatInput}
                    placeholder="Message..."
                    placeholderTextColor={theme.colors.textMuted}
                    style={[
                      styles.input,
                      {
                        borderColor: theme.colors.border,
                        color: theme.colors.text,
                        backgroundColor: theme.colors.inputBackground,
                      },
                    ]}
                  />
                  <Pressable
                    style={[
                      styles.sendButton,
                      {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
                    ]}
                    onPress={() => setChatInput('')}>
                    <Text style={{color: theme.colors.text}}>Send</Text>
                  </Pressable>
                </View>
              </View>
            ) : null}

            {tab === 'file' ? (
              <View style={styles.mainBlock}>
                <Text style={[styles.blockTitle, {borderColor: theme.colors.border, color: theme.colors.text}]}> 
                  {selectedFilePath ?? 'Select a file'}
                  {loadingProject ? ' (loading project...)' : ''}
                </Text>
                <ScrollView style={styles.scrollArea}>
                  {loadingFilePath ? (
                    <Text style={{color: theme.colors.textMuted}}>Loading file...</Text>
                  ) : selectedFilePath && isMarkdownPath(selectedFilePath) ? (
                    <MarkdownView content={selectedFileContent} theme={theme} />
                  ) : (
                    <CodeView path={selectedFilePath ?? 'file.txt'} code={selectedFileContent} theme={theme} />
                  )}
                </ScrollView>
              </View>
            ) : null}

            {tab === 'git' ? (
              <View style={styles.mainBlock}>
                <Text style={[styles.blockTitle, {borderColor: theme.colors.border, color: theme.colors.text}]}> 
                  {selectedDiffFile?.path ?? 'Select a changed file'}
                </Text>
                <ScrollView style={styles.scrollArea}>
                  <CodeView
                    path={selectedDiffFile?.path ?? 'diff.txt'}
                    code={selectedDiffFile?.diff ?? ''}
                    theme={theme}
                  />
                </ScrollView>
              </View>
            ) : null}
          </View>
        </View>

        {!isWide ? (
          <Modal visible={drawerOpen} animationType="slide" transparent>
            <Pressable style={styles.drawerMask} onPress={() => setDrawerOpen(false)}>
              <View style={[styles.drawer, {backgroundColor: theme.colors.panel}]}> 
                <Pressable>
                  <View style={styles.drawerInner}>{leftPanel}</View>
                </Pressable>
              </View>
            </Pressable>
          </Modal>
        ) : null}
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

function renderSidebar(args: {
  theme: ReturnType<typeof resolveTheme>;
  tab: WorkspaceTab;
  projects: RegistryProject[];
  selectedProjectId: string;
  onProjectSelect: (projectId: string) => void;
  expandedPaths: Set<string>;
  selectedFilePath: string | null;
  onFileSelect: (path: string) => void;
  selectedCommitIndex: number;
  onCommitSelect: (index: number) => void;
  selectedDiffFilePath: string;
  onDiffFileSelect: (path: string) => void;
  fileTree: FileNode;
}) {
  if (args.tab === 'chat') {
    return (
      <View style={styles.sideContainer}>
        <Text style={[styles.sideTitle, {color: args.theme.colors.textMuted}]}>PROJECTS</Text>
        {args.projects.map(project => (
          <Pressable
            key={project.projectId}
            style={[
              styles.sideRow,
              args.selectedProjectId === project.projectId && {
                backgroundColor: args.theme.colors.rowSelected,
              },
            ]}
            onPress={() => args.onProjectSelect(project.projectId)}>
            <Text style={{color: args.theme.colors.text}}>{project.name}</Text>
          </Pressable>
        ))}
        <Text style={[styles.sideTitle, {color: args.theme.colors.textMuted}]}>CHAT LIST</Text>
        {CHAT_SESSIONS.map(item => (
          <View key={item} style={styles.sideRow}>
            <Text style={{color: args.theme.colors.text}}>{item}</Text>
          </View>
        ))}
      </View>
    );
  }

  if (args.tab === 'file') {
    return (
      <ScrollView style={styles.sideContainer}>
        <Text style={[styles.sideTitle, {color: args.theme.colors.textMuted}]}>PROJECTS</Text>
        {args.projects.map(project => (
          <Pressable
            key={project.projectId}
            style={[
              styles.sideRow,
              args.selectedProjectId === project.projectId && {
                backgroundColor: args.theme.colors.rowSelected,
              },
            ]}
            onPress={() => args.onProjectSelect(project.projectId)}>
            <Text style={{color: args.theme.colors.text}}>{project.name}</Text>
          </Pressable>
        ))}
        <Text style={[styles.sideTitle, {color: args.theme.colors.textMuted}]}>EXPLORER</Text>
        {renderFileTree({
          node: args.fileTree,
          depth: 0,
          theme: args.theme,
          expandedPaths: args.expandedPaths,
          selectedFilePath: args.selectedFilePath,
          onFileSelect: args.onFileSelect,
        })}
      </ScrollView>
    );
  }

  return (
    <View style={styles.sideContainer}>
      <Text style={[styles.sideTitle, {color: args.theme.colors.textMuted}]}>COMMITS</Text>
      <ScrollView style={styles.flexOne}>
        {GIT_COMMITS.map((commit, index) => (
          <Pressable
            key={commit.hash}
            style={[
              styles.sideRow,
              index === args.selectedCommitIndex && {
                backgroundColor: args.theme.colors.rowSelected,
              },
            ]}
            onPress={() => args.onCommitSelect(index)}>
            <Text numberOfLines={2} style={{color: args.theme.colors.text}}>
              {commit.hash} {commit.message}
            </Text>
          </Pressable>
        ))}
      </ScrollView>
      <Text style={[styles.sideTitle, {color: args.theme.colors.textMuted}]}>CHANGED FILES</Text>
      <ScrollView style={styles.flexOne}>
        {GIT_COMMITS[args.selectedCommitIndex].files.map(file => {
          const icon = iconForPath(file.path);
          return (
            <Pressable
              key={file.path}
              style={[
                styles.sideRow,
                file.path === args.selectedDiffFilePath && {
                  backgroundColor: args.theme.colors.rowSelected,
                },
              ]}
              onPress={() => args.onDiffFileSelect(file.path)}>
              <Text numberOfLines={1} style={{color: args.theme.colors.text}}>
                <Text style={{color: icon.color}}>{icon.glyph} </Text>
                {file.path}
              </Text>
            </Pressable>
          );
        })}
      </ScrollView>
    </View>
  );
}

function renderFileTree(args: {
  node: FileNode;
  depth: number;
  theme: ReturnType<typeof resolveTheme>;
  expandedPaths: Set<string>;
  selectedFilePath: string | null;
  onFileSelect: (path: string) => void;
}): React.ReactNode {
  const indent = {paddingLeft: args.depth * 14 + 8};
  const icon = iconForPath(args.node.path);

  if (!args.node.isDir) {
    return (
      <Pressable
        key={args.node.path}
        style={[
          styles.sideRow,
          indent,
          args.node.path === args.selectedFilePath && {
            backgroundColor: args.theme.colors.rowSelected,
          },
        ]}
        onPress={() => args.onFileSelect(args.node.path)}>
        <Text style={{color: args.theme.colors.text}}>
          <Text style={{color: icon.color}}>{icon.glyph} </Text>
          {args.node.name}
        </Text>
      </Pressable>
    );
  }

  const isOpen = args.expandedPaths.has(args.node.path);
  const sortedChildren = [...(args.node.children ?? [])].sort(sortFileNode);
  return (
    <View key={args.node.path}>
      <View style={[styles.sideRow, indent]}>
        <Text style={{color: args.theme.colors.text}}>
          <Text style={{color: icon.color}}>{icon.glyph} </Text>
          {isOpen ? 'v ' : '> '}
          {args.node.name}
        </Text>
      </View>
      {isOpen
        ? sortedChildren.map(child =>
            renderFileTree({
              ...args,
              node: child,
              depth: args.depth + 1,
            }),
          )
        : null}
    </View>
  );
}

function sortFileNode(a: FileNode, b: FileNode): number {
  if (a.isDir && !b.isDir) {
    return -1;
  }
  if (!a.isDir && b.isDir) {
    return 1;
  }
  return a.name.localeCompare(b.name);
}

function buildFileTree(
  project: RegistryProject | undefined,
  entries: RegistryFsEntry[],
): FileNode {
  const projectName = project?.name || 'Project';
  const projectPath = project?.projectId || 'project';
  return {
    name: projectName,
    path: projectPath,
    isDir: true,
    children: entries.map(entry => ({
      name: entry.name,
      path: entry.path,
      isDir: entry.kind === 'dir',
    })),
  };
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
  },
  header: {
    height: 52,
    borderBottomWidth: 1,
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 8,
  },
  headerButton: {
    width: 32,
    height: 32,
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1,
    borderRadius: 4,
  },
  themeButton: {
    minWidth: 56,
    height: 32,
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1,
    borderRadius: 4,
    marginRight: 8,
    paddingHorizontal: 8,
  },
  headerTitle: {
    flex: 1,
    marginLeft: 8,
    marginRight: 8,
    fontSize: 14,
  },
  segmentWrap: {
    flexDirection: 'row',
    borderWidth: 1,
    borderRadius: 6,
    overflow: 'hidden',
  },
  segmentItem: {
    paddingVertical: 6,
    paddingHorizontal: 10,
    borderRightWidth: 1,
  },
  container: {
    flex: 1,
    flexDirection: 'row',
  },
  sidebar: {
    width: 320,
  },
  divider: {
    width: 1,
  },
  mainPane: {
    flex: 1,
  },
  sideContainer: {
    flex: 1,
  },
  sideTitle: {
    paddingHorizontal: 10,
    paddingVertical: 10,
    fontWeight: '600',
    fontSize: 12,
  },
  sideRow: {
    minHeight: 32,
    justifyContent: 'center',
    paddingHorizontal: 10,
  },
  mainBlock: {
    flex: 1,
  },
  blockTitle: {
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontWeight: '600',
    borderBottomWidth: 1,
  },
  scrollArea: {
    flex: 1,
    padding: 12,
  },
  chatBubble: {
    padding: 8,
    marginBottom: 8,
    borderWidth: 1,
    borderRadius: 6,
  },
  inputRow: {
    flexDirection: 'row',
    borderTopWidth: 1,
    padding: 10,
  },
  input: {
    flex: 1,
    borderWidth: 1,
    borderRadius: 6,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  sendButton: {
    marginLeft: 8,
    borderWidth: 1,
    borderRadius: 6,
    paddingHorizontal: 12,
    alignItems: 'center',
    justifyContent: 'center',
  },
  drawerMask: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.25)',
  },
  drawer: {
    width: 320,
    height: '100%',
  },
  drawerInner: {
    flex: 1,
  },
  flexOne: {
    flex: 1,
  },
});

