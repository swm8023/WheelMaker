import fs from 'fs';
import path from 'path';

function readMain(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
}

function readVirtualList(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(
    path.join(projectRoot, 'web', 'src', 'chat', 'ChatVirtuosoTurnList.tsx'),
    'utf8',
  );
}

function readStyles(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');
}

describe('web chat turn rendering', () => {
  test('renders chat turns through the virtual display index instead of prompt groups', () => {
    const main = readMain();

    expect(main).toContain('const ChatTurnView = React.memo');
    expect(main).toContain('const chatDisplayIndex = useMemo(() => buildChatDisplayIndex(chatMessages');
    expect(main).toContain('<ChatVirtuosoTurnList');
    expect(main).not.toContain('const chatPromptGroups = useMemo');
    expect(main).not.toContain('const renderedChatPromptGroups = useMemo');
    expect(main).not.toContain('{renderedChatPromptGroups}');
  });

  test('uses virtualized display metadata and full-store prompt copy', () => {
    const main = readMain();
    const virtualList = readVirtualList();

    expect(virtualList).toContain("import {Virtuoso, type Components, type VirtuosoHandle} from 'react-virtuoso';");
    expect(virtualList).toContain('export type ChatVirtuosoTurnListHandle = {');
    expect(virtualList).toContain('React.useImperativeHandle(ref, () => ({');
    expect(virtualList).toContain('const scrollToLastDisplayItem = React.useCallback(');
    expect(virtualList).toContain("index: 'LAST',");
    expect(virtualList).toContain("align: 'end',");
    expect(virtualList).toContain('offset: virtuosoContext.bottomBuffer,');
    expect(virtualList).toContain('virtuosoRef.current?.autoscrollToBottom();');
    expect(virtualList).toContain('components={ChatVirtuosoComponents}');
    expect(virtualList).toContain('context={virtuosoContext}');
    expect(virtualList).toContain('itemContent={(index, displayItem) => {');
    expect(virtualList).toContain('runtimeKey: string;');
    expect(virtualList).toContain('key={runtimeKey}');
    expect(virtualList).toContain('customScrollParent={scrollParent}');
    expect(virtualList).toContain('computeItemKey={(index, item) => item.key}');
    expect(virtualList).toContain('const initialTopMostItemIndex = React.useMemo(');
    expect(virtualList).toContain('initialTopMostItemIndex={initialTopMostItemIndex}');
    expect(virtualList).toContain('increaseViewportBy={{top: viewportIncrease, bottom: viewportIncrease}}');
    expect(virtualList).toContain('atBottomStateChange={handleAtBottomStateChange}');
    expect(virtualList).toContain("followOutput={() => (shouldAutoscrollNow() ? 'auto' : false)}");
    expect(virtualList).toContain('totalListHeightChanged={handleTotalListHeightChanged}');
    expect(virtualList).toContain('className="chat-virtuoso-footer"');
    expect(virtualList).not.toContain('@tanstack/react-virtual');
    expect(virtualList).not.toContain('chatVirtualMeasurements');
    expect(virtualList).not.toContain('shouldAdjustChatVirtualItemSizeChange');
    expect(main).toContain("import {buildChatDisplayIndex} from './chat/chatDisplayIndex';");
    expect(main).toContain("import {ChatVirtuosoTurnList, type ChatVirtuosoTurnListHandle} from './chat/ChatVirtuosoTurnList';");
    expect(main).toContain('const chatVirtuosoListRef = useRef<ChatVirtuosoTurnListHandle | null>(null);');
    expect(main).toContain("chatVirtuosoListRef.current?.scrollToBottom('auto');");
    expect(main).not.toContain('chatVirtuosoListRef.current?.autoscrollToBottom();');
    expect(main).toContain('const handleChatAtBottomChange = useCallback((atBottom: boolean) => {');
    expect(main).toContain('setChatShowScrollToBottom(!atBottom);');
    expect(main).toContain('ref={chatVirtuosoListRef}');
    expect(main).toContain('atBottomThreshold={CHAT_AUTO_SCROLL_BOTTOM_THRESHOLD}');
    expect(main).toContain('onAtBottomChange={handleChatAtBottomChange}');
    expect(main).toContain('shouldAutoscroll={shouldAutoscrollChat}');
    expect(main).toContain('runtimeKey={selectedChatEncodedKey}');
    expect(main).not.toContain('container.scrollTop = nextScrollTop;');
    expect(main).not.toContain('resolveChatBottomScrollTop');
    expect(main).not.toContain("from './chat/chatTurnWindow'");
    expect(main).toContain('buildPromptDoneCopyRange(selectedFullChatMessages, doneTurnIndex)');
    expect(main).toContain('copyDisabled={copyRange ? !copyRange.ok : true}');
  });

  test('matches virtual row gap to the normal markdown paragraph gap', () => {
    const virtualList = readVirtualList();
    const styles = readStyles();
    const paragraphMargin = styles.match(/\.chat-main-message p,[\s\S]*?margin: 0 0 (\d+)px 0;/)?.[1] ?? '';

    expect(paragraphMargin).toBe('7');
    expect(virtualList).toContain('rowGap = 7,');
    expect(virtualList).not.toContain('overscan = 12');
  });

  test('renders prompt responding status and delivery states for pending prompts', () => {
    const main = readMain();

    expect(main).toContain("import { resolvePromptDoneStatus, resolvePromptTurnStatus, type ChatPromptStatus } from './chat/chatPromptStatus';");
    expect(main).toContain('promptStatus?: ChatPromptStatus;');
    expect(main).toContain("promptStatus === 'responding'");
    expect(main).toContain("promptStatus === 'confirming'");
    expect(main).toContain("promptStatus === 'undelivered'");
    expect(main).toContain('className="chat-prompt-status-dots"');
    expect(main).toContain('className="chat-prompt-delivery-line"');
    expect(main).toContain('onRetryPendingPrompt?: () => void;');
    expect(main).toContain('onEditPendingPrompt?: () => void;');
    expect(main).toContain('const [chatPendingPromptsByKey, setChatPendingPromptsByKey] = useState');
    expect(main).toContain('const chatPendingPromptTimersRef = useRef<Record<string, number>>({});');
    expect(main).toContain('rememberPendingChatPrompt(runtimeKey, {');
    expect(main).toContain("status: 'confirming',");
    expect(main).toContain('markPendingChatPromptUndelivered(runtimeKey');
    expect(main).toContain('forgetPendingChatPrompt(runtimeKey);');
    expect(main).toContain('chatMessages.length === 0 && !selectedPendingPrompt');

    const pendingIndex = main.indexOf('rememberPendingChatPrompt(runtimeKey, {');
    const sendIndex = main.indexOf('const result = await service.sendProjectSessionMessage(selectedProjectId, {');
    expect(pendingIndex).toBeGreaterThanOrEqual(0);
    expect(sendIndex).toBeGreaterThan(pendingIndex);
  });

  test('renders prompt done stop reason labels without disabling copied partial output', () => {
    const main = readMain();

    expect(main).toContain('const doneStatus = resolvePromptDoneStatus(message.param);');
    expect(main).toContain('className={`chat-prompt-stop-reason ${doneStatus.kind}`}');
    expect(main).toContain('className={`chat-prompt-result-line ${doneStatus.kind}`}');
    expect(main).toContain('copyDisabled={copyRange ? !copyRange.ok : true}');
    expect(main).not.toContain("copyDisabled={failed ? true :");
  });

  test('renders a persistent composer cancel button driven by running session state', () => {
    const main = readMain();

    expect(main).toContain('const selectedChatPromptRunning =');
    expect(main).toContain('selectedChatSession?.running === true');
    expect(main).not.toContain('chatRunningSessionFlags[selectedChatEncodedKey] === true');
    expect(main).toContain('const [chatCancellingRuntimeKey, setChatCancellingRuntimeKey] = useState');
    expect(main).toContain('const cancelSelectedChatPrompt = async () => {');
    expect(main).toContain('service.cancelProjectSession(selectedKey.projectId, selectedKey.sessionId)');
    expect(main).toContain('className="chat-composer-input-row"');
    expect(main).toContain('className={`chat-composer-stop-trigger${selectedChatPromptRunning ? \' active\' : \'\'}`}');
    expect(main).toContain('disabled={!selectedChatPromptRunning || selectedChatPromptCancelling}');
  });

  test('shows a scroll-to-bottom button when the user is away from the bottom', () => {
    const main = readMain();

    expect(main).toContain('const [chatShowScrollToBottom, setChatShowScrollToBottom] = useState(false);');
    expect(main).toContain('setChatShowScrollToBottom(!atBottom);');
    expect(main).toContain('className="chat-scroll-bottom-button"');
    expect(main).toContain('className="chat-scroll-bottom-glyph"');
    expect(main).not.toContain('updateSelectedChatWindowFromScroll(event.currentTarget, direction);');
  });

  test('resets follow-bottom intent only for explicit latest-window resets', () => {
    const main = readMain();

    expect(main).toContain('resolveChatSessionReadWindowUpdate({');
    expect(main).toContain('useIncremental: appliedAfterTurnIndex > 0,');
    expect(main).toContain('followsLatest: chatAutoScrollFollowRef.current,');
    expect(main).toContain('const resettingToLatest = options?.resetToLatest === true;');
    expect(main).toContain('if (resettingToLatest && encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey) {');
    expect(main).toContain('chatAutoScrollFollowRef.current = true;');
    expect(main).toContain('chatUserScrollLockUntilRef.current = 0;');
  });
});
