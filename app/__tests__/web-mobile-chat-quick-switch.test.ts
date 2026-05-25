import {buildMobileChatQuickSwitchSections} from '../web/src/chat/mobileChatQuickSwitch';
import type {RegistryChatSession, RegistryProject} from '../web/src/types/registry';

function project(projectId: string, name = projectId): RegistryProject {
  return {
    projectId,
    name,
    online: true,
    path: `/${projectId}`,
  };
}

function session(
  sessionId: string,
  updatedAt: string,
  flags: Partial<Pick<RegistryChatSession, 'running' | 'unreadCount'>> = {},
): RegistryChatSession {
  return {
    sessionId,
    title: sessionId,
    preview: '',
    updatedAt,
    messageCount: 1,
    ...flags,
  };
}

describe('mobile chat quick switch', () => {
  test('selects up to five known sessions with unread and running sessions first', () => {
    const sections = buildMobileChatQuickSwitchSections({
      projects: [project('p1', 'Alpha'), project('p2', 'Beta'), project('p3', 'Gamma')],
      sessionsByProjectId: {
        p1: [
          session('p1-recent-read', '2026-05-05T00:00:00.000Z'),
          session('p1-unread', '2026-05-04T00:00:00.000Z', {unreadCount: 2}),
          session('p1-old-read', '2026-05-03T00:00:00.000Z'),
        ],
        p2: [
          session('p2-running', '2026-05-02T00:00:00.000Z', {running: true}),
          session('p2-read', '2026-05-01T00:00:00.000Z'),
        ],
        p3: [
          session('p3-unread', '2026-04-30T00:00:00.000Z', {unreadCount: 1}),
          session('p3-read', '2026-04-29T00:00:00.000Z'),
        ],
      },
    });

    expect(sections).toEqual([
      {
        projectId: 'p1',
        projectName: 'Alpha',
        sessions: [
          session('p1-recent-read', '2026-05-05T00:00:00.000Z'),
          session('p1-unread', '2026-05-04T00:00:00.000Z', {unreadCount: 2}),
          session('p1-old-read', '2026-05-03T00:00:00.000Z'),
        ],
      },
      {
        projectId: 'p2',
        projectName: 'Beta',
        sessions: [
          session('p2-running', '2026-05-02T00:00:00.000Z', {running: true}),
        ],
      },
      {
        projectId: 'p3',
        projectName: 'Gamma',
        sessions: [
          session('p3-unread', '2026-04-30T00:00:00.000Z', {unreadCount: 1}),
        ],
      },
    ]);
  });

  test('returns an empty section list when no known sessions exist', () => {
    expect(buildMobileChatQuickSwitchSections({
      projects: [project('p1')],
      sessionsByProjectId: {p1: []},
    })).toEqual([]);
  });
});
