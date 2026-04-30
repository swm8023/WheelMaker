import fs from 'fs';
import path from 'path';

describe('web chat read-on-demand behavior', () => {
  test('connect and project switch only load session list; reconnect hydrates only when currently in chat with selected session', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('hydrateMessages?: boolean');
    expect(mainTsx).toContain('hydrateMessages: false,');
    expect(mainTsx).toContain('const shouldHydrateOnReconnect =');
    expect(mainTsx).toContain("tabRef.current === 'chat'");
    expect(mainTsx).toContain('hydrateMessages: shouldHydrateOnReconnect,');
    expect(mainTsx).toContain('if (!canHydrateSelection) {');
  });
});
