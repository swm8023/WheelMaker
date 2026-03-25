enum GitFileStatus { added, modified, deleted, renamed }

enum GitDiffLineType { context, added, removed }

class GitDiffLine {
  final GitDiffLineType type;
  final int? oldLine;
  final int? newLine;
  final String text;

  const GitDiffLine({
    required this.type,
    required this.text,
    this.oldLine,
    this.newLine,
  });
}

class GitChangedFile {
  final String path;
  final GitFileStatus status;
  final List<GitDiffLine> lines;

  const GitChangedFile({
    required this.path,
    required this.status,
    required this.lines,
  });
}

class GitCommitItem {
  final String hash;
  final String message;
  final String author;
  final DateTime committedAt;
  final List<GitChangedFile> files;

  const GitCommitItem({
    required this.hash,
    required this.message,
    required this.author,
    required this.committedAt,
    required this.files,
  });
}
