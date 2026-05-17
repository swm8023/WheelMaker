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
    expect(mainTsx).toContain("renderSettingsSection('Chat'");
    expect(mainTsx).toContain('<span>Hide Tool Calls</span>');
    expect(mainTsx).toContain('hideToolCalls={hideToolCalls}');
    expect(mainTsx).toMatch(
      /if \(hideToolCalls && kind === 'tool'\) \{\s*return null;\s*\}/,
    );

    const turnStart = mainTsx.indexOf('const ChatTurnView = React.memo(function ChatTurnView(');
    const turnEnd = mainTsx.indexOf('function formatChatTimestamp(', turnStart);
    expect(turnStart).toBeGreaterThanOrEqual(0);
    expect(turnEnd).toBeGreaterThan(turnStart);
    const turnView = mainTsx.slice(turnStart, turnEnd);
    expect(turnView).toContain("if (kind === 'tool') {");
    expect(turnView).not.toContain('groupChatMessagesByPrompt');
  });
});
