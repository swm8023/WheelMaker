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
    final width = MediaQuery.sizeOf(context).width;
    final compact = width < 760;
    return Scaffold(
      appBar: AppBar(
        titleSpacing: 12,
        title: Row(
          children: [
            Text(compact ? 'Workspace' : 'Workspace (Debug)'),
            const SizedBox(width: 8),
            Expanded(
              child: Align(
                alignment: Alignment.centerRight,
                child: _buildSwitcher(compact: compact),
              ),
            ),
          ],
        ),
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
}
