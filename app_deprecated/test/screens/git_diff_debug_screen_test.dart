import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:wheelmaker/screens/git_diff_debug_screen.dart';

void main() {
  testWidgets('git diff debug screen updates file list and diff content', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1200, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(
      const MaterialApp(
        home: GitDiffDebugScreen(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const ValueKey('commit-row-1ddd7ad')), findsOneWidget);
    expect(find.byKey(const ValueKey('changed-file-row-app/lib/screens/code_language.dart')), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('changed-file-row-app/lib/screens/file_explorer_screen.dart')));
    await tester.pumpAndSettle();
    expect(find.text('    final editorFont = GoogleFonts.jetBrainsMono(...);'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('commit-row-d15a271')));
    await tester.pumpAndSettle();
    expect(find.byKey(const ValueKey('changed-file-row-app/lib/screens/code_language.dart')), findsNothing);
  });
}
