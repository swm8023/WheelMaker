import fs from 'fs';
import path from 'path';

describe('web chat read-on-demand behavior', () => {
  test('connect and project switch only load session list, not session messages', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('hydrateMessages?: boolean');
    expect(mainTsx).toContain('hydrateMessages: false,');
    expect(mainTsx).toContain(
      "await loadChatSessions('', result.hydrated.projectId, {",
    );
    expect(mainTsx).toContain('if (!options?.hydrateMessages) {');
  });
});
