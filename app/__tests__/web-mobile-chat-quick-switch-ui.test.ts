import fs from 'fs';
import path from 'path';

const root = path.resolve(__dirname, '..');
const mainTsx = fs.readFileSync(path.join(root, 'web', 'src', 'main.tsx'), 'utf8');
const stylesCss = fs.readFileSync(path.join(root, 'web', 'src', 'styles.css'), 'utf8');

describe('mobile chat quick switch UI source structure', () => {
  test('wires long press on the mobile chat button to a compact quick switch menu', () => {
    expect(mainTsx).toContain("import {buildMobileChatQuickSwitchSections} from './chat/mobileChatQuickSwitch';");
    expect(mainTsx).toContain('const [chatQuickSwitchMenuOpen, setChatQuickSwitchMenuOpen] = useState(false);');
    expect(mainTsx).toContain('const chatQuickSwitchTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);');
    expect(mainTsx).toContain('const mobileChatQuickSwitchSections = useMemo(');
    expect(mainTsx).toContain('buildMobileChatQuickSwitchSections({');
    expect(mainTsx).toContain('limit: 6,');
    expect(mainTsx).toContain('const handleChatQuickSwitchPointerDown = useCallback(');
    expect(mainTsx).toContain('setChatQuickSwitchMenuOpen(true);');
    expect(mainTsx).toContain('const handleMobileChatQuickSwitchSelect = useCallback(async (targetProjectId: string, session: RegistryChatSession) => {');
    expect(mainTsx).toContain('await selectProjectChatSession(targetProjectId, session.sessionId, {closeMobileDrawer: true});');
    expect(mainTsx).toContain('const chatQuickSwitchMenu = chatQuickSwitchMenuOpen && !mobilePortRelayFrameOpen ? (');
    expect(mainTsx).toContain('className="chat-quick-switch-menu"');
    expect(mainTsx).toContain('className="chat-quick-switch-project"');
    expect(mainTsx).toContain('className="chat-quick-switch-item"');
    expect(mainTsx).toContain('No chats');
    expect(mainTsx).toContain('onPointerDown={handleChatQuickSwitchPointerDown}');
    expect(mainTsx).toContain('{chatQuickSwitchMenu}');

    expect(stylesCss).toContain('.chat-quick-switch-menu');
    expect(stylesCss).toContain('.chat-quick-switch-project');
    expect(stylesCss).toContain('.chat-quick-switch-item');
    expect(stylesCss).toContain('.chat-quick-switch-empty');
  });
});
