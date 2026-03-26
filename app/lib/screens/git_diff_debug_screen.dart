import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';

import '../data/mock_git_diff_data.dart';
import '../models/git_diff_models.dart';

class GitDiffDebugScreen extends StatefulWidget {
  final bool showAppBar;
  final bool showSidebar;
  final int? selectedCommitIndex;
  final String? selectedFilePath;
  final ValueChanged<int>? onCommitSelected;
  final ValueChanged<String>? onFileSelected;

  const GitDiffDebugScreen({
    super.key,
    this.showAppBar = true,
    this.showSidebar = true,
    this.selectedCommitIndex,
    this.selectedFilePath,
    this.onCommitSelected,
    this.onFileSelected,
  });

  @override
  State<GitDiffDebugScreen> createState() => _GitDiffDebugScreenState();
}

class _GitDiffDebugScreenState extends State<GitDiffDebugScreen> {
  int _selectedCommitIndex = 0;
  String? _selectedFilePath;

  @override
  void initState() {
    super.initState();
    _selectedCommitIndex = widget.selectedCommitIndex ?? 0;
    final commit = mockGitCommits[_selectedCommitIndex];
    _selectedFilePath = widget.selectedFilePath ?? (commit.files.isNotEmpty ? commit.files.first.path : null);
  }

  @override
  void didUpdateWidget(covariant GitDiffDebugScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.selectedCommitIndex != null &&
        widget.selectedCommitIndex != oldWidget.selectedCommitIndex &&
        widget.selectedCommitIndex != _selectedCommitIndex) {
      _selectedCommitIndex = widget.selectedCommitIndex!;
      final commit = mockGitCommits[_selectedCommitIndex];
      _selectedFilePath = commit.files.isNotEmpty ? commit.files.first.path : null;
    }

    if (widget.selectedFilePath != null &&
        widget.selectedFilePath != oldWidget.selectedFilePath &&
        widget.selectedFilePath != _selectedFilePath) {
      _selectedFilePath = widget.selectedFilePath;
    }
  }

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final useSplit = widget.showSidebar && constraints.maxWidth >= 980;
        return Scaffold(
          appBar: widget.showAppBar ? AppBar(title: const Text('Git Diff (Debug)')) : null,
          drawer: useSplit ? null : Drawer(child: SafeArea(child: _buildLeftPane())),
          body: useSplit
              ? Row(
                  children: [
                    SizedBox(width: 360, child: _buildLeftPane()),
                    const VerticalDivider(width: 1),
                    Expanded(child: _buildDiffPane()),
                  ],
                )
              : _buildDiffPane(),
        );
      },
    );
  }

  Widget _buildLeftPane() {
    final commit = _selectedCommit;
    return Container(
      color: const Color(0xFF252526),
      child: Column(
        children: [
          _buildPaneTitle('COMMITS'),
          Expanded(
            flex: 5,
            child: ListView.builder(
              itemCount: mockGitCommits.length,
              itemBuilder: (context, index) {
                final item = mockGitCommits[index];
                final selected = index == _selectedCommitIndex;
                return InkWell(
                  key: ValueKey('commit-row-${item.hash}'),
                  onTap: () {
                    setState(() {
                      _selectedCommitIndex = index;
                      _selectedFilePath = item.files.isNotEmpty ? item.files.first.path : null;
                    });
                    widget.onCommitSelected?.call(index);
                    if (_selectedFilePath != null) {
                      widget.onFileSelected?.call(_selectedFilePath!);
                    }
                  },
                  child: Container(
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                    color: selected ? const Color(0xFF37373D) : Colors.transparent,
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          '${item.hash.substring(0, 7)}  ${_timeLabel(item.committedAt)}',
                          style: const TextStyle(color: Color(0xFF9DA0A6), fontSize: 11),
                        ),
                        const SizedBox(height: 4),
                        Text(
                          item.message,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(color: Color(0xFFD4D4D4), fontSize: 12),
                        ),
                      ],
                    ),
                  ),
                );
              },
            ),
          ),
          const Divider(height: 1),
          _buildPaneTitle('CHANGED FILES'),
          Expanded(
            flex: 5,
            child: ListView.builder(
              itemCount: commit.files.length,
              itemBuilder: (context, index) {
                final file = commit.files[index];
                final selected = file.path == _selectedFilePath;
                return InkWell(
                  key: ValueKey('changed-file-row-${file.path}'),
                  onTap: () {
                    setState(() => _selectedFilePath = file.path);
                    widget.onFileSelected?.call(file.path);
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

  Widget _buildDiffPane() {
    final file = _selectedFile;
    if (file == null) {
      return const Center(
        child: Text('Select a changed file', style: TextStyle(color: Color(0xFF9DA0A6))),
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
                _statusBadge(file.status),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(file.path,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(color: Color(0xFFD4D4D4), fontSize: 12)),
                ),
              ],
            ),
          ),
          Expanded(
            child: ListView.builder(
              itemCount: file.lines.length,
              itemBuilder: (context, index) {
                final line = file.lines[index];
                final bg = switch (line.type) {
                  GitDiffLineType.added => const Color(0x332EA043),
                  GitDiffLineType.removed => const Color(0x33F85149),
                  GitDiffLineType.context => Colors.transparent,
                };
                final prefix = switch (line.type) {
                  GitDiffLineType.added => '+',
                  GitDiffLineType.removed => '-',
                  GitDiffLineType.context => ' ',
                };
                return Container(
                  color: bg,
                  padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 2),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      SizedBox(
                        width: 46,
                        child: Text(
                          line.oldLine?.toString() ?? '',
                          textAlign: TextAlign.right,
                          style: const TextStyle(color: Color(0xFF6E7681), fontSize: 11),
                        ),
                      ),
                      const SizedBox(width: 8),
                      SizedBox(
                        width: 46,
                        child: Text(
                          line.newLine?.toString() ?? '',
                          textAlign: TextAlign.right,
                          style: const TextStyle(color: Color(0xFF6E7681), fontSize: 11),
                        ),
                      ),
                      const SizedBox(width: 10),
                      Text(prefix, style: const TextStyle(color: Color(0xFF9DA0A6), fontSize: 12)),
                      const SizedBox(width: 6),
                      Expanded(
                        child: Text(
                          line.text,
                          key: ValueKey('diff-line-$index-${line.text}'),
                          style: GoogleFonts.jetBrainsMono(
                            color: const Color(0xFFD4D4D4),
                            fontSize: 12,
                            height: 1.45,
                          ),
                        ),
                      ),
                    ],
                  ),
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildPaneTitle(String text) {
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
      child: Text(label, style: TextStyle(color: color, fontSize: 10, fontWeight: FontWeight.w700)),
    );
  }

  String _timeLabel(DateTime value) {
    final diff = DateTime.now().difference(value);
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }

  GitCommitItem get _selectedCommit => mockGitCommits[_selectedCommitIndex];

  GitChangedFile? get _selectedFile {
    for (final file in _selectedCommit.files) {
      if (file.path == _selectedFilePath) return file;
    }
    return _selectedCommit.files.isNotEmpty ? _selectedCommit.files.first : null;
  }
}
