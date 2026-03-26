class FileTreeNode {
  final String name;
  final String path;
  final bool isDirectory;
  final List<FileTreeNode> children;
  final String? content;

  const FileTreeNode._({
    required this.name,
    required this.path,
    required this.isDirectory,
    this.children = const [],
    this.content,
  });

  const FileTreeNode.dir({
    required String name,
    required String path,
    required List<FileTreeNode> children,
  }) : this._(
          name: name,
          path: path,
          isDirectory: true,
          children: children,
        );

  const FileTreeNode.file({
    required String name,
    required String path,
    required String content,
  }) : this._(
          name: name,
          path: path,
          isDirectory: false,
          content: content,
        );
}
