import 'package:flutter/material.dart';
import 'package:flutter_highlight/flutter_highlight.dart';
import 'package:google_fonts/google_fonts.dart';

import '../data/mock_wheelmaker_fs.dart';
import '../models/file_tree_node.dart';
import '../theme/app_theme_controller.dart';
import 'code_language.dart';

class FileExplorerScreen extends StatefulWidget {
  final bool showAppBar;
  final bool showSidebar;
  final String? selectedPath;
  final ValueChanged<String>? onFileSelected;

  const FileExplorerScreen({
    super.key,
    this.showAppBar = true,
    this.showSidebar = true,
    this.selectedPath,
    this.onFileSelected,
  });

  @override
  State<FileExplorerScreen> createState() => FileExplorerScreenState();
}

class FileExplorerScreenState extends State<FileExplorerScreen> {
  final Set<String> _expanded = {'/WheelMaker', '/WheelMaker/app'};
  FileTreeNode? _activeFile;
  String? _hoveredPath;

  @override
  void initState() {
    super.initState();
    _activeFile = widget.selectedPath == null
        ? _firstFile(mockWheelMakerRoot)
        : _findByPath(mockWheelMakerRoot, widget.selectedPath!);
    _activeFile ??= _firstFile(mockWheelMakerRoot);
  }

