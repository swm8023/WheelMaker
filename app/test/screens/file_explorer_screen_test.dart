import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:wheelmaker/screens/file_explorer_screen.dart';

void main() {
  testWidgets('updates active file path when selecting a file', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1200, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(
      const MaterialApp(
        home: FileExplorerScreen(),
      ),
    );

    expect(find.text('/WheelMaker/CLAUDE.md'), findsOneWidget);

    await tester.tap(find.text('server'));
    await tester.pumpAndSettle();
    expect(find.text('go.mod'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('file-row-/WheelMaker/server/go.mod')));
    await tester.pumpAndSettle();

    expect(find.text('/WheelMaker/server/go.mod'), findsOneWidget);
  });
}
