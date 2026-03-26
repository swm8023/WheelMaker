import React, { useMemo, useState } from 'react';
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
  { role: 'system', text: 'Connected to observe workspace.' },
  { role: 'agent', text: 'This is the React Native migration shell.' },
];

const FILE_TREE: FileNode = {
  name: 'WheelMaker',
  path: '/WheelMaker',
  isDir: true,
  children: [
    {
      name: 'app',
      path: '/WheelMaker/app',
      isDir: true,
      children: [
        {
          name: 'src',
          path: '/WheelMaker/app/src',
          isDir: true,
          children: [
            {
              name: 'services',
              path: '/WheelMaker/app/src/services',
              isDir: true,
              children: [
                {
                  name: 'observeRepository.ts',
                  path: '/WheelMaker/app/src/services/observeRepository.ts',
                  isDir: false,
                  content:
                    "export class ObserveRepository { /* request wrappers */ }",
                },
              ],
            },
            {
              name: 'README.md',
              path: '/WheelMaker/app/src/README.md',
              isDir: false,
              content: '# WheelMaker RN Foundation\n\nObserve protocol client.',
            },
          ],
        },
        {
          name: 'App.tsx',
          path: '/WheelMaker/app/App.tsx',
          isDir: false,
          content: 'export default function App() { return null; }',
        },
      ],
    },
    {
      name: 'server',
      path: '/WheelMaker/server',
      isDir: true,
      children: [
        {
          name: 'main.go',
          path: '/WheelMaker/server/main.go',
          isDir: false,
          content: 'package main\n\nfunc main() {}\n',
        },
      ],
    },
  ],
};

const GIT_COMMITS: GitCommit[] = [
  {
    hash: '14b16e2',
    message: 'feat(app): implement observe connect, project list, and files',
    files: [
      {
        path: 'app/lib/services/observe_ws_client.dart',
        diff: '@@ -1,3 +1,4 @@\n+class ObserveWsClient { ... }',
      },
      {
        path: 'app/lib/screens/connect_screen.dart',
        diff: '@@ -40,2 +75,20 @@\n+Future<void> _openObserveWorkspace() async { ... }',
      },
    ],
  },
  {
    hash: '270ded2',
    message: 'refactor(app): deprecate flutter app and promote react native app',
    files: [
      {
        path: 'app/App.tsx',
        diff: '@@ -1,10 +1,40 @@\n+React Native migration shell',
      },
    ],
  },
];

