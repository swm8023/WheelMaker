import React, { useEffect, useMemo, useState } from 'react';
import {
  Modal,
  Pressable,
  SafeAreaView,
  ScrollView,
  StatusBar,
  StyleSheet,
  Text,
  TextInput,
  useWindowDimensions,
  View,
} from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { createRegistryRepository } from './src/services';
import type { RegistryFsEntry, RegistryProject } from './src/types/observe';

type WorkspaceTab = 'chat' | 'file' | 'git';
type ThemeMode = 'light' | 'dark' | 'system';

type TreeNode = {
  name: string;
  path: string;
  isDir: boolean;
  loaded: boolean;
  children: TreeNode[];
};

type GitFile = { path: string; diff: string };
type GitCommit = { hash: string; message: string; files: GitFile[] };

const MOCK_PROJECTS: RegistryProject[] = [
  { projectId: 'wheelmaker', name: 'WheelMaker', online: true },
  { projectId: 'server', name: 'Server', online: true },
];

const CHAT_SESSIONS = ['General', 'WheelMaker App', 'Go Service', 'Review'];
const CHAT_MESSAGES = [
  { role: 'system', text: 'Registry workspace shell (React Native).' },
  { role: 'agent', text: 'Layout-first iteration. Data hooks are in place.' },
];

const GIT_COMMITS: GitCommit[] = [
  {
    hash: 'b37fa14',
    message: 'feat(app-rn): add adaptive chat git file workspace shell',
    files: [
      { path: 'app/App.tsx', diff: '@@ + layout skeleton ...' },
      { path: 'app/src/services/observeRepository.ts', diff: '@@ + registry request wrappers ...' },
    ],
  },
  {
    hash: '270ded2',
    message: 'refactor(app): deprecate flutter app and promote react native app',
    files: [{ path: 'app/package.json', diff: '@@ + react native scaffold ...' }],
  },
];

const MOCK_DIR_ENTRIES: Record<string, RegistryFsEntry[]> = {
  '.': [
    { name: 'app', path: 'app', kind: 'dir' },
    { name: 'docs', path: 'docs', kind: 'dir' },
    { name: 'server', path: 'server', kind: 'dir' },
    { name: 'README.md', path: 'README.md', kind: 'file' },
  ],
  app: [
    { name: 'src', path: 'app/src', kind: 'dir' },
    { name: 'App.tsx', path: 'app/App.tsx', kind: 'file' },
    { name: 'package.json', path: 'app/package.json', kind: 'file' },
  ],
  'app/src': [
    { name: 'services', path: 'app/src/services', kind: 'dir' },
    { name: 'types', path: 'app/src/types', kind: 'dir' },
    { name: 'README.md', path: 'app/src/README.md', kind: 'file' },
  ],
  'app/src/services': [
    {
      name: 'observeRepository.ts',
      path: 'app/src/services/observeRepository.ts',
      kind: 'file',
    },
    { name: 'observeClient.ts', path: 'app/src/services/observeClient.ts', kind: 'file' },
  ],
};

const MOCK_FILE_CONTENT: Record<string, string> = {
  'README.md': '# WheelMaker\n\nMonorepo for server and app clients.',
  'app/App.tsx': 'export default function App() { return null; }',
  'app/package.json': '{ "name": "WheelMakerRN" }',
  'app/src/README.md': '# App RN Foundation\n\nRegistry client bootstrap.',
  'app/src/services/observeRepository.ts':
    'export class RegistryRepository { /* project.list / fs.list / fs.read */ }',
  'app/src/services/observeClient.ts':
    'export class RegistryClient { /* request/response websocket */ }',
};

