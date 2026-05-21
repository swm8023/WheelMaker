import fs from 'fs';
import path from 'path';
import {
  CHAT_FONT_OPTIONS,
  DEFAULT_CHAT_FONT,
  isChatFontId,
  resolveChatFontFamily,
} from '../web/src/chat/chatTypography';

describe('web chat font settings', () => {
  test('keeps the current IBM Plex chat font as the default while offering a clearer system option', () => {
    expect(DEFAULT_CHAT_FONT).toBe('ibm-plex');
    expect(CHAT_FONT_OPTIONS.map(option => option.id)).toEqual([
      'ibm-plex',
      'system',
      'serif',
    ]);
    expect(resolveChatFontFamily(DEFAULT_CHAT_FONT)).toContain('IBM Plex Sans');
    expect(resolveChatFontFamily('system')).toContain('Segoe UI');
    expect(resolveChatFontFamily('system')).toContain('Microsoft YaHei UI');
    expect(isChatFontId('system')).toBe(true);
    expect(isChatFontId('bad-font')).toBe(false);
  });

  test('persists the chat font setting and exposes it only in Chat settings', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const persistence = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'),
      'utf8',
    );

    expect(persistence).toContain('chatFont: ChatFontId;');
    expect(persistence).toContain("chatFont: 'chatFont',");
    expect(persistence).toContain('chatFont: DEFAULT_CHAT_FONT,');
    expect(persistence).toContain("chatFont: typeof input.chatFont === 'string' && isChatFontId(input.chatFont) ? input.chatFont : base.chatFont");
    expect(persistence).toContain('{k: GLOBAL_KEYS.chatFont, v: serialize(next.chatFont), updatedAt: now}');

    expect(mainTsx).toContain('const [chatFont, setChatFont] = useState<ChatFontId>(');
    expect(mainTsx).toContain('const chatFontFamily = useMemo(');
    expect(mainTsx).toContain('chatFont,');
    expect(mainTsx).toContain("renderSettingsSection('Chat'");
    expect(mainTsx).toContain('Chat Font');
    expect(mainTsx).toContain('value={chatFont}');
    expect(mainTsx).toContain('if (isChatFontId(next)) setChatFont(next);');
    expect(mainTsx).toContain('CHAT_FONT_OPTIONS.map(item => (');
    expect(mainTsx).toContain('style={chatMainStyle}');
  });

  test('applies chat font through message CSS without changing code or composer fonts', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain("'--chat-message-font-family': chatFontFamily,");
    expect(stylesCss).toMatch(
      /\.chat-main-message \{[\s\S]*font-family: var\(--chat-message-font-family, 'IBM Plex Sans', 'Noto Sans', sans-serif\);[\s\S]*font-weight: 400;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.chat-main-message code:not\(\.wm-shiki-code\) \{[\s\S]*font-family: 'JetBrains Mono', Consolas, 'Courier New', monospace;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.chat-composer-input \{[\s\S]*font: inherit;[\s\S]*font-size: 14px;[\s\S]*\}/,
    );
  });
});
