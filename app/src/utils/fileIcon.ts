type ThemeMode = 'dark' | 'light';

type FileIcon = {
  glyph: string;
  color: string;
  fontFamily: 'vscode-seti';
};

type IconThemeSection = {
  file?: string;
  folder?: string;
  folderExpanded?: string;
  fileNames?: Record<string, string>;
  fileExtensions?: Record<string, string>;
};

type IconDefinition = {
  fontCharacter?: string;
  fontColor?: string;
};

type SetiThemeJson = IconThemeSection & {
  iconDefinitions: Record<string, IconDefinition>;
  light?: IconThemeSection;
};

const setiTheme = require('./vscode-seti-theme.json') as SetiThemeJson;

const fallbackIcon: FileIcon = {
  glyph: '•',
  color: '#c5c5c5',
  fontFamily: 'vscode-seti',
};

function toSetiGlyph(fontCharacter?: string): string {
  if (!fontCharacter) return fallbackIcon.glyph;
  const normalized = fontCharacter.replace(/^\\/, '');
  const codePoint = Number.parseInt(normalized, 16);
  if (Number.isNaN(codePoint)) return fallbackIcon.glyph;
  return String.fromCharCode(codePoint);
}

function resolveIconFromDefinition(defKey?: string): FileIcon {
  if (!defKey) return fallbackIcon;
  const definition = setiTheme.iconDefinitions?.[defKey];
  if (!definition) return fallbackIcon;
  return {
    glyph: toSetiGlyph(definition.fontCharacter),
    color: definition.fontColor ?? fallbackIcon.color,
    fontFamily: 'vscode-seti',
  };
}

function sectionForMode(mode: ThemeMode): Required<IconThemeSection> {
  const light = mode === 'light' ? setiTheme.light ?? {} : {};
  return {
    file: light.file ?? setiTheme.file ?? '',
    folder: light.folder ?? setiTheme.folder ?? '',
    folderExpanded: light.folderExpanded ?? setiTheme.folderExpanded ?? '',
    fileNames: {...(setiTheme.fileNames ?? {}), ...(light.fileNames ?? {})},
    fileExtensions: {
      ...(setiTheme.fileExtensions ?? {}),
      ...(light.fileExtensions ?? {}),
    },
  };
}

function extensionCandidates(base: string): string[] {
  const parts = base.split('.');
  if (parts.length <= 1) return [];
  const candidates: string[] = [];
  for (let i = 1; i < parts.length; i += 1) {
    candidates.push(parts.slice(i).join('.'));
  }
  return candidates;
}

function fileDefKey(path: string, mode: ThemeMode): string {
  const section = sectionForMode(mode);
  const base = (path.split('/').pop() ?? path).toLowerCase();

  const byName = section.fileNames[base];
  if (byName) return byName;

  for (const ext of extensionCandidates(base)) {
    const byExt = section.fileExtensions[ext];
    if (byExt) return byExt;
  }

  return section.file;
}

type IconOptions = {
  isDir?: boolean;
  expanded?: boolean;
  mode?: ThemeMode;
};

export function iconForPath(path: string, options: IconOptions = {}): FileIcon {
  const mode = options.mode ?? 'dark';
  const section = sectionForMode(mode);

  if (options.isDir) {
    return resolveIconFromDefinition(options.expanded ? section.folderExpanded : section.folder);
  }

  return resolveIconFromDefinition(fileDefKey(path, mode));
}

export type {FileIcon, ThemeMode};
