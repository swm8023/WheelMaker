import {
  classifyChatSessionEvent,
  createChatIndexState,
  finishChatIndexFullRefresh,
  finishChatIndexProjectRefresh,
  mergeChatIndexSession,
  requestChatIndexFullRefresh,
  requestChatIndexProjectRefresh,
  sortChatIndexProjects,
} from '../web/src/chat/chatIndexState';
import type { RegistryChatSession, RegistryProject } from '../web/src/types/registry';

function project(projectId: string, name: string): RegistryProject {
  return {
    projectId,
    name,
    online: true,
    path: `/${projectId}`,
  };
}

function session(sessionId: string, updatedAt: string): RegistryChatSession {
  return {
    sessionId,
    title: sessionId,
    preview: '',
    updatedAt,
    messageCount: 1,
  };
}

describe('chat index state helpers', () => {
  test('sorts projects by pinned, recent chat activity, then name', () => {
    const projects = [
      project('p-old', 'Beta'),
      project('p-new', 'Alpha'),
      project('p-pinned', 'Pinned'),
      project('p-empty', 'Aardvark'),
    ];
    const sorted = sortChatIndexProjects(
      projects,
      {
        'p-old': [session('old', '2026-01-01T00:00:00.000Z')],
        'p-new': [session('new', '2026-05-01T00:00:00.000Z')],
        'p-pinned': [session('pinned', '2025-01-01T00:00:00.000Z')],
      },
      ['p-pinned'],
    );

    expect(sorted.map(item => item.projectId)).toEqual([
      'p-pinned',
      'p-new',
      'p-old',
      'p-empty',
    ]);
  });

  test('patches or inserts session summaries for a known project', () => {
    const state = {
      ...createChatIndexState(),
      projects: [project('p1', 'Project 1')],
    };
    const inserted = mergeChatIndexSession(state, 'p1', session('s1', '2026-01-01T00:00:00.000Z'));
    const patched = mergeChatIndexSession(inserted, 'p1', {
      sessionId: 's1',
      preview: 'new preview',
      updatedAt: '2026-01-02T00:00:00.000Z',
    });

    expect(inserted.sessionsByProjectId.p1[0].sessionId).toBe('s1');
    expect(patched.sessionsByProjectId.p1[0]).toMatchObject({
      sessionId: 's1',
      title: 's1',
      preview: 'new preview',
      updatedAt: '2026-01-02T00:00:00.000Z',
    });
  });

  test('classifies session events without falling back to workspace project', () => {
    const state = mergeChatIndexSession(
      {
        ...createChatIndexState(),
        projects: [project('p1', 'Project 1')],
      },
      'p1',
      session('known', '2026-01-01T00:00:00.000Z'),
    );

    expect(classifyChatSessionEvent(state, {
      method: 'session.updated',
      projectId: 'p1',
      payload: { session: session('new', '2026-01-02T00:00:00.000Z') },
    })).toEqual({
      kind: 'patch',
      projectId: 'p1',
      session: session('new', '2026-01-02T00:00:00.000Z'),
    });

    expect(classifyChatSessionEvent(state, {
      method: 'session.message',
      projectId: 'p1',
      payload: { sessionId: 'unknown', turnIndex: 1, content: 'hello' },
    })).toEqual({ kind: 'refreshProject', projectId: 'p1' });

    expect(classifyChatSessionEvent(state, {
      method: 'session.updated',
      projectId: 'missing',
      payload: { session: session('new', '2026-01-02T00:00:00.000Z') },
    })).toEqual({ kind: 'refreshAll' });

    expect(classifyChatSessionEvent(state, {
      method: 'session.updated',
      payload: { session: session('new', '2026-01-02T00:00:00.000Z') },
    })).toEqual({ kind: 'ignore' });
  });

  test('coalesces full and project refresh requests', () => {
    let state = createChatIndexState();

    state = requestChatIndexFullRefresh(state);
    state = requestChatIndexFullRefresh(state);
    expect(state.refresh.fullRefreshInFlight).toBe(true);
    expect(state.refresh.fullRefreshDirty).toBe(true);

    state = finishChatIndexFullRefresh(state);
    expect(state.refresh.fullRefreshInFlight).toBe(true);
    expect(state.refresh.fullRefreshDirty).toBe(false);

    state = finishChatIndexFullRefresh(state);
    expect(state.refresh.fullRefreshInFlight).toBe(false);

    state = requestChatIndexProjectRefresh(state, 'p1');
    state = requestChatIndexProjectRefresh(state, 'p1');
    expect(state.refresh.projectRefreshInFlight.p1).toBe(true);
    expect(state.refresh.projectRefreshDirty.p1).toBe(true);

    state = finishChatIndexProjectRefresh(state, 'p1', 'failed');
    expect(state.refresh.projectRefreshInFlight.p1).toBe(true);
    expect(state.refresh.projectRefreshDirty.p1).toBe(false);
    expect(state.refresh.projectErrors.p1).toBe('failed');

    state = finishChatIndexProjectRefresh(state, 'p1');
    expect(state.refresh.projectRefreshInFlight.p1).toBe(false);
    expect(state.refresh.projectErrors.p1).toBeUndefined();
  });
});
