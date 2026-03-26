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
  ts: {glyph: '●', color: color.ts},
  tsx: {glyph: '●', color: color.ts},
  js: {glyph: '●', color: color.js},
  jsx: {glyph: '●', color: color.js},
  json: {glyph: '●', color: color.json},
  md: {glyph: '●', color: color.md},
  markdown: {glyph: '●', color: color.md},
  go: {glyph: '●', color: color.go},
  sh: {glyph: '●', color: color.shell},
  ps1: {glyph: '●', color: color.shell},
  yml: {glyph: '●', color: color.default},
  yaml: {glyph: '●', color: color.default},
};

const nameIconMap: Record<string, FileIcon> = {
  dockerfile: {glyph: '●', color: color.default},
  makefile: {glyph: '●', color: color.default},
  'package.json': {glyph: '●', color: color.json},
  'readme.md': {glyph: '●', color: color.md},
};

const folderIcon: FileIcon = {glyph: '▣', color: color.folder};
const fileIcon: FileIcon = {glyph: '●', color: color.default};

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
