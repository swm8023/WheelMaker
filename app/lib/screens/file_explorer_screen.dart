import 'package:flutter/material.dart';
import 'package:flutter_highlight/flutter_highlight.dart';
import 'package:flutter_highlight/themes/vs2015.dart';

import '../data/mock_wheelmaker_fs.dart';
import '../models/file_tree_node.dart';

class FileExplorerScreen extends StatefulWidget {
  const FileExplorerScreen({super.key});

  @override
  State<FileExplorerScreen> createState() => _FileExplorerScreenState();
}

class _FileExplorerScreenState extends State<FileExplorerScreen> {
  final Set<String> _expanded = {'/WheelMaker', '/WheelMaker/app'};
  FileTreeNode? _activeFile;

  @override
  void initState() {
    super.initState();
    _activeFile = _firstFile(mockWheelMakerRoot);
  }

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final useSplit = constraints.maxWidth >= 900;
        return Scaffold(
          appBar: AppBar(
            title: const Text('Explorer (Debug)'),
          ),
          drawer: useSplit
              ? null
              : Drawer(
                  child: SafeArea(child: _buildTreePane()),
                ),
          body: useSplit
              ? Row(
                  children: [
                    SizedBox(width: 320, child: _buildTreePane()),
                    const VerticalDivider(width: 1),
                    Expanded(child: _buildEditorPane()),
                  ],
                )
              : _buildEditorPane(),
        );
      },
    );
  }

  Widget _buildTreePane() {
    return Container(
      color: const Color(0xFF252526),
      child: ListView(
        children: [
          const Padding(
            padding: EdgeInsets.fromLTRB(12, 12, 12, 8),
            child: Text(
              'EXPLORER',
              style: TextStyle(
                color: Color(0xFFBBBBBB),
                fontWeight: FontWeight.w600,
                letterSpacing: 1.1,
                fontSize: 12,
              ),
            ),
          ),
          ..._buildTreeNodes(mockWheelMakerRoot, 0),
        ],
      ),
    );
  }

  List<Widget> _buildTreeNodes(FileTreeNode node, int depth) {
    final pad = EdgeInsets.only(left: 10 + depth * 14, right: 8);
    if (!node.isDirectory) {
      final selected = _activeFile?.path == node.path;
      return [
        InkWell(
          onTap: () => setState(() => _activeFile = node),
          child: Container(
            color: selected ? const Color(0xFF37373D) : Colors.transparent,
            padding: pad.add(const EdgeInsets.symmetric(vertical: 6)),
            child: Row(
              children: [
                const Icon(Icons.description_outlined,
                    size: 16, color: Color(0xFFCCCCCC)),
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

    final isOpen = _expanded.contains(node.path);
    final widgets = <Widget>[
      InkWell(
        onTap: () {
          setState(() {
            if (isOpen) {
              _expanded.remove(node.path);
            } else {
              _expanded.add(node.path);
            }
          });
        },
        child: Container(
          padding: pad.add(const EdgeInsets.symmetric(vertical: 6)),
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
        widgets.addAll(_buildTreeNodes(child, depth + 1));
      }
    }
    return widgets;
  }

  Widget _buildEditorPane() {
    final file = _activeFile;
    if (file == null) {
      return const Center(
        child: Text('Select a file'),
      );
    }

    return Container(
      color: const Color(0xFF1E1E1E),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(
            color: const Color(0xFF2D2D2D),
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            child: Row(
              children: [
                const Icon(Icons.insert_drive_file_outlined,
                    size: 16, color: Color(0xFFD4D4D4)),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    file.path,
                    style: const TextStyle(color: Color(0xFFD4D4D4), fontSize: 12),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          ),
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: HighlightView(
                file.content ?? '',
                language: _languageFromPath(file.path),
                theme: vs2015Theme,
                textStyle: const TextStyle(
                  fontFamily: 'Consolas',
                  fontSize: 13,
                  height: 1.45,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  FileTreeNode? _firstFile(FileTreeNode node) {
    if (!node.isDirectory) return node;
    for (final child in node.children) {
      final found = _firstFile(child);
      if (found != null) return found;
    }
    return null;
  }

  String _languageFromPath(String path) {
    if (path.endsWith('.dart')) return 'dart';
    if (path.endsWith('.go')) return 'go';
    if (path.endsWith('.json')) return 'json';
    if (path.endsWith('.yaml') || path.endsWith('.yml')) return 'yaml';
    if (path.endsWith('.md')) return 'markdown';
    if (path.endsWith('.ps1')) return 'powershell';
    return 'plaintext';
  }
}
