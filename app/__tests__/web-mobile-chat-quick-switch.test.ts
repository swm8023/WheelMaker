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
  test('selects six sessions with unread and running first, then newest updates', () => {
    const sections = buildMobileChatQuickSwitchSections({
      projects: [project('p1', 'Alpha'), project('p2', 'Beta'), project('p3', 'Gamma')],
      sessionsByProjectId: {
        p1: [
          session('p1-old-a', '2026-01-01T00:00:00.000Z'),
          session('p1-old-b', '2026-01-02T00:00:00.000Z'),
          session('p1-mid', '2026-05-04T00:00:00.000Z'),
        ],
        p2: [
          session('p2-newest', '2026-05-08T00:00:00.000Z'),
          session('p2-running-old', '2026-01-03T00:00:00.000Z', {running: true}),
          session('p2-new', '2026-05-07T00:00:00.000Z'),
        ],
        p3: [
          session('p3-unread-old', '2026-01-04T00:00:00.000Z', {unreadCount: 1}),
          session('p3-recent', '2026-05-06T00:00:00.000Z'),
          session('p3-newer', '2026-05-09T00:00:00.000Z'),
          session('p3-old', '2026-01-05T00:00:00.000Z'),
        ],
      },
    });

    expect(sections.flatMap(section =>
      section.sessions.map(item => `${section.projectId}:${item.sessionId}`),
    )).toEqual([
      'p3:p3-unread-old',
      'p3:p3-newer',
      'p3:p3-recent',
      'p2:p2-running-old',
      'p2:p2-newest',
      'p2:p2-new',
    ]);
    expect(sections.flatMap(section => section.sessions)).toHaveLength(6);
  });

  test('returns an empty section list when no known sessions exist', () => {
    expect(buildMobileChatQuickSwitchSections({
      projects: [project('p1')],
      sessionsByProjectId: {p1: []},
    })).toEqual([]);
  });
});
