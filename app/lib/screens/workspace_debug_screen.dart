import 'package:flutter/material.dart';

import '../data/mock_git_diff_data.dart';
import '../data/mock_wheelmaker_fs.dart';
import '../models/file_tree_node.dart';
import '../models/git_diff_models.dart';
import '../services/ws_service.dart';
import 'chat_screen.dart';
import 'connect_screen.dart';
import 'file_explorer_screen.dart';
import 'git_diff_debug_screen.dart';

enum WorkspaceTab { chat, files, diff }

class WorkspaceDebugScreen extends StatefulWidget {
  const WorkspaceDebugScreen({super.key});

  @override
  State<WorkspaceDebugScreen> createState() => _WorkspaceDebugScreenState();
}

class _WorkspaceDebugScreenState extends State<WorkspaceDebugScreen> {
  final _scaffoldKey = GlobalKey<ScaffoldState>();
  static const double _splitBreakpoint = 700;
  static const double _sidebarWidth = 320;
  late final WsService _previewService;
  WorkspaceTab _selected = WorkspaceTab.chat;
  static const List<String> _chatSessions = [
    'General',
    'WheelMaker App',
    'Go Service',
    'Review Notes',
  ];
  int _selectedChatIndex = 0;
  bool _sidebarCollapsed = false;
  final Set<String> _fileDrawerExpanded = {'/WheelMaker', '/WheelMaker/app'};
  String? _selectedFilePath;
  int _diffDrawerCommitIndex = 0;
  String? _diffDrawerFilePath;

  @override
  void initState() {
    super.initState();
    _previewService = WsService.localPreview();
    _selectedFilePath = _firstFilePath(mockWheelMakerRoot);
    _diffDrawerFilePath = mockGitCommits.first.files.first.path;
  }

