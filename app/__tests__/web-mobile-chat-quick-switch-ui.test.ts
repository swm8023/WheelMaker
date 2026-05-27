import fs from 'fs';
import path from 'path';

const root = path.resolve(__dirname, '..');
const mainTsx = fs.readFileSync(path.join(root, 'web', 'src', 'main.tsx'), 'utf8');
const stylesCss = fs.readFileSync(path.join(root, 'web', 'src', 'styles.css'), 'utf8');
const quickSwitchMenuPath = path.join(root, 'web', 'src', 'chat', 'ChatQuickSwitchMenu.tsx');
const quickSwitchMenuTsx = fs.existsSync(quickSwitchMenuPath)
  ? fs.readFileSync(quickSwitchMenuPath, 'utf8')
  : '';

function cssBlock(selector: string): string {
  return stylesCss.match(new RegExp(`${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')} \\{[\\s\\S]*?\\n\\}`))?.[0] ?? '';
}

describe('mobile chat quick switch UI source structure', () => {
  test('wires chat-page clicks on the mobile chat button to a compact quick switch menu', () => {
    expect(mainTsx).toContain("import {buildMobileChatQuickSwitchSections} from './chat/mobileChatQuickSwitch';");
    expect(mainTsx).toContain("import {ChatQuickSwitchMenu} from './chat/ChatQuickSwitchMenu';");
    expect(mainTsx).toContain("import {resolveDesktopChatQuickSwitchContextMenu} from './shell/desktop/chatQuickSwitchContextMenu';");
    expect(mainTsx).toContain('const [chatQuickSwitchMenuOpen, setChatQuickSwitchMenuOpen] = useState(false);');
    expect(mainTsx).toContain("const [chatQuickSwitchMenuPlacement, setChatQuickSwitchMenuPlacement] = useState<ChatQuickSwitchMenuPlacement>({kind: 'mobile'});");
    expect(mainTsx).not.toContain('type ChatQuickSwitchPressState =');
    expect(mainTsx).not.toContain('const chatQuickSwitchTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);');
    expect(mainTsx).not.toContain('const handleChatQuickSwitchPointerDown = useCallback(');
    expect(mainTsx).toContain('const mobileChatQuickSwitchSections = useMemo(');
    expect(mainTsx).toContain('buildMobileChatQuickSwitchSections({');
    expect(mainTsx).toContain('limit: 6,');
    expect(mainTsx).toContain('const handleFloatingChatSelect = useCallback(() => {');
    expect(mainTsx).toContain("if (tab !== 'chat' || sidebarSettingsOpen) {");
    expect(mainTsx).toContain("setTab('chat');");
    expect(mainTsx).toContain("setChatQuickSwitchMenuPlacement({kind: 'mobile'});");
    expect(mainTsx).toContain('setChatQuickSwitchMenuOpen(open => !open);');
    expect(mainTsx).toContain('const handleMobileChatQuickSwitchSelect = useCallback(async (targetProjectId: string, session: RegistryChatSession) => {');
    expect(mainTsx).toContain('await selectProjectChatSession(targetProjectId, session.sessionId, {closeMobileDrawer: true});');
    expect(mainTsx).toContain("if (tab !== 'chat' || sidebarSettingsOpen) {");
    expect(mainTsx).toContain('const handleChatQuickSwitchContextMenu = useCallback((event: React.MouseEvent<HTMLDivElement>) => {');
    expect(mainTsx).toContain('selectedText: window.getSelection()?.toString() ?? \'\',');
    expect(mainTsx).toContain('event.preventDefault();');
    expect(mainTsx).toContain("setChatQuickSwitchMenuPlacement({kind: 'desktop', style: result.style});");
    expect(mainTsx).toContain('onContextMenu={handleChatQuickSwitchContextMenu}');
    expect(mainTsx).toContain("if (chatQuickSwitchMenuPlacement.kind === 'desktop') {");
    expect(mainTsx).toContain('setChatQuickSwitchMenuOpen(false);');
    expect(mainTsx).toContain('const chatQuickSwitchMenu = chatQuickSwitchMenuOpen && tab === \'chat\' && !sidebarSettingsOpen && !mobilePortRelayFrameOpen ? (');
    expect(mainTsx).toContain('<ChatQuickSwitchMenu');
    expect(mainTsx).toContain('placement={chatQuickSwitchMenuPlacement.kind}');
    expect(quickSwitchMenuTsx).toContain('className="chat-quick-switch-menu"');
    expect(quickSwitchMenuTsx).toContain('className="chat-quick-switch-project"');
    expect(quickSwitchMenuTsx).toContain('className="chat-quick-switch-project-heading"');
    expect(quickSwitchMenuTsx).toContain('className="chat-quick-switch-project-hub"');
    expect(quickSwitchMenuTsx).toContain('className="chat-quick-switch-item"');
    expect(quickSwitchMenuTsx).not.toContain('className="chat-quick-switch-selected codicon codicon-check"');
    expect(quickSwitchMenuTsx).toContain('No chats');
    expect(quickSwitchMenuTsx).toContain('onPointerDown={event => event.stopPropagation()}');
    expect(mainTsx).toContain('onClick={handleFloatingChatSelect}');
    expect(mainTsx).toContain("chatQuickSwitchMenuPlacement.kind === 'mobile' ? chatQuickSwitchMenu : null");
    expect(mainTsx).toContain("chatQuickSwitchMenuPlacement.kind === 'desktop' ? chatQuickSwitchMenu : null");

    expect(stylesCss).toContain('.chat-quick-switch-menu');
    expect(stylesCss).toContain(".chat-quick-switch-menu[data-placement='desktop']");
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