  @override
  void didUpdateWidget(covariant FileExplorerScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.selectedPath != null &&
        widget.selectedPath != oldWidget.selectedPath) {
      final found = _findByPath(mockWheelMakerRoot, widget.selectedPath!);
      if (found != null && _activeFile?.path != found.path) {
        setState(() => _activeFile = found);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final useSplit = widget.showSidebar && constraints.maxWidth >= 900;
        return Scaffold(
          appBar: widget.showAppBar
              ? AppBar(title: const Text('Explorer (Debug)'))
              : null,
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

  bool get _isDark => Theme.of(context).brightness == Brightness.dark;

  Widget _buildTreePane() {
    final textColor = Theme.of(context).colorScheme.onSurface;
    final titleColor = Theme.of(context).colorScheme.onSurfaceVariant;
    return Container(
      color: _isDark ? const Color(0xFF252526) : const Color(0xFFF3F3F3),
      child: ListView(
        children: [
          Container(
            padding: const EdgeInsets.fromLTRB(12, 10, 10, 8),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    'EXPLORER',
                    style: TextStyle(
                      color: titleColor,
                      fontWeight: FontWeight.w600,
                      letterSpacing: 1.1,
                      fontSize: 11,
                    ),
                  ),
                ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 2, 12, 6),
            child: Text(
              'WHEELMAKER',
              style: TextStyle(
                color: textColor,
                fontWeight: FontWeight.w600,
                fontSize: 11,
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
          ? (_isDark ? const Color(0xFF37373D) : const Color(0xFFDCDCDC))
          : hovered
              ? (_isDark ? const Color(0xFF2A2D2E) : const Color(0xFFEAEAEA))
              : Colors.transparent;
      return [
        MouseRegion(
          onEnter: (_) => setState(() => _hoveredPath = node.path),
          onExit: (_) => setState(() => _hoveredPath = null),
          child: InkWell(
            key: ValueKey('file-row-${node.path}'),
            onTap: () => _setActiveFile(node, notify: true),
            child: Container(
              color: rowColor,
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
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.onSurface,
                        fontSize: 13,
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
            color: hovered
                ? (_isDark ? const Color(0xFF2A2D2E) : const Color(0xFFEAEAEA))
                : Colors.transparent,
            padding: pad.add(const EdgeInsets.symmetric(vertical: 5)),
            child: Row(
              children: [
                Icon(
                  isOpen
                      ? Icons.keyboard_arrow_down
                      : Icons.keyboard_arrow_right,
                  size: 16,
                  color: Theme.of(context).colorScheme.onSurfaceVariant,
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
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.onSurface,
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
      color: _isDark ? const Color(0xFF1E1E1E) : const Color(0xFFFFFFFF),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(
            color: _isDark ? const Color(0xFF2D2D2D) : const Color(0xFFF8F8F8),
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            child: Row(
              children: [
                Icon(Icons.insert_drive_file_outlined,
                    size: 16, color: Theme.of(context).colorScheme.onSurface),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    file.path,
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.onSurface,
                      fontSize: 12,
                    ),
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

  FileTreeNode? _findByPath(FileTreeNode node, String path) {
    if (!node.isDirectory) {
      return node.path == path ? node : null;
    }
    for (final child in node.children) {
      final found = _findByPath(child, path);
      if (found != null) return found;
    }
    return null;
  }

  void _setActiveFile(FileTreeNode file, {required bool notify}) {
    setState(() => _activeFile = file);
    if (notify) {
      widget.onFileSelected?.call(file.path);
    }
  }

  Widget _buildCodeView(FileTreeNode file) {
    final content = file.content ?? '';
    final language = languageFromPath(file.path);
    final themeCtrl = AppThemeScope.maybeOf(context);
    try {
      final editorFont = GoogleFonts.jetBrainsMono(
        fontSize: 13,
        height: 1.45,
        color: _isDark ? const Color(0xFFD4D4D4) : const Color(0xFF1F1F1F),
      );
      return SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: HighlightView(
          content,
          language: language,
          theme:
              _themeFor(themeCtrl?.editorTheme ?? EditorThemePreset.vscodeDark),
          textStyle: editorFont,
          padding: EdgeInsets.zero,
        ),
      );
    } catch (_) {
      return SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: SelectableText(
          content,
          style: GoogleFonts.jetBrainsMono(
            fontSize: 13,
            height: 1.45,
            color: _isDark ? const Color(0xFFD4D4D4) : const Color(0xFF1F1F1F),
          ),
        ),
      );
    }
  }

  Map<String, TextStyle> _themeFor(EditorThemePreset preset) {
    switch (preset) {
      case EditorThemePreset.vscodeLight:
        return _vscodeLightTheme;
      case EditorThemePreset.vscodeHighContrast:
        return _vscodeHighContrastTheme;
      case EditorThemePreset.vscodeDark:
        return _vscodeDarkTheme;
    }
  }

  static final Map<String, TextStyle> _vscodeDarkTheme = {
    'root': const TextStyle(
        color: Color(0xFFD4D4D4), backgroundColor: Color(0xFF1E1E1E)),
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
  };

  static final Map<String, TextStyle> _vscodeLightTheme = {
    'root': const TextStyle(
        color: Color(0xFF1F1F1F), backgroundColor: Color(0xFFFFFFFF)),
    'keyword': const TextStyle(color: Color(0xFF0000FF)),
    'built_in': const TextStyle(color: Color(0xFF267F99)),
    'type': const TextStyle(color: Color(0xFF267F99)),
    'literal': const TextStyle(color: Color(0xFF0000FF)),
    'number': const TextStyle(color: Color(0xFF098658)),
    'string': const TextStyle(color: Color(0xFFA31515)),
    'subst': const TextStyle(color: Color(0xFF1F1F1F)),
    'comment': const TextStyle(color: Color(0xFF008000)),
    'title': const TextStyle(color: Color(0xFF795E26)),
    'section': const TextStyle(color: Color(0xFF795E26)),
    'attribute': const TextStyle(color: Color(0xFF001080)),
    'meta': const TextStyle(color: Color(0xFF001080)),
  };

  static final Map<String, TextStyle> _vscodeHighContrastTheme = {
    'root': const TextStyle(
        color: Color(0xFFFFFFFF), backgroundColor: Color(0xFF000000)),
    'keyword': const TextStyle(color: Color(0xFFFF9D00)),
    'built_in': const TextStyle(color: Color(0xFF4FC1FF)),
    'type': const TextStyle(color: Color(0xFF4FC1FF)),
    'literal': const TextStyle(color: Color(0xFFFF9D00)),
    'number': const TextStyle(color: Color(0xFFB5CEA8)),
    'string': const TextStyle(color: Color(0xFFCE9178)),
    'subst': const TextStyle(color: Color(0xFFFFFFFF)),
    'comment': const TextStyle(color: Color(0xFF7CA668)),
    'title': const TextStyle(color: Color(0xFFFFD700)),
    'section': const TextStyle(color: Color(0xFFFFD700)),
    'attribute': const TextStyle(color: Color(0xFF9CDCFE)),
    'meta': const TextStyle(color: Color(0xFF9CDCFE)),
  };

  IconData _fileIcon(String path) {
    final lower = path.toLowerCase();
    if (lower.endsWith('.dart')) {
      return Icons.flutter_dash;
    }
    if (lower.endsWith('.go')) {
      return Icons.memory_outlined;
    }
    if (_isCppFile(lower)) {
      return Icons.code;
    }
    if (lower.endsWith('.yaml') || lower.endsWith('.yml')) {
      return Icons.settings_applications_outlined;
    }
    if (lower.endsWith('.json')) {
      return Icons.data_object;
    }
    if (lower.endsWith('.md')) {
      return Icons.article_outlined;
    }
    if (lower.endsWith('.ps1')) {
      return Icons.terminal_outlined;
    }
    return Icons.description_outlined;
  }

  Color _fileColor(String path) {
    final lower = path.toLowerCase();
    if (lower.endsWith('.dart')) {
      return const Color(0xFF42A5F5);
    }
    if (lower.endsWith('.go')) {
      return const Color(0xFF00ADD8);
    }
    if (_isCppFile(lower)) {
      return const Color(0xFF649AD2);
    }
    if (lower.endsWith('.yaml') || lower.endsWith('.yml')) {
      return const Color(0xFFCB9B41);
    }
    if (lower.endsWith('.json')) {
      return const Color(0xFFF1D04B);
    }
    if (lower.endsWith('.md')) {
      return const Color(0xFF519ABA);
    }
    if (lower.endsWith('.ps1')) {
      return const Color(0xFF4EC9B0);
    }
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
