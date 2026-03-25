import '../models/git_diff_models.dart';

final List<GitCommitItem> mockGitCommits = [
  GitCommitItem(
    hash: '1ddd7ad',
    message: 'feat(app): add cpp highlighting and improve code font',
    author: 'swm',
    committedAt: DateTime(2026, 3, 26, 9, 12),
    files: [
      const GitChangedFile(
        path: 'app/lib/screens/code_language.dart',
        status: GitFileStatus.modified,
        lines: [
          GitDiffLine(type: GitDiffLineType.context, oldLine: 1, newLine: 1, text: 'String languageFromPath(String path) {'),
          GitDiffLine(type: GitDiffLineType.context, oldLine: 2, newLine: 2, text: '  final lower = path.toLowerCase();'),
          GitDiffLine(type: GitDiffLineType.added, oldLine: null, newLine: 3, text: '  if (lower.endsWith(\'.cpp\') || lower.endsWith(\'.hpp\')) return \'cpp\';'),
          GitDiffLine(type: GitDiffLineType.context, oldLine: 3, newLine: 4, text: '  if (lower.endsWith(\'.dart\')) return \'dart\';'),
          GitDiffLine(type: GitDiffLineType.context, oldLine: 4, newLine: 5, text: '  return \'plaintext\';'),
        ],
      ),
      const GitChangedFile(
        path: 'app/lib/screens/file_explorer_screen.dart',
        status: GitFileStatus.modified,
        lines: [
          GitDiffLine(type: GitDiffLineType.removed, oldLine: 4, newLine: null, text: "import 'package:flutter_highlight/flutter_highlight.dart';"),
          GitDiffLine(type: GitDiffLineType.added, oldLine: null, newLine: 4, text: "import 'package:flutter_highlight/flutter_highlight.dart';"),
          GitDiffLine(type: GitDiffLineType.added, oldLine: null, newLine: 5, text: "import 'package:google_fonts/google_fonts.dart';"),
          GitDiffLine(type: GitDiffLineType.context, oldLine: 200, newLine: 201, text: '  Widget _buildCodeView(FileTreeNode file) {'),
          GitDiffLine(type: GitDiffLineType.added, oldLine: null, newLine: 208, text: '    final editorFont = GoogleFonts.jetBrainsMono(...);'),
        ],
      ),
    ],
  ),
  GitCommitItem(
    hash: 'd15a271',
    message: 'style(app): remove activity bar and editor tab strip',
    author: 'swm',
    committedAt: DateTime(2026, 3, 25, 21, 4),
    files: [
      const GitChangedFile(
        path: 'app/lib/screens/file_explorer_screen.dart',
        status: GitFileStatus.modified,
        lines: [
          GitDiffLine(type: GitDiffLineType.removed, oldLine: 30, newLine: null, text: '                    _buildActivityBar(),'),
          GitDiffLine(type: GitDiffLineType.context, oldLine: 31, newLine: 30, text: '                    SizedBox(width: 320, child: _buildTreePane()),'),
          GitDiffLine(type: GitDiffLineType.removed, oldLine: 188, newLine: null, text: '          Container('),
          GitDiffLine(type: GitDiffLineType.removed, oldLine: 189, newLine: null, text: '            color: const Color(0xFF252526),'),
          GitDiffLine(type: GitDiffLineType.added, oldLine: null, newLine: 186, text: '          Container('),
          GitDiffLine(type: GitDiffLineType.added, oldLine: null, newLine: 187, text: '            color: const Color(0xFF2D2D2D),'),
        ],
      ),
    ],
  ),
  GitCommitItem(
    hash: '2a2f351',
    message: 'style(app): remove explorer header action icons',
    author: 'swm',
    committedAt: DateTime(2026, 3, 25, 22, 50),
    files: [
      const GitChangedFile(
        path: 'app/lib/screens/file_explorer_screen.dart',
        status: GitFileStatus.modified,
        lines: [
          GitDiffLine(type: GitDiffLineType.context, oldLine: 56, newLine: 56, text: '                Expanded('),
          GitDiffLine(type: GitDiffLineType.removed, oldLine: 67, newLine: null, text: '                Icon(Icons.more_horiz, color: Color(0xFF9DA0A6), size: 16),'),
          GitDiffLine(type: GitDiffLineType.removed, oldLine: 68, newLine: null, text: '                SizedBox(width: 8),'),
          GitDiffLine(type: GitDiffLineType.removed, oldLine: 69, newLine: null, text: '                Icon(Icons.create_new_folder_outlined, color: Color(0xFF9DA0A6), size: 16),'),
        ],
      ),
    ],
  ),
];
