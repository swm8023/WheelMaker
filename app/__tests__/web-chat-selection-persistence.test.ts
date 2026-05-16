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

describe('global selected chat session persistence', () => {
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
