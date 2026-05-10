import fs from 'fs';
import path from 'path';

describe('web chat project scoping', () => {
  test('ignores stale chat loads from a previously selected project', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('if (activeProjectId !== projectIdRef.current) {');
    expect(mainTsx).toContain('return false;');
    expect(mainTsx).toContain('const nextSessions = sessions;');
    expect(mainTsx).toContain('setChatSessions(prev => {');
    expect(mainTsx).toContain('const byId = new Map(prev.map(item => [item.sessionId, item]));');
  });
});
