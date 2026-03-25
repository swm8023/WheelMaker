import 'package:flutter/material.dart';
import 'screens/connect_screen.dart';

void main() => runApp(const WheelMakerApp());

class WheelMakerApp extends StatelessWidget {
  const WheelMakerApp({super.key});

  @override
  Widget build(BuildContext context) {
    const vscodeBg = Color(0xFF1E1E1E);
    const vscodePanel = Color(0xFF252526);
    const vscodeSurface = Color(0xFF2D2D30);
    const vscodeBlue = Color(0xFF0E639C);

    final darkScheme = ColorScheme.fromSeed(
      seedColor: vscodeBlue,
      brightness: Brightness.dark,
    ).copyWith(
      primary: vscodeBlue,
      secondary: const Color(0xFF3794FF),
      surface: vscodeBg,
      surfaceContainerHighest: vscodeSurface,
      onSurface: const Color(0xFFD4D4D4),
      onSurfaceVariant: const Color(0xFF9DA0A6),
      outline: const Color(0xFF3E3E42),
      outlineVariant: const Color(0xFF333337),
    );

    return MaterialApp(
      title: 'WheelMaker',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        brightness: Brightness.dark,
        colorScheme: darkScheme,
        scaffoldBackgroundColor: vscodeBg,
        canvasColor: vscodePanel,
      ).copyWith(
        appBarTheme: const AppBarTheme(
          backgroundColor: vscodePanel,
          foregroundColor: Color(0xFFD4D4D4),
          elevation: 0,
        ),
        cardColor: vscodeSurface,
        dividerColor: const Color(0xFF333337),
        inputDecorationTheme: const InputDecorationTheme(
          filled: true,
          fillColor: Color(0xFF252526),
        ),
      ),
      home: const ConnectScreen(),
    );
  }
}
