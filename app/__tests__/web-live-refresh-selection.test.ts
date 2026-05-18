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

  test('refreshProject syncCheck uses last loaded rev refs instead of latest project metadata', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('knownProjectRev: knownProjectRevRef.current,');
    expect(mainTsx).toContain('knownGitRev: knownGitRevRef.current,');
    expect(mainTsx).toContain('knownWorktreeRev: knownWorktreeRevRef.current,');
    expect(mainTsx).not.toContain('knownGitRev: latestProject?.git?.gitRev ??');
    expect(mainTsx).not.toContain('knownWorktreeRev: latestProject?.git?.worktreeRev ??');
    expect(mainTsx).not.toContain('knownGitRevRef.current = currentProject?.git?.gitRev ??');
    expect(mainTsx).not.toContain('knownWorktreeRevRef.current = currentProject.git.worktreeRev;');
  });
});
