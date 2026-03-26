import 'package:flutter_test/flutter_test.dart';
import 'package:wheelmaker/data/project_data_source.dart';
import 'package:wheelmaker/models/file_tree_node.dart';
import 'package:wheelmaker/models/git_diff_models.dart';

class _ThrowingTreeDataSource extends ProjectDataSource {
  @override
  List<ProjectDescriptor> get projects =>
      const [ProjectDescriptor(id: 'p1', name: 'P1')];

  @override
  String get defaultProjectId => 'p1';

  @override
  Future<List<String>> fetchChatSessions(String projectId) async =>
      const ['General'];

  @override
  Future<List<GitCommitItem>> fetchDiffCommits(String projectId) async =>
      const [];

  @override
  Future<String> fetchFileContent(String projectId, String path) async =>
      'ok';

  @override
  Future<FileTreeNode> fetchFileTree(String projectId) async {
    throw Exception('Access is denied');
  }
}

class _ThrowingContentDataSource extends ProjectDataSource {
  @override
  List<ProjectDescriptor> get projects =>
      const [ProjectDescriptor(id: 'p1', name: 'P1')];

  @override
  String get defaultProjectId => 'p1';

  @override
  Future<List<String>> fetchChatSessions(String projectId) async =>
      const ['General'];

  @override
  Future<List<GitCommitItem>> fetchDiffCommits(String projectId) async =>
      const [];

  @override
  Future<String> fetchFileContent(String projectId, String path) async {
    throw Exception('open denied');
  }

  @override
  Future<FileTreeNode> fetchFileTree(String projectId) async {
    return FileTreeNode.dir(
      name: 'p1',
      path: '/p1',
      children: const [
        FileTreeNode.file(name: 'a.txt', path: '/p1/a.txt', content: ''),
      ],
    );
  }
}

void main() {
  test('buildInitialState tolerates file tree read failures', () async {
    final ds = _ThrowingTreeDataSource();

    final state = await ds.buildInitialState('p1');

    expect(state.files.root.isDirectory, isTrue);
    expect(state.files.root.children, isEmpty);
    expect(state.files.selectedFilePath, isNull);
  });

  test('buildInitialState tolerates file content read failures', () async {
    final ds = _ThrowingContentDataSource();

    final state = await ds.buildInitialState('p1');

    expect(state.files.selectedFilePath, '/p1/a.txt');
    expect(state.files.selectedFileContent, '');
  });
}
