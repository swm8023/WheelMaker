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
    expect(mainTsx).toContain('const shouldSyncSelectedSession =');
    expect(mainTsx).toContain("tabRef.current === 'chat'");
    expect(mainTsx).toContain('incremental: true,');
    expect(mainTsx).toContain('preserveUserSelection: true,');
    expect(mainTsx).toContain('selectionSnapshot: preferredSelectedChatId,');
    expect(mainTsx).toContain('if (useIncremental) {');
    expect(workspacePersistenceTs).toContain('selectedChatSessionId: string;');
    expect(workspaceStoreTs).toContain('getSelectedChatSessionId(projectId: string): string {');
    expect(workspaceStoreTs).toContain('rememberSelectedChatSession(projectId: string, sessionId: string): void {');
    expect(mainTsx).toContain('workspaceStore.getSelectedChatSessionId(activeProjectId)');
    expect(mainTsx).toContain('nextSessions[0]?.sessionId ||');
    expect(mainTsx).toContain('workspaceStore.rememberSelectedChatSession(targetProjectId, sessionId);');
    expect(mainTsx).toContain('loadChatSession(currentSelection, activeProjectId, {');
  });
});
