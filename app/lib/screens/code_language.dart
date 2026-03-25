String languageFromPath(String path) {
  final lower = path.toLowerCase();
  if (lower.endsWith('.dart')) return 'dart';
  if (lower.endsWith('.go')) return 'go';
  if (lower.endsWith('.cpp') ||
      lower.endsWith('.cc') ||
      lower.endsWith('.cxx') ||
      lower.endsWith('.c') ||
      lower.endsWith('.hpp') ||
      lower.endsWith('.hh') ||
      lower.endsWith('.hxx') ||
      lower.endsWith('.h')) {
    return 'cpp';
  }
  if (lower.endsWith('.json')) return 'json';
  if (lower.endsWith('.yaml') || lower.endsWith('.yml')) return 'yaml';
  if (lower.endsWith('.md')) return 'markdown';
  if (lower.endsWith('.ps1')) return 'powershell';
  return 'plaintext';
}
