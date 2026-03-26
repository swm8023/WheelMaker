import 'dart:async';

import 'package:flutter/material.dart';

import '../data/project_data_source.dart';
import '../models/file_tree_node.dart';
import '../models/git_diff_models.dart';
import '../models/project_workspace_state.dart';
import '../services/ws_service.dart';
import '../stores/project_workspace_store.dart';
import '../theme/app_theme_controller.dart';
import 'chat_screen.dart';
import 'connect_screen.dart';
import 'file_explorer_screen.dart';
import 'git_diff_debug_screen.dart';

class WorkspaceDebugScreen extends StatefulWidget {
  const WorkspaceDebugScreen({super.key});

  @override
  State<WorkspaceDebugScreen> createState() => _WorkspaceDebugScreenState();
}

class _WorkspaceDebugScreenState extends State<WorkspaceDebugScreen> {
  final _scaffoldKey = GlobalKey<ScaffoldState>();
  static const double _splitBreakpoint = 700;
  static const double _sidebarWidth = 320;

  late final ProjectWorkspaceStore _store;

  @override
  void initState() {
    super.initState();
    _store = ProjectWorkspaceStore(dataSource: MockProjectDataSource())
      ..addListener(_onStoreChanged);
    unawaited(_store.initialize());
  }

  @override
  void dispose() {
    _store
      ..removeListener(_onStoreChanged)
      ..dispose();
    super.dispose();
  }

  void _onStoreChanged() {
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final width = MediaQuery.sizeOf(context).width;
    final isSplit = width >= _splitBreakpoint;
    final compact = width < 560;

    if (!_store.isReady || _store.activeState == null) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }

