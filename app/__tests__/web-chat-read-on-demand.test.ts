import fs from 'fs';
import path from 'path';

describe('web chat read-on-demand behavior', () => {
  test('connect and project switch only load session list; reconnect hydrates only when currently in chat with selected session', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );
    const workspaceStoreTs = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspaceStore.ts'),
      'utf8',
    );
    const workspacePersistenceTs = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'),
      'utf8',
    );

    expect(mainTsx).toContain('incremental?: boolean;');
    expect(mainTsx).toContain('forceFull?: boolean;');
    expect(mainTsx).toContain('const useIncremental = requestedIncremental && !fallbackToFullRead;');
    expect(mainTsx).toContain('const syncChatSessionsAfterReconnect = async (');
    expect(mainTsx).toContain('chatActiveRuntimeSetRef.current.keys()');
    expect(mainTsx).toContain('runtimeKeys.add(selectedRuntimeKey);');
    expect(mainTsx).toContain('syncChatSessionsAfterReconnect(preferredSelectedChatKey).catch(() => undefined);');
    expect(mainTsx).toContain('useIncremental ? checkpointTurnIndex : 0,');
    expect(mainTsx).toContain('applySessionReadResult(');
    expect(mainTsx).toContain('readProjectSessionWithStaleCacheRepair(');
    expect(mainTsx).toContain('isStaleSessionReadResult(');
    expect(mainTsx).toContain('clearProjectSessionCache(activeProjectId, sessionId);');
    expect(mainTsx).toContain('service.readProjectSession(activeProjectId, sessionId, 0);');
    expect(mainTsx).toContain("const chatVisibleRuntimeKeyRef = useRef('');");
    expect(mainTsx).toContain("const chatSelectedLoadAttemptRuntimeKeyRef = useRef('');");
    expect(mainTsx).toContain('resolveSelectedChatVisibilityRecovery({');
    expect(mainTsx).toContain("if (selectedVisibilityRecovery === 'restore-cache') {");
    expect(mainTsx).toContain("if (selectedVisibilityRecovery === 'read-session') {");
    expect(workspacePersistenceTs).toContain('selectedChatProjectId: string;');
    expect(workspacePersistenceTs).toContain('selectedChatSessionId: string;');
    expect(workspaceStoreTs).toContain('getSelectedChatSessionId(projectId: string): string {');
    expect(workspaceStoreTs).toContain('rememberSelectedChatSession(projectId: string, sessionId: string): void {');
    expect(workspaceStoreTs).toContain('getSelectedChatSessionKey(): ChatSessionKey | null {');
    expect(workspaceStoreTs).toContain('rememberSelectedChatSessionKey(key: ChatSessionKey | null): void {');
    expect(mainTsx).toContain('workspaceStore.getSelectedChatSessionId(activeProjectId)');
    expect(mainTsx).toContain('resolveChatListSelection({');
    expect(mainTsx).toContain('workspaceStore.rememberSelectedChatSessionKey(nextSelectedKey);');
    expect(mainTsx).toContain('loadChatSession(currentSelection, activeProjectId, {');
    expect(mainTsx).toContain('shouldApplyLoadedChatSelection(');
    const loadChatSessionStart = mainTsx.indexOf('const loadChatSession = async (');
    const loadChatSessionEnd = mainTsx.indexOf('const refreshSessionTurns = async (', loadChatSessionStart);
    const loadChatSessionBlock = mainTsx.slice(loadChatSessionStart, loadChatSessionEnd);
    expect(loadChatSessionBlock).toContain(
      'const canApplyLoadedSelection = shouldApplyLoadedChatSelection(',
    );
    expect(loadChatSessionBlock).toContain('if (canApplyLoadedSelection) {');
    expect(loadChatSessionBlock).toContain('return canApplyLoadedSelection;');
  });
});
