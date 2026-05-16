import fs from 'fs';
import path from 'path';

describe('web chat session navigation expansion', () => {
  test('uses automatic local expansion instead of a Show more button', () => {
    const projectRoot = path.join(__dirname, '..');
    const main = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(main).not.toContain('Show more');
    expect(main).toContain('projectSessionSentinelRefs');
    expect(main).toContain('new IntersectionObserver');
    expect(main).toContain('wide-project-session-sentinel');
    expect(main).toContain('selectedChatEncodedKey === buildChatRuntimeKey(targetProjectId, session.sessionId)');
  });
});
