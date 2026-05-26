import fs from 'fs';
import path from 'path';

const root = path.resolve(__dirname, '..');
const mainTsx = fs.readFileSync(path.join(root, 'web', 'src', 'main.tsx'), 'utf8');
const stylesCss = fs.readFileSync(path.join(root, 'web', 'src', 'styles.css'), 'utf8');

function cssBlock(selector: string): string {
  return stylesCss.match(new RegExp(`${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')} \\{[\\s\\S]*?\\n\\}`))?.[0] ?? '';
}

describe('mobile chat quick switch UI source structure', () => {
  test('wires chat-page clicks on the mobile chat button to a compact quick switch menu', () => {
    expect(mainTsx).toContain("import {buildMobileChatQuickSwitchSections} from './chat/mobileChatQuickSwitch';");
    expect(mainTsx).toContain('const [chatQuickSwitchMenuOpen, setChatQuickSwitchMenuOpen] = useState(false);');
    expect(mainTsx).not.toContain('type ChatQuickSwitchPressState =');
    expect(mainTsx).not.toContain('const chatQuickSwitchTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);');
    expect(mainTsx).not.toContain('const handleChatQuickSwitchPointerDown = useCallback(');
    expect(mainTsx).toContain('const mobileChatQuickSwitchSections = useMemo(');
    expect(mainTsx).toContain('buildMobileChatQuickSwitchSections({');
    expect(mainTsx).toContain('limit: 6,');
    expect(mainTsx).toContain('const handleFloatingChatSelect = useCallback(() => {');
    expect(mainTsx).toContain("if (tab !== 'chat' || sidebarSettingsOpen) {");
    expect(mainTsx).toContain("setTab('chat');");
    expect(mainTsx).toContain('setChatQuickSwitchMenuOpen(open => !open);');
    expect(mainTsx).toContain('const handleMobileChatQuickSwitchSelect = useCallback(async (targetProjectId: string, session: RegistryChatSession) => {');
    expect(mainTsx).toContain('await selectProjectChatSession(targetProjectId, session.sessionId, {closeMobileDrawer: true});');
    expect(mainTsx).toContain("if (tab !== 'chat' || sidebarSettingsOpen) {");
    expect(mainTsx).toContain('const chatQuickSwitchMenu = chatQuickSwitchMenuOpen && tab === \'chat\' && !sidebarSettingsOpen && !mobilePortRelayFrameOpen ? (');
    expect(mainTsx).toContain('className="chat-quick-switch-menu"');
    expect(mainTsx).toContain('className="chat-quick-switch-project"');
    expect(mainTsx).toContain('className="chat-quick-switch-project-heading"');
    expect(mainTsx).toContain('className="chat-quick-switch-project-hub"');
    expect(mainTsx).toContain('className="chat-quick-switch-item"');
    expect(mainTsx).not.toContain('className="chat-quick-switch-selected codicon codicon-check"');
    expect(mainTsx).toContain('No chats');
    expect(mainTsx).toContain('onPointerDown={event => event.stopPropagation()}');
    expect(mainTsx).toContain('onClick={handleFloatingChatSelect}');
    expect(mainTsx).toContain('{chatQuickSwitchMenu}');

    expect(stylesCss).toContain('.chat-quick-switch-menu');
    expect(stylesCss).toContain('.chat-quick-switch-project');
    expect(stylesCss).toContain('.chat-quick-switch-item');
    expect(stylesCss).toContain('.chat-quick-switch-project-heading');
    expect(stylesCss).toContain('.chat-quick-switch-project-hub');
    expect(stylesCss).not.toContain('.chat-quick-switch-item::before');
    expect(stylesCss).not.toContain('.chat-quick-switch-item[data-selected=\'true\']::before');
    expect(cssBlock('.chat-quick-switch-menu')).not.toContain('overflow-y: auto;');
    expect(cssBlock('.chat-quick-switch-menu')).toContain('overflow: visible;');
    expect(cssBlock('.chat-quick-switch-item')).toContain('min-height: 32px;');
    expect(stylesCss).toContain('.chat-quick-switch-empty');
  });
});
