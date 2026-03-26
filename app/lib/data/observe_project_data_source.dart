import '../models/file_tree_node.dart';
import '../models/git_diff_models.dart';
import '../services/observe_ws_client.dart';
import 'project_data_source.dart';

class ObserveProjectDataSource extends ProjectDataSource {
  final ObserveWsClient _client;
  final List<ProjectDescriptor> _projects;

  ObserveProjectDataSource({
    required ObserveWsClient client,
    required List<ProjectDescriptor> projects,
  })  : _client = client,
        _projects = projects;

  @override
  List<ProjectDescriptor> get projects => _projects;

  @override
  String get defaultProjectId =>
      _projects.isNotEmpty ? _projects.first.id : 'default';

  @override
  Future<List<String>> fetchChatSessions(String projectId) async => ['General'];

  @override
  Future<List<GitCommitItem>> fetchDiffCommits(String projectId) async =>
      const [];

  @override
  Future<String> fetchFileContent(String projectId, String path) async {
    return _client.fsRead(projectId, _stripRoot(path));
  }

  @override
  Future<FileTreeNode> fetchFileTree(String projectId) async {
    final rootPath = '/$projectId';
    return _buildTree(
      projectId: projectId,
      requestPath: '.',
      nodeName: projectId,
      nodePath: rootPath,
    );
  }

  Future<FileTreeNode> _buildTree({
    required String projectId,
    required String requestPath,
    required String nodeName,
    required String nodePath,
  }) async {
    final entries = await _client.fsList(projectId, requestPath);
    final children = <FileTreeNode>[];
    for (final entry in entries) {
      final childPath = '$nodePath/${entry.name}'.replaceAll('//', '/');
      if (entry.isDir) {
        children.add(
          await _buildTree(
            projectId: projectId,
            requestPath: entry.path,
            nodeName: entry.name,
            nodePath: childPath,
          ),
        );
      } else {
        children.add(
          FileTreeNode.file(
            name: entry.name,
            path: childPath,
            content: '',
          ),
        );
      }
    }
    return FileTreeNode.dir(name: nodeName, path: nodePath, children: children);
  }

  String _stripRoot(String path) {
    final segments = path.split('/').where((it) => it.isNotEmpty).toList();
    if (segments.length <= 1) return '.';
    return segments.sublist(1).join('/');
  }

  @override
  void dispose() {
    _client.close();
  }
}
