import {iconForPath} from '../src/utils/fileIcon';

describe('iconForPath', () => {
  test('maps common file types and has fallback', () => {
    expect(iconForPath('/a')).toEqual(expect.objectContaining({name: 'folder'}));
    expect(iconForPath('/a/main.ts').name).toBe('file-code');
    expect(iconForPath('/a/README.md').name).toBe('book');
    expect(iconForPath('/a/package.json').name).toBe('json');
    expect(iconForPath('/a/unknown.zzz').name).toBe('file');
  });
});
