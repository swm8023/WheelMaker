import fs from 'fs';
import path from 'path';

describe('web hide tool calls setting', () => {
  test('persists a default-on setting and skips tool entries only while rendering chat', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const workspacePersistence = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'),
      'utf8',
    );

    expect(workspacePersistence).toContain('hideToolCalls: boolean;');
    expect(workspacePersistence).toContain("hideToolCalls: 'hideToolCalls',");
    expect(workspacePersistence).toContain('hideToolCalls: true,');
    expect(workspacePersistence).toContain(
      "hideToolCalls: typeof input.hideToolCalls === 'boolean' ? input.hideToolCalls : base.hideToolCalls",
    );
    expect(workspacePersistence).toContain(
      '{k: GLOBAL_KEYS.hideToolCalls, v: serialize(next.hideToolCalls), updatedAt: now}',
    );

    expect(mainTsx).toContain('const [hideToolCalls, setHideToolCalls] = useState(');
    expect(mainTsx).toMatch(
      /typeof persistedGlobal\.hideToolCalls === 'boolean'\r?\n\s*\? persistedGlobal\.hideToolCalls\r?\n\s*: true/,
    );
    expect(mainTsx).toContain("renderSettingsSection('Chat'");
    const chatSettingsStart = mainTsx.indexOf("renderSettingsSection('Chat'");
    const hideToolCallsSettingStart = mainTsx.indexOf('Hide Tool Calls', chatSettingsStart);
    expect(mainTsx).not.toContain('Use Latest Prompt Title');
    expect(hideToolCallsSettingStart).toBeGreaterThan(chatSettingsStart);
    expect(mainTsx).not.toContain('checked={useLatestPromptTitle}');
    expect(mainTsx).not.toContain('onChange={e => setUseLatestPromptTitle(e.target.checked)}');
    expect(mainTsx).toContain('Hide Tool Calls');
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
