import fs from 'fs';
import path from 'path';

describe('web hide tool calls setting', () => {
  test('persists a default-off setting and skips tool entries only while rendering chat', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const workspacePersistence = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'),
      'utf8',
    );

    expect(workspacePersistence).toContain('hideToolCalls: boolean;');
    expect(workspacePersistence).toContain("hideToolCalls: 'hideToolCalls',");
    expect(workspacePersistence).toContain('hideToolCalls: false,');
    expect(workspacePersistence).toContain(
      "hideToolCalls: typeof input.hideToolCalls === 'boolean' ? input.hideToolCalls : base.hideToolCalls",
    );
    expect(workspacePersistence).toContain(
      '{k: GLOBAL_KEYS.hideToolCalls, v: serialize(next.hideToolCalls), updatedAt: now}',
    );

    expect(mainTsx).toContain('const [hideToolCalls, setHideToolCalls] = useState(');
    expect(mainTsx).toContain('<span>Hide Tool Calls</span>');
    expect(mainTsx).toContain('hideToolCalls={hideToolCalls}');
    expect(mainTsx).toMatch(
      /if \(hideToolCalls && entry\.kind === 'tool'\) \{\s*return null;\s*\}/,
    );

    const groupingStart = mainTsx.indexOf('function groupChatMessagesByPrompt(');
    const groupingEnd = mainTsx.indexOf('function formatChatTimestamp(', groupingStart);
    expect(groupingStart).toBeGreaterThanOrEqual(0);
    expect(groupingEnd).toBeGreaterThan(groupingStart);
    const groupingFunction = mainTsx.slice(groupingStart, groupingEnd);
    expect(groupingFunction).toContain("kind = 'tool';");
    expect(groupingFunction).not.toContain('hideToolCalls');
  });
});
