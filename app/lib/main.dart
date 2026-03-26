import 'package:flutter/material.dart';
import 'screens/connect_screen.dart';
import 'theme/app_theme_controller.dart';

void main() => runApp(const WheelMakerApp());

class WheelMakerApp extends StatefulWidget {
  const WheelMakerApp({super.key});

  @override
  State<WheelMakerApp> createState() => _WheelMakerAppState();
}

class _WheelMakerAppState extends State<WheelMakerApp> {
  late final AppThemeController _themeController;

  @override
  void initState() {
    super.initState();
    _themeController = AppThemeController()..load();
  }

  @override
  void dispose() {
    _themeController.dispose();
    super.dispose();
  }

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

    final lightScheme = ColorScheme.fromSeed(
      seedColor: vscodeBlue,
      brightness: Brightness.light,
    ).copyWith(
      primary: vscodeBlue,
      secondary: const Color(0xFF006FC1),
      surface: const Color(0xFFF3F3F3),
      surfaceContainerHighest: const Color(0xFFE6E6E6),
      onSurface: const Color(0xFF1F1F1F),
      onSurfaceVariant: const Color(0xFF4E4E4E),
      outline: const Color(0xFFD0D0D0),
      outlineVariant: const Color(0xFFE2E2E2),
    );

    final darkTheme = ThemeData(
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
    );

    final lightTheme = ThemeData(
      useMaterial3: true,
      brightness: Brightness.light,
      colorScheme: lightScheme,
      scaffoldBackgroundColor: const Color(0xFFF3F3F3),
      canvasColor: const Color(0xFFEDEDED),
    ).copyWith(
      appBarTheme: const AppBarTheme(
        backgroundColor: Color(0xFFEDEDED),
        foregroundColor: Color(0xFF1F1F1F),
        elevation: 0,
      ),
      cardColor: const Color(0xFFFFFFFF),
      dividerColor: const Color(0xFFDADADA),
      inputDecorationTheme: const InputDecorationTheme(
        filled: true,
        fillColor: Color(0xFFFFFFFF),
      ),
    );

    return AppThemeScope(
      controller: _themeController,
      child: AnimatedBuilder(
        animation: _themeController,
        builder: (context, _) {
          return MaterialApp(
            title: 'WheelMaker',
            debugShowCheckedModeBanner: false,
            theme: lightTheme,
            darkTheme: darkTheme,
            themeMode: _themeController.themeMode,
            home: const ConnectScreen(),
          );
        },
      ),
    );
  }
}
