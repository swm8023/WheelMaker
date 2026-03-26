import React, {useEffect, useRef, useState} from 'react';
import {
  Animated,
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
import {resolveTheme, type ThemeMode} from '../theme';
import type {RegistryFsEntry, RegistryProject} from '../types/observe';
import {isMarkdownPath} from '../utils/codeLanguage';
import {iconForPath} from '../utils/fileIcon';

type WorkspaceTab = 'chat' | 'file' | 'git';

type FileNode = {
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
  onListDirectory: (path: string) => Promise<RegistryFsEntry[]>;
  onReadFile: (path: string) => Promise<string>;
  onLogout: () => void;
  themeMode: ThemeMode;
  onThemeModeChange: (mode: ThemeMode) => void;
};

export function WorkspaceScreen({
  projects,
  selectedProjectId,
  fileEntries,
  onSelectProject,
  onListDirectory,
  onReadFile,
  onLogout,
  themeMode,
  onThemeModeChange,
}: WorkspaceScreenProps) {
  const {width} = useWindowDimensions();
  const isWide = width >= 900;
  const compact = width < 560;
  const drawerWidth = Math.min(320, Math.floor(width * 0.88));

  const theme = resolveTheme(themeMode);

  const [tab, setTab] = useState<WorkspaceTab>('chat');
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [projectMenuOpen, setProjectMenuOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [chatSessionIndex] = useState(0);
  const [chatInput, setChatInput] = useState('');
  const [selectedCommitIndex, setSelectedCommitIndex] = useState(0);
  const [selectedDiffFilePath, setSelectedDiffFilePath] = useState(
    GIT_COMMITS[0].files[0].path,
  );
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(null);
  const [selectedFileContent, setSelectedFileContent] = useState('');
  const [loadingProject, setLoadingProject] = useState(false);
  const [loadingFilePath, setLoadingFilePath] = useState('');
  const [loadingDirs, setLoadingDirs] = useState<Set<string>>(new Set());
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set(['.']));
  const drawerProgress = useRef(new Animated.Value(0)).current;
  const [fileTree, setFileTree] = useState<FileNode>(() => ({
    name: 'Project',
    path: '.',
    isDir: true,
    loaded: true,
    children: [],
  }));

  const selectedProject =
    projects.find(item => item.projectId === selectedProjectId) ?? projects[0];
  const selectedCommit = GIT_COMMITS[selectedCommitIndex];
  const selectedDiffFile = selectedCommit.files.find(
    file => file.path === selectedDiffFilePath,
  );

  const openDrawer = () => {
    setDrawerVisible(true);
    Animated.timing(drawerProgress, {
      toValue: 1,
      duration: 180,
      useNativeDriver: true,
    }).start();
  };

  const closeDrawer = () => {
    Animated.timing(drawerProgress, {
      toValue: 0,
      duration: 160,
      useNativeDriver: true,
    }).start(({finished}) => {
      if (finished) {
        setDrawerVisible(false);
      }
    });
  };

  useEffect(() => {
    const root: FileNode = {
      name: selectedProject?.name ?? 'Project',
      path: '.',
      isDir: true,
      loaded: true,
      children: fileEntries.map(entryToNode).sort(sortFileNode),
    };
    setFileTree(root);
    setExpandedPaths(new Set(['.']));

    const first = findFirstFile(root);
    if (first) {
      setSelectedFilePath(first.path);
    } else {
      setSelectedFilePath(null);
      setSelectedFileContent('');
    }
  }, [selectedProject?.name, fileEntries]);

  useEffect(() => {
    if (isWide) {
      setDrawerVisible(false);
      drawerProgress.setValue(0);
    }
  }, [drawerProgress, isWide]);

  useEffect(() => {
    if (!selectedFilePath) {
      return;
    }

    let cancelled = false;
    const load = async () => {
      setLoadingFilePath(selectedFilePath);
      try {
        const content = await onReadFile(selectedFilePath);
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
  }, [selectedFilePath, onReadFile]);

  const switchProject = async (projectId: string) => {
    setLoadingProject(true);
    try {
      await onSelectProject(projectId);
      setProjectMenuOpen(false);
    } finally {
      setLoadingProject(false);
    }
    if (!isWide) {
      closeDrawer();
    }
  };

  const toggleDirectory = async (path: string) => {
    let shouldExpand = false;
    setExpandedPaths(prev => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
        shouldExpand = true;
      }
      return next;
    });

    if (!shouldExpand) {
      return;
    }

    const target = findNode(fileTree, path);
    if (!target || !target.isDir || target.loaded) {
      return;
    }

    setLoadingDirs(prev => new Set(prev).add(path));
    try {
      const entries = await onListDirectory(path);
      const children = entries.map(entryToNode).sort(sortFileNode);
      setFileTree(prev => patchNode(prev, path, node => ({...node, loaded: true, children})));
    } finally {
      setLoadingDirs(prev => {
        const next = new Set(prev);
        next.delete(path);
        return next;
      });
    }
  };

  const leftPanel = (
    <Sidebar
      theme={theme}
      tab={tab}
      tree={fileTree}
      expandedPaths={expandedPaths}
      loadingDirs={loadingDirs}
      selectedFilePath={selectedFilePath}
      onToggleDirectory={toggleDirectory}
      onFileSelect={async path => {
        setSelectedFilePath(path);
        if (!isWide) {
          closeDrawer();
        }
      }}
      selectedCommitIndex={selectedCommitIndex}
      onCommitSelect={index => {
        setSelectedCommitIndex(index);
        setSelectedDiffFilePath(GIT_COMMITS[index]?.files[0]?.path ?? '');
      }}
      selectedDiffFilePath={selectedDiffFilePath}
      onDiffFileSelect={path => {
        setSelectedDiffFilePath(path);
        if (!isWide) {
          closeDrawer();
        }
      }}
    />
  );

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
                openDrawer();
              }
            }}
            style={[styles.headerButton, {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary}]}> 
            <Text style={{color: theme.colors.text}}>{isWide ? (sidebarCollapsed ? '>' : '<') : '='}</Text>
          </Pressable>

          <Pressable
            onPress={() => setProjectMenuOpen(value => !value)}
            style={[
              styles.projectButton,
              {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
            ]}>
            <Text style={[styles.projectArrow, {color: theme.colors.text}]}>v</Text>
            <Text style={{color: theme.colors.text}} numberOfLines={1}>
              {selectedProject?.name ?? 'Project'}
              {loadingProject ? ' ...' : ''}
            </Text>
          </Pressable>

          <View style={styles.headerSpacer} />

          <View style={[styles.segmentWrap, {borderColor: theme.colors.border}]}> 
            {(['chat', 'file', 'git'] as WorkspaceTab[]).map((item, index, arr) => (
              <Pressable
                key={item}
                style={[
                  styles.segmentItem,
                  {borderColor: theme.colors.border},
                  index === arr.length - 1 && styles.segmentItemLast,
                  tab === item && {backgroundColor: theme.colors.rowSelected},
                ]}
                onPress={() => setTab(item)}>
                <Text style={{color: theme.colors.text}}>{compact ? item[0].toUpperCase() : item.toUpperCase()}</Text>
              </Pressable>
            ))}
          </View>

          <Pressable
            onPress={() => setSettingsOpen(true)}
            style={[
              styles.headerButton,
              styles.settingsButton,
              {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
            ]}>
            <Text style={{color: theme.colors.text}}>[]</Text>
          </Pressable>
        </View>

        {projectMenuOpen ? (
          <View
            style={[
              styles.projectMenu,
              {borderColor: theme.colors.border, backgroundColor: theme.colors.panel},
            ]}>
            {projects.map(project => (
              <Pressable
                key={project.projectId}
                style={[
                  styles.projectMenuItem,
                  project.projectId === selectedProjectId && {
                    backgroundColor: theme.colors.rowSelected,
                  },
                ]}
                onPress={() => {
                  switchProject(project.projectId).catch(() => undefined);
                }}>
                <Text style={{color: theme.colors.text}}>{project.name}</Text>
              </Pressable>
            ))}
          </View>
        ) : null}

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
                <ScrollView style={styles.scrollArea} contentContainerStyle={styles.mainScrollPad}>
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
                <ScrollView style={styles.scrollArea} contentContainerStyle={styles.mainScrollPad}>
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
                <ScrollView style={styles.scrollArea} contentContainerStyle={styles.mainScrollPad}>
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
          <Modal visible={drawerVisible} animationType="none" transparent onRequestClose={closeDrawer}>
            <View style={styles.drawerHost}>
              <Pressable style={styles.drawerOverlay} onPress={closeDrawer}>
                <Animated.View style={[styles.drawerMask, {opacity: drawerProgress}]} />
              </Pressable>
              <Animated.View
                style={[
                  styles.drawer,
                  {
                    backgroundColor: theme.colors.panel,
                    width: drawerWidth,
                    transform: [
                      {
                        translateX: drawerProgress.interpolate({
                          inputRange: [0, 1],
                          outputRange: [-drawerWidth, 0],
                        }),
                      },
                    ],
                  },
                ]}> 
                <Pressable>
                  <View style={styles.drawerInner}>{leftPanel}</View>
                </Pressable>
              </Animated.View>
            </View>
          </Modal>
        ) : null}

        <Modal visible={settingsOpen} animationType="slide">
          <SafeAreaView style={[styles.safeArea, {backgroundColor: theme.colors.background}]}> 
            <View style={[styles.settingsHeader, {borderColor: theme.colors.border}]}> 
              <Text style={[styles.settingsTitle, {color: theme.colors.text}]}>Settings</Text>
              <Pressable
                style={[
                  styles.headerButton,
                  {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
                ]}
                onPress={() => setSettingsOpen(false)}>
                <Text style={{color: theme.colors.text}}>X</Text>
              </Pressable>
            </View>
            <View style={styles.settingsSection}>
              <Text style={[styles.sideTitle, {color: theme.colors.textMuted}]}>THEME</Text>
              {(['dark', 'light'] as ThemeMode[]).map(mode => (
                <Pressable
                  key={mode}
                  style={[
                    styles.settingsItem,
                    {
                      borderColor: theme.colors.border,
                      backgroundColor:
                        themeMode === mode ? theme.colors.rowSelected : theme.colors.panelSecondary,
                    },
                  ]}
                  onPress={() => onThemeModeChange(mode)}>
                  <Text style={{color: theme.colors.text}}>{mode.toUpperCase()}</Text>
                </Pressable>
              ))}
              <Text style={[styles.sideTitle, {color: theme.colors.textMuted}]}>SESSION</Text>
              <Pressable
                style={[
                  styles.settingsItem,
                  {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
                ]}
                onPress={onLogout}>
                <Text style={{color: theme.colors.text}}>Back To Login</Text>
              </Pressable>
            </View>
          </SafeAreaView>
        </Modal>
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

function Sidebar(args: {
  theme: ReturnType<typeof resolveTheme>;
  tab: WorkspaceTab;
  tree: FileNode;
  expandedPaths: Set<string>;
  loadingDirs: Set<string>;
  selectedFilePath: string | null;
  onToggleDirectory: (path: string) => Promise<void>;
  onFileSelect: (path: string) => void;
  selectedCommitIndex: number;
  onCommitSelect: (index: number) => void;
  selectedDiffFilePath: string;
  onDiffFileSelect: (path: string) => void;
}) {
  if (args.tab === 'chat') {
    return (
      <ScrollView style={styles.sideContainer} contentContainerStyle={styles.sideScrollPad}>
        <Text style={[styles.sideTitle, {color: args.theme.colors.textMuted}]}>CHAT LIST</Text>
        {CHAT_SESSIONS.map(item => (
          <View key={item} style={styles.sideRow}>
            <Text style={{color: args.theme.colors.text}}>{item}</Text>
          </View>
        ))}
      </ScrollView>
    );
  }

  if (args.tab === 'file') {
    return (
      <ScrollView style={styles.sideContainer} contentContainerStyle={styles.sideScrollPad}>
        <Text style={[styles.sideTitle, {color: args.theme.colors.textMuted}]}>EXPLORER</Text>
        {renderFileTree({
          node: args.tree,
          depth: 0,
          theme: args.theme,
          expandedPaths: args.expandedPaths,
          loadingDirs: args.loadingDirs,
          selectedFilePath: args.selectedFilePath,
          onToggleDirectory: args.onToggleDirectory,
          onFileSelect: args.onFileSelect,
        })}
      </ScrollView>
    );
  }

  return (
    <View style={styles.sideContainer}>
      <View style={styles.gitHalf}>
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
      </View>
      <View style={[styles.gitDivider, {backgroundColor: args.theme.colors.border}]} />
      <View style={styles.gitHalf}>
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
    </View>
  );
}

function renderFileTree(args: {
  node: FileNode;
  depth: number;
  theme: ReturnType<typeof resolveTheme>;
  expandedPaths: Set<string>;
  loadingDirs: Set<string>;
  selectedFilePath: string | null;
  onToggleDirectory: (path: string) => Promise<void>;
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
  const isLoading = args.loadingDirs.has(args.node.path);
  const sortedChildren = [...args.node.children].sort(sortFileNode);

  return (
    <View key={args.node.path}>
      <Pressable
        style={[styles.sideRow, indent]}
        onPress={() => {
          args.onToggleDirectory(args.node.path).catch(() => undefined);
        }}>
        <Text style={{color: args.theme.colors.text}}>
          {isOpen ? 'v ' : '> '}
          <Text style={{color: icon.color}}>{icon.glyph} </Text>
          {args.node.name}
          {isLoading ? ' ...' : ''}
        </Text>
      </Pressable>
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
  if (a.isDir && !b.isDir) {
    return -1;
  }
  if (!a.isDir && b.isDir) {
    return 1;
  }
  return a.name.localeCompare(b.name);
}

function findNode(root: FileNode, path: string): FileNode | null {
  if (root.path === path) {
    return root;
  }
  for (const child of root.children) {
    const found = findNode(child, path);
    if (found) {
      return found;
    }
  }
  return null;
}

function patchNode(root: FileNode, path: string, updater: (node: FileNode) => FileNode): FileNode {
  if (root.path === path) {
    return updater(root);
  }
  return {
    ...root,
    children: root.children.map(child => patchNode(child, path, updater)),
  };
}

function findFirstFile(root: FileNode): FileNode | null {
  const sorted = [...root.children].sort(sortFileNode);
  for (const node of sorted) {
    if (!node.isDir) {
      return node;
    }
  }
  return null;
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
  projectButton: {
    height: 32,
    maxWidth: 260,
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1,
    borderRadius: 6,
    marginLeft: 8,
    paddingHorizontal: 10,
    flexDirection: 'row',
  },
  headerSpacer: {
    flex: 1,
  },
  settingsButton: {
    marginLeft: 8,
  },
  projectArrow: {
    marginRight: 6,
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
  segmentItemLast: {
    borderRightWidth: 0,
  },
  projectMenu: {
    position: 'absolute',
    top: 52,
    left: 48,
    width: 260,
    borderWidth: 1,
    zIndex: 20,
  },
  projectMenuItem: {
    minHeight: 36,
    justifyContent: 'center',
    paddingHorizontal: 10,
  },
  container: {
    flex: 1,
    flexDirection: 'row',
    minHeight: 0,
    minWidth: 0,
  },
  sidebar: {
    width: 320,
    minHeight: 0,
    overflow: 'hidden',
  },
  divider: {
    width: 1,
  },
  mainPane: {
    flex: 1,
    minHeight: 0,
    minWidth: 0,
    alignSelf: 'stretch',
    overflow: 'hidden',
  },
  sideContainer: {
    flex: 1,
    minHeight: 0,
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
    minHeight: 0,
    minWidth: 0,
  },
  blockTitle: {
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontWeight: '600',
    borderBottomWidth: 1,
  },
  scrollArea: {
    flex: 1,
    minHeight: 0,
    width: '100%',
    alignSelf: 'stretch',
  },
  sideScrollPad: {
    padding: 12,
    flexGrow: 1,
  },
  mainScrollPad: {
    padding: 12,
    flexGrow: 1,
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
  drawerHost: {
    flex: 1,
  },
  drawerOverlay: {
    ...StyleSheet.absoluteFillObject,
  },
  drawerMask: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: 'rgba(0,0,0,0.32)',
  },
  drawer: {
    position: 'absolute',
    left: 0,
    top: 0,
    height: '100%',
    minHeight: 0,
  },
  drawerInner: {
    flex: 1,
    minHeight: 0,
  },
  settingsHeader: {
    height: 52,
    borderBottomWidth: 1,
    paddingHorizontal: 10,
    alignItems: 'center',
    justifyContent: 'space-between',
    flexDirection: 'row',
  },
  settingsSection: {
    paddingHorizontal: 12,
    paddingTop: 12,
  },
  settingsTitle: {
    fontSize: 18,
    fontWeight: '600',
  },
  settingsItem: {
    borderWidth: 1,
    borderRadius: 6,
    minHeight: 40,
    justifyContent: 'center',
    paddingHorizontal: 10,
    marginBottom: 8,
  },
  flexOne: {
    flex: 1,
    minHeight: 0,
  },
  gitHalf: {
    flex: 1,
    minHeight: 0,
  },
  gitDivider: {
    height: 1,
  },
});






