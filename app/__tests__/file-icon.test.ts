import {iconForPath} from '../src/utils/fileIcon';

describe('iconForPath', () => {
  test('maps common file types and has fallback', () => {
    expect(iconForPath('/a')).toEqual(expect.objectContaining({glyph: '$(folder)'}));
    expect(iconForPath('/a/main.ts').glyph).toBe('$(symbol-method)');
    expect(iconForPath('/a/README.md').glyph).toBe('$(book)');
    expect(iconForPath('/a/package.json').glyph).toBe('$(json)');
    expect(iconForPath('/a/unknown.zzz').glyph).toBe('$(file)');
  });
});
