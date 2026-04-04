import fs from 'fs';
import path from 'path';

describe('web file read cache on notModified', () => {
  test('restores cached content when fs.read returns notModified', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const fileCacheRef = useRef<Record<string, string>>({});');
    expect(mainTsx).toContain('if (result.notModified) {');
    expect(mainTsx).toContain('const cachedContent = fileCacheRef.current[path];');
    expect(mainTsx).toContain("setFileContent(typeof cachedContent === 'string' ? cachedContent : '');");
    expect(mainTsx).toContain('fileCacheRef.current[path] = result.content;');
  });
});
