import {iconForPath} from '../src/utils/fileIcon';

describe('iconForPath', () => {
  test('maps common file types and has fallback', () => {
    expect(iconForPath('/a', {isDir: true})).toEqual(
      expect.objectContaining({fontFamily: 'vscode-seti'}),
    );
    expect(iconForPath('/a/main.ts').glyph.length).toBeGreaterThan(0);
    expect(iconForPath('/a/README.md').glyph.length).toBeGreaterThan(0);
    expect(iconForPath('/a/package.json').glyph.length).toBeGreaterThan(0);
    expect(iconForPath('/a/unknown.zzz').glyph.length).toBeGreaterThan(0);
  });
});
