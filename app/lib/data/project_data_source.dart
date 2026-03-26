import '../models/file_tree_node.dart';
import '../models/git_diff_models.dart';
import '../models/project_workspace_state.dart';
import 'mock_git_diff_data.dart';
import 'mock_wheelmaker_fs.dart';

class ProjectDescriptor {
  final String id;
  final String name;

  const ProjectDescriptor({required this.id, required this.name});
}

abstract class ProjectDataSource {
  List<ProjectDescriptor> get projects;
  String get defaultProjectId;

  Future<List<String>> fetchChatSessions(String projectId);
  Future<FileTreeNode> fetchFileTree(String projectId);
  Future<String> fetchFileContent(String projectId, String path);
  Future<List<GitCommitItem>> fetchDiffCommits(String projectId);
  void dispose() {}

  Future<ProjectWorkspaceState> buildInitialState(String projectId) async {
    List<String> sessions;
    try {
      sessions = await fetchChatSessions(projectId);
    } catch (_) {
      sessions = const ['General'];
    }

    FileTreeNode root;
    try {
      root = await fetchFileTree(projectId);
    } catch (_) {
      root = FileTreeNode.dir(
        name: projectId,
        path: '/$projectId',
        children: const [],
      );
    }

    List<GitCommitItem> commits;
    try {
      commits = await fetchDiffCommits(projectId);
    } catch (_) {
      commits = const [];
    }

    final firstFile = _firstFilePath(root);
    String initialContent = '';
    if (firstFile != null) {
      try {
        initialContent = await fetchFileContent(projectId, firstFile);
      } catch (_) {
        initialContent = '';
      }
    }

    return ProjectWorkspaceState(
      chat: ChatPaneState(sessions: sessions, selectedSessionIndex: 0),
      files: FilePaneState(
        root: root,
        expandedPaths: {root.path, '${root.path}/app'},
        selectedFilePath: firstFile,
        selectedFileContent: initialContent,
        contentLoading: false,
      ),
      diff: DiffPaneState(
        commits: commits,
        selectedCommitIndex: 0,
        selectedFilePath: commits.isNotEmpty && commits.first.files.isNotEmpty
            ? commits.first.files.first.path
            : null,
      ),
      ui: const UiPaneState(
        selectedTab: WorkspaceTab.chat,
        sidebarCollapsed: false,
      ),
    );
  }
}

class MockProjectDataSource extends ProjectDataSource {
  static const _projects = <ProjectDescriptor>[
    ProjectDescriptor(id: 'wheelmaker', name: 'WheelMaker'),
    ProjectDescriptor(id: 'wheelmaker-mobile', name: 'WheelMaker Mobile'),
  ];

  @override
  List<ProjectDescriptor> get projects => _projects;

  @override
  String get defaultProjectId => _projects.first.id;

  @override
  Future<List<String>> fetchChatSessions(String projectId) async {
    if (projectId == 'wheelmaker-mobile') {
      return const ['General', 'Flutter UI', 'Android Build', 'iOS QA'];
    }
    return const ['General', 'WheelMaker App', 'Go Service', 'Review Notes'];
  }

  @override
  Future<List<GitCommitItem>> fetchDiffCommits(String projectId) async {
    if (projectId == 'wheelmaker-mobile') {
      return mockGitCommits
          .map(
            (c) => GitCommitItem(
              hash: c.hash,
              message: '[mobile] ${c.message}',
              author: c.author,
              committedAt: c.committedAt,
              files: c.files
                  .map(
                    (f) => GitChangedFile(
                      path: f.path.replaceFirst('app/', 'mobile_app/'),
                      status: f.status,
                      lines: f.lines,
                    ),
                  )
                  .toList(),
            ),
          )
          .toList();
    }
    return mockGitCommits;
  }

  @override
  Future<FileTreeNode> fetchFileTree(String projectId) async {
    if (projectId == 'wheelmaker-mobile') {
      return _cloneTree(
        mockWheelMakerRoot,
        replaceRootName: 'WheelMakerMobile',
        replaceRootPath: '/WheelMakerMobile',
      );
    }
    return mockWheelMakerRoot;
  }

  @override
  Future<String> fetchFileContent(String projectId, String path) async {
    final root = await fetchFileTree(projectId);
    final node = _findByPath(root, path);
    if (node == null || node.isDirectory) return '';
    return node.content ?? '';
  }

  FileTreeNode _cloneTree(
    FileTreeNode node, {
    required String replaceRootName,
    required String replaceRootPath,
  }) {
    const fromRoot = '/WheelMaker';
    final toRoot = replaceRootPath;
    if (node.isDirectory) {
      return FileTreeNode.dir(
        name: node.path == fromRoot ? replaceRootName : node.name,
        path: node.path.replaceFirst(fromRoot, toRoot),
        children: node.children
            .map(
              (child) => _cloneTree(
                child,
                replaceRootName: replaceRootName,
                replaceRootPath: replaceRootPath,
              ),
            )
            .toList(),
      );
    }
    return FileTreeNode.file(
      name: node.name,
      path: node.path.replaceFirst(fromRoot, toRoot),
      content: node.content ?? '',
    );
  }
}

String? _firstFilePath(FileTreeNode node) {
  if (!node.isDirectory) return node.path;
  for (final child in node.children) {
    final found = _firstFilePath(child);
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
