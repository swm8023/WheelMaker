import 'dart:async';

import 'package:flutter/foundation.dart';

import '../data/project_data_source.dart';
import '../models/file_tree_node.dart';
import '../models/git_diff_models.dart';
import '../models/project_workspace_state.dart';

class ProjectWorkspaceStore extends ChangeNotifier {
  final ProjectDataSource _dataSource;
  final Map<String, ProjectWorkspaceState> _projects = {};

  ProjectWorkspaceStore({required ProjectDataSource dataSource})
      : _dataSource = dataSource,
        _activeProjectId = dataSource.defaultProjectId;

  bool _ready = false;
  String _activeProjectId;

  List<ProjectDescriptor> get projectList => _dataSource.projects;
  bool get isReady => _ready;
  String get activeProjectId => _activeProjectId;
  String get activeProjectName {
    for (final project in projectList) {
      if (project.id == _activeProjectId) return project.name;
    }
    return _activeProjectId;
  }

  ProjectWorkspaceState? get activeState => _projects[_activeProjectId];

  Future<void> initialize() async {
    await _ensureLoaded(_activeProjectId);
    _ready = true;
    notifyListeners();
    unawaited(refreshProject(_activeProjectId));
  }

  Future<void> switchProject(String projectId) async {
    if (projectId == _activeProjectId) return;
    final previousTab = activeState?.ui.selectedTab;
    _activeProjectId = projectId;
    notifyListeners();
    await _ensureLoaded(projectId);
    if (previousTab != null) {
      final target = _projects[projectId];
      if (target != null && target.ui.selectedTab != previousTab) {
        _projects[projectId] = target.copyWith(
          ui: target.ui.copyWith(selectedTab: previousTab),
        );
      }
    }
    notifyListeners();
    unawaited(refreshProject(projectId));
  }

  Future<void> refreshProject(String projectId) async {
    final current = _projects[projectId];
    if (current == null) return;

    final results = await Future.wait<Object?>([
      _safeFetch<List<String>>(() => _dataSource.fetchChatSessions(projectId)),
      _safeFetch<FileTreeNode>(() => _dataSource.fetchFileTree(projectId)),
      _safeFetch<List<GitCommitItem>>(
          () => _dataSource.fetchDiffCommits(projectId)),
    ]);
    final sessions = results[0] as List<String>?;
    final root = results[1] as FileTreeNode?;
    final commits = results[2] as List<GitCommitItem>?;

    var next = current;
    if (sessions != null) {
      final max = sessions.isEmpty ? 0 : sessions.length - 1;
      final selected = current.chat.selectedSessionIndex.clamp(0, max).toInt();
      next = next.copyWith(
        chat: current.chat.copyWith(
          sessions: sessions,
          selectedSessionIndex: selected,
        ),
      );
    }

    if (root != null) {
      final selected =
          _resolveFileSelection(root, current.files.selectedFilePath);
      next = next.copyWith(
        files: current.files.copyWith(root: root, selectedFilePath: selected),
      );
    }

    if (commits != null) {
      final diff = _mergeDiffState(current.diff, commits);
      next = next.copyWith(diff: diff);
    }

    _projects[projectId] = next;
    notifyListeners();
  }

  void setTab(WorkspaceTab tab) {
    final state = activeState;
    if (state == null || state.ui.selectedTab == tab) return;
    _projects[_activeProjectId] = state.copyWith(
      ui: state.ui.copyWith(selectedTab: tab),
    );
    notifyListeners();
  }

  void toggleSidebarCollapsed() {
    final state = activeState;
    if (state == null) return;
    _projects[_activeProjectId] = state.copyWith(
      ui: state.ui.copyWith(sidebarCollapsed: !state.ui.sidebarCollapsed),
    );
    notifyListeners();
  }

  void selectChatSession(int index) {
    final state = activeState;
    if (state == null || state.chat.sessions.isEmpty) return;
    final clamped = index.clamp(0, state.chat.sessions.length - 1).toInt();
    _projects[_activeProjectId] = state.copyWith(
      chat: state.chat.copyWith(selectedSessionIndex: clamped),
    );
    notifyListeners();
  }

  void toggleFolder(String path) {
    final state = activeState;
    if (state == null) return;
    final expanded = Set<String>.from(state.files.expandedPaths);
    if (expanded.contains(path)) {
      expanded.remove(path);
    } else {
      expanded.add(path);
    }
    _projects[_activeProjectId] = state.copyWith(
      files: state.files.copyWith(expandedPaths: expanded),
    );
    notifyListeners();
  }

  void selectFile(String path) {
    final state = activeState;
    if (state == null) return;
    _projects[_activeProjectId] = state.copyWith(
      files: state.files.copyWith(selectedFilePath: path),
    );
    notifyListeners();
  }

  void selectDiffCommit(int index) {
    final state = activeState;
    if (state == null || state.diff.commits.isEmpty) return;
    final clamped = index.clamp(0, state.diff.commits.length - 1).toInt();
    final commit = state.diff.commits[clamped];
    _projects[_activeProjectId] = state.copyWith(
      diff: state.diff.copyWith(
        selectedCommitIndex: clamped,
        selectedFilePath:
            commit.files.isNotEmpty ? commit.files.first.path : null,
      ),
    );
    notifyListeners();
  }

  void selectDiffFile(String path) {
    final state = activeState;
    if (state == null) return;
    _projects[_activeProjectId] = state.copyWith(
      diff: state.diff.copyWith(selectedFilePath: path),
    );
    notifyListeners();
  }

  Future<void> _ensureLoaded(String projectId) async {
    if (_projects.containsKey(projectId)) return;
    _projects[projectId] = await _dataSource.buildInitialState(projectId);
  }

  Future<T?> _safeFetch<T>(Future<T> Function() load) async {
    try {
      return await load();
    } catch (_) {
      return null;
    }
  }

  String? _resolveFileSelection(FileTreeNode root, String? selectedPath) {
    if (selectedPath != null && _containsFilePath(root, selectedPath)) {
      return selectedPath;
    }
    return _firstFilePath(root);
  }

  bool _containsFilePath(FileTreeNode node, String path) {
    if (!node.isDirectory) return node.path == path;
    for (final child in node.children) {
      if (_containsFilePath(child, path)) return true;
    }
    return false;
  }

  String? _firstFilePath(FileTreeNode node) {
    if (!node.isDirectory) return node.path;
    for (final child in node.children) {
      final found = _firstFilePath(child);
      if (found != null) return found;
    }
    return null;
  }

  DiffPaneState _mergeDiffState(
    DiffPaneState current,
    List<GitCommitItem> commits,
  ) {
    if (commits.isEmpty) {
      return const DiffPaneState(
        commits: [],
        selectedCommitIndex: 0,
        selectedFilePath: null,
      );
    }
    final selectedHash = current.selectedCommitIndex >= 0 &&
            current.selectedCommitIndex < current.commits.length
        ? current.commits[current.selectedCommitIndex].hash
        : null;
    var nextCommitIndex = 0;
    if (selectedHash != null) {
      final found = commits.indexWhere((commit) => commit.hash == selectedHash);
      if (found >= 0) nextCommitIndex = found;
    }
    final nextCommit = commits[nextCommitIndex];
    final nextFilePath = nextCommit.files
            .any((file) => file.path == current.selectedFilePath)
        ? current.selectedFilePath
        : (nextCommit.files.isNotEmpty ? nextCommit.files.first.path : null);
    return DiffPaneState(
      commits: commits,
      selectedCommitIndex: nextCommitIndex,
      selectedFilePath: nextFilePath,
    );
  }
}
