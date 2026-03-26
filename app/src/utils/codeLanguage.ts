const extensionToLanguage: Record<string, string> = {
  js: 'javascript',
  jsx: 'jsx',
  ts: 'typescript',
  tsx: 'tsx',
  json: 'json',
  md: 'markdown',
  markdown: 'markdown',
  yml: 'yaml',
  yaml: 'yaml',
  xml: 'xml',
  html: 'xml',
  css: 'css',
  scss: 'scss',
  go: 'go',
  rs: 'rust',
  py: 'python',
  java: 'java',
  kt: 'kotlin',
  c: 'c',
  h: 'c',
  cpp: 'cpp',
  cc: 'cpp',
  cxx: 'cpp',
  cs: 'csharp',
  php: 'php',
  sh: 'bash',
  bash: 'bash',
  zsh: 'bash',
  ps1: 'powershell',
  sql: 'sql',
  swift: 'swift',
  rb: 'ruby',
  dockerfile: 'dockerfile',
  toml: 'ini',
  ini: 'ini',
  lock: 'plaintext',
  txt: 'plaintext',
};

const fileNameToLanguage: Record<string, string> = {
  dockerfile: 'dockerfile',
  makefile: 'makefile',
  readme: 'markdown',
};

export function languageFromPath(path: string): string {
  const normalized = path.toLowerCase().trim();
  if (!normalized) {
    return 'plaintext';
  }

  const base = normalized.split('/').pop() ?? normalized;
  const pureBase = base.startsWith('.') ? base.slice(1) : base;
  if (fileNameToLanguage[pureBase]) {
    return fileNameToLanguage[pureBase];
  }

  const dot = base.lastIndexOf('.');
  if (dot < 0 || dot === base.length - 1) {
    return 'plaintext';
  }
  const ext = base.slice(dot + 1);
  return extensionToLanguage[ext] ?? 'plaintext';
}

export function isMarkdownPath(path: string): boolean {
  const language = languageFromPath(path);
  return language === 'markdown';
}
