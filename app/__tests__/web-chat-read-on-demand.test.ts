import fs from 'fs';
import path from 'path';

describe('web chat hydration behavior', () => {
  test('connect and project switch hydrate selected session messages for baseline sync and reconnect recovery', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('hydrateMessages?: boolean');
    expect(mainTsx).toContain('hydrateMessages: true,');
    expect(mainTsx).toContain(
      "await loadChatSessions('', result.hydrated.projectId, {",
    );
    expect(mainTsx).toContain('if (!options?.hydrateMessages) {');
  });
});
