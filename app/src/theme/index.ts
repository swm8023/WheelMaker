import {
  type AppTheme,
  type ThemeMode,
  vscodeModernDarkTheme,
  vscodeModernLightTheme,
} from './tokens';

export const themeByMode: Record<ThemeMode, AppTheme> = {
  dark: vscodeModernDarkTheme,
  light: vscodeModernLightTheme,
};

export function nextThemeMode(mode: ThemeMode): ThemeMode {
  return mode === 'dark' ? 'light' : 'dark';
}

export function resolveTheme(mode: ThemeMode): AppTheme {
  return themeByMode[mode];
}

export type {AppTheme, ThemeMode};
