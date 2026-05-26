import fs from 'fs';
import path from 'path';

describe('web session search UI wiring', () => {
  test('keeps search in the session list area with prompt turn navigation', () => {
    const projectRoot = path.join(__dirname, '..');
    const main = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const styles = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(main).toContain('renderSessionSearchControls');
    expect(main).toContain('searchResultsByProjectId');
    expect(main).toContain('startSessionSearch');
    expect(main).toContain('querySessionSearch');
    expect(main).toContain('cancelSessionSearch');
    expect(main).toContain('scrollToTurnIndex');
    expect(main).toContain('sessionSearchTargetTurn');
    expect(styles).toContain('.session-search-control');
    expect(styles).toContain('.session-search-result-meta');
    expect(styles).toContain('.chat-turn-search-highlight');
  });
});
