import {languageFromPath} from '../src/utils/codeLanguage';

describe('languageFromPath', () => {
  test('resolves common languages and falls back to plaintext', () => {
    expect(languageFromPath('/a/main.ts')).toBe('typescript');
    expect(languageFromPath('/a/main.tsx')).toBe('tsx');
    expect(languageFromPath('/a/app.jsx')).toBe('jsx');
    expect(languageFromPath('/a/server.go')).toBe('go');
    expect(languageFromPath('/a/README.md')).toBe('markdown');
    expect(languageFromPath('/a/config.yaml')).toBe('yaml');
    expect(languageFromPath('/a/unknown.zzz')).toBe('plaintext');
  });
});
