jest.mock('../web/src/services/shikiRenderer', () => ({
  DEFAULT_CODE_FONT: 'jetbrains',
  DEFAULT_CODE_FONT_SIZE: 13,
  DEFAULT_CODE_LINE_HEIGHT: 1.5,
  DEFAULT_CODE_TAB_SIZE: 2,
  DEFAULT_CODE_THEME: 'dark-plus',
  isCodeFontId: () => true,
  isCodeThemeId: () => true,
}));

import { WorkspaceStore } from '../web/src/services/workspaceStore';
import { reconcilePersistedChatSessionCache } from '../web/src/services/workspacePersistence';

function createFakePersistence(globalPatch: Record<string, unknown> = {}) {
  const globalState = {
    selectedChatProjectId: '',
    selectedChatSessionId: '',
    ...globalPatch,
  };
  const projectStates: Record<string, { selectedChatSessionId: string }> = {};
  return {
    globalState,
    projectStates,
    patchGlobalState: jest.fn((patch: Record<string, unknown>) => {
      Object.assign(globalState, patch);
    }),
    getGlobalState: jest.fn(() => globalState),
    getProjectState: jest.fn((projectId: string) => projectStates[projectId] ?? { selectedChatSessionId: '' }),
  };
}

function createFakeChatPersistence() {
  let entries: Array<{ session: any; cursor: any }> = [];
  return {
    getProjectChatSessions: jest.fn(() => entries),
    replaceProjectChatSessions: jest.fn((_projectId: string, nextEntries: Array<{ session: any; cursor: any }>) => {
      entries = nextEntries;
    }),
    patchProjectChatSession: jest.fn((_projectId: string, session: any, cursor: any) => {
      entries = [
        ...entries.filter(entry => entry.session.sessionId !== session.sessionId),
        { session, cursor },
      ];
    }),
  };
}

describe('global selected chat session persistence', () => {
  test('clears the saved registry token without clearing the address', () => {
    const persistence = createFakePersistence({
      address: 'ws://registry.example/ws',
      token: 'secret-token',
    });
    const store = new WorkspaceStore(persistence as any);

    store.clearLocalToken();

    expect(persistence.patchGlobalState).toHaveBeenCalledWith({token: ''});
    expect(persistence.globalState.address).toBe('ws://registry.example/ws');
    expect(persistence.globalState.token).toBe('');
  });

  test('remembers and restores one global project-scoped chat key', () => {
    const persistence = createFakePersistence();
    const store = new WorkspaceStore(persistence as any);

    (store as any).rememberSelectedChatSessionKey({
      projectId: 'p1',
      sessionId: 's1',
    });

    expect(persistence.patchGlobalState).toHaveBeenCalledWith({
      selectedChatProjectId: 'p1',
      selectedChatSessionId: 's1',
    });
    expect((store as any).getSelectedChatSessionKey()).toEqual({
      projectId: 'p1',
      sessionId: 's1',
    });
  });

  test('migrates from project selected session only when no global key exists', () => {
    const persistence = createFakePersistence();
    persistence.projectStates.p1 = { selectedChatSessionId: 'legacy-session' };
    const store = new WorkspaceStore(persistence as any);

    expect((store as any).migrateSelectedChatSessionKey('p1')).toEqual({
      projectId: 'p1',
      sessionId: 'legacy-session',
    });
    expect(persistence.patchGlobalState).toHaveBeenCalledWith({
      selectedChatProjectId: 'p1',
      selectedChatSessionId: 'legacy-session',
    });
  });

  test('does not let legacy per-project selection override an existing global key', () => {
    const persistence = createFakePersistence({
      selectedChatProjectId: 'global-project',
      selectedChatSessionId: 'global-session',
    });
    persistence.projectStates.p1 = { selectedChatSessionId: 'legacy-session' };
    const store = new WorkspaceStore(persistence as any);

    expect((store as any).migrateSelectedChatSessionKey('p1')).toEqual({
      projectId: 'global-project',
      sessionId: 'global-session',
    });
    expect(persistence.patchGlobalState).not.toHaveBeenCalled();
  });
});

describe('chat session index persistence', () => {
  test('resets persisted turn cache when session summary latest is behind local cursor', () => {
    const repaired = reconcilePersistedChatSessionCache(
      {
        sessionId: 's1',
        title: 'Session',
        preview: '',
        updatedAt: '2026-05-19T12:48:22.000Z',
        messageCount: 728,
        latestTurnIndex: 728,
      },
      { turnIndex: 1000 },
      [1, 2, 3].map(index => ({
        turnIndex: index,
        content: JSON.stringify({method: 'agent_message_chunk', param: {text: `turn-${index}`}}),
        finished: true,
      })),
    );

    expect(repaired).toEqual({
      cursor: { turnIndex: 0 },
      stale: true,
      turns: [],
    });
  });

  test('preserves config options and commands when patching with a summary that omits them', () => {
    const persistence = createFakeChatPersistence();
    const store = new WorkspaceStore(persistence as any);

    store.replaceChatSessions('p1', [
      {
        sessionId: 's1',
        title: 'Session',
        preview: '',
        updatedAt: '2026-01-01T00:00:00.000Z',
        messageCount: 1,
        configOptions: [{ id: 'model', currentValue: 'gpt-5.3-codex' }],
        commands: [{ name: '/plan' }],
      },
    ], { s1: { turnIndex: 3 } });

    store.rememberChatSession('p1', {
      sessionId: 's1',
      title: 'Session',
      preview: 'updated',
      updatedAt: '2026-01-02T00:00:00.000Z',
      messageCount: 2,
    }, { turnIndex: 4 });

    expect(persistence.patchProjectChatSession).toHaveBeenLastCalledWith(
      'p1',
      expect.objectContaining({
        sessionId: 's1',
        preview: 'updated',
        configOptions: [{ id: 'model', currentValue: 'gpt-5.3-codex' }],
        commands: [{ name: '/plan' }],
      }),
      { turnIndex: 4 },
    );
  });

  test('preserves config options and commands when replacing with summaries that omit them', () => {
    const persistence = createFakeChatPersistence();
    const store = new WorkspaceStore(persistence as any);

    store.replaceChatSessions('p1', [
      {
        sessionId: 's1',
        title: 'Session',
        preview: '',
        updatedAt: '2026-01-01T00:00:00.000Z',
        messageCount: 1,
        configOptions: [{ id: 'mode', currentValue: 'code' }],
        commands: [{ name: '/status' }],
      },
    ], { s1: { turnIndex: 3 } });

    store.replaceChatSessions('p1', [
      {
        sessionId: 's1',
        title: 'Session',
        preview: 'listed',
        updatedAt: '2026-01-03T00:00:00.000Z',
        messageCount: 3,
      },
    ], { s1: { turnIndex: 5 } });

    expect(persistence.replaceProjectChatSessions).toHaveBeenLastCalledWith(
      'p1',
      [
        {
          session: expect.objectContaining({
            sessionId: 's1',
            preview: 'listed',
            configOptions: [{ id: 'mode', currentValue: 'code' }],
            commands: [{ name: '/status' }],
          }),
          cursor: { turnIndex: 5 },
        },
      ],
    );
  });
});
