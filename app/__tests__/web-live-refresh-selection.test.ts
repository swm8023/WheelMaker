import fs from 'fs';
import path from 'path';

describe('web live refresh uses latest selection', () => {
  test('refreshProject reads refs instead of stale closure state', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("const selectedFileRef = useRef('');");
    expect(mainTsx).toContain("const expandedDirsRef = useRef<string[]>(['.']);");
    expect(mainTsx).toContain('const currentProjectRef = useRef<RegistryProject | null>(null);');
    expect(mainTsx).toContain('const latestSelectedFile = selectedFileRef.current;');
    expect(mainTsx).toContain('await readSelectedFile(latestSelectedFile);');
  });
});
