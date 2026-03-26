import 'package:flutter/material.dart';

import '../services/ws_service.dart';
import 'chat_screen.dart';
import 'file_explorer_screen.dart';
import 'git_diff_debug_screen.dart';

enum WorkspaceTab { chat, files, diff }

class WorkspaceDebugScreen extends StatefulWidget {
  const WorkspaceDebugScreen({super.key});

  @override
  State<WorkspaceDebugScreen> createState() => _WorkspaceDebugScreenState();
}

class _WorkspaceDebugScreenState extends State<WorkspaceDebugScreen> {
  late final WsService _previewService;
  WorkspaceTab _selected = WorkspaceTab.chat;

  @override
  void initState() {
    super.initState();
    _previewService = WsService.localPreview();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Workspace (Debug)'),
        actions: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
            child: SegmentedButton<WorkspaceTab>(
              segments: const [
                ButtonSegment(value: WorkspaceTab.chat, label: Text('Chat')),
                ButtonSegment(value: WorkspaceTab.files, label: Text('Files')),
                ButtonSegment(value: WorkspaceTab.diff, label: Text('Diff')),
              ],
              selected: {_selected},
              showSelectedIcon: false,
              onSelectionChanged: (selected) {
                if (selected.isEmpty) return;
                setState(() => _selected = selected.first);
              },
            ),
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
}