function App() {
  const { width } = useWindowDimensions();
  const isWide = width >= 920;

  const repository = useMemo(() => createRegistryRepository(), []);

  const [themeMode, setThemeMode] = useState<ThemeMode>('system');
  const [activeTab, setActiveTab] = useState<WorkspaceTab>('chat');
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [settingsVisible, setSettingsVisible] = useState(false);
  const [projectMenuVisible, setProjectMenuVisible] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  const [projects, setProjects] = useState<RegistryProject[]>(MOCK_PROJECTS);
  const [selectedProjectId, setSelectedProjectId] = useState(MOCK_PROJECTS[0].projectId);
  const [connected, setConnected] = useState(false);

  const [chatSessionIndex, setChatSessionIndex] = useState(0);
  const [chatInput, setChatInput] = useState('');

  const [fileRoot, setFileRoot] = useState<TreeNode | null>(null);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [loadingPaths, setLoadingPaths] = useState<Set<string>>(new Set());
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(null);
  const [selectedFileContent, setSelectedFileContent] = useState('');
  const [fileLoading, setFileLoading] = useState(false);

  const [selectedCommitIndex, setSelectedCommitIndex] = useState(0);
  const [selectedDiffFilePath, setSelectedDiffFilePath] = useState(GIT_COMMITS[0].files[0].path);

  const selectedProject = projects.find(p => p.projectId === selectedProjectId) ?? projects[0];
  const selectedCommit = GIT_COMMITS[selectedCommitIndex];
  const selectedDiffFile = selectedCommit.files.find(f => f.path === selectedDiffFilePath);
  const rootPath = selectedProject ? `/${selectedProject.projectId}` : '/project';

  useEffect(() => {
    let active = true;
    const bootstrap = async () => {
      try {
        await repository.initialize('ws://127.0.0.1:9527/ws');
        const projectList = await repository.listProjects();
        if (!active) return;
        if (projectList.length > 0) {
          setProjects(projectList);
          setSelectedProjectId(projectList[0].projectId);
        }
        setConnected(true);
      } catch {
        if (!active) return;
        setConnected(false);
      }
    };
    bootstrap().catch(() => undefined);
    return () => {
      active = false;
      repository.close();
    };
  }, [repository]);

  useEffect(() => {
    const root: TreeNode = {
      name: selectedProject?.name ?? 'Project',
      path: rootPath,
      isDir: true,
      loaded: false,
      children: [],
    };
    setFileRoot(root);
    setExpandedPaths(new Set([root.path]));
    setSelectedFilePath(null);
    setSelectedFileContent('');
    loadDirectory(root.path).catch(() => undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedProjectId, rootPath, selectedProject?.name]);

  useEffect(() => {
    if (isWide) setDrawerVisible(false);
  }, [isWide]);

  const onToggleSidebar = () => {
    if (isWide) {
      setSidebarCollapsed(v => !v);
    } else {
      setDrawerVisible(true);
    }
  };

  const onToggleDirectory = async (path: string) => {
    setExpandedPaths(prev => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
    await loadDirectory(path);
  };

  const onSelectFile = async (path: string) => {
    setSelectedFilePath(path);
    setFileLoading(true);
    try {
      const requestPath = projectRelativePath(path, selectedProjectId);
      const content = connected
        ? await repository.readFile(selectedProjectId, requestPath)
        : MOCK_FILE_CONTENT[requestPath] ?? '';
      setSelectedFileContent(content);
    } catch {
      setSelectedFileContent('');
    } finally {
      setFileLoading(false);
      if (!isWide) setDrawerVisible(false);
    }
  };

  const loadDirectory = async (nodePath: string) => {
    if (!fileRoot) return;
    const target = findNode(fileRoot, nodePath);
    if (!target || !target.isDir || target.loaded) return;

    setLoadingPaths(prev => new Set(prev).add(nodePath));
    try {
      const requestPath = projectRelativePath(nodePath, selectedProjectId);
      const entries = connected
        ? await repository.listFiles(selectedProjectId, requestPath)
        : MOCK_DIR_ENTRIES[requestPath] ?? [];
      const children = mapEntriesToNodes(entries, selectedProjectId).sort(sortNode);
      setFileRoot(prev =>
        prev ? patchNode(prev, nodePath, node => ({ ...node, loaded: true, children })) : prev,
      );
    } catch {
      setFileRoot(prev =>
        prev ? patchNode(prev, nodePath, node => ({ ...node, loaded: true, children: [] })) : prev,
      );
    } finally {
      setLoadingPaths(prev => {
        const next = new Set(prev);
        next.delete(nodePath);
        return next;
      });
    }
  };

  const sidebar = (
    <WorkspaceSidebar
      activeTab={activeTab}
      chatSessionIndex={chatSessionIndex}
      onChatSessionSelect={index => {
        setChatSessionIndex(index);
        if (!isWide) setDrawerVisible(false);
      }}
      fileRoot={fileRoot}
      expandedPaths={expandedPaths}
      loadingPaths={loadingPaths}
      selectedFilePath={selectedFilePath}
      onToggleDirectory={onToggleDirectory}
      onSelectFile={onSelectFile}
      selectedCommitIndex={selectedCommitIndex}
      onSelectCommit={index => {
        setSelectedCommitIndex(index);
        const firstFile = GIT_COMMITS[index]?.files[0]?.path ?? '';
        setSelectedDiffFilePath(firstFile);
      }}
      selectedDiffFilePath={selectedDiffFilePath}
      onSelectDiffFile={path => {
        setSelectedDiffFilePath(path);
        if (!isWide) setDrawerVisible(false);
      }}
    />
  );

  return (
    <SafeAreaProvider>
      <StatusBar barStyle="dark-content" />
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.topBar}>
          <Pressable style={styles.iconButton} onPress={onToggleSidebar}>
            <Text style={styles.iconButtonText}>{isWide ? (sidebarCollapsed ? '|<' : '>|') : '|||'} </Text>
          </Pressable>

          <Pressable style={styles.projectButton} onPress={() => setProjectMenuVisible(v => !v)}>
            <Text style={styles.projectArrow}>v</Text>
            <Text numberOfLines={1} style={styles.projectText}>
              {selectedProject?.name ?? 'Project'}
            </Text>
          </Pressable>

          <View style={styles.topBarSpacer} />

          <View style={styles.tabSwitch}>
            {(['chat', 'file', 'git'] as WorkspaceTab[]).map((tab, index, arr) => (
              <Pressable
                key={tab}
                style={[
                  styles.tabSwitchItem,
                  index === arr.length - 1 && styles.tabSwitchItemLast,
                  activeTab === tab && styles.tabSwitchItemActive,
                ]}
                onPress={() => setActiveTab(tab)}>
                <Text style={styles.tabSwitchText}>{tab.toUpperCase()}</Text>
              </Pressable>
            ))}
          </View>

          <Pressable style={styles.iconButton} onPress={() => setSettingsVisible(true)}>
            <Text style={styles.iconButtonText}>[]</Text>
          </Pressable>
        </View>

        {projectMenuVisible ? (
          <View style={styles.projectMenu}>
            {projects.map(project => (
              <Pressable
                key={project.projectId}
                style={[
                  styles.projectMenuItem,
                  project.projectId === selectedProjectId && styles.projectMenuItemSelected,
                ]}
                onPress={() => {
                  setSelectedProjectId(project.projectId);
                  setProjectMenuVisible(false);
                }}>
                <Text>{project.name}</Text>
              </Pressable>
            ))}
          </View>
        ) : null}

        <View style={styles.workspace}>
          {isWide && !sidebarCollapsed ? <View style={styles.sidebar}>{sidebar}</View> : null}
          {isWide && !sidebarCollapsed ? <View style={styles.divider} /> : null}

          <View style={styles.main}>
            {activeTab === 'chat' ? (
              <View style={styles.mainPane}>
                <Text style={styles.mainTitle}>CHAT - {CHAT_SESSIONS[chatSessionIndex]}</Text>
                <ScrollView style={styles.scroll} contentContainerStyle={styles.scrollContent}>
                  {CHAT_MESSAGES.map((msg, index) => (
                    <View key={`${msg.role}-${index}`} style={styles.chatBubble}>
                      <Text>
                        [{msg.role}] {msg.text}
                      </Text>
                    </View>
                  ))}
                </ScrollView>
                <View style={styles.inputRow}>
                  <TextInput
                    value={chatInput}
                    onChangeText={setChatInput}
                    placeholder="Message..."
                    style={styles.input}
                  />
                  <Pressable style={styles.sendButton} onPress={() => setChatInput('')}>
                    <Text>Send</Text>
                  </Pressable>
                </View>
              </View>
            ) : null}

            {activeTab === 'file' ? (
              <View style={styles.mainPane}>
                <Text style={styles.mainTitle}>{selectedFilePath ?? 'Select a file'}</Text>
                <ScrollView style={styles.scroll} contentContainerStyle={styles.scrollContent}>
                  <Text selectable>{fileLoading ? 'Loading...' : selectedFileContent}</Text>
                </ScrollView>
              </View>
            ) : null}

            {activeTab === 'git' ? (
              <View style={styles.mainPane}>
                <Text style={styles.mainTitle}>{selectedDiffFile?.path ?? 'Select a changed file'}</Text>
                <ScrollView style={styles.scroll} contentContainerStyle={styles.scrollContent}>
                  <Text selectable>{selectedDiffFile?.diff ?? ''}</Text>
                </ScrollView>
              </View>
            ) : null}
          </View>
        </View>

        {!isWide ? (
          <Modal visible={drawerVisible} animationType="slide" transparent>
            <Pressable style={styles.drawerMask} onPress={() => setDrawerVisible(false)}>
              <Pressable style={[styles.drawer, { width: Math.min(340, width * 0.86) }]} onPress={() => undefined}>
                {sidebar}
              </Pressable>
            </Pressable>
          </Modal>
        ) : null}

        <Modal visible={settingsVisible} animationType="slide">
          <SafeAreaView style={styles.settingsPage}>
            <View style={styles.settingsHeader}>
              <Text style={styles.settingsTitle}>Settings</Text>
              <Pressable style={styles.iconButton} onPress={() => setSettingsVisible(false)}>
                <Text style={styles.iconButtonText}>X</Text>
              </Pressable>
            </View>
            <View style={styles.settingsSection}>
              <Text style={styles.settingsLabel}>Theme</Text>
              {(['system', 'light', 'dark'] as ThemeMode[]).map(mode => (
                <Pressable
                  key={mode}
                  style={[styles.settingsOption, themeMode === mode && styles.settingsOptionSelected]}
                  onPress={() => setThemeMode(mode)}>
                  <Text>{mode}</Text>
                </Pressable>
              ))}
            </View>
          </SafeAreaView>
        </Modal>
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

function WorkspaceSidebar(props: {
  activeTab: WorkspaceTab;
  chatSessionIndex: number;
  onChatSessionSelect: (index: number) => void;
  fileRoot: TreeNode | null;
  expandedPaths: Set<string>;
  loadingPaths: Set<string>;
  selectedFilePath: string | null;
  onToggleDirectory: (path: string) => Promise<void>;
  onSelectFile: (path: string) => void;
  selectedCommitIndex: number;
  onSelectCommit: (index: number) => void;
  selectedDiffFilePath: string;
  onSelectDiffFile: (path: string) => void;
}) {
  return (
    <View style={styles.sidebarRow}>
      <View style={styles.sidebarBody}>
        {props.activeTab === 'chat' ? (
          <ScrollView style={styles.fill} contentContainerStyle={styles.scrollContent}>
            <Text style={styles.sidebarTitle}>CHAT LIST</Text>
            {CHAT_SESSIONS.map((item, index) => (
              <Pressable
                key={item}
                style={[styles.row, index === props.chatSessionIndex && styles.rowSelected]}
                onPress={() => props.onChatSessionSelect(index)}>
                <Text>{item}</Text>
              </Pressable>
            ))}
          </ScrollView>
        ) : null}

        {props.activeTab === 'file' ? (
          <ScrollView style={styles.fill} contentContainerStyle={styles.scrollContent}>
            <Text style={styles.sidebarTitle}>EXPLORER</Text>
            {props.fileRoot
              ? renderTree({
                  node: props.fileRoot,
                  depth: 0,
                  expandedPaths: props.expandedPaths,
                  loadingPaths: props.loadingPaths,
                  selectedFilePath: props.selectedFilePath,
                  onToggleDirectory: props.onToggleDirectory,
                  onSelectFile: props.onSelectFile,
                })
              : null}
          </ScrollView>
        ) : null}

        {props.activeTab === 'git' ? (
          <View style={styles.fill}>
            <View style={styles.gitHalf}>
              <Text style={styles.sidebarTitle}>COMMITS</Text>
              <ScrollView style={styles.fill}>
                {GIT_COMMITS.map((commit, index) => (
                  <Pressable
                    key={commit.hash}
                    style={[styles.row, index === props.selectedCommitIndex && styles.rowSelected]}
                    onPress={() => props.onSelectCommit(index)}>
                    <Text numberOfLines={2}>
                      {commit.hash} {commit.message}
                    </Text>
                  </Pressable>
                ))}
              </ScrollView>
            </View>
            <View style={styles.gitHalfDivider} />
            <View style={styles.gitHalf}>
              <Text style={styles.sidebarTitle}>CHANGED FILES</Text>
              <ScrollView style={styles.fill}>
                {GIT_COMMITS[props.selectedCommitIndex].files.map(file => (
                  <Pressable
                    key={file.path}
                    style={[styles.row, file.path === props.selectedDiffFilePath && styles.rowSelected]}
                    onPress={() => props.onSelectDiffFile(file.path)}>
                    <Text numberOfLines={1}>{file.path}</Text>
                  </Pressable>
                ))}
              </ScrollView>
            </View>
          </View>
        ) : null}
      </View>
    </View>
  );
}

function renderTree(args: {
  node: TreeNode;
  depth: number;
  expandedPaths: Set<string>;
  loadingPaths: Set<string>;
  selectedFilePath: string | null;
  onToggleDirectory: (path: string) => Promise<void>;
  onSelectFile: (path: string) => void;
}): React.ReactNode {
  const indentStyle = { paddingLeft: args.depth * 12 + 8 };
  if (!args.node.isDir) {
    return (
      <Pressable
        key={args.node.path}
        style={[styles.row, indentStyle, args.node.path === args.selectedFilePath && styles.rowSelected]}
        onPress={() => args.onSelectFile(args.node.path)}>
        <Text numberOfLines={1}>- {args.node.name}</Text>
      </Pressable>
    );
  }

  const isExpanded = args.expandedPaths.has(args.node.path);
  const isLoading = args.loadingPaths.has(args.node.path);
  return (
    <View key={args.node.path}>
      <Pressable
        style={[styles.row, indentStyle]}
        onPress={() => {
          args.onToggleDirectory(args.node.path).catch(() => undefined);
        }}>
        <Text numberOfLines={1}>
          {isExpanded ? 'v ' : '> '}
          {args.node.name}
          {isLoading ? ' ...' : ''}
        </Text>
      </Pressable>
      {isExpanded
        ? args.node.children.map(child =>
            renderTree({
              ...args,
              node: child,
              depth: args.depth + 1,
            }),
          )
        : null}
    </View>
  );
}

function mapEntriesToNodes(entries: RegistryFsEntry[], projectId: string): TreeNode[] {
  return entries.map(entry => ({
    name: entry.name,
    path: normalizeNodePath(entry.path, entry.name, projectId),
    isDir: entry.kind === 'dir',
    loaded: false,
    children: [],
  }));
}

function normalizeNodePath(rawPath: string, fallbackName: string, projectId: string): string {
  const rootPath = `/${projectId}`;
  const trimmed = (rawPath ?? '').trim();
  if (trimmed.length === 0 || trimmed === '.') {
    return joinPath(rootPath, fallbackName);
  }
  if (trimmed.startsWith('/')) {
    return trimmed;
  }
  return joinPath(rootPath, trimmed);
}

function joinPath(base: string, name: string): string {
  return `${base}/${name}`.replace(/\/+/g, '/');
}

function projectRelativePath(path: string, projectId: string): string {
  const rootPath = `/${projectId}`;
  if (path === rootPath) {
    return '.';
  }
  if (path.startsWith(`${rootPath}/`)) {
    return path.slice(rootPath.length + 1);
  }
  const normalized = path.replace(/^\/+/g, '');
  return normalized.length > 0 ? normalized : '.';
}

function findNode(root: TreeNode, path: string): TreeNode | null {
  if (root.path === path) return root;
  for (const child of root.children) {
    const found = findNode(child, path);
    if (found) return found;
  }
  return null;
}

function patchNode(root: TreeNode, targetPath: string, updater: (node: TreeNode) => TreeNode): TreeNode {
  if (root.path === targetPath) return updater(root);
  return {
    ...root,
    children: root.children.map(child => patchNode(child, targetPath, updater)),
  };
}

function sortNode(a: TreeNode, b: TreeNode): number {
  if (a.isDir && !b.isDir) return -1;
  if (!a.isDir && b.isDir) return 1;
  return a.name.localeCompare(b.name);
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: '#fff',
  },
  topBar: {
    height: 52,
    borderBottomWidth: 1,
    borderBottomColor: '#ddd',
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 8,
  },
  topBarSpacer: {
    flex: 1,
  },
  tabSwitch: {
    flexDirection: 'row',
    marginRight: 8,
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 6,
    overflow: 'hidden',
  },
  tabSwitchItem: {
    minWidth: 58,
    minHeight: 32,
    alignItems: 'center',
    justifyContent: 'center',
    borderRightWidth: 1,
    borderRightColor: '#ddd',
    paddingHorizontal: 8,
  },
  tabSwitchItemActive: {
    backgroundColor: '#eee',
  },
  tabSwitchItemLast: {
    borderRightWidth: 0,
  },
  tabSwitchText: {
    fontSize: 11,
    fontWeight: '600',
  },
  iconButton: {
    width: 32,
    height: 32,
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 4,
    alignItems: 'center',
    justifyContent: 'center',
  },
  iconButtonText: {
    fontSize: 13,
    fontWeight: '600',
  },
  projectButton: {
    flexDirection: 'row',
    alignItems: 'center',
    maxWidth: 300,
    marginLeft: 8,
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 6,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  projectText: {
    fontSize: 13,
    maxWidth: 250,
  },
  projectArrow: {
    marginRight: 6,
    fontSize: 10,
  },
  projectMenu: {
    position: 'absolute',
    top: 52,
    left: 48,
    width: 260,
    borderWidth: 1,
    borderColor: '#ddd',
    backgroundColor: '#fff',
    zIndex: 10,
  },
  projectMenuItem: {
    minHeight: 36,
    justifyContent: 'center',
    paddingHorizontal: 10,
  },
  projectMenuItemSelected: {
    backgroundColor: '#eee',
  },
  workspace: {
    flex: 1,
    flexDirection: 'row',
    minHeight: 0,
  },
  sidebar: {
    width: 340,
    minHeight: 0,
  },
  divider: {
    width: 1,
    backgroundColor: '#ddd',
  },
  main: {
    flex: 1,
    minHeight: 0,
  },
  mainPane: {
    flex: 1,
    minHeight: 0,
  },
  mainTitle: {
    paddingHorizontal: 12,
    paddingVertical: 10,
    borderBottomWidth: 1,
    borderBottomColor: '#ddd',
    fontWeight: '600',
  },
  scroll: {
    flex: 1,
    minHeight: 0,
  },
  scrollContent: {
    paddingBottom: 12,
  },
  chatBubble: {
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 6,
    padding: 8,
    marginHorizontal: 12,
    marginTop: 12,
  },
  inputRow: {
    flexDirection: 'row',
    borderTopWidth: 1,
    borderTopColor: '#ddd',
    padding: 10,
  },
  input: {
    flex: 1,
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 6,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  sendButton: {
    marginLeft: 8,
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 6,
    paddingHorizontal: 12,
    justifyContent: 'center',
    alignItems: 'center',
  },
  sidebarRow: {
    flex: 1,
    minHeight: 0,
  },
  sidebarBody: {
    flex: 1,
    minHeight: 0,
  },
  sidebarTitle: {
    fontSize: 12,
    fontWeight: '600',
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  row: {
    minHeight: 32,
    justifyContent: 'center',
    paddingHorizontal: 10,
  },
  rowSelected: {
    backgroundColor: '#eee',
  },
  fill: {
    flex: 1,
    minHeight: 0,
  },
  gitHalf: {
    flex: 1,
    minHeight: 0,
  },
  gitHalfDivider: {
    height: 1,
    backgroundColor: '#ddd',
  },
  drawerMask: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.24)',
  },
  drawer: {
    height: '100%',
    backgroundColor: '#fff',
    minHeight: 0,
  },
  settingsPage: {
    flex: 1,
    backgroundColor: '#fff',
  },
  settingsHeader: {
    height: 52,
    borderBottomWidth: 1,
    borderBottomColor: '#ddd',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 10,
  },
  settingsTitle: {
    fontSize: 18,
    fontWeight: '700',
  },
  settingsSection: {
    paddingHorizontal: 12,
    paddingTop: 16,
  },
  settingsLabel: {
    fontSize: 14,
    fontWeight: '600',
    marginBottom: 8,
  },
  settingsOption: {
    minHeight: 36,
    justifyContent: 'center',
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 6,
    paddingHorizontal: 10,
    marginBottom: 8,
  },
  settingsOptionSelected: {
    backgroundColor: '#eee',
  },
});

export default App;

