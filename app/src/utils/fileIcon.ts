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
  ts: {glyph: '$(symbol-method)', color: color.ts},
  tsx: {glyph: '$(symbol-method)', color: color.ts},
  js: {glyph: '$(symbol-method)', color: color.js},
  jsx: {glyph: '$(symbol-method)', color: color.js},
  json: {glyph: '$(json)', color: color.json},
  md: {glyph: '$(book)', color: color.md},
  markdown: {glyph: '$(book)', color: color.md},
  go: {glyph: '$(symbol-namespace)', color: color.go},
  sh: {glyph: '$(terminal)', color: color.shell},
  ps1: {glyph: '$(terminal)', color: color.shell},
  yml: {glyph: '$(settings)', color: color.default},
  yaml: {glyph: '$(settings)', color: color.default},
};

const nameIconMap: Record<string, FileIcon> = {
  dockerfile: {glyph: '$(package)', color: color.default},
  makefile: {glyph: '$(tools)', color: color.default},
  'package.json': {glyph: '$(json)', color: color.json},
  'readme.md': {glyph: '$(book)', color: color.md},
};

const folderIcon: FileIcon = {glyph: '$(folder)', color: color.folder};
const fileIcon: FileIcon = {glyph: '$(file)', color: color.default};

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
