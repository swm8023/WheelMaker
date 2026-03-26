import {Platform} from 'react-native';

export type ThemeMode = 'dark' | 'light';

export type AppTheme = {
  mode: ThemeMode;
  colors: {
    background: string;
    panel: string;
    panelSecondary: string;
    border: string;
    text: string;
    textMuted: string;
    accent: string;
    rowHover: string;
    rowSelected: string;
    inputBackground: string;
    error: string;
    codeBackground: string;
    markdownBackground: string;
  };
  font: {
    ui: string | undefined;
    code: string | undefined;
  };
};

const codeFont = Platform.select({
  web: "'Cascadia Code','Fira Code','JetBrains Mono',Consolas,'Courier New',monospace",
  default: 'monospace',
});

const uiFont = Platform.select({
  web: "'Segoe WPC','Segoe UI',-apple-system,BlinkMacSystemFont,'Helvetica Neue',sans-serif",
  default: undefined,
});

export const vscodeModernDarkTheme: AppTheme = {
  mode: 'dark',
  colors: {
    background: '#1e1e1e',
    panel: '#252526',
    panelSecondary: '#2d2d2d',
    border: '#3c3c3c',
    text: '#d4d4d4',
    textMuted: '#9da0a6',
    accent: '#0e639c',
    rowHover: '#2a2d2e',
    rowSelected: '#37373d',
    inputBackground: '#3c3c3c',
    error: '#f48771',
    codeBackground: '#1e1e1e',
    markdownBackground: '#1e1e1e',
  },
  font: {
    ui: uiFont,
    code: codeFont,
  },
};

export const vscodeModernLightTheme: AppTheme = {
  mode: 'light',
  colors: {
    background: '#ffffff',
    panel: '#f3f3f3',
    panelSecondary: '#ececec',
    border: '#d4d4d4',
    text: '#1f2328',
    textMuted: '#57606a',
    accent: '#005fb8',
    rowHover: '#e9edf2',
    rowSelected: '#dbe9ff',
    inputBackground: '#ffffff',
    error: '#d1242f',
    codeBackground: '#ffffff',
    markdownBackground: '#ffffff',
  },
  font: {
    ui: uiFont,
    code: codeFont,
  },
};
