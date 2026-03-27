import React, {useEffect, useMemo, useRef, useState} from 'react';
import {
  Animated,
  Modal,
  PanResponder,
  Pressable,
  ScrollView,
  StatusBar,
  StyleSheet,
  Switch,
  Text,
  TextInput,
  Platform,
  useWindowDimensions,
  View,
} from 'react-native';
import {SafeAreaProvider, SafeAreaView} from 'react-native-safe-area-context';

import {InlineDiffView, MarkdownView, PrismCodeView} from '../components';
import {
  CHAT_MESSAGES,
  CHAT_SESSIONS,
  type FileNode,
  type GitCommitFileView,
  type WorkspaceTab,
  useWorkspaceData,
} from '../state/workspaceData';
import {resolveTheme, type ThemeMode} from '../theme';
import type {
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryProject,
} from '../types/observe';
import {isMarkdownPath} from '../utils/codeLanguage';
import {iconForPath, type FileIcon} from '../utils/fileIcon';

type WorkspaceScreenProps = {
  projects: RegistryProject[];
  selectedProjectId: string;
  fileEntries: RegistryFsEntry[];
  onSelectProject: (projectId: string) => Promise<void>;
  onListDirectory: (path: string) => Promise<RegistryFsEntry[]>;
  onReadFile: (path: string) => Promise<string>;
  onListGitBranches: () => Promise<{current: string; branches: string[]}>;
  onListGitCommits: (ref?: string) => Promise<RegistryGitCommit[]>;
  onListGitCommitFiles: (sha: string) => Promise<RegistryGitCommitFile[]>;
  onReadGitFileDiff: (sha: string, path: string) => Promise<RegistryGitFileDiff>;
  onLogout: () => void;
  themeMode: ThemeMode;
  onThemeModeChange: (mode: ThemeMode) => void;
};

const WORKSPACE_TABS: WorkspaceTab[] = ['chat', 'file', 'git'];

