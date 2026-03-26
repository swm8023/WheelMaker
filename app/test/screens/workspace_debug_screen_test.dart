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
}
