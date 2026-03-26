import 'package:flutter/material.dart';

import '../data/mock_git_diff_data.dart';
import '../data/mock_wheelmaker_fs.dart';
import '../models/file_tree_node.dart';
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
  late final WsService _previewService;
  WorkspaceTab _selected = WorkspaceTab.chat;
  int _selectedChatIndex = 0;

  @override
  void initState() {
    super.initState();
    _previewService = WsService.localPreview();
  }

  @override
  Widget build(BuildContext context) {
    final width = MediaQuery.sizeOf(context).width;
    final compact = width < 760;
    return Scaffold(
      key: _scaffoldKey,
      drawer: Drawer(
        child: SafeArea(child: _buildDrawerContent()),
      ),
      appBar: AppBar(
        automaticallyImplyLeading: false,
        titleSpacing: 12,
        title: Row(
          children: [
            IconButton(
              icon: const Icon(Icons.menu),
              onPressed: () => _scaffoldKey.currentState?.openDrawer(),
              tooltip: 'Open list',
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
              constraints: BoxConstraints(maxWidth: compact ? 180 : 320),
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
      body: IndexedStack(
        index: _selected.index,
        children: [
          ChatScreen(service: _previewService, showAppBar: false),
          const FileExplorerScreen(showAppBar: false),
          const GitDiffDebugScreen(showAppBar: false),
        ],
      ),
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

  Widget _buildDrawerContent() {
    switch (_selected) {
      case WorkspaceTab.chat:
        return _buildChatList();
      case WorkspaceTab.files:
        return _buildFileList();
      case WorkspaceTab.diff:
        return _buildDiffList();
    }
  }

  Widget _buildChatList() {
    const chats = [
      'General',
      'WheelMaker App',
      'Go Service',
      'Review Notes',
    ];
    return Container(
      color: const Color(0xFF252526),
      child: Column(
        children: [
          _drawerTitle('CHAT LIST'),
          Expanded(
            child: ListView.builder(
              itemCount: chats.length,
              itemBuilder: (context, index) {
                final selected = index == _selectedChatIndex;
                return InkWell(
                  onTap: () {
                    setState(() => _selectedChatIndex = index);
                    Navigator.pop(context);
                  },
                  child: Container(
                    color: selected ? const Color(0xFF37373D) : Colors.transparent,
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                    child: Text(
                      chats[index],
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

  Widget _buildFileList() {
    final files = <FileTreeNode>[];
    _collectFiles(mockWheelMakerRoot, files);
    return Container(
      color: const Color(0xFF252526),
      child: Column(
        children: [
          _drawerTitle('FILES'),
          Expanded(
            child: ListView.builder(
              itemCount: files.length,
              itemBuilder: (context, index) {
                final file = files[index];
                return ListTile(
                  dense: true,
                  title: Text(
                    file.path,
                    style: const TextStyle(color: Color(0xFFD4D4D4), fontSize: 12),
                    overflow: TextOverflow.ellipsis,
                  ),
                  onTap: () => Navigator.pop(context),
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildDiffList() {
    return Container(
      color: const Color(0xFF252526),
      child: Column(
        children: [
          _drawerTitle('COMMITS'),
          Expanded(
            child: ListView.builder(
              itemCount: mockGitCommits.length,
              itemBuilder: (context, index) {
                final commit = mockGitCommits[index];
                return ListTile(
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
                  onTap: () => Navigator.pop(context),
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

  void _collectFiles(FileTreeNode node, List<FileTreeNode> out) {
    if (!node.isDirectory) {
      out.add(node);
      return;
    }
    for (final child in node.children) {
      _collectFiles(child, out);
    }
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
