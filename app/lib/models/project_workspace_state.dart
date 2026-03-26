import 'file_tree_node.dart';
import 'git_diff_models.dart';

enum WorkspaceTab { chat, files, diff }

class ChatPaneState {
  final List<String> sessions;
  final int selectedSessionIndex;

  const ChatPaneState({
    required this.sessions,
    required this.selectedSessionIndex,
  });

  String get selectedSession {
    if (sessions.isEmpty) return 'General';
    if (selectedSessionIndex < 0 || selectedSessionIndex >= sessions.length) {
      return sessions.first;
    }
    return sessions[selectedSessionIndex];
  }

  ChatPaneState copyWith({List<String>? sessions, int? selectedSessionIndex}) {
    return ChatPaneState(
      sessions: sessions ?? this.sessions,
      selectedSessionIndex: selectedSessionIndex ?? this.selectedSessionIndex,
    );
  }
}

class FilePaneState {
  final FileTreeNode root;
  final Set<String> expandedPaths;
  final String? selectedFilePath;

  const FilePaneState({
    required this.root,
    required this.expandedPaths,
    required this.selectedFilePath,
  });

  FilePaneState copyWith({
    FileTreeNode? root,
    Set<String>? expandedPaths,
    String? selectedFilePath,
  }) {
    return FilePaneState(
      root: root ?? this.root,
      expandedPaths: expandedPaths ?? this.expandedPaths,
      selectedFilePath: selectedFilePath ?? this.selectedFilePath,
    );
  }
}

class DiffPaneState {
  final List<GitCommitItem> commits;
  final int selectedCommitIndex;
  final String? selectedFilePath;

  const DiffPaneState({
    required this.commits,
    required this.selectedCommitIndex,
    required this.selectedFilePath,
  });

  DiffPaneState copyWith({
    List<GitCommitItem>? commits,
    int? selectedCommitIndex,
    String? selectedFilePath,
  }) {
    return DiffPaneState(
      commits: commits ?? this.commits,
      selectedCommitIndex: selectedCommitIndex ?? this.selectedCommitIndex,
      selectedFilePath: selectedFilePath ?? this.selectedFilePath,
    );
  }
}

class UiPaneState {
  final WorkspaceTab selectedTab;
  final bool sidebarCollapsed;

  const UiPaneState({
    required this.selectedTab,
    required this.sidebarCollapsed,
  });

  UiPaneState copyWith({WorkspaceTab? selectedTab, bool? sidebarCollapsed}) {
    return UiPaneState(
      selectedTab: selectedTab ?? this.selectedTab,
      sidebarCollapsed: sidebarCollapsed ?? this.sidebarCollapsed,
    );
  }
}

class ProjectWorkspaceState {
  final ChatPaneState chat;
  final FilePaneState files;
  final DiffPaneState diff;
  final UiPaneState ui;

  const ProjectWorkspaceState({
    required this.chat,
    required this.files,
    required this.diff,
    required this.ui,
  });

  ProjectWorkspaceState copyWith({
    ChatPaneState? chat,
    FilePaneState? files,
    DiffPaneState? diff,
    UiPaneState? ui,
  }) {
    return ProjectWorkspaceState(
      chat: chat ?? this.chat,
      files: files ?? this.files,
      diff: diff ?? this.diff,
      ui: ui ?? this.ui,
    );
  }
}
