import fs from 'fs';
import path from 'path';

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
    expect(repositoryTs).not.toContain("method: 'session.markRead'");
    expect(repositoryTs).not.toContain('turnId = typeof input.turnId');
    expect(repositoryTs).not.toContain("method: 'chat.permission.respond'");
    expect(workspaceServiceTs).toContain('async listSessions(');
    expect(workspaceServiceTs).toContain('async readSession(');
    expect(workspaceServiceTs).toContain('async createSession(');
    expect(workspaceServiceTs).toContain('async createSession(agentType: string, title?: string)');
    expect(workspaceServiceTs).toContain('async sendSessionMessage(');
    expect(workspaceServiceTs).not.toContain('async markSessionRead(');
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
    expect(mainTsx).not.toContain('await service.markSessionRead(');
    expect(mainTsx).toContain('chatSyncIndexRef');
    expect(mainTsx).not.toContain('chatPromptSnapshotVersion');
    expect(mainTsx).toContain('nextSessions.some(session => session.sessionId === currentSelection)');
    expect(mainTsx).not.toContain('result.lastIndex < afterIndex');
    expect(mainTsx).toContain('preserveUserSelection');
    expect(mainTsx).toContain('const shouldSyncSelectedSession =');
    expect(mainTsx).toContain('selectionSnapshot');
    expect(mainTsx).toContain('chatSelectedIdRef.current = resultSessionId');
    expect(mainTsx).toContain('sessionId');
    expect(mainTsx).toContain('const [newChatAgentPickerOpen, setNewChatAgentPickerOpen] = useState(false);');
    expect(mainTsx).toContain('const [pendingNewChatDraft, setPendingNewChatDraft] = useState<PendingNewChatDraft | null>(null);');
    expect(mainTsx).toContain('const resetChatComposer = () => {');
    expect(mainTsx).toContain("chatComposerTextRef.current = '';");
    expect(mainTsx).toContain('chatAttachmentsRef.current = [];');
    expect(mainTsx).toContain('bumpChatDraftGeneration(currentChatDraftKeyRef.current);');
    expect(mainTsx).toContain('const result = await service.createSession(normalizedAgentType, title);');
    expect(mainTsx).toContain('const completeNewChatFlow = async (agentType: string) => {');
    expect(mainTsx).toContain('project?.agents ?? []');
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
    expect(mainTsx).toContain('chatAutoScrollFollowRef.current = isChatScrolledNearBottom(container);');
    expect(mainTsx).toContain('const scrollChatToBottom = useCallback((force = false) => {');
    expect(mainTsx).toContain('if (!force && (!chatAutoScrollFollowRef.current || chatPointerScrollingRef.current)) {');
    expect(mainTsx).toContain('const forceChatScrollToBottom = useCallback(() => {');
    expect(mainTsx).toContain('chatAutoScrollFollowRef.current = true;');
    expect(mainTsx).toContain('scrollChatToBottom(true);');
    expect(mainTsx).toContain('useLayoutEffect(() => {');
    expect(mainTsx).toContain('resizeChatComposerTextarea();');
    expect(mainTsx).toContain('}, [resizeChatComposerTextarea, chatComposerText, tab, selectedChatId, currentChatDraftKey]);');
    expect(mainTsx).toContain('}, [tab, selectedChatId, chatMessages, chatLoading, chatKeyboardInset, resizeChatComposerTextarea, scrollChatToBottom]);');
    expect(mainTsx).toContain('onScroll={event => updateChatFollowModeFromScroll(event.currentTarget)}');
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
    expect(stylesCss).toContain('transform: translateY(calc(-100% + 6px));');
    expect(stylesCss).toContain('.chat-session-item');
    expect(stylesCss).toContain('.chat-attachment-preview-list {');
    expect(stylesCss).toContain('.chat-config-overflow-anchor {');
    expect(stylesCss).toContain('.chat-config-overflow-button {');
    expect(stylesCss).toContain('.chat-config-overflow-menu {');
    expect(mainTsx).not.toContain('className="status-bar"');
    expect(mainTsx).not.toContain('gitStatusSummary');
    expect(mainTsx).not.toContain('chat-thought-label');
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
    expect(mainTsx).toMatch(
      /\{isWide && sidebarSettingsOpen \? renderSettingsContent\(true\) : renderSidebarMain\(\)\}/,
    );
    expect(mainTsx).toContain('const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (');
    expect(mainTsx).toContain('className="mobile-settings-screen"');
    expect(mainTsx).toContain('aria-modal="true"');
    expect(mainTsx).toContain('className="mobile-settings-nav"');
    expect(mainTsx).toContain('className="mobile-settings-back"');
    expect(mainTsx).toContain('<div className="mobile-settings-title">Settings</div>');
    expect(mainTsx).toContain('className="mobile-settings-group"');
    expect(mainTsx).toMatch(
      /\{isWide \? \(\s*<div className="sidebar-footer">/,
    );
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
    expect(mainTsx).toContain('{!isWide && chatConfigOverflowOptions.length > 0 ? (');
    expect(mainTsx).toContain('className="chat-config-overflow-anchor"');
    expect(mainTsx).toContain('chat-config-overflow-button');
    expect(mainTsx).toContain('chat-config-overflow-menu');
    expect(mainTsx).toContain('className="codicon codicon-settings-gear"');
    expect(mainTsx).toContain('className="codicon codicon-chevron-down"');
    expect(mainTsx).not.toContain('project-menu-state');
    expect(mainTsx).not.toContain("projectItem.online ? 'online' : 'offline'");
    expect(mainTsx).not.toContain('+{chatConfigOverflowOptions.length}');
    expect(mainTsx).toContain('title="More config options"');
    expect(mainTsx).toContain('function chooseChatEntryText(previousText: string, nextText: string): string {');
    expect(mainTsx).toContain('text: chooseChatEntryText(previous.text, text),');
    expect(mainTsx).toContain("const shouldRefreshCompletedPrompt = message.method === 'prompt_done';");
    expect(mainTsx).toContain('const latestSyncCursor = getLatestSessionReadCursor(merged);');
    expect(mainTsx).toContain('const readCursorForGap = shouldRequestSessionReadForIncomingTurn(');
    expect(mainTsx).toContain('messages.filter(isFinishedChatMessage)');
    expect(mainTsx).toContain('needsPromptTurnRefresh(');
    expect(mainTsx).toContain('refreshSessionTurns(');
    expect(mainTsx).not.toContain('if (shouldRefreshCompletedPrompt && isSelectedSession) {\n          loadChatSession(sessionId, projectIdRef.current, {\n            forceFull: true,');
    expect(mainTsx).toContain('if (payload.session?.sessionId === chatSelectedIdRef.current) {');
    expect(mainTsx).toContain('loadChatSession(payload.session.sessionId, projectIdRef.current, {');
    expect(mainTsx).toContain("className={`header-btn refresh-btn${hasPendingProjectUpdates && !refreshingProject && !reconnecting ? ' has-update-badge' : ''}`}");
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
    expect(stylesCss).toContain('max-width: min(42%, 160px);');
    expect(mainTsx).toContain('chatAttachments.map(attachment => (');
    expect(mainTsx).toContain('onClick={() => removeChatAttachment(attachment.id)}');
    expect(mainTsx).toContain('disabled={chatSending || chatAttachmentReadPending}');
    expect(stylesCss).not.toContain('.project-presence {');
    expect(stylesCss).not.toContain('.project-dirty {');
    expect(stylesCss).not.toContain('.chat-permission-button');
  });
});
