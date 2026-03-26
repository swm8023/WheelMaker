import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

enum UiThemeMode { system, light, dark }

enum EditorThemePreset { vscodeDark, vscodeLight, vscodeHighContrast }

class AppThemeController extends ChangeNotifier {
  static const _uiModeKey = 'wm_ui_theme_mode';
  static const _editorThemeKey = 'wm_editor_theme';

  UiThemeMode _uiMode = UiThemeMode.dark;
  EditorThemePreset _editorTheme = EditorThemePreset.vscodeDark;
  bool _ready = false;

  UiThemeMode get uiMode => _uiMode;
  EditorThemePreset get editorTheme => _editorTheme;
  bool get isReady => _ready;

  ThemeMode get themeMode {
    switch (_uiMode) {
      case UiThemeMode.system:
        return ThemeMode.system;
      case UiThemeMode.light:
        return ThemeMode.light;
      case UiThemeMode.dark:
        return ThemeMode.dark;
    }
  }

  Future<void> load() async {
    final prefs = await SharedPreferences.getInstance();
    _uiMode = UiThemeMode.values.byName(
      prefs.getString(_uiModeKey) ?? UiThemeMode.dark.name,
    );
    _editorTheme = EditorThemePreset.values.byName(
      prefs.getString(_editorThemeKey) ?? EditorThemePreset.vscodeDark.name,
    );
    _ready = true;
    notifyListeners();
  }

  Future<void> setUiMode(UiThemeMode mode) async {
    if (_uiMode == mode) return;
    _uiMode = mode;
    notifyListeners();
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_uiModeKey, mode.name);
  }

  Future<void> setEditorTheme(EditorThemePreset preset) async {
    if (_editorTheme == preset) return;
    _editorTheme = preset;
    notifyListeners();
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_editorThemeKey, preset.name);
  }
}

class AppThemeScope extends InheritedNotifier<AppThemeController> {
  const AppThemeScope({
    super.key,
    required AppThemeController controller,
    required super.child,
  }) : super(notifier: controller);

  static AppThemeController of(BuildContext context) {
    final scope = context.dependOnInheritedWidgetOfExactType<AppThemeScope>();
    assert(scope != null, 'AppThemeScope not found in widget tree');
    return scope!.notifier!;
  }

  static AppThemeController? maybeOf(BuildContext context) {
    final scope = context.dependOnInheritedWidgetOfExactType<AppThemeScope>();
    return scope?.notifier;
  }
}
