import 'package:flutter/material.dart';
import 'package:flutter_highlight/flutter_highlight.dart';

import '../data/mock_wheelmaker_fs.dart';
import '../models/file_tree_node.dart';
import 'code_language.dart';

class FileExplorerScreen extends StatefulWidget {
  const FileExplorerScreen({super.key});

  @override
  State<FileExplorerScreen> createState() => _FileExplorerScreenState();
}

class _FileExplorerScreenState extends State<FileExplorerScreen> {
  final Set<String> _expanded = {'/WheelMaker', '/WheelMaker/app'};
  FileTreeNode? _activeFile;
  String? _hoveredPath;

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
      final hovered = _hoveredPath == node.path;
      final rowColor = selected
          ? const Color(0xFF37373D)
          : hovered
              ? const Color(0xFF2A2D2E)
              : Colors.transparent;
      return [
        MouseRegion(
          onEnter: (_) => setState(() => _hoveredPath = node.path),
          onExit: (_) => setState(() => _hoveredPath = null),
          child: InkWell(
            key: ValueKey('file-row-${node.path}'),
            onTap: () => setState(() => _activeFile = node),
            child: Container(
              color: rowColor,
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
        ),
      ];
    }

    final isOpen = _expanded.contains(node.path);
    final hovered = _hoveredPath == node.path;
    final widgets = <Widget>[
      MouseRegion(
        onEnter: (_) => setState(() => _hoveredPath = node.path),
        onExit: (_) => setState(() => _hoveredPath = null),
        child: InkWell(
          key: ValueKey('folder-row-${node.path}'),
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
            color: hovered ? const Color(0xFF2A2D2E) : Colors.transparent,
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
            child: _buildCodeView(file),
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

  Widget _buildCodeView(FileTreeNode file) {
    final content = file.content ?? '';
    final language = languageFromPath(file.path);
    try {
      return SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: HighlightView(
          content,
          language: language,
          theme: _vscodeTheme,
          textStyle: const TextStyle(
            fontFamily: 'Consolas',
            fontSize: 13,
            height: 1.45,
            color: Color(0xFFD4D4D4),
          ),
          padding: EdgeInsets.zero,
        ),
      );
    } catch (_) {
      return SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: SelectableText(
          content,
          style: const TextStyle(
            fontFamily: 'Consolas',
            fontSize: 13,
            height: 1.45,
            color: Color(0xFFD4D4D4),
          ),
        ),
      );
    }
  }

  static final Map<String, TextStyle> _vscodeTheme = {
    'root': const TextStyle(color: Color(0xFFD4D4D4), backgroundColor: Color(0xFF1E1E1E)),
    'keyword': const TextStyle(color: Color(0xFF569CD6)),
    'built_in': const TextStyle(color: Color(0xFF4EC9B0)),
    'type': const TextStyle(color: Color(0xFF4EC9B0)),
    'literal': const TextStyle(color: Color(0xFF569CD6)),
    'number': const TextStyle(color: Color(0xFFB5CEA8)),
    'string': const TextStyle(color: Color(0xFFCE9178)),
    'subst': const TextStyle(color: Color(0xFFD4D4D4)),
    'comment': const TextStyle(color: Color(0xFF6A9955)),
    'title': const TextStyle(color: Color(0xFFDCDCAA)),
    'section': const TextStyle(color: Color(0xFFDCDCAA)),
    'attribute': const TextStyle(color: Color(0xFF9CDCFE)),
    'meta': const TextStyle(color: Color(0xFF9CDCFE)),
    'selector-tag': const TextStyle(color: Color(0xFF569CD6)),
    'selector-id': const TextStyle(color: Color(0xFFD7BA7D)),
    'selector-class': const TextStyle(color: Color(0xFFD7BA7D)),
    'symbol': const TextStyle(color: Color(0xFFB5CEA8)),
    'bullet': const TextStyle(color: Color(0xFFB5CEA8)),
    'link': const TextStyle(color: Color(0xFF3794FF)),
    'emphasis': const TextStyle(fontStyle: FontStyle.italic),
    'strong': const TextStyle(fontWeight: FontWeight.w700),
  };
}