function App() {
  const { width } = useWindowDimensions();
  const isWide = width >= 900;
  const compact = width < 560;

  const [tab, setTab] = useState<WorkspaceTab>('chat');
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  const [chatSessionIndex, setChatSessionIndex] = useState(0);
  const [chatInput, setChatInput] = useState('');

  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(
    new Set(['/WheelMaker', '/WheelMaker/app']),
  );
  const [selectedFilePath, setSelectedFilePath] = useState<string>(
    '/WheelMaker/app/src/README.md',
  );

  const [selectedCommitIndex, setSelectedCommitIndex] = useState(0);
  const [selectedDiffFilePath, setSelectedDiffFilePath] = useState(
    GIT_COMMITS[0].files[0].path,
  );

  const selectedCommit = GIT_COMMITS[selectedCommitIndex];
  const selectedDiffFile = selectedCommit.files.find(
    file => file.path === selectedDiffFilePath,
  );
  const selectedFile = useMemo(
    () => findNodeByPath(FILE_TREE, selectedFilePath),
    [selectedFilePath],
  );

  const leftPanel = renderSidebar({
    tab,
    chatSessionIndex,
    onChatSessionSelect: index => {
      setChatSessionIndex(index);
      if (!isWide) setDrawerOpen(false);
    },
    expandedPaths,
    onTogglePath: path => {
      setExpandedPaths(prev => {
        const next = new Set(prev);
        if (next.has(path)) next.delete(path);
        else next.add(path);
        return next;
      });
    },
    selectedFilePath,
    onFileSelect: path => {
      setSelectedFilePath(path);
      if (!isWide) setDrawerOpen(false);
    },
    selectedCommitIndex,
    onCommitSelect: index => {
      setSelectedCommitIndex(index);
      const firstFile = GIT_COMMITS[index]?.files[0]?.path ?? '';
      setSelectedDiffFilePath(firstFile);
    },
    selectedDiffFilePath,
    onDiffFileSelect: path => {
      setSelectedDiffFilePath(path);
      if (!isWide) setDrawerOpen(false);
    },
  });

  return (
    <SafeAreaProvider>
      <StatusBar barStyle="light-content" />
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.header}>
          <Pressable
            onPress={() => {
              if (isWide) setSidebarCollapsed(v => !v);
              else setDrawerOpen(true);
            }}
            style={styles.headerButton}>
            <Text>{isWide ? (sidebarCollapsed ? '>' : '<') : '≡'}</Text>
          </Pressable>
          <Text style={styles.headerTitle} numberOfLines={1}>
            WheelMaker Project
          </Text>
          <View style={styles.segmentWrap}>
            {(['chat', 'file', 'git'] as WorkspaceTab[]).map(item => (
              <Pressable
                key={item}
                style={[styles.segmentItem, tab === item && styles.segmentItemSelected]}
                onPress={() => setTab(item)}>
                <Text>{compact ? item[0].toUpperCase() : item.toUpperCase()}</Text>
              </Pressable>
            ))}
          </View>
        </View>

        <View style={styles.container}>
          {isWide && !sidebarCollapsed ? (
            <View style={styles.sidebar}>{leftPanel}</View>
          ) : null}
          {isWide && !sidebarCollapsed ? <View style={styles.divider} /> : null}

          <View style={styles.mainPane}>
            {tab === 'chat' ? (
              <View style={styles.mainBlock}>
                <Text style={styles.blockTitle}>CHAT - {CHAT_SESSIONS[chatSessionIndex]}</Text>
                <ScrollView style={styles.scrollArea}>
                  {CHAT_MESSAGES.map((msg, idx) => (
                    <View key={idx} style={styles.chatBubble}>
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

            {tab === 'file' ? (
              <View style={styles.mainBlock}>
                <Text style={styles.blockTitle}>{selectedFile?.path ?? 'Select a file'}</Text>
                <ScrollView style={styles.scrollArea}>
                  <Text selectable>{selectedFile?.content ?? ''}</Text>
                </ScrollView>
              </View>
            ) : null}

            {tab === 'git' ? (
              <View style={styles.mainBlock}>
                <Text style={styles.blockTitle}>
                  {selectedDiffFile?.path ?? 'Select a changed file'}
                </Text>
                <ScrollView style={styles.scrollArea}>
                  <Text selectable>{selectedDiffFile?.diff ?? ''}</Text>
                </ScrollView>
              </View>
            ) : null}
          </View>
        </View>

        {!isWide ? (
          <Modal visible={drawerOpen} animationType="slide" transparent>
            <Pressable style={styles.drawerMask} onPress={() => setDrawerOpen(false)}>
              <View style={styles.drawer}>
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
  tab: WorkspaceTab;
  chatSessionIndex: number;
  onChatSessionSelect: (index: number) => void;
  expandedPaths: Set<string>;
  onTogglePath: (path: string) => void;
  selectedFilePath: string;
  onFileSelect: (path: string) => void;
  selectedCommitIndex: number;
  onCommitSelect: (index: number) => void;
  selectedDiffFilePath: string;
  onDiffFileSelect: (path: string) => void;
}) {
  if (args.tab === 'chat') {
    return (
      <View style={styles.sideContainer}>
        <Text style={styles.sideTitle}>CHAT LIST</Text>
        {CHAT_SESSIONS.map((item, index) => (
          <Pressable
            key={item}
            style={[
              styles.sideRow,
              index === args.chatSessionIndex && styles.sideRowSelected,
            ]}
            onPress={() => args.onChatSessionSelect(index)}>
            <Text>{item}</Text>
          </Pressable>
        ))}
      </View>
    );
  }

  if (args.tab === 'file') {
    return (
      <ScrollView style={styles.sideContainer}>
        <Text style={styles.sideTitle}>EXPLORER</Text>
        {renderFileTree({
          node: FILE_TREE,
          depth: 0,
          expandedPaths: args.expandedPaths,
          onTogglePath: args.onTogglePath,
          selectedFilePath: args.selectedFilePath,
          onFileSelect: args.onFileSelect,
        })}
      </ScrollView>
    );
  }

  return (
    <View style={styles.sideContainer}>
      <Text style={styles.sideTitle}>COMMITS</Text>
      <ScrollView style={styles.flexOne}>
        {GIT_COMMITS.map((commit, index) => (
          <Pressable
            key={commit.hash}
            style={[
              styles.sideRow,
              index === args.selectedCommitIndex && styles.sideRowSelected,
            ]}
            onPress={() => args.onCommitSelect(index)}>
            <Text numberOfLines={2}>
              {commit.hash} {commit.message}
            </Text>
          </Pressable>
        ))}
      </ScrollView>
      <Text style={styles.sideTitle}>CHANGED FILES</Text>
      <ScrollView style={styles.flexOne}>
        {GIT_COMMITS[args.selectedCommitIndex].files.map(file => (
          <Pressable
            key={file.path}
            style={[
              styles.sideRow,
              file.path === args.selectedDiffFilePath && styles.sideRowSelected,
            ]}
            onPress={() => args.onDiffFileSelect(file.path)}>
            <Text numberOfLines={1}>{file.path}</Text>
          </Pressable>
        ))}
      </ScrollView>
    </View>
  );
}

function renderFileTree(args: {
  node: FileNode;
  depth: number;
  expandedPaths: Set<string>;
  onTogglePath: (path: string) => void;
  selectedFilePath: string;
  onFileSelect: (path: string) => void;
}): React.ReactNode {
  const indent = { paddingLeft: args.depth * 14 + 8 };
  if (!args.node.isDir) {
    return (
      <Pressable
        key={args.node.path}
        style={[
          styles.sideRow,
          indent,
          args.node.path === args.selectedFilePath && styles.sideRowSelected,
        ]}
        onPress={() => args.onFileSelect(args.node.path)}>
        <Text>{args.node.name}</Text>
      </Pressable>
    );
  }
  const isOpen = args.expandedPaths.has(args.node.path);
  const sortedChildren = [...(args.node.children ?? [])].sort(sortFileNode);
  return (
    <View key={args.node.path}>
      <Pressable
        style={[styles.sideRow, indent]}
        onPress={() => args.onTogglePath(args.node.path)}>
        <Text>{isOpen ? '▾ ' : '▸ '}{args.node.name}</Text>
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

function sortFileNode(a: FileNode, b: FileNode): number {
  if (a.isDir && !b.isDir) return -1;
  if (!a.isDir && b.isDir) return 1;
  return a.name.localeCompare(b.name);
}

function findNodeByPath(root: FileNode, path: string): FileNode | undefined {
  if (root.path === path) return root;
  for (const child of root.children ?? []) {
    const found = findNodeByPath(child, path);
    if (found) return found;
  }
  return undefined;
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: '#fff',
  },
  header: {
    height: 52,
    borderBottomWidth: 1,
    borderColor: '#ddd',
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
    borderColor: '#ddd',
    borderRadius: 4,
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
    borderColor: '#ddd',
    borderRadius: 6,
    overflow: 'hidden',
  },
  segmentItem: {
    paddingVertical: 6,
    paddingHorizontal: 10,
    borderRightWidth: 1,
    borderColor: '#ddd',
  },
  segmentItemSelected: {
    backgroundColor: '#eee',
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
    backgroundColor: '#ddd',
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
  sideRowSelected: {
    backgroundColor: '#eee',
  },
  mainBlock: {
    flex: 1,
  },
  blockTitle: {
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontWeight: '600',
    borderBottomWidth: 1,
    borderColor: '#ddd',
  },
  scrollArea: {
    flex: 1,
    padding: 12,
  },
  chatBubble: {
    padding: 8,
    marginBottom: 8,
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 6,
  },
  inputRow: {
    flexDirection: 'row',
    borderTopWidth: 1,
    borderColor: '#ddd',
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
    backgroundColor: '#fff',
  },
  drawerInner: {
    flex: 1,
  },
  flexOne: {
    flex: 1,
  },
});

export default App;