    final state = _store.activeState!;
    if (isSplit && (_scaffoldKey.currentState?.isDrawerOpen ?? false)) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _scaffoldKey.currentState?.closeDrawer();
      });
    }

    return Scaffold(
      key: _scaffoldKey,
      drawer: Drawer(
        child: SafeArea(child: _buildDrawerContent(state, closeOnSelect: true)),
      ),
      appBar: AppBar(
        automaticallyImplyLeading: false,
        titleSpacing: 8,
        title: Row(
          children: [
            IconButton(
              key: const ValueKey('workspace-sidebar-toggle'),
              icon: Icon(
                isSplit
                    ? (state.ui.sidebarCollapsed
                        ? Icons.keyboard_double_arrow_right
                        : Icons.keyboard_double_arrow_left)
                    : Icons.menu,
              ),
              onPressed: () {
                if (isSplit) {
                  _store.toggleSidebarCollapsed();
                } else {
                  _scaffoldKey.currentState?.openDrawer();
                }
              },
              tooltip: isSplit ? 'Toggle sidebar' : 'Open list',
            ),
            Expanded(child: _buildProjectSelector(compact: compact)),
            const SizedBox(width: 8),
            ConstrainedBox(
              constraints: BoxConstraints(maxWidth: compact ? 176 : 240),
              child: _buildSwitcher(
                compact: compact,
                selected: state.ui.selectedTab,
              ),
            ),
          ],
        ),
        actions: [
          IconButton(
            icon: const Icon(Icons.settings_outlined),
            tooltip: 'Settings',
            onPressed: () {
              Navigator.push(
                context,
                MaterialPageRoute(
                  builder: (_) => const _WorkspaceSettingsScreen(),
                ),
              );
            },
          ),
        ],
      ),
      body: _buildCurrentBody(isSplit, state),
    );
  }

  Widget _buildProjectSelector({required bool compact}) {
    return PopupMenuButton<String>(
      onSelected: (projectId) => unawaited(_store.switchProject(projectId)),
      itemBuilder: (context) {
        return _store.projectList
            .map(
              (project) => PopupMenuItem<String>(
                value: project.id,
                child: Text(project.name),
              ),
            )
            .toList();
      },
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
        child: Row(
          children: [
            Container(
              width: 20,
              height: 20,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(4),
                color: Theme.of(context).colorScheme.surfaceContainerHighest,
              ),
              child: Icon(
                Icons.expand_more,
                size: 16,
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                compact
                    ? _store.activeProjectName
                    : '${_store.activeProjectName} Project',
                maxLines: 1,
                overflow: TextOverflow.fade,
                softWrap: false,
                style: const TextStyle(fontSize: 15),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSwitcher({
    required bool compact,
    required WorkspaceTab selected,
  }) {
    return SegmentedButton<WorkspaceTab>(
      segments: compact
          ? const [
              ButtonSegment(value: WorkspaceTab.chat, label: Text('C')),
              ButtonSegment(value: WorkspaceTab.files, label: Text('F')),
              ButtonSegment(value: WorkspaceTab.diff, label: Text('D')),
            ]
          : const [
              ButtonSegment(value: WorkspaceTab.chat, label: Text('Chat')),
              ButtonSegment(value: WorkspaceTab.files, label: Text('Files')),
              ButtonSegment(value: WorkspaceTab.diff, label: Text('Diff')),
            ],
      selected: {selected},
      showSelectedIcon: false,
      style: ButtonStyle(
        visualDensity: compact ? VisualDensity.compact : VisualDensity.standard,
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
      ),
      onSelectionChanged: (selectedSet) {
        if (selectedSet.isEmpty) return;
        _store.setTab(selectedSet.first);
      },
    );
  }

  Widget _buildDrawerContent(
    ProjectWorkspaceState state, {
    required bool closeOnSelect,
  }) {
    switch (state.ui.selectedTab) {
      case WorkspaceTab.chat:
        return _buildChatList(state, closeOnSelect: closeOnSelect);
      case WorkspaceTab.files:
        return _buildFileDrawerTree(state, closeOnSelect: closeOnSelect);
      case WorkspaceTab.diff:
        return _buildDiffDrawerSplit(state, closeOnSelect: closeOnSelect);
    }
  }

  Widget _buildCurrentBody(bool isSplit, ProjectWorkspaceState state) {
    if (!isSplit) {
      return _buildCurrentMainContent(state);
    }
    return Row(
      children: [
        if (!state.ui.sidebarCollapsed)
          SizedBox(
            width: _sidebarWidth,
            child: _buildDrawerContent(state, closeOnSelect: false),
          ),
        if (!state.ui.sidebarCollapsed) const VerticalDivider(width: 1),
        Expanded(child: _buildCurrentMainContent(state)),
      ],
    );
  }

  Widget _buildCurrentMainContent(ProjectWorkspaceState state) {
    switch (state.ui.selectedTab) {
      case WorkspaceTab.chat:
        return ChatScreen(
          service: WsService.localPreview(),
          showAppBar: false,
          sessionName: state.chat.selectedSession,
        );
      case WorkspaceTab.files:
        return FileExplorerScreen(
          showAppBar: false,
          showSidebar: false,
          selectedPath: state.files.selectedFilePath,
          onFileSelected: _store.selectFile,
        );
      case WorkspaceTab.diff:
        return GitDiffDebugScreen(
          showAppBar: false,
          showSidebar: false,
          selectedCommitIndex: state.diff.selectedCommitIndex,
          selectedFilePath: state.diff.selectedFilePath,
          onCommitSelected: _store.selectDiffCommit,
          onFileSelected: _store.selectDiffFile,
        );
    }
  }

  Widget _buildChatList(
    ProjectWorkspaceState state, {
    required bool closeOnSelect,
  }) {
    return Container(
      key: const ValueKey('workspace-sidebar-chat'),
      color: const Color(0xFF252526),
      child: Column(
        children: [
          _drawerTitle('CHAT LIST'),
          Expanded(
            child: ListView.builder(
              itemCount: state.chat.sessions.length,
              itemBuilder: (context, index) {
                final selected = index == state.chat.selectedSessionIndex;
                return InkWell(
                  key: ValueKey('workspace-chat-row-$index'),
                  onTap: () {
                    _store.selectChatSession(index);
                    if (closeOnSelect && Navigator.of(context).canPop()) {
                      Navigator.pop(context);
                    }
                  },
                  child: Container(
                    color:
                        selected ? const Color(0xFF37373D) : Colors.transparent,
                    padding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 10,
                    ),
                    child: Text(
                      state.chat.sessions[index],
                      style: const TextStyle(
                        color: Color(0xFFD4D4D4),
                        fontSize: 13,
                      ),
                    ),
                  ),
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildFileDrawerTree(
    ProjectWorkspaceState state, {
    required bool closeOnSelect,
  }) {
    return Container(
      key: const ValueKey('workspace-sidebar-files'),
      color: const Color(0xFF252526),
      child: ListView(
        children: [
          _drawerTitle('EXPLORER'),
          const Padding(
            padding: EdgeInsets.fromLTRB(12, 2, 12, 6),
            child: Text(
              'WHEELMAKER',
              style: TextStyle(
                color: Color(0xFFD4D4D4),
                fontWeight: FontWeight.w600,
                fontSize: 11,
              ),
            ),
          ),
          ..._buildFileTreeNodes(
            state.files.root,
            state.files,
            0,
            closeOnSelect,
          ),
        ],
      ),
    );
  }

  List<Widget> _buildFileTreeNodes(
    FileTreeNode node,
    FilePaneState files,
    int depth,
    bool closeOnSelect,
  ) {
    final pad = EdgeInsets.only(left: 10 + depth * 14, right: 8);
    if (!node.isDirectory) {
      final selected = node.path == files.selectedFilePath;
      return [
        InkWell(
          key: ValueKey('workspace-file-row-${node.path}'),
          onTap: () {
            _store.selectFile(node.path);
            if (closeOnSelect && Navigator.of(context).canPop()) {
              Navigator.pop(context);
            }
          },
          child: Container(
            color: selected ? const Color(0xFF37373D) : Colors.transparent,
            padding: pad.add(const EdgeInsets.symmetric(vertical: 5)),
            child: Row(
              children: [
                Icon(
                  _fileIcon(node.path),
                  size: 16,
                  color: _fileColor(node.path),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    node.name,
                    style: const TextStyle(
                      color: Color(0xFFD4D4D4),
                      fontSize: 13,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          ),
        ),
      ];
    }

    final isOpen = files.expandedPaths.contains(node.path);
    final rows = <Widget>[
      InkWell(
        key: ValueKey('workspace-folder-row-${node.path}'),
        onTap: () => _store.toggleFolder(node.path),
        child: Container(
          padding: pad.add(const EdgeInsets.symmetric(vertical: 5)),
          child: Row(
            children: [
              Icon(
                isOpen ? Icons.keyboard_arrow_down : Icons.keyboard_arrow_right,
                size: 16,
                color: const Color(0xFFCCCCCC),
              ),
              Icon(
                isOpen ? Icons.folder_open : Icons.folder,
                size: 16,
                color: const Color(0xFFE8AB53),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  node.name,
                  style: const TextStyle(
                    color: Color(0xFFD4D4D4),
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                  ),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
        ),
      ),
    ];
    if (isOpen) {
      for (final child in node.children) {
        rows.addAll(
          _buildFileTreeNodes(child, files, depth + 1, closeOnSelect),
        );
      }
    }
    return rows;
  }

  Widget _buildDiffDrawerSplit(
    ProjectWorkspaceState state, {
    required bool closeOnSelect,
  }) {
    if (state.diff.commits.isEmpty) {
      return Container(
        key: const ValueKey('workspace-sidebar-diff'),
        color: const Color(0xFF252526),
        alignment: Alignment.center,
        child: const Text(
          'No commits',
          style: TextStyle(color: Color(0xFFD4D4D4)),
        ),
      );
    }

    final commit = state.diff.commits[state.diff.selectedCommitIndex];
    return Container(
      key: const ValueKey('workspace-sidebar-diff'),
      color: const Color(0xFF252526),
      child: Column(
        children: [
          _drawerTitle('COMMITS'),
          Expanded(
            flex: 5,
            child: ListView.builder(
              itemCount: state.diff.commits.length,
              itemBuilder: (context, index) {
                final item = state.diff.commits[index];
                final selected = index == state.diff.selectedCommitIndex;
                return ListTile(
                  key: ValueKey('workspace-commit-row-${item.hash}'),
                  tileColor:
                      selected ? const Color(0xFF37373D) : Colors.transparent,
                  dense: true,
                  title: Text(
                    '${item.hash.substring(0, 7)} ${item.message}',
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      color: Color(0xFFD4D4D4),
                      fontSize: 12,
                    ),
                  ),
                  subtitle: Text(
                    '${item.files.length} files',
                    style: const TextStyle(
                      color: Color(0xFF9DA0A6),
                      fontSize: 11,
                    ),
                  ),
                  onTap: () => _store.selectDiffCommit(index),
                );
              },
            ),
          ),
          const Divider(height: 1),
          _drawerTitle('CHANGED FILES'),
          Expanded(
            flex: 5,
            child: ListView.builder(
              itemCount: commit.files.length,
              itemBuilder: (context, index) {
                final file = commit.files[index];
                final selected = file.path == state.diff.selectedFilePath;
                return InkWell(
                  key: ValueKey('workspace-diff-file-row-${file.path}'),
                  onTap: () {
                    _store.selectDiffFile(file.path);
                    if (closeOnSelect && Navigator.of(context).canPop()) {
                      Navigator.pop(context);
                    }
                  },
                  child: Container(
                    color:
                        selected ? const Color(0xFF37373D) : Colors.transparent,
                    padding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 9,
                    ),
                    child: Row(
                      children: [
                        _statusBadge(file.status),
                        const SizedBox(width: 8),
                        Expanded(
                          child: Text(
                            file.path,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(
                              color: Color(0xFFD4D4D4),
                              fontSize: 12,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  Widget _drawerTitle(String text) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.fromLTRB(12, 10, 12, 8),
      child: Text(
        text,
        style: const TextStyle(
          color: Color(0xFFBBBBBB),
          fontWeight: FontWeight.w600,
          letterSpacing: 1.1,
          fontSize: 11,
        ),
      ),
    );
  }

  Widget _statusBadge(GitFileStatus status) {
    final (label, color) = switch (status) {
      GitFileStatus.added => ('A', const Color(0xFF2EA043)),
      GitFileStatus.modified => ('M', const Color(0xFF9E6A03)),
      GitFileStatus.deleted => ('D', const Color(0xFFF85149)),
      GitFileStatus.renamed => ('R', const Color(0xFF1F6FEB)),
    };
    return Container(
      width: 18,
      height: 18,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.2),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color.withValues(alpha: 0.55)),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: color,
          fontSize: 10,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }

  IconData _fileIcon(String path) {
    final lower = path.toLowerCase();
    if (lower.endsWith('.dart')) return Icons.flutter_dash;
    if (lower.endsWith('.go')) return Icons.memory_outlined;
    if (_isCppFile(lower)) return Icons.code;
    if (lower.endsWith('.yaml') || lower.endsWith('.yml')) {
      return Icons.settings_applications_outlined;
    }
    if (lower.endsWith('.json')) return Icons.data_object;
    if (lower.endsWith('.md')) return Icons.article_outlined;
    if (lower.endsWith('.ps1')) return Icons.terminal_outlined;
    return Icons.description_outlined;
  }

  Color _fileColor(String path) {
    final lower = path.toLowerCase();
    if (lower.endsWith('.dart')) return const Color(0xFF42A5F5);
    if (lower.endsWith('.go')) return const Color(0xFF00ADD8);
    if (_isCppFile(lower)) return const Color(0xFF649AD2);
    if (lower.endsWith('.yaml') || lower.endsWith('.yml')) {
      return const Color(0xFFCB9B41);
    }
    if (lower.endsWith('.json')) return const Color(0xFFF1D04B);
    if (lower.endsWith('.md')) return const Color(0xFF519ABA);
    if (lower.endsWith('.ps1')) return const Color(0xFF4EC9B0);
    return const Color(0xFFCCCCCC);
  }

  bool _isCppFile(String lowerPath) {
    return lowerPath.endsWith('.cpp') ||
        lowerPath.endsWith('.cc') ||
        lowerPath.endsWith('.cxx') ||
        lowerPath.endsWith('.c') ||
        lowerPath.endsWith('.hpp') ||
        lowerPath.endsWith('.hh') ||
        lowerPath.endsWith('.hxx') ||
        lowerPath.endsWith('.h');
  }
}

class _WorkspaceSettingsScreen extends StatelessWidget {
  const _WorkspaceSettingsScreen();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Settings')),
      body: ListView(
        children: [
          const ListTile(
            title: Text('Workspace Settings'),
            subtitle: Text('Debug settings and session actions'),
          ),
          const Divider(height: 1),
          ListTile(
            leading: const Icon(Icons.contrast_outlined),
            title: const Text('UI Theme'),
            subtitle: DropdownButtonHideUnderline(
              child: DropdownButton<UiThemeMode>(
                value: AppThemeScope.of(context).uiMode,
                isExpanded: true,
                items: const [
                  DropdownMenuItem(
                    value: UiThemeMode.system,
                    child: Text('System'),
                  ),
                  DropdownMenuItem(
                    value: UiThemeMode.light,
                    child: Text('Light'),
                  ),
                  DropdownMenuItem(
                    value: UiThemeMode.dark,
                    child: Text('Dark'),
                  ),
                ],
                onChanged: (value) {
                  if (value != null) {
                    AppThemeScope.of(context).setUiMode(value);
                  }
                },
              ),
            ),
          ),
          ListTile(
            leading: const Icon(Icons.code_outlined),
            title: const Text('Editor Theme'),
            subtitle: DropdownButtonHideUnderline(
              child: DropdownButton<EditorThemePreset>(
                value: AppThemeScope.of(context).editorTheme,
                isExpanded: true,
                items: const [
                  DropdownMenuItem(
                    value: EditorThemePreset.vscodeDark,
                    child: Text('VS Code Modern Dark'),
                  ),
                  DropdownMenuItem(
                    value: EditorThemePreset.vscodeLight,
                    child: Text('VS Code Light+'),
                  ),
                  DropdownMenuItem(
                    value: EditorThemePreset.vscodeHighContrast,
                    child: Text('VS Code High Contrast'),
                  ),
                ],
                onChanged: (value) {
                  if (value != null) {
                    AppThemeScope.of(context).setEditorTheme(value);
                  }
                },
              ),
            ),
          ),
          const Divider(height: 1),
          ListTile(
            leading: const Icon(Icons.logout),
            title: const Text('Back To Login'),
            onTap: () {
              Navigator.pushAndRemoveUntil(
                context,
                MaterialPageRoute(builder: (_) => const ConnectScreen()),
                (route) => false,
              );
            },
          ),
        ],
      ),
    );
  }
}
