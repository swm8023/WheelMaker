import fs from 'fs';
import path from 'path';

function cssRuleBlock(stylesCss: string, selector: string): string {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = stylesCss.match(new RegExp(`${escapedSelector} \\{([\\s\\S]*?)\\}`));
  return match?.[1] ?? '';
}

function cssNumericProperty(stylesCss: string, selector: string, property: string): number {
  const block = cssRuleBlock(stylesCss, selector);
  const escapedProperty = property.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = block.match(new RegExp(`${escapedProperty}:\\s*(\\d+)\\s*;`));
  return match ? Number(match[1]) : Number.NaN;
}

describe('web chat integration', () => {
  test('defines registry session protocol and uses real chat UI instead of placeholder sessions', () => {
    const projectRoot = path.join(__dirname, '..');
    const registryTypes = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'types', 'registry.ts'), 'utf8');
    const repositoryTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryRepository.ts'), 'utf8');
    const workspaceServiceTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryWorkspaceService.ts'), 'utf8');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(registryTypes).toContain('export interface RegistrySessionSummary');
    expect(registryTypes).toContain('export interface RegistrySessionMessage');
    expect(registryTypes).not.toContain('promptIndex: number;');
    expect(registryTypes).toContain('turnIndex: number;');
    expect(registryTypes).toContain('finished: boolean;');
    expect(registryTypes).not.toContain('done?: boolean;');
    expect(registryTypes).not.toContain('export interface RegistrySessionPromptSnapshot');
    expect(registryTypes).not.toContain('updateIndex: number;');
    expect(registryTypes).not.toContain('lastIndex');
    expect(repositoryTs).toContain("method: 'session.list'");
    expect(repositoryTs).toContain("method: 'session.read'");
    expect(repositoryTs).toContain('payload: afterTurnIndex > 0 ? {sessionId, afterTurnIndex} : {sessionId}');
    expect(repositoryTs).toContain('turns?: unknown[];');
    expect(repositoryTs).toContain('normalizeSessionReadPayload(');
    expect(registryTypes).toContain('export interface RegistrySessionTurn');
    expect(registryTypes).toContain('turn: RegistrySessionTurn;');
    expect(repositoryTs).not.toContain('prompts: []');
    expect(registryTypes).toContain('session?: RegistrySessionSummary;');
    expect(repositoryTs).not.toContain('afterIndex');
    expect(repositoryTs).not.toContain('afterSubIndex');
    expect(repositoryTs).toContain("method: 'session.new'");
    expect(registryTypes).toContain('agentType?: string;');
    expect(registryTypes).toContain('agents?: string[];');
    expect(repositoryTs).toContain('async createSession(projectId: string, agentType: string, title?: string)');
    expect(repositoryTs).toContain('payload: title?.trim() ? {agentType, title: title.trim()} : {agentType}');
    expect(repositoryTs).toContain("method: 'session.send'");
    expect(repositoryTs).toContain("method: 'session.markRead'");
    expect(repositoryTs).not.toContain('turnId = typeof input.turnId');
    expect(repositoryTs).not.toContain("method: 'chat.permission.respond'");
    expect(workspaceServiceTs).toContain('async listSessions(');
    expect(workspaceServiceTs).toContain('async readSession(');
    expect(workspaceServiceTs).toContain('async createSession(');
    expect(workspaceServiceTs).toContain('async createSession(agentType: string, title?: string)');
    expect(workspaceServiceTs).toContain('async sendSessionMessage(');
    expect(workspaceServiceTs).toContain('async markSessionRead(');
    expect(workspaceServiceTs).toContain('async markProjectSessionRead(');
    expect(workspaceServiceTs).not.toContain('async respondToSessionPermission(');
    expect(workspaceServiceTs).toContain('private eventListeners = new Set');
    expect(workspaceServiceTs).toContain('private closeListeners = new Set');
    expect(registryTypes).not.toContain('turnId: string;');
    expect(registryTypes).not.toContain('turnId?: string;');
    expect(mainTsx).toContain('chatComposerText');
    expect(mainTsx).toContain('chatMessages');
    expect(mainTsx).toContain('session.message');
    expect(mainTsx).toContain('normalizeSessionMessagePayload(payload)');
    expect(mainTsx).toContain('decodeSessionTurnToMessage(normalizedPayload.sessionId, normalizedPayload.turn)');
    expect(mainTsx).not.toContain('updateIndex');
    expect(mainTsx).toContain('service.markProjectSessionRead(activeProjectId, sessionId, cursor)');
    expect(mainTsx).toContain('chatFinishedCursorRef');
    expect(mainTsx).not.toContain('chatSyncIndexRef');
    expect(mainTsx).not.toContain('chatPromptSnapshotVersion');
    expect(mainTsx).toContain('resolveChatListSelection({');
    expect(mainTsx).not.toContain('result.lastIndex < afterIndex');
    expect(mainTsx).toContain('preserveUserSelection');
    expect(mainTsx).toContain('const canApplyLoadedSelection = shouldApplyLoadedChatSelection(');
    expect(mainTsx).toContain('selectionSnapshot');
    expect(mainTsx).toContain('const nextSelectedKey = chatSessionKeyFromParts(activeProjectId, resultSessionId);');
    expect(mainTsx).toContain('applySelectedChatKey(nextSelectedKey);');
    expect(mainTsx).toContain('workspaceStore.rememberSelectedChatSessionKey(nextSelectedKey);');
    expect(mainTsx).toContain('sessionId');
    expect(mainTsx).not.toContain('newChatAgentPickerOpen');
    expect(mainTsx).not.toContain('resumeAgentPickerOpen');
    expect(mainTsx).not.toContain('legacy-chat-session-swipe-row');
    expect(mainTsx).not.toContain('chat-session-reload-action');
    expect(mainTsx).not.toContain('chat-session-delete-action');
    expect(mainTsx).not.toContain('chat-session-item');
    expect(mainTsx).not.toContain("const renderSidebarMain = (showSectionTitle = true) => {\n    if (tab === 'chat') {");
    expect(mainTsx).toContain('const resetChatComposer = () => {');
    expect(mainTsx).toContain("chatComposerTextRef.current = '';");
    expect(mainTsx).toContain('chatAttachmentsRef.current = [];');
    expect(mainTsx).toContain('bumpChatDraftGeneration(currentChatDraftKeyRef.current);');
    expect(mainTsx).not.toContain('const result = await service.createSession(normalizedAgentType, title);');
    expect(mainTsx).toContain("const result = await service.createProjectSession(targetProjectId, agentType, '');");
    expect(mainTsx).not.toContain('const completeNewChatFlow = async (agentType: string) => {');
    expect(mainTsx).toContain('for (const item of projectItem.agents ?? [])');
    expect(mainTsx).toContain('resetChatComposer();');
    expect(mainTsx).toContain('attachments: ChatAttachment[];');
    expect(mainTsx).toContain("const EMPTY_CHAT_COMPOSER_DRAFT: ChatComposerDraft = { text: '', attachments: [] };");
    expect(mainTsx).toContain('const [chatAttachments, setChatAttachments] = useState<ChatAttachment[]>([]);');
    expect(mainTsx).toContain('const [chatAttachmentReadPending, setChatAttachmentReadPending] = useState(false);');
    expect(mainTsx).toContain('const chatConfigOverflowOpen = workspaceUiState.mobile.chatConfigOverflowOpen;');
    expect(mainTsx).toContain("dispatchWorkspaceUi({ type: 'mobile/setChatConfigOverflowOpen', next });");
    expect(mainTsx).toContain('const chatAttachmentsRef = useRef<ChatAttachment[]>([]);');
    expect(mainTsx).toContain('const chatAutoScrollFollowRef = useRef(true);');
    expect(mainTsx).toContain('const chatPointerScrollingRef = useRef(false);');
    expect(mainTsx).toContain('const chatUserScrollLockUntilRef = useRef(0);');
    expect(mainTsx).toContain('const chatVirtuosoListRef = useRef<ChatVirtuosoTurnListHandle | null>(null);');
    expect(mainTsx).not.toContain('const chatDisplayItemCountRef = useRef(0);');
    expect(mainTsx).not.toContain('const chatProgrammaticScrollRef = useRef(false);');
    expect(mainTsx).toContain('const CHAT_AUTO_SCROLL_BOTTOM_THRESHOLD = 80;');
    expect(mainTsx).not.toContain('function isChatScrolledNearBottom(container: HTMLElement): boolean {');
    expect(mainTsx).not.toContain('const updateChatFollowModeFromScroll = useCallback(');
    expect(mainTsx).toContain('const handleChatAtBottomChange = useCallback((atBottom: boolean) => {');
    expect(mainTsx).toContain('chatAutoScrollFollowRef.current = atBottom;');
    expect(mainTsx).toContain('setChatShowScrollToBottom(!atBottom);');
    expect(mainTsx).toContain('const handleChatScroll = useCallback((event: React.UIEvent<HTMLDivElement>) => {');
    expect(mainTsx).toContain('resolveChatScrollToBottomVisibility({');
    expect(mainTsx).toContain('const scrollChatToBottom = useCallback((force = false) => {');
    expect(mainTsx).toContain('shouldAutoScrollChatToBottom({');
    expect(mainTsx).toContain("chatVirtuosoListRef.current?.scrollToBottom('auto');");
    expect(mainTsx).not.toContain('const autoscrollChatToBottom = useCallback(() => {');
    expect(mainTsx).not.toContain('chatVirtuosoListRef.current?.autoscrollToBottom();');
    expect(mainTsx).not.toContain('container.scrollTop = nextScrollTop;');
    expect(mainTsx).toContain('const forceChatScrollToBottom = useCallback(() => {');
    expect(mainTsx).toContain('chatAutoScrollFollowRef.current = true;');
    expect(mainTsx).toContain('scrollChatToBottom(true);');
    expect(mainTsx).toContain('useLayoutEffect(() => {');
    expect(mainTsx).toContain('resizeChatComposerTextarea();');
    expect(mainTsx).toContain('}, [resizeChatComposerTextarea, chatComposerText, tab, selectedChatId, currentChatDraftKey]);');
    expect(mainTsx).not.toContain('const chatBottomFollowAction = resolveChatBottomFollowAction({');
    expect(mainTsx).not.toContain("if (chatBottomFollowAction === 'scrollToBottom') {");
    expect(mainTsx).toContain('}, [tab, selectedChatId, chatMessages, chatPendingPromptsByKey, chatLoading, resizeChatComposerTextarea]);');
    expect(mainTsx).not.toContain('chatLoading, chatKeyboardInset, resizeChatComposerTextarea');
    expect(mainTsx).toContain('onScroll={handleChatScroll}');
    expect(mainTsx).toContain('onWheel={event => { if (event.deltaY < 0) { markChatUserScrollIntent(); } }}');
    expect(mainTsx).toContain('<ChatVirtuosoTurnList');
    expect(mainTsx).toContain('ref={chatVirtuosoListRef}');
    expect(mainTsx).toContain('atBottomThreshold={CHAT_AUTO_SCROLL_BOTTOM_THRESHOLD}');
    expect(mainTsx).toContain('onAtBottomChange={handleChatAtBottomChange}');
    expect(mainTsx).toContain('shouldAutoscroll={shouldAutoscrollChat}');
    expect(mainTsx).toContain('onPointerDown={() => { chatPointerScrollingRef.current = true; }}');
    expect(mainTsx).toContain('onPointerUp={() => { chatPointerScrollingRef.current = false; }}');
    expect(mainTsx).toContain('onTouchStart={() => { chatPointerScrollingRef.current = true; }}');
    expect(mainTsx).toContain('onTouchEnd={() => { chatPointerScrollingRef.current = false; }}');
    expect(mainTsx).toContain('const chatDraftGenerationRef = useRef<Record<string, number>>({});');
    expect(mainTsx).toContain('const applyChatAttachments = useCallback(');
    expect(mainTsx).toContain('const next = updater(chatAttachmentsRef.current);');
    expect(mainTsx).toContain('chatAttachmentsRef.current = next;');
    expect(mainTsx).toContain('const appendChatAttachments = useCallback(');
    expect(mainTsx).toContain('draftKey = currentChatDraftKeyRef.current');
    expect(mainTsx).toContain('expectedGeneration = getChatDraftGeneration(draftKey)');
    expect(mainTsx).toContain('if (expectedGeneration !== getChatDraftGeneration(normalizedDraftKey)) {');
    expect(mainTsx).toContain('const removeChatAttachment = useCallback(');
    expect(mainTsx).toContain('const readChatAttachmentFile = useCallback(');
    expect(mainTsx).toContain('const supportsChatClipboardImages = useMemo(');
    expect(mainTsx).toContain('const userAgent = window.navigator.userAgent || \'\';');
    expect(mainTsx).toContain('const platform = window.navigator.platform || \'\';');
    expect(mainTsx).toContain('if (/iPad|iPhone|iPod/i.test(userAgent)) {');
    expect(mainTsx).toContain('/Macintosh/i.test(userAgent) &&');
    expect(mainTsx).toContain('(window.navigator.maxTouchPoints ?? 0) > 1');
    expect(mainTsx).toContain('return true;');
    expect(mainTsx).toContain('return false;');
    expect(mainTsx).toContain('item.type.toLowerCase().startsWith(\'image/\')');
    expect(mainTsx).toContain('Promise.all(');
    expect(mainTsx).toContain('readChatAttachmentFile(file, `pasted-image-${index + 1}.png`)');
    expect(mainTsx).toContain('const attachmentDraftKey = currentChatDraftKeyRef.current;');
    expect(mainTsx).toContain('const attachmentDraftGeneration = getChatDraftGeneration(attachmentDraftKey);');
    expect(mainTsx).toContain('appendChatAttachments(');
    expect(mainTsx).toContain('attachments,');
    expect(mainTsx).toContain('const sourceAttachments = options.attachmentsOverride ?? chatAttachments;');
    expect(mainTsx).toContain('blocksOverride?: RegistryChatContentBlock[];');
    expect(mainTsx).toContain('const blocks: RegistryChatContentBlock[] = options.blocksOverride');
    expect(mainTsx).toContain('...sourceAttachments.map(attachment => ({');
    expect(mainTsx).toContain("if (!options.blocksOverride && sourceAttachments.length > 0 && chatAttachmentReadPending) {");
    expect(mainTsx).toContain("setError('Wait for images to finish loading.');");
    expect(mainTsx).toContain('type="file"');
    expect(mainTsx).toContain('multiple');
    expect(mainTsx).toContain('onPaste={event => {');
    expect(mainTsx).toContain('if (!supportsChatClipboardImages) {');
    expect(mainTsx).toContain('appendChatAttachments(');
    expect(mainTsx).toContain('attachmentDraftKey,');
    expect(mainTsx).toContain('attachmentDraftGeneration,');
    expect(mainTsx).toContain('if (chatSending || chatAttachmentReadPending) {');
    expect(mainTsx).not.toContain('respondToChatPermission');
    expect(mainTsx).not.toContain("const [chatSessions] = useState(['General', 'WheelMaker App', 'Go Service']);");
    expect(stylesCss).toContain('.chat-composer');
    expect(stylesCss).not.toContain('.chat-composer::before {');
    expect(stylesCss).not.toContain('--chat-history-bottom-buffer');
    expect(stylesCss).toMatch(
      /\.chat-main \{[\s\S]*display: flex;[\s\S]*flex-direction: column;[\s\S]*gap: 5px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-virtuoso-footer {');
    expect(stylesCss).toMatch(
      /\.chat-composer \{[\s\S]*position: relative;[\s\S]*z-index: 1;[\s\S]*padding: 0 14px 12px;[\s\S]*background: transparent;/,
    );
    expect(stylesCss).not.toContain('--chat-composer-frame-top');
    expect(stylesCss).not.toContain('--chat-composer-fade-distance');
    expect(stylesCss).not.toContain('margin-top: calc(-1 * var(--chat-composer-frame-top));');
    expect(stylesCss).not.toContain('transform: translateY(calc(-100% + 4px));');
    expect(stylesCss).not.toContain('.chat-session-item');
    expect(stylesCss).not.toContain('.chat-session-swipe-row');
    expect(stylesCss).not.toContain('.chat-session-reload-action');
    expect(stylesCss).not.toContain('.chat-session-delete-action');
    expect(stylesCss).not.toContain('.chat-sessions-header');
    expect(stylesCss).toContain('.chat-attachment-preview-list {');
    expect(stylesCss).toContain('.chat-config-overflow-anchor {');
    expect(stylesCss).toContain('.chat-config-overflow-button {');
    expect(stylesCss).toContain('.chat-config-overflow-menu {');
    expect(mainTsx).not.toContain('className="status-bar"');
    expect(mainTsx).not.toContain('gitStatusSummary');
    expect(mainTsx).not.toContain('chat-thought-label');
    expect(mainTsx).toContain("import { buildPromptDoneCopyRange } from './chat/chatCopyRange';");
    expect(mainTsx).toContain('const copyRange = message.method === \'prompt_done\'');
    expect(mainTsx).toContain('className="chat-prompt-actions"');
    expect(mainTsx).toContain('className="chat-prompt-action-button"');
    expect(mainTsx).toContain('aria-label="Copy response markdown"');
    expect(mainTsx).toContain('codicon codicon-copy');
    expect(stylesCss).toContain('.chat-prompt-actions {');
    expect(stylesCss).toContain('.chat-prompt-action-button {');
    const sendExistingStart = mainTsx.indexOf('const sendChatMessage = async');
    const sendEnd = mainTsx.indexOf('const sendDirectChatText = async (text: string) => {', sendExistingStart);
    const sendBlock = mainTsx.slice(sendExistingStart, sendEnd);
    const sendAwait = mainTsx.indexOf('const result = await service.sendProjectSessionMessage(selectedProjectId, {', sendExistingStart);
    expect(sendExistingStart).toBeGreaterThanOrEqual(0);
    expect(sendEnd).toBeGreaterThan(sendExistingStart);
    expect(sendBlock).toContain("if (trimmedText === '/cancel' && sourceAttachments.length === 0 && !options.blocksOverride) {");
    expect(sendBlock).toContain("setError('Use the stop button to cancel in app.');");
    expect(sendBlock).toContain('rememberPendingChatPrompt(runtimeKey, {');
    expect(sendBlock).toContain("status: 'confirming',");
    expect(sendBlock).toContain('const result = await service.sendProjectSessionMessage(selectedProjectId, {');
    expect(sendBlock).toContain('if (!result.ok) {');
    expect(sendBlock).toContain('markPendingChatPromptUndelivered(runtimeKey');
    expect(sendBlock).toContain('if (shouldApplySentChatSelection(selectedChatKeyRef.current, selectedKey)) {');
    const sendSelectionGuard = sendBlock.indexOf('if (shouldApplySentChatSelection(selectedChatKeyRef.current, selectedKey)) {');
    const sendSelectionApply = sendBlock.indexOf('applySelectedChatKey(nextSelectedKey);', sendSelectionGuard);
    expect(sendSelectionGuard).toBeGreaterThan(sendBlock.indexOf('const nextSelectedKey = chatSessionKeyFromParts(selectedProjectId, nextSessionId);'));
    expect(sendSelectionApply).toBeGreaterThan(sendSelectionGuard);
    expect(sendBlock).not.toContain('markChatSessionRunning(');
    expect(sendAwait).toBeGreaterThan(sendExistingStart);
    expect(mainTsx).toContain('const [hasPendingProjectUpdates, setHasPendingProjectUpdates] = useState(false);');
    expect(mainTsx).toContain('if (!eventProjectId || eventProjectId === projectIdRef.current) {');
    expect(mainTsx).toContain('setHasPendingProjectUpdates(true);');
    expect(mainTsx).toContain('if (!silent) {');
    expect(mainTsx).toContain('setHasPendingProjectUpdates(false);');
    expect(mainTsx).not.toContain('setChatPromptSnapshotVersion(version => version + 1);');
    expect(mainTsx).toContain('const nextSessions = mergeChatSessionList(knownSessions, listedSessions);');
    expect(mainTsx).toContain('setChatSessions(prev => mergeChatSessionList(prev, listedSessions));');
    expect(mainTsx).toContain('const mergedSessions = mergeChatSessionList(knownSessions, sortedSessions);');
    expect(mainTsx).toContain('return mergeChatSession([projectSession], currentProjectSession)[0];');
    expect(mainTsx).toContain('const CHAT_CONFIG_PRIORITY_IDS = [');
    expect(mainTsx).toContain("const CHAT_CONFIG_PRIORITY_MATCHERS = ['mode', 'model', 'effort', 'thought']");
    expect(mainTsx).toContain("const FLOATING_CONTROL_SLOT_ORDER = ['upper', 'upper-middle', 'center', 'lower-middle'] as const;");
    expect(mainTsx).not.toContain('className="header-bubble"');
    expect(mainTsx).toContain('className="drawer-project-header"');
    expect(mainTsx).toContain('className="drawer-project-pill"');
    expect(mainTsx).toContain('className="drawer-settings-icon-btn"');
    expect(mainTsx).toMatch(
      /className="drawer-project-header"[\s\S]*?className="drawer-settings-icon-btn"[\s\S]*?className="drawer-project-pill"[\s\S]*?className="project-wrap"/,
    );
    expect(mainTsx).toContain('setSidebarSettingsOpen(true);');
    expect(mainTsx).toContain("tab === 'chat' && !isWide ? renderMobileChatSessionSheet() : renderSidebarMain()");
    expect(mainTsx).toContain("tab === 'chat' ? renderWideProjectSessionNav() : renderSidebarMain(false)");
    expect(mainTsx).toContain('className="sidebar-title-row"');
    expect(mainTsx).toContain('className="desktop-activity-bar"');
    expect(mainTsx).toContain('const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (');
    expect(mainTsx).toContain('className="mobile-settings-screen"');
    expect(mainTsx).toContain('aria-modal="true"');
    expect(mainTsx).toContain('className="mobile-settings-nav"');
    expect(mainTsx).toContain('className="mobile-settings-back"');
    expect(mainTsx).toContain('<div className="mobile-settings-title">{mobileSettingsTitle}</div>');
    expect(mainTsx).toContain('className="mobile-settings-group"');
    const chatSettingsStart = mainTsx.indexOf("renderSettingsSection('Chat'");
    const latestTitleSettingStart = mainTsx.indexOf('Use Latest Prompt Title', chatSettingsStart);
    const hideToolCallsSettingStart = mainTsx.indexOf('Hide Tool Calls', chatSettingsStart);
    expect(chatSettingsStart).toBeGreaterThanOrEqual(0);
    expect(latestTitleSettingStart).toBeGreaterThan(chatSettingsStart);
    expect(hideToolCallsSettingStart).toBeGreaterThan(latestTitleSettingStart);
    expect(mainTsx).not.toContain('className="sidebar-footer"');
    expect(mainTsx).toContain('className="floating-control-stack"');
    expect(mainTsx).toContain('className="floating-nav-group"');
    expect(mainTsx).toContain('className="drawer-toggle-bubble"');
    expect(mainTsx).toContain('const handleFloatingControlButtonPointerDown = useCallback(');
    expect(mainTsx).toMatch(
      /const handleFloatingControlButtonPointerDown = useCallback\([\s\S]*?beginFloatingPress\(event\);[\s\S]*?event\.stopPropagation\(\);/,
    );
    expect(mainTsx).toContain('event.stopPropagation();');
    expect(mainTsx).toMatch(
      /className="floating-nav-button"[\s\S]*?onPointerDown=\{handleFloatingControlButtonPointerDown\}[\s\S]*?onClick=\{\(\) => handleFloatingNavSelect\('chat'\)\}/,
    );
    expect(mainTsx).toMatch(
      /className="floating-nav-button"[\s\S]*?onPointerDown=\{handleFloatingControlButtonPointerDown\}[\s\S]*?onClick=\{\(\) => handleFloatingNavSelect\('file'\)\}/,
    );
    expect(mainTsx).toMatch(
      /className="floating-nav-button"[\s\S]*?onPointerDown=\{handleFloatingControlButtonPointerDown\}[\s\S]*?onClick=\{\(\) => handleFloatingNavSelect\('git'\)\}/,
    );
    expect(mainTsx).toMatch(
      /className="drawer-toggle-bubble"[\s\S]*?onPointerDown=\{handleFloatingControlButtonPointerDown\}[\s\S]*?onClick=\{handleFloatingDrawerToggle\}/,
    );
    expect(mainTsx).toContain('const floatingControlSlot = workspaceUiState.mobile.floatingControlSlot;');
    expect(mainTsx).toContain('const floatingDragState = workspaceUiState.transient.floatingDragState as FloatingDragState | null;');
    expect(mainTsx).toContain('const floatingKeyboardOffset = workspaceUiState.transient.floatingKeyboardOffset;');
    expect(mainTsx).not.toContain('style={narrowContentInsetStyle}');
    expect(mainTsx).toContain('className="breadcrumb-title"');
    expect(mainTsx).toContain('className="breadcrumb-project-name"');
    expect(mainTsx).toContain('No Selected Session');
    expect(mainTsx).toContain('No Selected Diff');
    expect(mainTsx).toContain('data-active={drawerOpen}');
    expect(mainTsx).toContain("CHAT - {selectedChatDisplayTitle || 'New Session'}");
    expect(mainTsx).toContain("{selectedFile || 'Select a file'}");
    expect(mainTsx).toContain("{selectedDiff || 'Select a changed file'}");
    expect(mainTsx).toContain('aria-expanded={drawerOpen}');
    expect(mainTsx).toContain('const chatConfigDisplay = useMemo(() => {');
    expect(mainTsx).toContain('className="chat-config-options-shell"');
    expect(mainTsx).toContain('<div ref={chatConfigOptionsRef} className="chat-config-options">');
    expect(mainTsx).toContain('{chatConfigOverflowOptions.length > 0 ? (');
    expect(mainTsx).toContain('className="chat-config-overflow-anchor"');
    expect(mainTsx).toContain('chat-config-overflow-button');
    expect(mainTsx).toContain('chat-config-overflow-menu');
    expect(mainTsx).toContain('className="codicon codicon-settings-gear"');
    expect(mainTsx).toContain('className="codicon codicon-chevron-down"');
    expect(mainTsx).not.toContain('project-menu-state');
    expect(mainTsx).not.toContain("projectItem.online ? 'online' : 'offline'");
    expect(mainTsx).not.toContain('+{chatConfigOverflowOptions.length}');
    expect(mainTsx).toContain('title="More config options"');
    expect(mainTsx).not.toContain('function chooseChatEntryText(previousText: string, nextText: string): string {');
    expect(mainTsx).not.toContain('text: chooseChatEntryText(previous.text, text),');
    expect(mainTsx).not.toContain('function groupChatMessagesByPrompt(');
    expect(mainTsx).not.toContain("const shouldRefreshCompletedPrompt = message.method === 'prompt_done';");
    expect(mainTsx).not.toContain('const shouldMarkSessionRunning = isChatSessionRunningMessage(message);');
    expect(mainTsx).toContain('const normalizedPayload = normalizeSessionMessagePayload(payload);');
    expect(mainTsx).toContain('const gapReadCursor = shouldReadRepairForIncomingTurn(turnState, incomingTurn);');
    expect(mainTsx).toContain('mergeRealtimeTurn(turnState, incomingTurn);');
    expect(mainTsx).toContain('const merged = messagesFromTurnStore(runtimeKey, sessionId);');
    expect(mainTsx).toContain('chatReadRepairQueueRef.current.request(runtimeKey, gapReadCursor.turnIndex');
    const normalizedPayload = mainTsx.indexOf('const normalizedPayload = normalizeSessionMessagePayload(payload);');
    const gapReadCursor = mainTsx.indexOf('const gapReadCursor = shouldReadRepairForIncomingTurn(turnState, incomingTurn);', normalizedPayload);
    const realtimeMerge = mainTsx.indexOf('mergeRealtimeTurn(turnState, incomingTurn);', gapReadCursor);
    const incomingStoreApply = mainTsx.indexOf('chatMessageStoreRef.current[runtimeKey] = merged;', realtimeMerge);
    const incomingVisibleApply = mainTsx.indexOf('setVisibleChatMessagesForRuntimeKey(runtimeKey, merged, {', incomingStoreApply);
    const promptGapRead = mainTsx.indexOf('chatReadRepairQueueRef.current.request(runtimeKey, gapReadCursor.turnIndex', incomingVisibleApply);
    expect(normalizedPayload).toBeGreaterThanOrEqual(0);
    expect(gapReadCursor).toBeGreaterThan(normalizedPayload);
    expect(realtimeMerge).toBeGreaterThan(gapReadCursor);
    expect(incomingStoreApply).toBeGreaterThan(realtimeMerge);
    expect(incomingVisibleApply).toBeGreaterThan(incomingStoreApply);
    expect(promptGapRead).toBeGreaterThan(incomingVisibleApply);
    expect(mainTsx).not.toContain('lastReadTurnIndex: isSelectedSession && completedTurnIndex > 0');
    expect(mainTsx).toContain('workspaceStore.rememberChatSessionTurns(activeProjectId, sessionId, turnState.finished);');
    expect(mainTsx).toContain('needsPromptTurnRefresh(');
    expect(mainTsx).toContain('refreshSessionTurns(');
    expect(mainTsx).not.toContain('if (shouldRefreshCompletedPrompt && isSelectedSession) {\n          loadChatSession(sessionId, projectIdRef.current, {\n            forceFull: true,');
    const eventTurnState = mainTsx.indexOf('const turnState = ensureChatTurnStore(runtimeKey);');
    const eventGapRead = mainTsx.indexOf('const gapReadCursor = shouldReadRepairForIncomingTurn(turnState, incomingTurn);', eventTurnState);
    const eventMerge = mainTsx.indexOf('mergeRealtimeTurn(turnState, incomingTurn);', eventGapRead);
    const eventRepair = mainTsx.indexOf('chatReadRepairQueueRef.current.request(runtimeKey, gapReadCursor.turnIndex', eventMerge);
    expect(eventTurnState).toBeGreaterThanOrEqual(0);
    expect(eventGapRead).toBeGreaterThan(eventTurnState);
    expect(eventMerge).toBeGreaterThan(eventGapRead);
    expect(eventRepair).toBeGreaterThan(eventMerge);
    expect(mainTsx).toContain('const runtimeKey = buildChatRuntimeKey(eventProjectId, payload.session.sessionId);');
    const sessionUpdatedBlockStart = mainTsx.indexOf("if (event.method === 'session.updated') {");
    const sessionMessageBlockStart = mainTsx.indexOf("if (event.method === 'session.message') {");
    const sessionUpdatedBlock = mainTsx.slice(sessionUpdatedBlockStart, sessionMessageBlockStart);
    expect(sessionUpdatedBlock).not.toContain('loadChatSession(');
    expect(sessionUpdatedBlock).toContain('if (payload.session.running === false) {');
    expect(sessionUpdatedBlock).toContain('const mergedSession = mergeKnownChatSessionForProject(eventProjectId, payload.session);');
    expect(sessionUpdatedBlock).toContain('rememberChatSessionSummary(eventProjectId, mergedSession);');
    const sessionMessageBlockEnd = mainTsx.indexOf('const unsubscribeClose = service.onClose', sessionMessageBlockStart);
    const sessionMessageBlock = mainTsx.slice(sessionMessageBlockStart, sessionMessageBlockEnd);
    expect(sessionMessageBlock).not.toContain('applySelectedChatKey(');
    expect(sessionMessageBlock).not.toContain('workspaceStore.rememberSelectedChatSessionKey(');
    expect(sessionMessageBlock).toContain("if (message.method === 'prompt_done' && isSelectedSession) {");
    expect(mainTsx).toContain("className={`desktop-activity-button refresh-btn${hasPendingProjectUpdates && !refreshingProject && !reconnecting ? ' has-update-badge' : ''}`}");
    expect(mainTsx).not.toContain('project-presence');
    expect(mainTsx).not.toContain('project-dirty');
    expect(stylesCss).not.toContain('.status-bar {');
    expect(stylesCss).not.toContain('.chat-thought-label {');
    expect(stylesCss).toContain('.refresh-btn.has-update-badge::after {');
    expect(stylesCss).not.toContain('.header-bubble {');
    expect(stylesCss).toContain('.drawer-project-header {');
    expect(stylesCss).toContain('.drawer-project-pill {');
    expect(stylesCss).toContain('.drawer-settings-icon-btn {');
    expect(stylesCss).toContain('.mobile-settings-screen {');
    expect(stylesCss).toContain('.mobile-settings-nav {');
    expect(stylesCss).toContain('.mobile-settings-back {');
    expect(stylesCss).toContain('.mobile-settings-group {');
    expect(stylesCss).toContain('.mobile-settings-screen .settings-row {');
    expect(stylesCss).toContain('.mobile-settings-screen .settings-danger-row {');
    expect(stylesCss).not.toContain('.project-menu-state');
    expect(stylesCss).toMatch(
      /\.project-menu-hub \{[\s\S]*background: color-mix\(in srgb, var\(--accent\) 18%, var\(--panel-2\)\);/,
    );
    expect(stylesCss).toMatch(
      /\.project-menu-hub \{[\s\S]*border: 1px solid color-mix\(in srgb, var\(--accent\) 42%, transparent\);/,
    );
    expect(stylesCss).toMatch(
      /\.header \.project-btn \{[\s\S]*max-width: none;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.header \.project-name \{[\s\S]*overflow: visible;[\s\S]*text-overflow: clip;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('padding: calc(var(--wm-safe-area-top) + 8px) 8px 10px;');
    expect(stylesCss).toContain('.floating-control-stack {');
    expect(stylesCss).toContain('.floating-nav-group {');
    expect(stylesCss).toMatch(
      /\.floating-nav-group \{[\s\S]*width: 50px;[\s\S]*grid-template-rows: repeat\(3, 40px\);[\s\S]*padding: 4px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.floating-nav-indicator {');
    expect(stylesCss).toMatch(
      /\.floating-nav-indicator \{[\s\S]*background: color-mix\(in srgb, var\(--accent\) 28%, transparent\);[\s\S]*border: 1px solid color-mix\(in srgb, var\(--accent\) 32%, transparent\);/,
    );
    expect(stylesCss).toContain(".floating-nav-button[data-active='true']:hover {");
    expect(stylesCss).toContain('.drawer-toggle-bubble {');
    expect(stylesCss).toMatch(
      /\.drawer-toggle-bubble\[data-active='true'\] \{[\s\S]*background: color-mix\(in srgb, var\(--accent\) 28%, transparent\);[\s\S]*border-color: color-mix\(in srgb, var\(--accent\) 32%, transparent\);/,
    );
    expect(stylesCss).toContain('-webkit-tap-highlight-color: transparent;');
    expect(stylesCss).toContain('.breadcrumb-title {');
    expect(stylesCss).toContain('.breadcrumb-project-name {');
    expect(stylesCss).not.toContain('max-width: min(42%, 160px);');
    expect(stylesCss).toMatch(
      /\.breadcrumb-project-name \{[\s\S]*flex: 0 0 auto;[\s\S]*max-width: none;[\s\S]*border: 1px solid color-mix\(in srgb, var\(--accent\) 54%, transparent\);[\s\S]*border-radius: 8px;[\s\S]*background: color-mix\(in srgb, var\(--accent\) 13%, var\(--panel\)\);[\s\S]*color: color-mix\(in srgb, var\(--accent\) 78%, var\(--text\)\);[\s\S]*\}/,
    );
    const breadcrumbProjectBlock = stylesCss.match(/\.breadcrumb-project-name \{[\s\S]*?\n    \}/)?.[0] ?? '';
    expect(breadcrumbProjectBlock).not.toContain('box-shadow: inset 3px 0 0 var(--accent);');
    expect(stylesCss).toMatch(
      /\.breadcrumb-current \{[\s\S]*min-width: 0;[\s\S]*overflow: hidden;[\s\S]*text-overflow: ellipsis;[\s\S]*\}/,
    );
    expect(mainTsx).toContain('chatAttachments.map(attachment => (');
    expect(mainTsx).toContain('onClick={() => removeChatAttachment(attachment.id)}');
    expect(mainTsx).toContain('disabled={chatSending || chatAttachmentReadPending}');
    expect(stylesCss).not.toContain('.project-presence {');
    expect(stylesCss).not.toContain('.project-dirty {');
    expect(stylesCss).not.toContain('.chat-permission-button');
  });

  test('chat breadcrumb title uses the selected session project', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    const chatProjectNameStart = mainTsx.indexOf('const chatBreadcrumbProjectName = useMemo(');
    const chatLabelStart = mainTsx.indexOf('const chatBreadcrumbLabel = useMemo(', chatProjectNameStart);
    expect(chatProjectNameStart).toBeGreaterThanOrEqual(0);
    expect(chatLabelStart).toBeGreaterThan(chatProjectNameStart);

    const chatProjectNameBlock = mainTsx.slice(chatProjectNameStart, chatLabelStart);
    expect(chatProjectNameBlock).toContain('selectedChatKey?.projectId');
    expect(chatProjectNameBlock).toContain('projects.find(item => item.projectId === selectedProjectId)?.name');
    expect(chatProjectNameBlock).toContain('breadcrumbProjectName');
    expect(mainTsx).toContain("import { resolveChatSessionTitle } from './chat/chatSessionTitle';");
    expect(mainTsx).toContain('const [useLatestPromptTitle, setUseLatestPromptTitle] = useState(');
    expect(mainTsx).toContain('const selectedChatDisplayTitle = useMemo(');
    expect(mainTsx).toContain("resolveChatSessionTitle(selectedChatSession?.title ?? '', useLatestPromptTitle)");
    expect(mainTsx).toContain('resolveSessionDisplayTitle(session)');
    expect(mainTsx).not.toContain('session.title || session.sessionId');
    expect(mainTsx).not.toContain('selectedChatSession?.title ||');
    expect(mainTsx).toContain("() => selectedChatDisplayTitle || 'No Selected Session'");
    expect(mainTsx).toContain('checked={useLatestPromptTitle}');
    expect(mainTsx).toContain('onChange={e => setUseLatestPromptTitle(e.target.checked)}');
    expect(mainTsx).toContain('Use Latest Prompt Title');
    expect(mainTsx).not.toContain('className="chat-title-option"');
    expect(mainTsx).toContain('renderBreadcrumbTitle(chatBreadcrumbProjectName, chatBreadcrumbLabel)');
    expect(mainTsx).toContain('renderBreadcrumbTitle(breadcrumbProjectName, fileBreadcrumbLabel)');
    expect(mainTsx).toContain('renderBreadcrumbTitle(breadcrumbProjectName, gitBreadcrumbLabel)');
  });

  test('chat drawer title shows hub count summary with dropdown details', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('const [chatHubMenuOpen, setChatHubMenuOpen] = useState(false);');
    expect(mainTsx).toContain('const chatHubMenuRef = useRef<HTMLDivElement | null>(null);');
    expect(mainTsx).toContain('const renderChatHubSummary = useCallback(() => {');
    expect(mainTsx).toContain('const hubCount = registryHubs.length;');
    expect(mainTsx).toContain('if (!chatHubMenuOpen) return;');
    expect(mainTsx).toContain("if (event.key === 'Escape') {");
    expect(mainTsx).toContain("if (tab !== 'chat' || sidebarSettingsOpen) {");
    expect(mainTsx).toContain('chatHubMenuRef.current?.contains(target)');
    expect(mainTsx).toContain('aria-label="Show connected hubs"');
    expect(mainTsx).toContain('aria-expanded={chatHubMenuOpen}');
    expect(mainTsx).toContain('<span className="chat-hub-summary-label">Hubs</span>');
    expect(mainTsx).toContain('<span className="chat-hub-summary-count">{hubCount}</span>');
    expect(mainTsx).toContain('{registryHubs.length > 0 ? (');
    expect(mainTsx).toContain('registryHubs.map(hub => {');
    expect(mainTsx).toContain('<span className="chat-hub-row-name">{hub.hubId}</span>');
    expect(mainTsx).toContain('<div className="chat-hub-empty">No hubs</div>');
    expect(mainTsx).toContain('<div className="mobile-chat-title-row">');
    expect(mainTsx).toContain('<span className="mobile-chat-drawer-title">Chats</span>');
    expect(mainTsx).toContain('renderChatHubSummary()');
    expect(mainTsx).toMatch(
      /<div className="mobile-chat-title-row">[\s\S]*?<span className="mobile-chat-drawer-title">Chats<\/span>[\s\S]*?renderChatHubSummary\(\)/,
    );
    expect(mainTsx).toMatch(
      /<div className="sidebar-title-row">[\s\S]*?<span className="sidebar-title-text">\{wideSidebarTitle\}<\/span>[\s\S]*?\{tab === 'chat' && !sidebarSettingsOpen \? renderChatHubSummary\(\) : null\}/,
    );
    const renderMainStart = mainTsx.indexOf('const renderMain = () => {');
    const chatMainStart = mainTsx.indexOf("if (tab === 'chat') {", renderMainStart);
    const chatMainEnd = mainTsx.indexOf('if (tab === ', chatMainStart + 1);
    expect(renderMainStart).toBeGreaterThanOrEqual(0);
    expect(chatMainStart).toBeGreaterThan(renderMainStart);
    expect(chatMainEnd).toBeGreaterThan(chatMainStart);
    const chatMainBlock = mainTsx.slice(chatMainStart, chatMainEnd);
    expect(chatMainBlock).not.toContain('renderChatHubSummary()');
    expect(stylesCss).toContain('.mobile-chat-title-row {');
    expect(stylesCss).toContain('.sidebar-title-row .chat-hub-summary {');
    expect(stylesCss).toContain('.chat-hub-summary {');
    expect(stylesCss).toContain('.chat-hub-summary-button {');
    expect(stylesCss).toContain('.chat-hub-summary-count {');
    expect(stylesCss).toContain('.chat-hub-popover {');
    expect(stylesCss).toMatch(
      /\.chat-hub-summary-button \{[\s\S]*letter-spacing: 0;[\s\S]*text-transform: none;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.chat-hub-popover \{[\s\S]*letter-spacing: 0;[\s\S]*text-transform: none;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-hub-row-name {');
    expect(stylesCss).toContain('.chat-hub-empty {');
  });

  test('chat composer is a unified command frame with compact custom config pills', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('const CHAT_CONFIG_INLINE_LIMIT = 3;');
    expect(mainTsx).not.toContain('CHAT_QUICK_REPLY_OPTIONS');
    expect(mainTsx).toContain('const [chatPromptMenuOpen, setChatPromptMenuOpen] = useState(false);');
    expect(mainTsx).not.toContain('chatQuickReplyMenuOpen');
    expect(mainTsx).toContain('const [chatFileMentionMenuOpen, setChatFileMentionMenuOpen] = useState(false);');
    expect(mainTsx).toContain("const [chatConfigMenuOptionId, setChatConfigMenuOptionId] = useState('');");
    expect(mainTsx).toContain('selectedChatConfigOptions.length <= CHAT_CONFIG_INLINE_LIMIT');
    expect(mainTsx).toContain('prioritized.slice(0, CHAT_CONFIG_INLINE_LIMIT)');
    expect(mainTsx).toContain('className="chat-composer-frame"');
    expect(mainTsx).toContain('className="chat-composer-input-row"');
    expect(mainTsx).toContain('className={`chat-composer-stop-trigger${selectedChatPromptRunning ? \' active\' : \'\'}`}');
    expect(mainTsx).toContain('title="Skills"');
    expect(mainTsx).toContain('aria-label="Open skills"');
    expect(mainTsx).not.toContain('className="chat-composer-skill-trigger"');
    expect(mainTsx).not.toContain('codicon-terminal');
    expect(mainTsx).not.toContain('className="chat-composer-quick-trigger"');
    expect(mainTsx).not.toContain('title="Quick replies"');
    expect(mainTsx).not.toContain('aria-label="Quick replies"');
    expect(mainTsx).not.toContain('className="chat-quick-reply-menu"');
    expect(mainTsx).not.toContain('className="chat-quick-reply-item"');
    expect(mainTsx).not.toContain('openChatQuickReplyMenu');
    expect(mainTsx).not.toContain('handleChatQuickReplySelect');
    expect(mainTsx).not.toContain('chatComposerText.length === 0 ? (');
    expect(mainTsx).not.toContain('if (chatComposerText.length > 0) {');
    expect(mainTsx).toContain('className="chat-composer-toolbar"');
    expect(mainTsx).toContain('className="chat-send-control"');
    expect(mainTsx).not.toContain('className={`chat-cancel-button${selectedChatPromptRunning ? \' active\' : \'\'}`}');
    expect(mainTsx).toContain('title={selectedChatPromptRunning ? \'Cancel prompt\' : \'No prompt running\'}');
    expect(mainTsx).toContain('aria-label="Cancel prompt"');
    expect(mainTsx).toContain('codicon-debug-stop');
    expect(mainTsx).toContain('className="chat-composer-tools"');
    expect(mainTsx).toContain('className="chat-tool-button chat-slash-button"');
    expect(mainTsx).toContain('className="chat-slash-symbol"');
    expect(mainTsx).toContain('className="chat-tool-button chat-mention-button"');
    expect(mainTsx).toContain('title="Mention files"');
    expect(mainTsx).toContain('aria-label="Mention files"');
    expect(mainTsx).toContain('className="chat-mention-symbol"');
    expect(mainTsx).toContain('className="chat-file-mention-menu"');
    expect(mainTsx).toContain('className="chat-file-mention-empty"');
    expect(mainTsx).toContain('File mentions coming soon');
    expect(mainTsx).toContain('const openChatFileMentionMenu = useCallback(() => {');
    expect(mainTsx).not.toContain('className="chat-tool-button chat-skill-button"');
    expect(mainTsx).not.toContain('codicon-wand');
    expect(mainTsx).not.toContain('codicon-symbol-keyword');
    expect(mainTsx).toContain('className="chat-tool-button chat-photo-button"');
    expect(mainTsx).toContain('codicon-device-camera');
    expect(mainTsx).not.toContain('codicon-file-media');
    expect(mainTsx).toContain('chatFileInputRef.current?.click();');
    expect(mainTsx).not.toContain('className={`chat-tool-button chat-stop-button${selectedChatPromptRunning ? \' active\' : \'\'}`}');
    expect(mainTsx).not.toContain('chat-voice-button');
    expect(mainTsx).not.toContain("setError('Voice input is not available yet.');");
    expect(mainTsx).toContain('extractChatOptionReplies(text)');
    expect(mainTsx).toContain('splitChatOptionReplyText(text)');
    expect(mainTsx).toContain('extractChatConfirmationReply(text)');
    expect(mainTsx).toContain('splitChatConfirmationReplyText(text)');
    expect(mainTsx).toContain('const optionReplyParts = splitChatOptionReplyText(text);');
    expect(mainTsx).toContain('const confirmationReplyParts = splitChatConfirmationReplyText(text);');
    expect(mainTsx).toContain("const hasOptionReplyParts = optionReplyParts.some(part => part.type === 'option');");
    expect(mainTsx).toContain('const selectableOptionReplies = optionReplies.length > 0;');
    expect(mainTsx).toContain('const selectableConfirmationReply = optionReplies.length === 0 ? confirmationReply : null;');
    expect(mainTsx).toContain('className="chat-option-reply-line"');
    expect(mainTsx).toContain('className="chat-option-reply-inline-button"');
    expect(mainTsx).toContain('className="chat-option-reply-static"');
    expect(mainTsx).toContain('className="chat-confirmation-reply-line"');
    expect(mainTsx).toContain('className="chat-confirmation-reply-action"');
    expect(mainTsx).toContain('className="chat-confirmation-reply-check"');
    expect(mainTsx).toContain('className="chat-confirmation-reply-text"');
    expect(mainTsx).toContain('onClick={() => onSelectConfirmationReply?.(part.reply.replyText)}');
    expect(mainTsx).not.toContain('className="chat-option-replies"');
    expect(mainTsx).toContain('onSelectOptionReply?: (label: string) => void;');
    expect(mainTsx).toContain('onSelectConfirmationReply?: (replyText: string) => void;');
    expect(mainTsx).toContain('if (selectedPendingPrompt) {');
    expect(mainTsx).toContain('className="chat-config-pill"');
    expect(mainTsx).toContain('className="chat-config-value-menu"');
    expect(mainTsx).toContain('chat-config-value-option${selected ?');
    expect(mainTsx).toContain('className="chat-config-value-label"');
    expect(mainTsx).toContain('className="chat-config-overflow-group"');
    expect(mainTsx).not.toContain('className="chat-action-menu chat-action-menu-inline');
    expect(mainTsx).not.toContain('Photo Library');
    expect(mainTsx).not.toContain('className="chat-config-select"');
    expect(mainTsx).not.toContain('showChatConfigLabels');
    expect(mainTsx).not.toContain('chatConfigFeedback');
    expect(mainTsx).not.toContain('Applying config');
    expect(mainTsx).toContain("import { insertChatSlashCommandText } from './chat/chatSlashInsertion';");
    expect(mainTsx).toContain('const inserted = insertChatSlashCommandText(');

    const stopTriggerClassStart = mainTsx.indexOf('className={`chat-composer-stop-trigger${selectedChatPromptRunning ? \' active\' : \'\'}`}');
    const stopTriggerStart = mainTsx.lastIndexOf('<button', stopTriggerClassStart);
    const stopTriggerEnd = mainTsx.indexOf('className="chat-composer-input-shell"', stopTriggerStart);
    expect(stopTriggerStart).toBeGreaterThanOrEqual(0);
    expect(stopTriggerEnd).toBeGreaterThan(stopTriggerStart);
    const stopTriggerBlock = mainTsx.slice(stopTriggerStart, stopTriggerEnd);
    expect(stopTriggerBlock).toContain('onPointerDown={event => event.preventDefault()}');
    expect(stopTriggerBlock).toContain('onClick={() => cancelSelectedChatPrompt().catch(() => undefined)}');
    expect(stopTriggerBlock).toContain('disabled={!selectedChatPromptRunning || selectedChatPromptCancelling}');
    expect(stopTriggerBlock).toContain("selectedChatPromptCancelling ? 'codicon-loading codicon-modifier-spin' : 'codicon-debug-stop'");

    const promptMenuOpenStart = mainTsx.indexOf('const openChatPromptMenu = useCallback(() => {');
    const promptMenuOpenEnd = mainTsx.indexOf('const openChatFileMentionMenu = useCallback(() => {', promptMenuOpenStart);
    expect(promptMenuOpenStart).toBeGreaterThanOrEqual(0);
    expect(promptMenuOpenEnd).toBeGreaterThan(promptMenuOpenStart);
    const promptMenuOpenBody = mainTsx.slice(promptMenuOpenStart, promptMenuOpenEnd);
    expect(promptMenuOpenBody).toContain('setChatFileMentionMenuOpen(false);');
    expect(promptMenuOpenBody).toContain('chatComposerTextareaRef.current?.focus();');
    expect(promptMenuOpenBody).not.toContain('chatComposerTextareaRef.current?.blur();');

    const fileMentionMenuOpenStart = mainTsx.indexOf('const openChatFileMentionMenu = useCallback(() => {');
    const fileMentionMenuOpenEnd = mainTsx.indexOf('const getChatDraftGeneration = useCallback', fileMentionMenuOpenStart);
    expect(fileMentionMenuOpenStart).toBeGreaterThanOrEqual(0);
    expect(fileMentionMenuOpenEnd).toBeGreaterThan(fileMentionMenuOpenStart);
    const fileMentionMenuOpenBody = mainTsx.slice(fileMentionMenuOpenStart, fileMentionMenuOpenEnd);
    expect(fileMentionMenuOpenBody).toContain('setChatPromptMenuOpen(false);');
    expect(fileMentionMenuOpenBody).toContain('setChatConfigMenuOptionId(\'\');');
    expect(fileMentionMenuOpenBody).toContain('setChatConfigOverflowOpen(false);');
    expect(fileMentionMenuOpenBody).toContain('setChatFileMentionMenuOpen(value => !value);');
    expect(fileMentionMenuOpenBody).not.toContain('updateChatComposerText');

    const toolsStart = mainTsx.indexOf('className="chat-composer-tools"');
    const toolsEnd = mainTsx.indexOf('className="chat-config-options-wrap"', toolsStart);
    expect(toolsStart).toBeGreaterThanOrEqual(0);
    expect(toolsEnd).toBeGreaterThan(toolsStart);
    const toolsBlock = mainTsx.slice(toolsStart, toolsEnd);
    expect(toolsBlock.indexOf('chat-slash-button')).toBeGreaterThanOrEqual(0);
    expect(toolsBlock.indexOf('chat-mention-button')).toBeGreaterThan(toolsBlock.indexOf('chat-slash-button'));
    expect(toolsBlock.indexOf('chat-photo-button')).toBeGreaterThan(toolsBlock.indexOf('chat-mention-button'));
    expect(toolsBlock).not.toContain('chat-stop-button');
    const slashButtonStart = toolsBlock.indexOf('chat-slash-button');
    const slashButtonBlock = toolsBlock.slice(toolsBlock.lastIndexOf('<button', slashButtonStart), toolsBlock.indexOf('</button>', slashButtonStart));
    expect(slashButtonBlock).toContain('ref={chatPromptButtonRef}');
    expect(slashButtonBlock).toContain('onClick={openChatPromptMenu}');

    const configPillStart = mainTsx.indexOf('const renderChatConfigPill = (option: RegistrySessionConfigOption) => {');
    const configPillEnd = mainTsx.indexOf('if (tab === \'chat\')', configPillStart);
    expect(configPillStart).toBeGreaterThanOrEqual(0);
    expect(configPillEnd).toBeGreaterThan(configPillStart);
    const configPillBlock = mainTsx.slice(configPillStart, configPillEnd);
    expect(configPillBlock).not.toContain('codicon-chevron-down');

    const configChangeStart = mainTsx.indexOf('const handleChatConfigOptionChange = async');
    const configChangeEnd = mainTsx.indexOf('const handleChatFileChange = async', configChangeStart);
    const configChangeBody = mainTsx.slice(configChangeStart, configChangeEnd);
    const setConfigCall = configChangeBody.indexOf('const result = await service.setProjectSessionConfig');
    expect(setConfigCall).toBeGreaterThanOrEqual(0);
    expect(configChangeBody).toContain('selectedKey.projectId');
    expect(configChangeBody.indexOf('applyChatSessionConfigOptions')).toBeGreaterThan(setConfigCall);
    expect(configChangeBody).not.toContain('setChatSessions(prev =>');

    const slashApplyStart = mainTsx.indexOf('const applyChatSlashCommand = useCallback(');
    const slashApplyEnd = mainTsx.indexOf('const openChatPromptMenu = useCallback', slashApplyStart);
    expect(slashApplyStart).toBeGreaterThanOrEqual(0);
    expect(slashApplyEnd).toBeGreaterThan(slashApplyStart);
    const slashApplyBody = mainTsx.slice(slashApplyStart, slashApplyEnd);
    expect(slashApplyBody).toContain('insertChatSlashCommandText(');
    expect(slashApplyBody).toContain('chatComposerText,');
    expect(slashApplyBody).toContain('input?.selectionStart ?? chatComposerText.length');
    expect(slashApplyBody).toContain('updateChatComposerText(inserted.text);');
    expect(slashApplyBody).toContain('input.setSelectionRange(inserted.selectionStart, inserted.selectionEnd);');
    expect(slashApplyBody).toContain('setChatFileMentionMenuOpen(false);');
    expect(slashApplyBody).not.toContain('updateChatComposerText(next);');

    expect(stylesCss).toMatch(
      /button,\s*\[role='button'\],\s*\[role='menuitemradio'\],\s*\[role='option'\]\s*\{[\s\S]*-webkit-tap-highlight-color: transparent;/,
    );
    expect(stylesCss).toMatch(
      /\.chat-composer \{[\s\S]*padding: 0 14px 12px;[\s\S]*background: transparent;/,
    );
    expect(stylesCss).not.toContain('.chat-composer::before {');
    expect(stylesCss).not.toContain('--chat-composer-frame-top');
    expect(stylesCss).not.toContain('--chat-composer-fade-distance');
    expect(stylesCss).toContain('.chat-composer-frame {');
    expect(stylesCss).toMatch(
      /\.chat-composer-frame \{[\s\S]*gap: 0;[\s\S]*padding: 5px 6px 3px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-composer-input-row {');
    expect(stylesCss).toMatch(
      /\.chat-composer-input-row \{[\s\S]*gap: 5px;[\s\S]*min-height: 32px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-composer-stop-trigger {');
    expect(stylesCss).toMatch(
      /\.chat-composer-stop-trigger \{[\s\S]*width: 22px;[\s\S]*height: 30px;[\s\S]*display: inline-flex;[\s\S]*align-items: center;[\s\S]*justify-content: center;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-composer-stop-trigger.active {');
    expect(stylesCss).not.toContain('.chat-composer-skill-trigger {');
    expect(stylesCss).not.toContain('.chat-composer-quick-trigger {');
    expect(stylesCss).not.toContain('.chat-quick-trigger-label {');
    expect(stylesCss).not.toContain('.chat-quick-reply-menu {');
    expect(stylesCss).not.toContain('.chat-quick-reply-item {');
    expect(stylesCss).toContain('.chat-file-mention-menu {');
    expect(stylesCss).toContain('.chat-file-mention-empty {');
    expect(stylesCss).toContain('.chat-option-reply-line {');
    expect(stylesCss).toContain('.chat-option-reply-inline-button {');
    expect(stylesCss).toContain('.chat-option-reply-static {');
    expect(stylesCss).toContain('.chat-confirmation-reply-line {');
    expect(stylesCss).toContain('.chat-confirmation-reply-action {');
    expect(stylesCss).toContain('.chat-confirmation-reply-check {');
    expect(stylesCss).toContain('.chat-confirmation-reply-text {');
    expect(stylesCss).toMatch(
      /\.chat-confirmation-reply-action \{[\s\S]*border: 1px solid color-mix\(in srgb, var\(--accent\) 22%, var\(--border\)\);[\s\S]*background: color-mix\(in srgb, var\(--surface-1\) 88%, var\(--accent\)\);[\s\S]*padding: 4px 8px;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.chat-option-reply-inline-button \{[\s\S]*border-color: color-mix\(in srgb, var\(--accent\) 22%, var\(--border\)\);[\s\S]*background: color-mix\(in srgb, var\(--surface-1\) 88%, var\(--accent\)\);/,
    );
    expect(stylesCss).toMatch(
      /\.chat-option-reply-inline-button,\s*\.chat-scroll-bottom-button \{[\s\S]*backdrop-filter: blur\(1px\);[\s\S]*\}/,
    );
    const historicalOptionBlocks = stylesCss.match(/\.chat-option-reply-static \{[\s\S]*?\n\}/g) ?? [];
    const historicalOptionBlock = historicalOptionBlocks[historicalOptionBlocks.length - 1] ?? '';
    expect(historicalOptionBlock).toContain('border-color: var(--border);');
    expect(historicalOptionBlock).toContain('background: transparent;');
    expect(historicalOptionBlock).not.toContain('background: color-mix');
    expect(stylesCss).toMatch(
      /\.chat-option-reply-static \.chat-option-reply-label \{[\s\S]*color: var\(--muted\);[\s\S]*\}/,
    );
    expect(stylesCss).not.toContain('.chat-option-replies {');
    expect(stylesCss).not.toContain('.chat-option-reply-button {');
    expect(stylesCss).toMatch(
      /\.chat-composer-input \{[\s\S]*min-height: 30px;[\s\S]*padding: 5px 0 1px;[\s\S]*font-size: 14px;[\s\S]*line-height: 1.4;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.chat-send-button \{[\s\S]*width: 32px;[\s\S]*height: 32px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-send-control {');
    expect(stylesCss).not.toContain('.chat-cancel-button {');
    expect(stylesCss).toMatch(
      /\.chat-send-button \.codicon \{[\s\S]*font-size: 15px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-scroll-bottom-button {');
    expect(stylesCss).not.toContain('.chat-title-tools {');
    expect(stylesCss).not.toContain('.chat-title-option {');
    expect(stylesCss).toContain('.chat-composer-toolbar {');
    expect(stylesCss).toMatch(
      /\.chat-composer-toolbar \{[\s\S]*min-height: 24px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-composer-tools {');
    expect(stylesCss).toContain('.chat-tool-button {');
    expect(stylesCss).toMatch(
      /\.chat-tool-button \{[\s\S]*width: 24px;[\s\S]*height: 24px;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.chat-tool-button \{[\s\S]*border: none;[\s\S]*background: transparent;/,
    );
    expect(stylesCss).toMatch(
      /\.chat-slash-button \{[\s\S]*color: color-mix\(in srgb, var\(--accent\) 72%, var\(--text\)\);[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-slash-symbol {');
    expect(stylesCss).toMatch(
      /\.chat-mention-button \{[\s\S]*color: color-mix\(in srgb, var\(--accent\) 72%, var\(--text\)\);[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-mention-symbol {');
    expect(stylesCss).not.toContain('.chat-skill-button {');
    expect(stylesCss).toMatch(
      /\.chat-photo-button \{[\s\S]*color: color-mix\(in srgb, #4db6ac 72%, var\(--text\)\);[\s\S]*\}/,
    );
    expect(stylesCss).not.toContain('.chat-stop-button {');
    expect(stylesCss).not.toContain('.chat-stop-button.active {');
    expect(stylesCss).toContain('.chat-config-pill {');
    expect(stylesCss).toContain('.chat-config-value-menu {');
    expect(stylesCss).toMatch(
      /\.chat-config-value-menu \{[\s\S]*width: max-content;[\s\S]*min-width: 100%;[\s\S]*max-width: min\(320px, calc\(100vw - 24px\)\);[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-config-value-option {');
    expect(stylesCss).toContain('.chat-config-value-menu .chat-config-value-option {');
    expect(stylesCss).toMatch(
      /\.chat-config-value-menu \.chat-config-value-option \{[\s\S]*width: 100%;[\s\S]*height: auto;[\s\S]*align-items: flex-start;[\s\S]*padding: 6px 8px;[\s\S]*line-height: 1.25;/,
    );
    expect(stylesCss).toContain('.chat-config-value-label {');
    expect(stylesCss).toMatch(
      /\.chat-config-value-label \{[\s\S]*overflow-wrap: anywhere;[\s\S]*text-align: left;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-config-options .chat-config-item:first-child:not(:only-child) .chat-config-value-menu {');
    expect(stylesCss).toContain('.chat-config-options .chat-config-item:only-child .chat-config-value-menu {');
    expect(stylesCss).toContain('.chat-config-overflow-group {');
    expect(stylesCss).not.toContain('.chat-config-select {');
    expect(stylesCss).not.toContain('.chat-config-feedback {');
  });

  test('keeps the mobile drawer wide while preserving floating control clicks and backdrop dismissal', () => {
    const projectRoot = path.join(__dirname, '..');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    const backdropLayer = cssNumericProperty(stylesCss, '.drawer-overlay', 'z-index');
    const drawerLayer = cssNumericProperty(stylesCss, '.drawer', 'z-index');
    const floatingLayer = cssNumericProperty(stylesCss, '.floating-control-stack-layer', 'z-index');
    const mobileSettingsLayer = cssNumericProperty(stylesCss, '.mobile-settings-screen', 'z-index');

    expect(backdropLayer).toBeLessThan(floatingLayer);
    expect(drawerLayer).toBeGreaterThan(floatingLayer);
    expect(mobileSettingsLayer).toBeGreaterThan(drawerLayer);
    expect(stylesCss).toContain('--mobile-floating-control-lane: 56px;');
    expect(stylesCss).toMatch(
      /\.drawer-overlay \{[\s\S]*inset: 0;[\s\S]*z-index: 43;[\s\S]*\}/,
    );
    expect(stylesCss).not.toMatch(
      /\.drawer-overlay \{[\s\S]*right: var\(--mobile-floating-control-lane\);[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.drawer \{[\s\S]*position: fixed;[\s\S]*width: min\(440px, calc\(100vw - var\(--mobile-floating-control-lane\) - env\(safe-area-inset-right, 0px\)\)\);[\s\S]*z-index: 50;[\s\S]*\}/,
    );
  });

  test('keeps the mobile three-tab floating nav and drawer button translucent over content', () => {
    const projectRoot = path.join(__dirname, '..');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(stylesCss).toMatch(
      /\.floating-nav-group,\s*\.drawer-toggle-bubble \{[\s\S]*background: color-mix\(in srgb, var\(--panel\) 34%, transparent\);[\s\S]*backdrop-filter: blur\(1px\);[\s\S]*\}/,
    );
    expect(stylesCss).not.toContain('background: color-mix(in srgb, var(--panel) 78%, transparent);');
  });

  test('mobile chat drawer uses a cross-project project session sheet', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | null;");
    expect(mainTsx).toContain('const [settingsDetailView, setSettingsDetailView] = useState<SettingsDetailView>(null);');
    expect(mainTsx).toContain('const [mobileProjectActionMenu, setMobileProjectActionMenu] = useState<MobileProjectActionMenuState | null>(null);');
    expect(mainTsx).toContain('const refreshMobileChatProjectSessions = async () => {');
    expect(mainTsx).toContain('await refreshChatIndex();');
    expect(mainTsx).toContain('latestProjects.map(projectItem =>');
    expect(mainTsx).toContain('refreshChatProjectSessions(projectItem.projectId)');
    expect(mainTsx).toContain('const renderMobileChatSessionSheet = () => {');
    expect(mainTsx).toContain('className="mobile-chat-drawer-header"');
    expect(mainTsx).toContain('<span className="mobile-chat-drawer-title">Chats</span>');
    expect(mainTsx).toContain('className="mobile-project-session-nav"');
    expect(mainTsx).toContain('className="mobile-project-action-panel"');
    expect(mainTsx).toContain('className="mobile-project-session-error"');
    expect(mainTsx).toContain('if (!isWide) setDrawerOpen(false);');
    expect(mainTsx).toContain("tab === 'chat' && !isWide ? renderMobileChatSessionSheet() : renderSidebarMain()");
    expect(mainTsx).toContain("setSettingsDetailView('tokenStats');");
    expect(mainTsx).toContain("settingsDetailView === 'tokenStats'");
    expect(mainTsx).toContain('const renderSettingsSection = (title: string, rows: React.ReactNode, icon?: string) => (');
    expect(mainTsx).toContain("renderSettingsSection('Appearance'");
    expect(mainTsx).toContain("renderSettingsSection('Chat'");
    expect(mainTsx).toContain("renderSettingsSection('Code Display'");
    expect(mainTsx).toContain("renderSettingsSection('More'");
    const appearanceSettingsIndex = mainTsx.indexOf("renderSettingsSection('Appearance'");
    const chatSettingsIndex = mainTsx.indexOf("renderSettingsSection('Chat'");
    const codeDisplaySettingsIndex = mainTsx.indexOf("renderSettingsSection('Code Display'");
    const moreSettingsIndex = mainTsx.indexOf("renderSettingsSection('More'");
    expect(appearanceSettingsIndex).toBeLessThan(chatSettingsIndex);
    expect(chatSettingsIndex).toBeLessThan(codeDisplaySettingsIndex);
    expect(codeDisplaySettingsIndex).toBeLessThan(moreSettingsIndex);
    expect(mainTsx).toContain("setSettingsDetailView('database');");
    expect(mainTsx).toContain("settingsDetailView === 'database'");
    expect(mainTsx).toContain('renderDatabaseSettingsDetail(options)');
    expect(mainTsx).toContain('className="settings-section-title"');
    expect(mainTsx).toContain('className="settings-row settings-detail-row"');
    expect(mainTsx).not.toContain('title="Token stats"');
    expect(mainTsx).not.toContain('title="Agent info"');
    expect(mainTsx).not.toContain('className="chat-session-swipe-row');

    const mobileSheetStart = mainTsx.indexOf('const renderMobileChatSessionSheet = () => {');
    const mobileSheetEnd = mainTsx.indexOf('const renderSidebar = () => {', mobileSheetStart);
    expect(mobileSheetStart).toBeGreaterThanOrEqual(0);
    expect(mobileSheetEnd).toBeGreaterThan(mobileSheetStart);
    const mobileSheet = mainTsx.slice(mobileSheetStart, mobileSheetEnd);
    expect(mobileSheet).not.toContain('className="project-wrap"');
    expect(mobileSheet).toContain('renderProjectSessionActionStrip(targetProjectId, session)');
    expect(mobileSheet).toContain('onPointerDown={event => startProjectSessionLongPress(targetProjectId, session.sessionId, event)}');
    expect(mobileSheet).not.toContain('chat-session-swipe-row');
    expect(mobileSheet).toContain("tagVariantClass('wide-project-hub', projectItem.hubId || 'local')");
    expect(mobileSheet).toContain("tagVariantClass('wide-session-agent', sessionAgent)");

    expect(stylesCss).toContain('.mobile-chat-drawer-header {');
    expect(stylesCss).toContain('.mobile-project-session-nav {');
    expect(stylesCss).toContain('.mobile-project-action-panel {');
    expect(stylesCss).toContain('.mobile-project-session-error {');
    expect(stylesCss).toContain('.settings-detail-header {');
    expect(stylesCss).toContain('.settings-detail-row {');
    expect(stylesCss).toContain('.settings-section-title {');
    expect(stylesCss).toContain('.settings-row {');
    expect(stylesCss).toContain('.settings-danger-row {');
    expect(stylesCss).toContain('.settings-metadata-list {');
    expect(stylesCss).toContain('.settings-database-dump {');
  });

  test('wide layout uses a project session rail instead of the header project picker', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('const WIDE_PROJECT_SESSION_LIMIT = 5;');
    expect(mainTsx).toContain('const PROJECT_PIN_LONG_PRESS_MS = 450;');
    expect(mainTsx).toContain('function tagVariantClass(prefix: string, value: string): string {');
    expect(mainTsx).toContain('const sortedProjectItems = useMemo(() => sortProjectsByPin(projects, pinnedProjectIds), [projects, pinnedProjectIds]);');
    expect(mainTsx).toContain('const togglePinnedProject = useCallback(');
    expect(mainTsx).toContain('const startProjectPinLongPress = useCallback(');
    expect(mainTsx).toContain('const consumeProjectPinLongPressClick = useCallback(');
    expect(mainTsx).toContain('const renderWideProjectSessionNav = () => {');
    expect(mainTsx).toContain('className="wide-project-session-nav"');
    expect(mainTsx).toContain('className="wide-project-title-group"');
    expect(mainTsx).toContain("collapsed ? 'codicon-folder' : 'codicon-folder-opened'");
    expect(mainTsx).toContain("className=\"codicon codicon-pinned wide-project-pin-badge\"");
    expect(mainTsx).toContain('onPointerDown={event => startProjectPinLongPress(targetProjectId, event)}');
    expect(mainTsx).toContain('onPointerUp={finishProjectPinLongPress}');
    expect(mainTsx).toContain('onContextMenu={event => event.preventDefault()}');
    expect(mainTsx).toContain("tagVariantClass('wide-project-hub', projectItem.hubId || 'local')");
    expect(mainTsx).toContain('className="wide-project-hub-dot"');
    expect(mainTsx).toContain('className="wide-project-hub-label"');
    expect(mainTsx).toContain('className="wide-project-session-list"');
    expect(mainTsx).toContain('className="wide-project-action-btn"');
    expect(mainTsx).toContain('className="wide-project-action-popover"');
    expect(mainTsx).toContain("import {resolveWideProjectActionPopoverPlacement");
    expect(mainTsx).toContain('style={wideProjectActionMenu.popover');
    expect(mainTsx).toContain('className="wide-project-action-title"');
    expect(mainTsx).toContain("wideProjectActionMenu.kind === 'new' ? 'New Session' : 'Resume Session'");
    expect(mainTsx).toContain("const sessionAgent = (session.agentType || '').trim();");
    expect(mainTsx).toContain("tagVariantClass('wide-session-agent', sessionAgent)");
    expect(mainTsx).toContain('const [projectSessionActionMenu, setProjectSessionActionMenu] = useState<ProjectSessionActionMenuState | null>(null);');
    expect(mainTsx).toContain('const PROJECT_SESSION_LONG_PRESS_MS = 450;');
    expect(mainTsx).not.toContain('const handleDeleteProjectSession = async (targetProjectId: string, sessionId: string) => {');
    expect(mainTsx).not.toContain('service.deleteProjectSession(');
    expect(mainTsx).toContain('const handleReloadProjectSession = async (targetProjectId: string, sessionId: string) => {');
    expect(mainTsx).toContain('const result = await service.reloadProjectSession(targetProjectId, normalizedSessionId);');
    expect(mainTsx).toContain('const [confirmTarget, setConfirmTarget] = useState<ConfirmTarget | null>(null);');
    expect(mainTsx).toContain("const [confirmError, setConfirmError] = useState('');");
    expect(mainTsx).toContain('const handleArchiveProjectSession = async (targetProjectId: string, sessionId: string) => {');
    expect(mainTsx).toContain('const result = await service.archiveProjectSession(targetProjectId, normalizedSessionId);');
    expect(mainTsx).toContain('const message = err instanceof Error ? err.message : String(err);');
    expect(mainTsx).toContain('setConfirmError(message);');
    expect(mainTsx).toContain('const appConfirmDialog = confirmTarget ? (');
    expect(mainTsx).toContain('className="app-confirm-backdrop"');
    expect(mainTsx).toContain('Archived sessions leave the chat list.');
    expect(mainTsx).toContain('className="app-confirm-error"');
    expect(mainTsx).not.toContain('Sessions with fewer than 3 turns are permanently removed.');
    expect(mainTsx).toContain('const renderProjectSessionActionStrip = (targetProjectId: string, session: RegistrySessionSummary) => {');
    expect(mainTsx).toContain('className="project-session-action-strip"');
    expect(mainTsx).toContain('className="project-session-action-btn reload"');
    expect(mainTsx).toContain('className="project-session-action-btn archive"');
    expect(mainTsx).not.toContain('className="project-session-action-btn delete"');
    expect(mainTsx).toContain('const sessionActionDisabled = !!session.running ||');
    expect(mainTsx).toContain('className="project-session-action-label">Reload</span>');
    expect(mainTsx).toContain('className="project-session-action-label">Archive</span>');
    expect(mainTsx).not.toContain('className="project-session-action-label">Delete</span>');
    expect(mainTsx).toContain("if (target?.closest('.project-session-action-btn')) {");
    expect(mainTsx).not.toContain("target?.closest('.project-session-row-wrap')");
    expect(mainTsx).toContain('renderProjectSessionActionStrip(targetProjectId, session)');
    expect(mainTsx).toContain('onPointerDown={event => startProjectSessionLongPress(targetProjectId, session.sessionId, event)}');
    const actionStripStart = mainTsx.indexOf('const renderProjectSessionActionStrip = (targetProjectId: string, session: RegistrySessionSummary) => {');
    const actionStripEnd = mainTsx.indexOf('const refreshProject = async', actionStripStart);
    expect(actionStripStart).toBeGreaterThanOrEqual(0);
    expect(actionStripEnd).toBeGreaterThan(actionStripStart);
    const actionStrip = mainTsx.slice(actionStripStart, actionStripEnd);
    expect(actionStrip).not.toContain('return null;');
    expect(mainTsx).toContain("tab === 'chat' && !isWide ? renderMobileChatSessionSheet() : renderSidebarMain()");
    expect(mainTsx).toContain("tab === 'chat' ? renderWideProjectSessionNav() : renderSidebarMain(false)");
    expect(mainTsx).toContain('const wideSidebarMain = sidebarSettingsOpen');
    expect(mainTsx).toContain('? renderSettingsContent(false)');
    expect(mainTsx).toContain('const wideSidebarTitle = sidebarSettingsOpen');
    expect(mainTsx).toContain('className="sidebar-title-row"');
    expect(mainTsx).toContain('const handleDesktopActivitySelect = useCallback((nextTab: Tab) => {');
    expect(mainTsx).toContain('const handleDesktopSettingsSelect = useCallback(() => {');
    expect(mainTsx).toContain('const beginDesktopSidebarResize = useCallback(');
    expect(mainTsx).toContain('className={`desktop-sidebar-resize-handle${desktopSidebarResizing ?');
    expect(mainTsx).toContain('desktopSidebarWidth={effectiveDesktopSidebarWidth}');

    const wideRailStart = mainTsx.indexOf('const renderWideProjectSessionNav = () => {');
    const wideRailEnd = mainTsx.indexOf('const renderSidebar = () => {', wideRailStart);
    expect(wideRailStart).toBeGreaterThanOrEqual(0);
    expect(wideRailEnd).toBeGreaterThan(wideRailStart);
    const wideRail = mainTsx.slice(wideRailStart, wideRailEnd);
    expect(wideRail).not.toContain('codicon-chevron-right');
    expect(wideRail).not.toContain('codicon-chevron-down');

    expect(mainTsx).not.toContain('const wideHeader = isWide ? (');
    expect(mainTsx).not.toContain('className="header"');

    const activityBarStart = mainTsx.indexOf('const desktopActivityBar = isWide ? (');
    const activityBarEnd = mainTsx.indexOf('const floatingControlStack = !isWide ? (', activityBarStart);
    expect(activityBarStart).toBeGreaterThanOrEqual(0);
    expect(activityBarEnd).toBeGreaterThan(activityBarStart);
    const activityBar = mainTsx.slice(activityBarStart, activityBarEnd);
    expect(activityBar).toContain('className="desktop-activity-bar"');
    expect(activityBar).toContain("onClick={() => handleDesktopActivitySelect('chat')}");
    expect(activityBar).toContain("onClick={() => handleDesktopActivitySelect('file')}");
    expect(activityBar).toContain("onClick={() => handleDesktopActivitySelect('git')}");
    expect(activityBar).toContain('onClick={handleDesktopSettingsSelect}');
    expect(activityBar).toContain('onClick={() => refreshProject().catch(() => undefined)}');
    expect(activityBar.indexOf("title={reconnecting ? 'Reconnecting...' : 'Refresh project'}")).toBeLessThan(
      activityBar.indexOf('title="Settings"'),
    );
    expect(activityBar).not.toContain('className="project-wrap"');
    expect(activityBar).not.toContain('className="project-btn"');
    expect(activityBar).not.toContain('className="tabs"');

    expect(stylesCss).toContain('.wide-project-session-nav {');
    expect(stylesCss).toContain('--desktop-side-surface: color-mix(in srgb, var(--panel) 62%, var(--panel-3));');
    expect(stylesCss).toContain('.desktop-activity-bar {');
    expect(stylesCss).toContain('.desktop-activity-button {');
    expect(stylesCss).toContain('.desktop-activity-button.active::before {');
    expect(stylesCss).toContain('.sidebar-title-row {');
    expect(stylesCss).toMatch(
      /\.desktop-activity-bar \{[\s\S]*background: var\(--desktop-side-surface\);[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.workspace-left \{[\s\S]*background: var\(--desktop-side-surface\);[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.workspace-left \{[\s\S]*width: var\(--desktop-sidebar-width, 380px\);[\s\S]*min-width: 320px;[\s\S]*max-width: min\(560px, 45vw\);[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.desktop-sidebar-resize-handle {');
    expect(stylesCss).toMatch(
      /\.desktop-sidebar-resize-handle \{[\s\S]*cursor: ew-resize;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.desktop-activity-button\.active::before \{[\s\S]*top: 0;[\s\S]*bottom: 0;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.sidebar-title-row \{[\s\S]*border-bottom: 0;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.wide-project-row {');
    expect(stylesCss).toContain('.wide-project-folder-wrap {');
    expect(stylesCss).toContain('.wide-project-folder-icon {');
    expect(stylesCss).toContain('.wide-project-pin-badge {');
    expect(stylesCss).toContain('.wide-project-title-group {');
    expect(stylesCss).toContain('.wide-project-hub-tag {');
    expect(stylesCss).toContain('.wide-project-hub-dot {');
    expect(stylesCss).toContain('.wide-project-hub-label {');
    expect(stylesCss).toContain('.wide-project-hub-0 {');
    expect(stylesCss).toContain('.wide-project-action-btn {');
    expect(stylesCss).toContain('.wide-project-action-title {');
    expect(stylesCss).toContain('.wide-session-row {');
    expect(stylesCss).toContain('.project-session-row-wrap {');
    expect(stylesCss).toContain('.project-session-action-strip {');
    expect(stylesCss).toContain('.project-session-action-btn.reload {');
    expect(stylesCss).toContain('.project-session-action-btn.archive {');
    expect(stylesCss).not.toContain('.project-session-action-btn.delete {');
    expect(stylesCss).toContain('.app-confirm-backdrop {');
    expect(stylesCss).toContain('.app-confirm-dialog {');
    expect(stylesCss).toContain('.app-confirm-error {');
    expect(stylesCss).toContain('.project-session-action-label {');
    expect(stylesCss).toContain('.wide-session-agent-tag {');
    expect(stylesCss).toContain('.wide-session-agent-0 {');
    expect(stylesCss).toContain('.wide-session-time {');
    expect(stylesCss).toContain('.wide-project-action-popover {');
    expect(stylesCss).toMatch(
      /\.wide-project-action-popover \{[\s\S]*position: fixed;[\s\S]*overflow-y: auto;[\s\S]*overscroll-behavior: contain;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(/\.wide-project-row \{[^}]*min-height: 32px;[^}]*\}/);
    expect(stylesCss).toMatch(/\.wide-project-toggle \{[^}]*height: 30px;[^}]*\}/);
    expect(stylesCss).toMatch(/\.wide-session-row \{[^}]*min-height: 26px;[^}]*\}/);
    const wideSessionRowBlock = stylesCss.match(/\.wide-session-row \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(wideSessionRowBlock).toContain('grid-template-columns: 9px minmax(0, 1fr) auto auto;');
    expect(wideSessionRowBlock).toContain('gap: 5px;');
    expect(wideSessionRowBlock).toContain('padding: 0 6px 0 3px;');
    const sessionStateMarkerBlock = stylesCss.match(/\.session-state-marker \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(sessionStateMarkerBlock).toContain('width: 9px;');
    expect(sessionStateMarkerBlock).toContain('flex: 0 0 9px;');
    expect(sessionStateMarkerBlock).not.toContain('transform: translateX');
    const sessionStateRunningBlock = stylesCss.match(/\.session-state-marker\.running \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(sessionStateRunningBlock).toContain('font-size: 11px;');
    expect(stylesCss).toMatch(/\.mobile-session-row \{[^}]*min-height: 30px;[^}]*\}/);
    expect(stylesCss).toContain('font-size: 10.5px;');
    expect(stylesCss).toContain('.wide-project-folder-icon.codicon-folder {');
    expect(stylesCss).toContain('.wide-project-folder-icon.codicon-folder-opened {');
    expect(stylesCss).toMatch(
      /\.wide-project-folder-icon\.codicon-folder-opened \{[\s\S]*color: color-mix\(in srgb, var\(--hub-accent\) 82%, var\(--text\)\);[\s\S]*\}/,
    );
    const selectedSessionRowBlock = stylesCss.match(/\.wide-session-row\.selected \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(selectedSessionRowBlock).toContain('background: color-mix(in srgb, var(--accent) 18%, var(--panel-2));');
    expect(selectedSessionRowBlock).not.toContain('box-shadow: inset 3px 0 0 var(--accent);');
    expect(stylesCss).not.toContain('.wide-session-row::after');
    expect(stylesCss).not.toContain('.project-session-row-wrap.actions-open .wide-session-row {');
    expect(stylesCss).toMatch(
      /\.project-session-action-strip \{[^}]*top: 50%;[^}]*height: 30px;[^}]*transform: translateY\(-50%\);[^}]*width: min\(188px, calc\(100% - 4px\)\);[^}]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.project-session-action-strip \{[^}]*display: none;[^}]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.project-session-row-wrap\.actions-open \.project-session-action-strip \{[\s\S]*display: inline-flex;[\s\S]*\}/,
    );
    expect(stylesCss).not.toContain('.project-session-row-wrap:hover .project-session-action-strip');
    expect(stylesCss).not.toContain('.project-session-row-wrap:focus-within .project-session-action-strip');
    expect(stylesCss).toMatch(
      /\.project-session-action-btn \{[^}]*height: 28px;[^}]*gap: 5px;[^}]*padding: 0 8px;[^}]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.project-session-action-btn\.reload \{[^}]*background: color-mix\(in srgb, #2f9e44 18%, transparent\);[^}]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.project-session-action-btn\.archive \{[^}]*background: color-mix\(in srgb, #2b6cb0 14%, transparent\);[^}]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.wide-session-title \{[\s\S]*font-weight: 400;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.wide-project-hub-tag \{[\s\S]*border: none;[\s\S]*background: transparent;[\s\S]*\}/,
    );
    const wideProjectTitleGroupBlock = stylesCss.match(/\.wide-project-title-group \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(wideProjectTitleGroupBlock).toContain('overflow: hidden;');
    const wideProjectNameBlock = stylesCss.match(/\.wide-project-name \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(wideProjectNameBlock).toContain('flex: 0 0 auto;');
    expect(wideProjectNameBlock).toContain('max-width: 100%;');
    const wideProjectHubTagBlock = stylesCss.match(/\.wide-project-hub-tag \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(wideProjectHubTagBlock).toContain('flex: 1 1 0;');
    expect(wideProjectHubTagBlock).toContain('min-width: 0;');
    expect(wideProjectHubTagBlock).toContain('max-width: max-content;');
    expect(wideProjectHubTagBlock).toContain('overflow: hidden;');
    const wideProjectHubLabelBlock = stylesCss.match(/\.wide-project-hub-label \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(wideProjectHubLabelBlock).toContain('flex: 1 1 auto;');
    expect(stylesCss).toMatch(
      /\.wide-project-pin-badge \{[\s\S]*position: absolute;[\s\S]*right: -4px;[\s\S]*top: -5px;[\s\S]*\}/,
    );
  });

  test('wide project session rail actions use project-scoped chat flows', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const selectWideProjectSession = async (targetProjectId: string, sessionId: string) => {');
    expect(mainTsx).toContain('const selectProjectChatSession = async (');
    expect(mainTsx).toContain('workspaceStore.rememberSelectedChatSessionKey(nextSelectedKey);');
    expect(mainTsx).toContain("setTab('chat');");
    expect(mainTsx).toContain('loadChatSession(sessionId, targetProjectId, {');
    const selectProjectStart = mainTsx.indexOf('const selectProjectChatSession = async (');
    const selectProjectEnd = mainTsx.indexOf('const selectWideProjectSession = async', selectProjectStart);
    expect(selectProjectStart).toBeGreaterThanOrEqual(0);
    expect(selectProjectEnd).toBeGreaterThan(selectProjectStart);
    const selectProjectBody = mainTsx.slice(selectProjectStart, selectProjectEnd);
    expect(selectProjectBody).not.toContain('switchProject(');
    expect(selectProjectBody).toContain('hydrateChatSessionContentFromCache(sessionId, targetProjectId)');
    expect(selectProjectBody).toContain('selectionSnapshot: runtimeKey');
    expect(mainTsx).toContain('const handleWideProjectCreateSession = async (targetProjectId: string, agentType: string) => {');
    expect(mainTsx).toContain("const result = await service.createProjectSession(targetProjectId, agentType, '');");
    expect(mainTsx).toContain('const handleWideProjectResumeAgent = async (targetProjectId: string, agentType: string) => {');
    expect(mainTsx).toContain('const sessions = await service.listProjectResumableSessions(targetProjectId, agentType);');
    expect(mainTsx).toContain('const handleWideProjectResumeImport = async (targetProjectId: string, agentType: string, sessionId: string) => {');
    expect(mainTsx).toContain('const imported = await service.importProjectResumedSession(targetProjectId, agentType, sessionId);');
    expect(mainTsx).toContain('const reloaded = await service.reloadProjectSession(targetProjectId, importedSessionId);');
    expect(mainTsx).toContain('wideProjectActionMenuRef.current?.contains(target)');
  });
});
