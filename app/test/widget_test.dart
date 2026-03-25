import 'package:flutter_test/flutter_test.dart';

import 'package:wheelmaker/main.dart';

void main() {
  testWidgets('app renders connect screen', (WidgetTester tester) async {
    await tester.pumpWidget(const WheelMakerApp());
    await tester.pumpAndSettle();

    expect(find.text('WheelMaker'), findsOneWidget);
    expect(find.text('Connect'), findsOneWidget);
  });
}
