import fs from 'fs';
import path from 'path';

function readMain(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
}

describe('web chat turn rendering', () => {
  test('renders visible turns directly instead of prompt groups', () => {
    const main = readMain();

    expect(main).toContain('const ChatTurnView = React.memo');
    expect(main).toContain('const renderedChatTurns = chatMessages.map');
    expect(main).toContain('{renderedChatTurns}');
    expect(main).not.toContain('const chatPromptGroups = useMemo');
    expect(main).not.toContain('const renderedChatPromptGroups = useMemo');
    expect(main).not.toContain('{renderedChatPromptGroups}');
  });

  test('uses turn windows and full-store prompt copy', () => {
    const main = readMain();

    expect(main).toContain('createLatestTurnWindow(fullMessages)');
    expect(main).toContain('sliceTurnsForWindow(fullMessages, nextWindow)');
    expect(main).toContain('expandTurnWindowEarlier(fullMessages, currentWindow)');
    expect(main).toContain('expandTurnWindowLater(fullMessages, currentWindow)');
    expect(main).toContain('followLatestTurnWindow(fullMessages, currentWindow)');
    expect(main).toContain('buildPromptDoneCopyRange(selectedFullChatMessages, doneTurnIndex)');
    expect(main).toContain('copyDisabled={copyRange ? !copyRange.ok : true}');
  });

  test('renders prompt responding status and optimistic pending prompt while sending', () => {
    const main = readMain();

    expect(main).toContain("import { resolvePromptTurnStatus, type ChatPromptStatus } from './chat/chatPromptStatus';");
    expect(main).toContain('promptStatus?: ChatPromptStatus;');
    expect(main).toContain("promptStatus === 'responding'");
    expect(main).toContain('className="chat-prompt-status-dots"');
    expect(main).toContain('const [chatPendingPromptsByKey, setChatPendingPromptsByKey] = useState');
    expect(main).toContain('rememberPendingChatPrompt(runtimeKey, {');
    expect(main).toContain('forgetPendingChatPrompt(runtimeKey);');
    expect(main).toContain('chatMessages.length === 0 && !selectedPendingPrompt');

    const pendingIndex = main.indexOf('rememberPendingChatPrompt(runtimeKey, {');
    const sendIndex = main.indexOf('const result = await service.sendProjectSessionMessage(selectedProjectId, {');
    expect(pendingIndex).toBeGreaterThanOrEqual(0);
    expect(sendIndex).toBeGreaterThan(pendingIndex);
  });

  test('shows a scroll-to-bottom button when the user is away from the bottom', () => {
    const main = readMain();

    expect(main).toContain('const [chatShowScrollToBottom, setChatShowScrollToBottom] = useState(false);');
    expect(main).toContain('setChatShowScrollToBottom(!followsLatest);');
    expect(main).toContain('className="chat-scroll-bottom-button"');
    expect(main).toContain('className="chat-scroll-bottom-glyph"');
    expect(main).toContain('updateSelectedChatWindowFromScroll(event.currentTarget, direction);');
  });
});
