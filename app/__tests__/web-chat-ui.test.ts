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
    expect(repositoryTs).toContain('Array.isArray(payload.turns) ? payload.turns : []');
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
    expect(mainTsx).toContain('return { sessionId, turnIndex, method, param, finished };');
    expect(mainTsx).not.toContain('updateIndex');
    expect(mainTsx).toContain('service.markProjectSessionRead(activeProjectId, sessionId, cursor)');
    expect(mainTsx).toContain('chatSyncIndexRef');
    expect(mainTsx).not.toContain('chatPromptSnapshotVersion');
    expect(mainTsx).toContain('nextSessions.some(session => session.sessionId === currentSelection)');
    expect(mainTsx).not.toContain('result.lastIndex < afterIndex');
    expect(mainTsx).toContain('preserveUserSelection');
    expect(mainTsx).toContain('const shouldSyncSelectedSession =');
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
    expect(mainTsx).toContain('const CHAT_AUTO_SCROLL_BOTTOM_THRESHOLD = 80;');
    expect(mainTsx).toContain('function isChatScrolledNearBottom(container: HTMLElement): boolean {');
    expect(mainTsx).toContain('const updateChatFollowModeFromScroll = useCallback(');
    expect(mainTsx).toContain('const nearBottom = isChatScrolledNearBottom(container);');
    expect(mainTsx).toContain('chatAutoScrollFollowRef.current = nearBottom;');
    expect(mainTsx).toContain('setChatShowScrollToBottom(!nearBottom);');
    expect(mainTsx).toContain('const scrollChatToBottom = useCallback((force = false) => {');
    expect(mainTsx).toContain('if (!force && (!chatAutoScrollFollowRef.current || chatPointerScrollingRef.current)) {');
    expect(mainTsx).toContain('const forceChatScrollToBottom = useCallback(() => {');
    expect(mainTsx).toContain('chatAutoScrollFollowRef.current = true;');
    expect(mainTsx).toContain('scrollChatToBottom(true);');
    expect(mainTsx).toContain('useLayoutEffect(() => {');
    expect(mainTsx).toContain('resizeChatComposerTextarea();');
    expect(mainTsx).toContain('}, [resizeChatComposerTextarea, chatComposerText, tab, selectedChatId, currentChatDraftKey]);');
    expect(mainTsx).toContain('}, [tab, selectedChatId, chatMessages, chatPendingPromptsByKey, chatLoading, chatKeyboardInset, resizeChatComposerTextarea, scrollChatToBottom]);');
    expect(mainTsx).toMatch(
      /onScroll=\{event => \{[\s\S]*updateChatFollowModeFromScroll\(event\.currentTarget\);[\s\S]*expandSelectedChatWindowEarlier\(event\.currentTarget\);[\s\S]*\}\}/,
    );
    expect(mainTsx).toContain('onPointerDown={() => { chatPointerScrollingRef.current = true; }}');
    expect(mainTsx).toContain('onPointerUp={() => { chatPointerScrollingRef.current = false; updateChatFollowModeFromScroll(); }}');
    expect(mainTsx).toContain('onTouchStart={() => { chatPointerScrollingRef.current = true; }}');
    expect(mainTsx).toContain('onTouchEnd={() => { chatPointerScrollingRef.current = false; updateChatFollowModeFromScroll(); }}');
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
    expect(mainTsx).toContain('blocks.push(...chatAttachments.map(attachment => ({');
    expect(mainTsx).toContain("if (chatAttachmentReadPending) {");
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
    expect(stylesCss).toContain('.chat-composer::before {');
    expect(stylesCss).toMatch(
      /\.chat-composer \{[\s\S]*--chat-composer-frame-top: 12px;[\s\S]*--chat-composer-fade-distance: 28px;[\s\S]*margin-top: calc\(-1 \* var\(--chat-composer-frame-top\)\);[\s\S]*padding: var\(--chat-composer-frame-top\) 14px 12px;[\s\S]*background: transparent;/,
    );
    expect(stylesCss).toMatch(
      /\.chat-composer::before \{[\s\S]*top: calc\(var\(--chat-composer-frame-top\) - var\(--chat-composer-fade-distance\)\);[\s\S]*height: var\(--chat-composer-fade-distance\);[\s\S]*transparent 0%,[\s\S]*color-mix\(in srgb, var\(--bg\) 22%, transparent\) 34%,[\s\S]*color-mix\(in srgb, var\(--bg\) 78%, transparent\) 76%,[\s\S]*var\(--bg\) 100%/,
    );
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
    const sendExistingStart = mainTsx.indexOf('const selectedKey = selectedChatKeyRef.current;');
    const firstRunningMark = mainTsx.indexOf('markChatSessionRunning(selectedProjectId, sessionId,', sendExistingStart);
    const sendAwait = mainTsx.indexOf('const result = await service.sendProjectSessionMessage(selectedProjectId, {', sendExistingStart);
    expect(sendExistingStart).toBeGreaterThanOrEqual(0);
    expect(firstRunningMark).toBeGreaterThan(sendExistingStart);
    expect(sendAwait).toBeGreaterThan(firstRunningMark);
    expect(mainTsx).toContain('const [hasPendingProjectUpdates, setHasPendingProjectUpdates] = useState(false);');
    expect(mainTsx).toContain('if (!eventProjectId || eventProjectId === projectIdRef.current) {');
    expect(mainTsx).toContain('setHasPendingProjectUpdates(true);');
    expect(mainTsx).toContain('if (!silent) {');
    expect(mainTsx).toContain('setHasPendingProjectUpdates(false);');
    expect(mainTsx).not.toContain('setChatPromptSnapshotVersion(version => version + 1);');
    expect(mainTsx).toContain('setChatSessions(prev => {');
    expect(mainTsx).toContain('const byId = new Map(prev.map(item => [item.sessionId, item]));');
    expect(mainTsx).toContain('const merged = mergeChatSession(next, session);');
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
    expect(mainTsx).toContain('<div className="mobile-settings-title">Settings</div>');
    expect(mainTsx).toContain('className="mobile-settings-group"');
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
    expect(mainTsx).toContain("CHAT - {selectedChatSession?.title || 'New Session'}");
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
    expect(mainTsx).toContain("const shouldRefreshCompletedPrompt = message.method === 'prompt_done';");
    expect(mainTsx).toContain('const shouldMarkSessionRunning = isChatSessionRunningMessage(message);');
    expect(mainTsx).toContain('if (shouldMarkSessionRunning) {');
    expect(mainTsx).toContain('const latestSyncCursor = getLatestSessionReadCursor(merged);');
    expect(mainTsx).toContain('const readCursorForGap = shouldRequestSessionReadForIncomingTurn(');
    const promptDoneSignal = mainTsx.indexOf("const shouldRefreshCompletedPrompt = message.method === 'prompt_done';");
    const promptDoneStateApply = mainTsx.indexOf('if (shouldRefreshCompletedPrompt) {', promptDoneSignal);
    const promptGapRead = mainTsx.indexOf('if (readCursorForGap) {', promptDoneSignal);
    expect(promptDoneSignal).toBeGreaterThanOrEqual(0);
    expect(promptDoneStateApply).toBeGreaterThan(promptDoneSignal);
    expect(promptGapRead).toBeGreaterThan(promptDoneStateApply);
    expect(mainTsx).toContain('lastReadTurnIndex: isSelectedSession && completedTurnIndex > 0');
    expect(mainTsx).toContain('messages.filter(isFinishedChatMessage)');
    expect(mainTsx).toContain('needsPromptTurnRefresh(');
    expect(mainTsx).toContain('refreshSessionTurns(');
    expect(mainTsx).not.toContain('if (shouldRefreshCompletedPrompt && isSelectedSession) {\n          loadChatSession(sessionId, projectIdRef.current, {\n            forceFull: true,');
    const eventRunningSignal = mainTsx.indexOf('const shouldMarkSessionRunning = isChatSessionRunningMessage(message);');
    const eventRunningApply = mainTsx.indexOf('if (shouldMarkSessionRunning) {', eventRunningSignal);
    const eventGapRead = mainTsx.indexOf('const readCursorForGap = shouldRequestSessionReadForIncomingTurn(', eventRunningSignal);
    expect(eventRunningSignal).toBeGreaterThanOrEqual(0);
    expect(eventRunningApply).toBeGreaterThan(eventRunningSignal);
    expect(eventGapRead).toBeGreaterThan(eventRunningApply);
    expect(mainTsx).toContain('const runtimeKey = buildChatRuntimeKey(eventProjectId, payload.session.sessionId);');
    expect(mainTsx).toContain('encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey');
    expect(mainTsx).toContain('loadChatSession(payload.session.sessionId, eventProjectId, {');
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
    expect(stylesCss).toContain('.mobile-settings-screen .sidebar-setting-row {');
    expect(stylesCss).toContain('.mobile-settings-screen .sidebar-clear-cache-btn {');
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

  test('chat composer is a unified command frame with compact custom config pills', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('const CHAT_CONFIG_INLINE_LIMIT = 3;');
    expect(mainTsx).toContain('const [chatPromptMenuOpen, setChatPromptMenuOpen] = useState(false);');
    expect(mainTsx).toContain("const [chatConfigMenuOptionId, setChatConfigMenuOptionId] = useState('');");
    expect(mainTsx).toContain('selectedChatConfigOptions.length <= CHAT_CONFIG_INLINE_LIMIT');
    expect(mainTsx).toContain('prioritized.slice(0, CHAT_CONFIG_INLINE_LIMIT)');
    expect(mainTsx).toContain('className="chat-composer-frame"');
    expect(mainTsx).toContain('className="chat-composer-input-row"');
    expect(mainTsx).toContain('className="chat-composer-prompt-trigger"');
    expect(mainTsx).toContain('title="Commands"');
    expect(mainTsx).toContain("{'>'}");
    expect(mainTsx).not.toContain('chatComposerText.length === 0 ? (');
    expect(mainTsx).not.toContain('if (chatComposerText.length > 0) {');
    expect(mainTsx).toContain('className="chat-composer-toolbar"');
    expect(mainTsx).toContain('className="chat-composer-tools"');
    expect(mainTsx).toContain('className="chat-tool-button chat-photo-button"');
    expect(mainTsx).toContain('className="chat-tool-button chat-voice-button"');
    expect(mainTsx).toContain('chatFileInputRef.current?.click();');
    expect(mainTsx).toContain("setError('Voice input is not available yet.');");
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

    const configChangeStart = mainTsx.indexOf('const handleChatConfigOptionChange = async');
    const configChangeEnd = mainTsx.indexOf('const handleChatFileChange = async', configChangeStart);
    const configChangeBody = mainTsx.slice(configChangeStart, configChangeEnd);
    const setConfigCall = configChangeBody.indexOf('const result = await service.setProjectSessionConfig');
    expect(setConfigCall).toBeGreaterThanOrEqual(0);
    expect(configChangeBody).toContain('selectedKey.projectId');
    expect(configChangeBody.indexOf('applyChatSessionConfigOptions')).toBeGreaterThan(setConfigCall);
    expect(configChangeBody).not.toContain('setChatSessions(prev =>');

    expect(stylesCss).toMatch(
      /button,\s*\[role='button'\],\s*\[role='menuitemradio'\],\s*\[role='option'\]\s*\{[\s\S]*-webkit-tap-highlight-color: transparent;/,
    );
    expect(stylesCss).toMatch(
      /\.chat-composer \{[\s\S]*--chat-composer-frame-top: 12px;[\s\S]*--chat-composer-fade-distance: 28px;[\s\S]*background: transparent;/,
    );
    expect(stylesCss).toMatch(
      /\.chat-composer::before \{[\s\S]*top: calc\(var\(--chat-composer-frame-top\) - var\(--chat-composer-fade-distance\)\);[\s\S]*height: var\(--chat-composer-fade-distance\);/,
    );
    expect(stylesCss).toContain('.chat-composer-frame {');
    expect(stylesCss).toMatch(
      /\.chat-composer-frame \{[\s\S]*gap: 0;[\s\S]*padding: 5px 6px 3px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-composer-input-row {');
    expect(stylesCss).toMatch(
      /\.chat-composer-input-row \{[\s\S]*gap: 5px;[\s\S]*min-height: 32px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-composer-prompt-trigger {');
    expect(stylesCss).toMatch(
      /\.chat-composer-prompt-trigger \{[\s\S]*width: 22px;[\s\S]*height: 30px;[\s\S]*display: inline-flex;[\s\S]*align-items: center;[\s\S]*justify-content: center;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.chat-composer-input \{[\s\S]*min-height: 30px;[\s\S]*padding: 5px 0 1px;[\s\S]*font-size: 14px;[\s\S]*line-height: 1.4;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.chat-send-button \{[\s\S]*width: 32px;[\s\S]*height: 32px;[\s\S]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.chat-send-button \.codicon \{[\s\S]*font-size: 15px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-composer-toolbar {');
    expect(stylesCss).toMatch(
      /\.chat-composer-toolbar \{[\s\S]*min-height: 24px;[\s\S]*\}/,
    );
    expect(stylesCss).toContain('.chat-composer-tools {');
    expect(stylesCss).toContain('.chat-tool-button {');
    expect(stylesCss).toMatch(
      /\.chat-tool-button \{[\s\S]*width: 24px;[\s\S]*height: 24px;[\s\S]*\}/,
    );
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

  test('mobile chat drawer uses a cross-project project session sheet', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

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
    expect(mainTsx).not.toContain('title="Token stats"');
    expect(mainTsx).not.toContain('title="Agent info"');
    expect(mainTsx).not.toContain('className="chat-session-swipe-row');

    const mobileSheetStart = mainTsx.indexOf('const renderMobileChatSessionSheet = () => {');
    const mobileSheetEnd = mainTsx.indexOf('const renderSidebar = () => {', mobileSheetStart);
    expect(mobileSheetStart).toBeGreaterThanOrEqual(0);
    expect(mobileSheetEnd).toBeGreaterThan(mobileSheetStart);
    const mobileSheet = mainTsx.slice(mobileSheetStart, mobileSheetEnd);
    expect(mobileSheet).not.toContain('className="project-wrap"');
    expect(mobileSheet).toContain('renderProjectSessionActionStrip(targetProjectId, session.sessionId)');
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
    expect(mainTsx).toContain('className="wide-project-action-title"');
    expect(mainTsx).toContain("wideProjectActionMenu.kind === 'new' ? 'New Session' : 'Resume Session'");
    expect(mainTsx).toContain("const sessionAgent = (session.agentType || '').trim();");
    expect(mainTsx).toContain("tagVariantClass('wide-session-agent', sessionAgent)");
    expect(mainTsx).toContain('const [projectSessionActionMenu, setProjectSessionActionMenu] = useState<ProjectSessionActionMenuState | null>(null);');
    expect(mainTsx).toContain('const PROJECT_SESSION_LONG_PRESS_MS = 450;');
    expect(mainTsx).toContain('const handleDeleteProjectSession = async (targetProjectId: string, sessionId: string) => {');
    expect(mainTsx).toContain('const result = await service.deleteProjectSession(targetProjectId, normalizedSessionId);');
    expect(mainTsx).toContain('const handleReloadProjectSession = async (targetProjectId: string, sessionId: string) => {');
    expect(mainTsx).toContain('const result = await service.reloadProjectSession(targetProjectId, normalizedSessionId);');
    expect(mainTsx).toContain('const renderProjectSessionActionStrip = (targetProjectId: string, sessionId: string) => {');
    expect(mainTsx).toContain('className="project-session-action-strip"');
    expect(mainTsx).toContain('className="project-session-action-btn reload"');
    expect(mainTsx).toContain('className="project-session-action-btn delete"');
    expect(mainTsx).toContain('className="project-session-action-label">Reload</span>');
    expect(mainTsx).toContain('className="project-session-action-label">Delete</span>');
    expect(mainTsx).toContain("if (target?.closest('.project-session-action-btn')) {");
    expect(mainTsx).not.toContain("target?.closest('.project-session-row-wrap')");
    expect(mainTsx).toContain('renderProjectSessionActionStrip(targetProjectId, session.sessionId)');
    expect(mainTsx).toContain('onPointerDown={event => startProjectSessionLongPress(targetProjectId, session.sessionId, event)}');
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
    expect(stylesCss).toContain('.project-session-action-btn.delete {');
    expect(stylesCss).toContain('.project-session-action-label {');
    expect(stylesCss).toContain('.wide-session-agent-tag {');
    expect(stylesCss).toContain('.wide-session-agent-0 {');
    expect(stylesCss).toContain('.wide-session-time {');
    expect(stylesCss).toContain('.wide-project-action-popover {');
    expect(stylesCss).toMatch(/\.wide-project-row \{[^}]*min-height: 32px;[^}]*\}/);
    expect(stylesCss).toMatch(/\.wide-project-toggle \{[^}]*height: 30px;[^}]*\}/);
    expect(stylesCss).toMatch(/\.wide-session-row \{[^}]*min-height: 26px;[^}]*\}/);
    const wideSessionRowBlock = stylesCss.match(/\.wide-session-row \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(wideSessionRowBlock).toContain('grid-template-columns: 6px minmax(0, 1fr) auto auto;');
    expect(wideSessionRowBlock).toContain('gap: 5px;');
    const sessionStateMarkerBlock = stylesCss.match(/\.session-state-marker \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(sessionStateMarkerBlock).toContain('width: 6px;');
    expect(sessionStateMarkerBlock).toContain('flex: 0 0 6px;');
    expect(sessionStateMarkerBlock).toContain('transform: translateX(-1px);');
    const sessionStateRunningBlock = stylesCss.match(/\.session-state-marker\.running \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(sessionStateRunningBlock).toContain('font-size: 10px;');
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
    expect(stylesCss).toMatch(
      /\.project-session-row-wrap.actions-open \.wide-session-row::after \{[\s\S]*background: linear-gradient\([\s\S]*\}/,
    );
    expect(stylesCss).not.toContain('.project-session-row-wrap.actions-open .wide-session-row {');
    expect(stylesCss).toMatch(
      /\.project-session-row-wrap\.actions-open \.wide-session-row::after \{[^}]*width: min\(162px, 68%\);[^}]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.project-session-action-strip \{[^}]*top: 50%;[^}]*height: 30px;[^}]*transform: translateY\(-50%\);[^}]*width: min\(154px, 64%\);[^}]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.project-session-action-btn \{[^}]*height: 28px;[^}]*gap: 5px;[^}]*padding: 0 8px;[^}]*\}/,
    );
    expect(stylesCss).toMatch(
      /\.project-session-action-btn\.reload \{[^}]*background: color-mix\(in srgb, #2f9e44 18%, transparent\);[^}]*\}/,
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
