type FileIcon = {
  glyph: string;
  color: string;
};

const color = {
  folder: '#dcb67a',
  ts: '#519aba',
  js: '#cbcb41',
  json: '#cbcb41',
  md: '#519aba',
  go: '#519aba',
  shell: '#89e051',
  default: '#c5c5c5',
};

const extensionIconMap: Record<string, FileIcon> = {
  ts: {glyph: 'TS', color: color.ts},
  tsx: {glyph: 'TS', color: color.ts},
  js: {glyph: 'JS', color: color.js},
  jsx: {glyph: 'JS', color: color.js},
  json: {glyph: '{}', color: color.json},
  md: {glyph: 'MD', color: color.md},
  markdown: {glyph: 'MD', color: color.md},
  go: {glyph: 'GO', color: color.go},
  sh: {glyph: 'SH', color: color.shell},
  ps1: {glyph: 'SH', color: color.shell},
  yml: {glyph: 'YML', color: color.default},
  yaml: {glyph: 'YML', color: color.default},
};

const nameIconMap: Record<string, FileIcon> = {
  dockerfile: {glyph: 'DK', color: color.default},
  makefile: {glyph: 'MK', color: color.default},
  'package.json': {glyph: '{}', color: color.json},
  'readme.md': {glyph: 'MD', color: color.md},
};

const folderIcon: FileIcon = {glyph: '▸', color: color.folder};
const fileIcon: FileIcon = {glyph: '•', color: color.default};

export function iconForPath(path: string): FileIcon {
  const normalized = path.toLowerCase().trim();
  if (!normalized) {
    return fileIcon;
  }

  const base = normalized.split('/').pop() ?? normalized;
  if (!base.includes('.')) {
    return folderIcon;
  }

  if (nameIconMap[base]) {
    return nameIconMap[base];
  }

  const ext = base.slice(base.lastIndexOf('.') + 1);
  return extensionIconMap[ext] ?? fileIcon;
}

export type {FileIcon};
