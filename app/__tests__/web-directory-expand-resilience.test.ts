import fs from 'fs';
import path from 'path';

describe('web directory expand resilience', () => {
  test('shows directory load failure, avoids fire-and-forget swallow, and refreshes expanded dirs sequentially', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('Failed to load directory');
    expect(mainTsx).toContain('setExpandedDirs(prev => prev.filter(item => item !== path));');
    expect(mainTsx).not.toContain('toggleDirectory(entry.path).catch(() => undefined);');
    expect(mainTsx).toContain('for (const path of latestExpandedDirs) {');
    expect(mainTsx).not.toContain('Promise.all(latestExpandedDirs.map(path => loadDirectory(path)))');
  });

  test('uses longer timeout for fs.list requests', () => {
    const projectRoot = path.join(__dirname, '..');
    const repositoryTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryRepository.ts'), 'utf8');

    expect(repositoryTs).toContain('method: \'fs.list\'');
    expect(repositoryTs).toContain('timeoutMs: 20000');
  });
});
