import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:wheelmaker/screens/workspace_debug_screen.dart';

void main() {
  testWidgets('switches among chat, files and diff tabs', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1280, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(
      const MaterialApp(
        home: WorkspaceDebugScreen(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Message…'), findsOneWidget);
    expect(find.byKey(const ValueKey('workspace-sidebar-chat')), findsOneWidget);

    await tester.tap(find.text('Files'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('workspace-sidebar-files')), findsOneWidget);

    await tester.tap(find.text('Diff'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('workspace-sidebar-diff')), findsOneWidget);
  });

  testWidgets('keeps sidebar collapsed state across tabs in wide layout', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1280, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(
      const MaterialApp(
        home: WorkspaceDebugScreen(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('workspace-sidebar-chat')), findsOneWidget);
    await tester.tap(find.byKey(const ValueKey('workspace-sidebar-toggle')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('workspace-sidebar-chat')), findsNothing);

    await tester.tap(find.text('Files'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('workspace-sidebar-files')), findsNothing);

    await tester.tap(find.text('Diff'));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('workspace-sidebar-diff')), findsNothing);

    await tester.tap(find.byKey(const ValueKey('workspace-sidebar-toggle')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('workspace-sidebar-diff')), findsOneWidget);
  });

  testWidgets('narrow mode drawer selections link chat/files/diff content', (tester) async {
    await tester.binding.setSurfaceSize(const Size(600, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(
      const MaterialApp(
        home: WorkspaceDebugScreen(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('chat-session-label')), findsOneWidget);
    expect(tester.widget<Text>(find.byKey(const ValueKey('chat-session-label'))).data, 'General');
    await tester.dragFrom(const Offset(0, 200), const Offset(320, 0));
    await tester.pumpAndSettle();
    await tester.tap(find.text('WheelMaker App').last);
    await tester.pumpAndSettle();
    expect(tester.widget<Text>(find.byKey(const ValueKey('chat-session-label'))).data, 'WheelMaker App');

    await tester.tap(find.text('Files'));
    await tester.pumpAndSettle();
    await tester.dragFrom(const Offset(0, 200), const Offset(320, 0));
    await tester.pumpAndSettle();
    final fileRow = find.byKey(const ValueKey('workspace-file-row-/WheelMaker/app/pubspec.yaml')).last;
    await tester.ensureVisible(fileRow);
    await tester.tap(fileRow, warnIfMissed: false);
    await tester.pumpAndSettle();
    expect(find.text('/WheelMaker/app/pubspec.yaml'), findsOneWidget);

    await tester.tap(find.text('Diff'));
    await tester.pumpAndSettle();
    await tester.dragFrom(const Offset(0, 200), const Offset(320, 0));
    await tester.pumpAndSettle();
    final commitRow = find.byKey(const ValueKey('workspace-commit-row-d15a271')).last;
    await tester.ensureVisible(commitRow);
    await tester.tap(commitRow, warnIfMissed: false);
    await tester.pumpAndSettle();
    final diffFileRow =
        find.byKey(const ValueKey('workspace-diff-file-row-app/lib/screens/file_explorer_screen.dart')).last;
    await tester.ensureVisible(diffFileRow);
    await tester.tap(diffFileRow, warnIfMissed: false);
    await tester.pumpAndSettle();
    expect(find.text('app/lib/screens/file_explorer_screen.dart'), findsWidgets);
  });
}
