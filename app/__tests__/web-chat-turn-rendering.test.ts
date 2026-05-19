import fs from 'fs';
import path from 'path';

function readMain(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
}

function readVirtualList(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(
    path.join(projectRoot, 'web', 'src', 'chat', 'ChatVirtualTurnList.tsx'),
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
    expect(main).toContain('<ChatVirtualTurnList');
    expect(main).not.toContain('const chatPromptGroups = useMemo');
    expect(main).not.toContain('const renderedChatPromptGroups = useMemo');
    expect(main).not.toContain('{renderedChatPromptGroups}');
  });

  test('uses virtualized display metadata and full-store prompt copy', () => {
    const main = readMain();
    const virtualList = readVirtualList();

    expect(virtualList).toContain('measureElement as measureVirtualElement,');
    expect(virtualList).toContain('useVirtualizer,');
    expect(virtualList).toContain('virtualItems.map(virtualItem => {');
    expect(virtualList).toContain('runtimeKey: string;');
    expect(virtualList).toContain('getItemKey: index => displayIndex.items[index]?.key ?? index,');
    expect(virtualList).toContain('estimateSize: index => resolveChatVirtualItemEstimate({');
    expect(virtualList).toContain('writeChatVirtualMeasuredHeight(chatVirtualHeightCache, {');
    expect(virtualList).toContain('resolveChatVirtualAnchor({');
    expect(virtualList).toContain('const measurements: ChatVirtualMeasurement[] = instance.getVirtualItems();');
    expect(virtualList).not.toContain('instance.measurementsCache.length > 0');
    expect(virtualList).toContain('resolveChatVirtualAnchorScrollTop({');
    expect(virtualList).toContain('preserveAnchorDuringMeasureRef.current = true;');
    expect(virtualList).toContain('selectChatVirtualPremeasureItems({');
    expect(virtualList).toContain('virtualizer.resizeItem(index, measuredHeight);');
    expect(virtualList).not.toContain('scheduleVirtualMeasureRef');
    expect(virtualList).not.toContain('scheduleAnchorRestoreRef');
    expect(virtualList).toContain('useAnimationFrameWithResizeObserver: true,');
    expect(virtualList).toContain('virtualizer.shouldAdjustScrollPositionOnItemSizeChange = (item, _delta, instance) =>');
    expect(virtualList).toContain('shouldAdjustChatVirtualItemSizeChange({');
    expect(virtualList).toContain("paddingBottom: `${rowGap}px`,");
    expect(main).toContain("import {buildChatDisplayIndex} from './chat/chatDisplayIndex';");
    expect(main).toContain("import {ChatVirtualTurnList} from './chat/ChatVirtualTurnList';");
    expect(main).toContain('runtimeKey={selectedChatEncodedKey}');
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
    expect(main).toContain('setChatShowScrollToBottom(!followsLatest);');
    expect(main).toContain('className="chat-scroll-bottom-button"');
    expect(main).toContain('className="chat-scroll-bottom-glyph"');
    expect(main).not.toContain('updateSelectedChatWindowFromScroll(event.currentTarget, direction);');
  });

  test('resets follow-bottom intent only for explicit latest-window resets', () => {
    const main = readMain();

    expect(main).toContain('resolveChatSessionReadWindowUpdate({');
    expect(main).toContain('useIncremental,');
    expect(main).toContain('followsLatest: chatAutoScrollFollowRef.current,');
    expect(main).toContain('const resettingToLatest = options?.resetToLatest === true;');
    expect(main).toContain('if (resettingToLatest && encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey) {');
    expect(main).toContain('chatAutoScrollFollowRef.current = true;');
    expect(main).toContain('chatUserScrollLockUntilRef.current = 0;');
  });
});
