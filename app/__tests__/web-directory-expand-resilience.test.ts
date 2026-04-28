import fs from 'fs';
import path from 'path';

describe('web directory expand resilience', () => {
  test('shows directory load failure, avoids fire-and-forget swallow, and refreshes expanded dirs sequentially', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const workspaceControllerTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'workspaceController.ts'), 'utf8');

    expect(mainTsx).toContain('Failed to load directory');
    expect(mainTsx).toContain('setExpandedDirs(prev => prev.filter(item => item !== path));');
    expect(mainTsx).not.toContain('toggleDirectory(entry.path).catch(() => undefined);');
    expect(mainTsx).toContain('workspaceController.refreshProject(projectId, [');
    expect(workspaceControllerTs).toContain('for (const dirPath of expandedSnapshot) {');
    expect(workspaceControllerTs).not.toContain('Promise.all(expandedSnapshot.map');
  });

  test('uses longer timeout for fs.list requests', () => {
    const projectRoot = path.join(__dirname, '..');
    const repositoryTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryRepository.ts'), 'utf8');

    expect(repositoryTs).toContain('method: \'fs.list\'');
    expect(repositoryTs).toContain('timeoutMs: 20000');
  });
});