  @override
  Widget build(BuildContext context) {
    final width = MediaQuery.sizeOf(context).width;
    final isSplit = width >= _splitBreakpoint;
    final compact = width < 560;
    return Scaffold(
      key: _scaffoldKey,
      drawer: Drawer(
        child: SafeArea(child: _buildDrawerContent(closeOnSelect: true)),
      ),
      appBar: AppBar(
        automaticallyImplyLeading: false,
        titleSpacing: 12,
        title: Row(
          children: [
            IconButton(
              key: const ValueKey('workspace-sidebar-toggle'),
              icon: Icon(
                isSplit
                    ? (_sidebarCollapsed ? Icons.chevron_right : Icons.chevron_left)
                    : Icons.menu,
              ),
              onPressed: () {
                if (isSplit) {
                  setState(() => _sidebarCollapsed = !_sidebarCollapsed);
                } else {
                  _scaffoldKey.currentState?.openDrawer();
                }
              },
              tooltip: isSplit ? 'Toggle sidebar' : 'Open list',
            ),
            Expanded(
              child: Text(
                compact ? 'WheelMaker Project' : 'WheelMaker Project Workspace',
                maxLines: 1,
                softWrap: false,
                overflow: TextOverflow.fade,
                style: const TextStyle(fontSize: 15),
              ),
            ),
            const SizedBox(width: 8),
            ConstrainedBox(
              constraints: BoxConstraints(maxWidth: compact ? 180 : 240),
              child: _buildSwitcher(compact: compact),
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
                MaterialPageRoute(builder: (_) => const _WorkspaceSettingsScreen()),
              );
            },
          ),
        ],
      ),
      body: _buildCurrentBody(isSplit),
    );
  }

  Widget _buildSwitcher({required bool compact}) {
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
      selected: {_selected},
      showSelectedIcon: false,
      style: ButtonStyle(
        visualDensity: compact ? VisualDensity.compact : VisualDensity.standard,
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
      ),
      onSelectionChanged: (selected) {
        if (selected.isEmpty) return;
        setState(() => _selected = selected.first);
      },
    );
  }

  Widget _buildDrawerContent({required bool closeOnSelect}) {
    switch (_selected) {
      case WorkspaceTab.chat:
        return _buildChatList(closeOnSelect: closeOnSelect);
      case WorkspaceTab.files:
        return _buildFileDrawerTree(closeOnSelect: closeOnSelect);
      case WorkspaceTab.diff:
        return _buildDiffDrawerSplit(closeOnSelect: closeOnSelect);
    }
  }

  Widget _buildCurrentBody(bool isSplit) {
    if (!isSplit) {
      return _buildCurrentMainContent();
    }
    return Row(
      children: [
        if (!_sidebarCollapsed)
          SizedBox(
            width: _sidebarWidth,
            child: _buildDrawerContent(closeOnSelect: false),
          ),
        if (!_sidebarCollapsed) const VerticalDivider(width: 1),
        Expanded(child: _buildCurrentMainContent()),
      ],
    );
  }

  Widget _buildCurrentMainContent() {
    switch (_selected) {
      case WorkspaceTab.chat:
        return ChatScreen(
          service: _previewService,
          showAppBar: false,
          sessionName: _chatSessions[_selectedChatIndex],
        );
      case WorkspaceTab.files:
        return FileExplorerScreen(
          showAppBar: false,
          showSidebar: false,
          selectedPath: _selectedFilePath,
          onFileSelected: (path) => setState(() => _selectedFilePath = path),
        );
      case WorkspaceTab.diff:
        return GitDiffDebugScreen(
          showAppBar: false,
          showSidebar: false,
          selectedCommitIndex: _diffDrawerCommitIndex,
          selectedFilePath: _diffDrawerFilePath,
          onCommitSelected: (index) => setState(() => _diffDrawerCommitIndex = index),
          onFileSelected: (path) => setState(() => _diffDrawerFilePath = path),
        );
    }
  }

  Widget _buildChatList({required bool closeOnSelect}) {
    return Container(
      key: const ValueKey('workspace-sidebar-chat'),
      color: const Color(0xFF252526),
      child: Column(
        children: [
          _drawerTitle('CHAT LIST'),
          Expanded(
            child: ListView.builder(
              itemCount: _chatSessions.length,
              itemBuilder: (context, index) {
                final selected = index == _selectedChatIndex;
                return InkWell(
                  key: ValueKey('workspace-chat-row-$index'),
                  onTap: () {
                    setState(() => _selectedChatIndex = index);
                    if (closeOnSelect && Navigator.of(context).canPop()) {
                      Navigator.pop(context);
                    }
                  },
                  child: Container(
                    color: selected ? const Color(0xFF37373D) : Colors.transparent,
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                    child: Text(
                      _chatSessions[index],
                      style: const TextStyle(color: Color(0xFFD4D4D4), fontSize: 13),
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

  Widget _buildFileDrawerTree({required bool closeOnSelect}) {
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
          ..._buildFileTreeNodes(mockWheelMakerRoot, 0, closeOnSelect),
        ],
      ),
    );
  }

  List<Widget> _buildFileTreeNodes(FileTreeNode node, int depth, bool closeOnSelect) {
    final pad = EdgeInsets.only(left: 10 + depth * 14, right: 8);
    if (!node.isDirectory) {
      final selected = node.path == _selectedFilePath;
      return [
        InkWell(
          key: ValueKey('workspace-file-row-${node.path}'),
          onTap: () {
            setState(() => _selectedFilePath = node.path);
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
                    style: const TextStyle(color: Color(0xFFD4D4D4), fontSize: 13),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          ),
        ),
      ];
    }

    final isOpen = _fileDrawerExpanded.contains(node.path);
    final rows = <Widget>[
      InkWell(
        key: ValueKey('workspace-folder-row-${node.path}'),
        onTap: () {
          setState(() {
            if (isOpen) {
              _fileDrawerExpanded.remove(node.path);
            } else {
              _fileDrawerExpanded.add(node.path);
            }
          });
        },
        child: Container(
          padding: pad.add(const EdgeInsets.symmetric(vertical: 5)),
          child: Row(
            children: [
              Icon(isOpen ? Icons.keyboard_arrow_down : Icons.keyboard_arrow_right,
                  size: 16, color: const Color(0xFFCCCCCC)),
              Icon(isOpen ? Icons.folder_open : Icons.folder, size: 16, color: const Color(0xFFE8AB53)),
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
        rows.addAll(_buildFileTreeNodes(child, depth + 1, closeOnSelect));
      }
    }
    return rows;
  }

  Widget _buildDiffDrawerSplit({required bool closeOnSelect}) {
    final commit = mockGitCommits[_diffDrawerCommitIndex];
    return Container(
      key: const ValueKey('workspace-sidebar-diff'),
      color: const Color(0xFF252526),
      child: Column(
        children: [
          _drawerTitle('COMMITS'),
          Expanded(
            flex: 5,
            child: ListView.builder(
              itemCount: mockGitCommits.length,
              itemBuilder: (context, index) {
                final commit = mockGitCommits[index];
                final selected = index == _diffDrawerCommitIndex;
                return ListTile(
                  key: ValueKey('workspace-commit-row-${commit.hash}'),
                  tileColor: selected ? const Color(0xFF37373D) : Colors.transparent,
                  dense: true,
                  title: Text(
                    '${commit.hash.substring(0, 7)} ${commit.message}',
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(color: Color(0xFFD4D4D4), fontSize: 12),
                  ),
                  subtitle: Text(
                    '${commit.files.length} files',
                    style: const TextStyle(color: Color(0xFF9DA0A6), fontSize: 11),
                  ),
                  onTap: () {
                    setState(() {
                      _diffDrawerCommitIndex = index;
                      _diffDrawerFilePath =
                          commit.files.isNotEmpty ? commit.files.first.path : null;
                    });
                  },
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
                final selected = file.path == _diffDrawerFilePath;
                return InkWell(
                  key: ValueKey('workspace-diff-file-row-${file.path}'),
                  onTap: () {
                    setState(() => _diffDrawerFilePath = file.path);
                    if (closeOnSelect && Navigator.of(context).canPop()) {
                      Navigator.pop(context);
                    }
                  },
                  child: Container(
                    color: selected ? const Color(0xFF37373D) : Colors.transparent,
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
                    child: Row(
                      children: [
                        _statusBadge(file.status),
                        const SizedBox(width: 8),
                        Expanded(
                          child: Text(
                            file.path,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(color: Color(0xFFD4D4D4), fontSize: 12),
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
        style: TextStyle(color: color, fontSize: 10, fontWeight: FontWeight.w700),
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

  String? _firstFilePath(FileTreeNode node) {
    if (!node.isDirectory) return node.path;
    for (final child in node.children) {
      final found = _firstFilePath(child);
      if (found != null) return found;
    }
    return null;
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