export function WorkspaceScreen({
  projects,
  selectedProjectId,
  fileEntries,
  onSelectProject,
  onListDirectory,
  onReadFile,
  onListGitBranches,
  onListGitCommits,
  onListGitCommitFiles,
  onReadGitFileDiff,
  onLogout,
  themeMode,
  onThemeModeChange,
}: WorkspaceScreenProps) {
  const {width} = useWindowDimensions();
  const tabs = WORKSPACE_TABS;
  const isWide = width >= 900;
  const compact = width < 560;
  const drawerWidth = Math.min(320, Math.floor(width * 0.88));
  const settingsPanelWidth = Math.min(360, Math.floor(width * 0.88));

  const theme = resolveTheme(themeMode);

  const [tab, setTab] = useState<WorkspaceTab>('chat');
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [projectMenuOpen, setProjectMenuOpen] = useState(false);
  const [quickSettingsOpen, setQuickSettingsOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [wrapLines, setWrapLines] = useState(false);
  const [showLineNumbers, setShowLineNumbers] = useState(true);
  const [loadingProject, setLoadingProject] = useState(false);
  const [refreshingProject, setRefreshingProject] = useState(false);
  const drawerProgress = useRef(new Animated.Value(0)).current;
  const settingsProgress = useRef(new Animated.Value(0)).current;
  const sidebarWidthAnim = useRef(new Animated.Value(320)).current;
  const workspaceData = useWorkspaceData({
    projects,
    selectedProjectId,
    fileEntries,
    onListDirectory,
    onReadFile,
    onListGitBranches,
    onListGitCommits,
    onListGitCommitFiles,
    onReadGitFileDiff,
  });

  const selectedProject =
    projects.find(item => item.projectId === selectedProjectId) ?? projects[0];
  const selectedCommit = workspaceData.projectState.gitCommits.find(
    item => item.sha === workspaceData.projectState.selectedCommitSha,
  );
  const selectedCommitFiles =
    workspaceData.projectState.gitFilesBySha[workspaceData.projectState.selectedCommitSha] ?? [];
  const selectedDiffFile = selectedCommitFiles.find(
    file => file.path === workspaceData.projectState.selectedDiffFilePath,
  );
  const activeTabIndex = tabs.indexOf(tab);
  const [mainPaneWidth, setMainPaneWidth] = useState(0);
  const baseTranslateX = useRef(new Animated.Value(0)).current;
  const dragTranslateX = useRef(new Animated.Value(0)).current;
  const tabIndex = tabs.indexOf(tab);

  useEffect(() => {
    if (mainPaneWidth <= 0 || tabIndex < 0) {
      return;
    }
    Animated.timing(baseTranslateX, {
      toValue: -tabIndex * mainPaneWidth,
      duration: 180,
      useNativeDriver: true,
    }).start();
  }, [baseTranslateX, mainPaneWidth, tabIndex]);

  const panResponder = useMemo(
    () =>
      PanResponder.create({
        onMoveShouldSetPanResponderCapture: (_evt, gestureState) =>
          gestureState.numberActiveTouches >= 2 &&
          mainPaneWidth > 0 &&
          Math.abs(gestureState.dx) > 18 &&
          Math.abs(gestureState.dx) > Math.abs(gestureState.dy) * 1.15,
        onPanResponderMove: (_evt, gestureState) => {
          if (gestureState.numberActiveTouches < 2) {
            return;
          }
          const atFirst = tabIndex <= 0 && gestureState.dx > 0;
          const atLast = tabIndex >= tabs.length - 1 && gestureState.dx < 0;
          if (atFirst || atLast) {
            dragTranslateX.setValue(gestureState.dx * 0.28);
            return;
          }
          dragTranslateX.setValue(gestureState.dx);
        },
        onPanResponderRelease: (_evt, gestureState) => {
          const shouldSwitch = Math.abs(gestureState.dx) > 56 || Math.abs(gestureState.vx) > 0.35;
          let nextIndex = tabIndex;
          if (shouldSwitch) {
            if (gestureState.dx < 0 && tabIndex < tabs.length - 1) {
              nextIndex = tabIndex + 1;
            } else if (gestureState.dx > 0 && tabIndex > 0) {
              nextIndex = tabIndex - 1;
            }
          }
          if (nextIndex !== tabIndex) {
            setTab(tabs[nextIndex]);
          }
          Animated.timing(dragTranslateX, {
            toValue: 0,
            duration: 140,
            useNativeDriver: true,
          }).start();
        },
        onPanResponderTerminate: () => {
          Animated.timing(dragTranslateX, {
            toValue: 0,
            duration: 140,
            useNativeDriver: true,
          }).start();
        },
      }),
    [dragTranslateX, mainPaneWidth, tabIndex, tabs],
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
    if (isWide) {
      setDrawerVisible(false);
      drawerProgress.setValue(0);
    }
  }, [drawerProgress, isWide]);

  useEffect(() => {
    Animated.timing(sidebarWidthAnim, {
      toValue: isWide && !sidebarCollapsed ? 320 : 0,
      duration: 160,
      useNativeDriver: false,
    }).start();
  }, [isWide, sidebarCollapsed, sidebarWidthAnim]);

  const openSettings = () => {
    setQuickSettingsOpen(false);
    setSettingsOpen(true);
    Animated.timing(settingsProgress, {
      toValue: 1,
      duration: 180,
      useNativeDriver: true,
    }).start();
  };

  const closeSettings = () => {
    Animated.timing(settingsProgress, {
      toValue: 0,
      duration: 150,
      useNativeDriver: true,
    }).start(({finished}) => {
      if (finished) {
        setSettingsOpen(false);
      }
    });
  };

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

  const renderMainTab = (currentTab: WorkspaceTab) => {
    if (currentTab === 'chat') {
      return (
        <View style={styles.mainBlock}>
          <Text style={[styles.blockTitle, {borderColor: theme.colors.border, color: theme.colors.text}]}>
            CHAT - {CHAT_SESSIONS[workspaceData.projectState.chatSessionIndex]}
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
              value={workspaceData.projectState.chatInput}
              onChangeText={workspaceData.setChatInput}
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
              onPress={() => workspaceData.setChatInput('')}>
              <Text style={{color: theme.colors.text}}>Send</Text>
            </Pressable>
          </View>
        </View>
      );
    }

    if (currentTab === 'file') {
      return (
        <View style={styles.mainBlock}>
          <Text style={[styles.blockTitle, {borderColor: theme.colors.border, color: theme.colors.text}]}>
            {workspaceData.projectState.selectedFilePath ?? 'Select a file'}
            {loadingProject ? ' (loading project...)' : ''}
          </Text>
          <ScrollView style={styles.scrollArea} contentContainerStyle={styles.mainScrollPad}>
            {workspaceData.projectState.loadingFilePath ? (
              <Text style={{color: theme.colors.textMuted}}>Loading file...</Text>
            ) : workspaceData.projectState.selectedFilePath &&
              isMarkdownPath(workspaceData.projectState.selectedFilePath) ? (
              <MarkdownView content={workspaceData.projectState.selectedFileContent} theme={theme} />
            ) : (
              <PrismCodeView
                path={workspaceData.projectState.selectedFilePath ?? 'file.txt'}
                code={workspaceData.projectState.selectedFileContent}
                theme={theme}
                wrapLines={wrapLines}
                showLineNumbers={showLineNumbers}
              />
            )}
          </ScrollView>
        </View>
      );
    }

    return (
      <View style={styles.mainBlock}>
        <Text style={[styles.blockTitle, {borderColor: theme.colors.border, color: theme.colors.text}]}>
          {selectedDiffFile?.path ?? selectedCommit?.title ?? 'Select a changed file'}
        </Text>
        <ScrollView style={styles.scrollArea} contentContainerStyle={styles.mainScrollPad}>
          {workspaceData.projectState.gitLoading ? (
            <Text style={{color: theme.colors.textMuted}}>Loading commits...</Text>
          ) : workspaceData.projectState.gitError ? (
            <Text style={{color: theme.colors.error}}>{workspaceData.projectState.gitError}</Text>
          ) : selectedDiffFile?.loadingDiff ? (
            <Text style={{color: theme.colors.textMuted}}>Loading diff...</Text>
          ) : selectedDiffFile?.diffError ? (
            <Text style={{color: theme.colors.error}}>{selectedDiffFile.diffError}</Text>
          ) : (
            <InlineDiffView diff={selectedDiffFile?.diff ?? ''} theme={theme} wrapLines />
          )}
        </ScrollView>
      </View>
    );
  };

  const leftPanel = (
    <Sidebar
      theme={theme}
      tab={tab}
      tree={workspaceData.projectState.fileTree}
      expandedPaths={workspaceData.projectState.expandedPaths}
      loadingDirs={workspaceData.projectState.loadingDirs}
      selectedFilePath={workspaceData.projectState.selectedFilePath}
      onToggleDirectory={workspaceData.toggleDirectory}
      onFileSelect={async path => {
        workspaceData.selectFile(path);
        if (!isWide) {
          closeDrawer();
        }
      }}
      gitLoading={workspaceData.projectState.gitLoading}
      gitError={workspaceData.projectState.gitError}
      gitCommits={workspaceData.projectState.gitCommits}
      gitCurrentBranch={workspaceData.projectState.gitCurrentBranch}
      selectedCommitSha={workspaceData.projectState.selectedCommitSha}
      onCommitSelect={workspaceData.selectCommit}
      gitFiles={selectedCommitFiles}
      selectedDiffFilePath={workspaceData.projectState.selectedDiffFilePath}
      onDiffFileSelect={path => {
        workspaceData.selectDiffFile(path);
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
              {loadingProject || refreshingProject ? ' ...' : ''}
            </Text>
          </Pressable>
          <Pressable
            onPress={() => {
              setRefreshingProject(true);
              workspaceData
                .refreshProject()
                .catch(() => undefined)
                .finally(() => setRefreshingProject(false));
            }}
            style={[
              styles.headerButton,
              styles.projectRefreshButton,
              {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
            ]}>
            <Text style={[styles.projectRefreshIcon, {color: theme.colors.text}]}>
              {refreshingProject ? '...' : '\u21bb'}
            </Text>
          </Pressable>

          <View style={styles.headerSpacer} />

          <View style={[styles.segmentWrap, {borderColor: theme.colors.border}]}> 
            {tabs.map((item, index, arr) => (
              <Pressable
                key={item}
                style={[
                  styles.segmentItem,
                  {borderColor: theme.colors.border},
                  index === arr.length - 1 && styles.segmentItemLast,
                  tab === item && {backgroundColor: theme.colors.rowSelected},
                ]}
                onPress={() => {
                  setTab(item);
                  setQuickSettingsOpen(false);
                }}>
                <Text style={{color: theme.colors.text}}>{compact ? item[0].toUpperCase() : item.toUpperCase()}</Text>
              </Pressable>
            ))}
          </View>

          <Pressable
            onPress={() => setQuickSettingsOpen(value => !value)}
            style={[
              styles.headerButton,
              styles.settingsButton,
              {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
            ]}>
            <Text style={{color: theme.colors.text}}>⚙</Text>
          </Pressable>
        </View>

        {quickSettingsOpen ? (
          <View
            style={[
              styles.quickSettingsMenu,
              {borderColor: theme.colors.border, backgroundColor: theme.colors.panel},
            ]}>
            <Text style={[styles.sideTitle, {color: theme.colors.textMuted}]}>QUICK SETTINGS</Text>
            <Pressable
              style={styles.quickSwitchItem}
              onPress={() => onThemeModeChange(themeMode === 'dark' ? 'light' : 'dark')}>
              <Text style={{color: theme.colors.text}}>Dark Mode</Text>
              <Switch
                value={themeMode === 'dark'}
                onValueChange={value => onThemeModeChange(value ? 'dark' : 'light')}
              />
            </Pressable>
            <Pressable style={styles.quickSwitchItem} onPress={() => setWrapLines(value => !value)}>
              <Text style={{color: theme.colors.text}}>Wrap Line</Text>
              <Switch value={wrapLines} onValueChange={setWrapLines} />
            </Pressable>
            <Pressable style={styles.quickSwitchItem} onPress={() => setShowLineNumbers(value => !value)}>
              <Text style={{color: theme.colors.text}}>Line Number</Text>
              <Switch value={showLineNumbers} onValueChange={setShowLineNumbers} />
            </Pressable>
            <View style={[styles.quickMoreDivider, {backgroundColor: theme.colors.border}]} />
            <Pressable
              style={styles.quickMoreButton}
              onPress={() => {
                setQuickSettingsOpen(false);
                openSettings();
              }}>
              <Text style={{color: theme.colors.accent}}>More Settings</Text>
            </Pressable>
          </View>
        ) : null}

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
          {isWide ? (
            <Animated.View
              style={[
                styles.sidebar,
                {backgroundColor: theme.colors.panel, width: sidebarWidthAnim},
              ]}
              pointerEvents={sidebarCollapsed ? 'none' : 'auto'}>
              {leftPanel}
            </Animated.View>
          ) : null}
          {isWide ? (
            <Animated.View
              style={[
                styles.divider,
                {
                  width: sidebarWidthAnim.interpolate({
                    inputRange: [0, 320],
                    outputRange: [0, 1],
                  }),
                  backgroundColor: theme.colors.border,
                  opacity: sidebarWidthAnim.interpolate({
                    inputRange: [0, 320],
                    outputRange: [0, 1],
                  }),
                },
              ]}
            />
          ) : null}

          <View
            style={[styles.mainPane, {backgroundColor: theme.colors.background}]}
            onLayout={event => {
              const nextWidth = event.nativeEvent.layout.width;
              if (nextWidth > 0 && nextWidth !== mainPaneWidth) {
                setMainPaneWidth(nextWidth);
              }
            }}
            {...panResponder.panHandlers}>
            <Animated.View
              style={[
                styles.mainPager,
                {
                  width: (mainPaneWidth || 1) * tabs.length,
                  transform: [{translateX: Animated.add(baseTranslateX, dragTranslateX)}],
                },
              ]}>
              {tabs.map(item => (
                <View key={item} style={[styles.mainPage, {width: mainPaneWidth || 1}]}>
                  {Math.abs(tabs.indexOf(item) - activeTabIndex) <= 1 ? renderMainTab(item) : null}
                </View>
              ))}
            </Animated.View>
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

        <Modal visible={settingsOpen} animationType="none" transparent onRequestClose={closeSettings}>
          <View style={styles.drawerHost}>
            <Pressable style={styles.drawerOverlay} onPress={closeSettings}>
              <Animated.View style={[styles.drawerMask, {opacity: settingsProgress}]} />
            </Pressable>
            <Animated.View
              style={[
                styles.settingsDrawer,
                {
                  width: settingsPanelWidth,
                  borderColor: theme.colors.border,
                  backgroundColor: theme.colors.panel,
                  transform: [
                    {
                      translateX: settingsProgress.interpolate({
                        inputRange: [0, 1],
                        outputRange: [settingsPanelWidth, 0],
                      }),
                    },
                  ],
                },
              ]}>
              <SafeAreaView style={styles.safeArea}>
                <View style={[styles.settingsHeader, {borderColor: theme.colors.border}]}>
                  <Text style={[styles.settingsTitle, {color: theme.colors.text}]}>Settings</Text>
                  <Pressable
                    style={[
                      styles.headerButton,
                      {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
                    ]}
                    onPress={closeSettings}>
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
            </Animated.View>
          </View>
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
  gitLoading: boolean;
  gitError: string;
  gitCommits: RegistryGitCommit[];
  gitCurrentBranch: string;
  selectedCommitSha: string;
  onCommitSelect: (index: number) => void;
  gitFiles: GitCommitFileView[];
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
          {args.gitLoading ? <Text style={{color: args.theme.colors.textMuted}}>Loading...</Text> : null}
          {args.gitError ? <Text style={{color: args.theme.colors.error}}>{args.gitError}</Text> : null}
          {args.gitCommits.map((commit, index) => {
            return (
              <Pressable
                key={commit.sha}
                style={[
                  styles.commitRow,
                  commit.sha === args.selectedCommitSha && {
                    backgroundColor: args.theme.colors.rowSelected,
                  },
                ]}
                onPress={() => args.onCommitSelect(index)}>
                <View style={styles.commitGraph}>
                  <View style={styles.commitLaneWrap}>
                    <View style={[styles.commitLaneLine, {backgroundColor: args.theme.colors.border}]} />
                  </View>
                  <View style={[styles.commitNode, {backgroundColor: args.theme.colors.accent}]} />
                </View>
                <View style={styles.commitTextWrap}>
                  <View style={styles.commitTitleRow}>
                    <Text numberOfLines={1} style={{color: args.theme.colors.text}}>
                      {commit.title}
                    </Text>
                    {index === 0 && args.gitCurrentBranch ? (
                      <View style={styles.commitBadgeRow}>
                        <View style={[styles.branchBadge, {borderColor: args.theme.colors.accent}]}>
                          <Text style={[styles.branchBadgeText, {color: args.theme.colors.accent}]}>HEAD</Text>
                        </View>
                        <View style={[styles.branchBadge, {borderColor: args.theme.colors.accent}]}>
                          <Text style={[styles.branchBadgeText, {color: args.theme.colors.accent}]}>
                            {args.gitCurrentBranch}
                          </Text>
                        </View>
                      </View>
                    ) : null}
                  </View>
                </View>
              </Pressable>
            );
          })}
        </ScrollView>
      </View>
      <View style={[styles.gitDivider, {backgroundColor: args.theme.colors.border}]} />
      <View style={styles.gitHalf}>
        <Text style={[styles.sideTitle, {color: args.theme.colors.textMuted}]}>CHANGED FILES</Text>
        <ScrollView style={styles.flexOne}>
          {args.gitFiles.map(file => {
            const icon = iconForPath(file.path, {mode: args.theme.mode});
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
                  <VsIcon icon={icon} /><Text> </Text>
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
  const titleProps = Platform.OS === 'web' ? ({title: args.node.name} as object) : {};

  if (!args.node.isDir) {
    const icon = iconForPath(args.node.path, {mode: args.theme.mode});
    return (
      <Pressable
        {...(titleProps as any)}
        key={args.node.path}
        style={[
          styles.sideRow,
          indent,
          args.node.path === args.selectedFilePath && {
            backgroundColor: args.theme.colors.rowSelected,
          },
        ]}
        onPress={() => args.onFileSelect(args.node.path)}>
        <Text numberOfLines={1} ellipsizeMode="tail" style={[styles.treeText, {color: args.theme.colors.text}]}>
          <VsIcon icon={icon} /><Text> </Text>
          {args.node.name}
        </Text>
      </Pressable>
    );
  }

  const isOpen = args.expandedPaths.has(args.node.path);
  const isLoading = args.loadingDirs.has(args.node.path);
  const sortedChildren = [...args.node.children].sort(sortFileNode);
  const icon = iconForPath(args.node.path, {
    isDir: true,
    expanded: isOpen,
    mode: args.theme.mode,
  });

  return (
    <View key={args.node.path}>
      <Pressable
        {...(titleProps as any)}
        style={[styles.sideRow, indent]}
        onPress={() => {
          args.onToggleDirectory(args.node.path).catch(() => undefined);
        }}>
        <Text numberOfLines={1} ellipsizeMode="tail" style={[styles.treeText, {color: args.theme.colors.text}]}>
          {isOpen ? 'v ' : '> '}
          <VsIcon icon={icon} /><Text> </Text>
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

function sortFileNode(a: FileNode, b: FileNode): number {
  if (a.isDir && !b.isDir) {
    return -1;
  }
  if (!a.isDir && b.isDir) {
    return 1;
  }
  return a.name.localeCompare(b.name);
}
function VsIcon({icon}: {icon: FileIcon}) {
  const webIconStyle = {
    color: icon.color,
    ...styles.webCodicon,
    fontFamily: icon.fontFamily,
  } as const;

  if (Platform.OS === 'web') {
    return <span style={webIconStyle}>{icon.glyph}</span>;
  }

  const nativeFontFamily = icon.fontFamily === 'vscode-codicon' ? 'codicon' : 'seti';
  return <Text style={{color: icon.color, fontFamily: nativeFontFamily}}>{icon.glyph}</Text>;
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
  projectRefreshButton: {
    marginLeft: 8,
  },
  projectRefreshIcon: {
    fontSize: 18,
    lineHeight: 18,
    fontWeight: '700',
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
  quickSettingsMenu: {
    position: 'absolute',
    top: 52,
    right: 8,
    width: 220,
    borderWidth: 1,
    borderRadius: 8,
    zIndex: 25,
    paddingBottom: 8,
  },
  quickSwitchItem: {
    minHeight: 38,
    paddingHorizontal: 10,
    alignItems: 'center',
    flexDirection: 'row',
    justifyContent: 'space-between',
  },
  quickMoreDivider: {
    height: 1,
    marginTop: 6,
    marginBottom: 4,
    marginHorizontal: 10,
  },
  quickMoreButton: {
    minHeight: 32,
    justifyContent: 'center',
    paddingHorizontal: 10,
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
  mainPager: {
    flex: 1,
    flexDirection: 'row',
    minHeight: 0,
  },
  mainPage: {
    flex: 1,
    minHeight: 0,
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
    fontSize: 11,
    lineHeight: 16,
  },
  sideRow: {
    minHeight: 22,
    justifyContent: 'center',
    paddingHorizontal: 10,
  },
  commitRow: {
    minHeight: 40,
    flexDirection: 'row',
    alignItems: 'stretch',
    paddingLeft: 6,
    paddingRight: 10,
  },
  commitGraph: {
    width: 22,
    justifyContent: 'center',
    marginRight: 8,
    position: 'relative',
  },
  commitLaneWrap: {
    flex: 1,
    paddingHorizontal: 6,
  },
  commitLaneLine: {
    width: 1,
    height: '100%',
  },
  commitNode: {
    width: 8,
    height: 8,
    borderRadius: 4,
    position: 'absolute',
    top: '50%',
    marginTop: -4,
  },
  commitTextWrap: {
    flex: 1,
    justifyContent: 'center',
    minHeight: 0,
  },
  commitTitleRow: {
    flexDirection: 'row',
    alignItems: 'center',
  },
  commitBadgeRow: {
    flexDirection: 'row',
    alignItems: 'center',
  },
  branchBadge: {
    marginLeft: 6,
    borderWidth: 1,
    borderRadius: 10,
    paddingHorizontal: 6,
    paddingVertical: 1,
  },
  branchBadgeText: {
    fontSize: 11,
    lineHeight: 14,
    fontWeight: '600',
  },
  treeText: {
    fontSize: 13,
    lineHeight: 20,
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
  settingsDrawer: {
    position: 'absolute',
    right: 0,
    top: 0,
    height: '100%',
    borderLeftWidth: 1,
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
  webCodicon: {
    fontSize: 14,
    verticalAlign: 'middle',
  },
});











