type FileIcon = {
  name: string;
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
  ts: {name: 'file-code', color: color.ts},
  tsx: {name: 'file-code', color: color.ts},
  js: {name: 'file-code', color: color.js},
  jsx: {name: 'file-code', color: color.js},
  json: {name: 'json', color: color.json},
  md: {name: 'book', color: color.md},
  markdown: {name: 'book', color: color.md},
  go: {name: 'symbol-namespace', color: color.go},
  sh: {name: 'terminal', color: color.shell},
  ps1: {name: 'terminal', color: color.shell},
  yml: {name: 'settings', color: color.default},
  yaml: {name: 'settings', color: color.default},
};

const nameIconMap: Record<string, FileIcon> = {
  dockerfile: {name: 'package', color: color.default},
  makefile: {name: 'tools', color: color.default},
  'package.json': {name: 'json', color: color.json},
  'readme.md': {name: 'book', color: color.md},
};

const folderIcon: FileIcon = {name: 'folder', color: color.folder};
const fileIcon: FileIcon = {name: 'file', color: color.default};

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
