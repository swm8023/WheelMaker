import {
  buildSessionSearchSections,
  mergeSessionSearchResultsByProject,
  resolveSessionSearchPollDelay,
  splitSessionSearchTitleHighlight,
} from '../web/src/chat/sessionSearchState';
import type {RegistryChatSession, RegistryProject, RegistrySessionSearchResult} from '../web/src/types/registry';

function session(sessionId: string, title: string): RegistryChatSession {
  return {
    sessionId,
    title,
    preview: '',
    updatedAt: '',
    messageCount: 0,
  };
}

const projects: RegistryProject[] = [
  {projectId: 'p1', name: 'One', online: true, path: '/one'},
  {projectId: 'p2', name: 'Two', online: true, path: '/two'},
];

describe('session search state helpers', () => {
  test('backs off polling after three unchanged query responses', () => {
    expect(resolveSessionSearchPollDelay({changed: true, unchangedPolls: 0})).toBe(300);
    expect(resolveSessionSearchPollDelay({changed: false, unchangedPolls: 2})).toBe(300);
    expect(resolveSessionSearchPollDelay({changed: false, unchangedPolls: 3})).toBe(800);
  });

  test('replaces full per-project result sets and reports visible changes', () => {
    const first: RegistrySessionSearchResult[] = [
      {projectId: 'p1', sessionId: 's1', source: 'title'},
    ];
    const second: RegistrySessionSearchResult[] = [
      {projectId: 'p1', sessionId: 's2', source: 'prompt', turnIndex: 7},
    ];

    const afterFirst = mergeSessionSearchResultsByProject({}, 'p1', first);
    const afterSame = mergeSessionSearchResultsByProject(afterFirst.resultsByProjectId, 'p1', first);
    const afterSecond = mergeSessionSearchResultsByProject(afterFirst.resultsByProjectId, 'p1', second);

    expect(afterFirst.changed).toBe(true);
    expect(afterSame.changed).toBe(false);
    expect(afterSecond.changed).toBe(true);
    expect(afterSecond.resultsByProjectId.p1).toEqual(second);
  });

  test('builds sections in normal project and session order', () => {
    const sections = buildSessionSearchSections({
      projects,
      sessionsByProjectId: {
        p1: [session('s2', 'Second'), session('s1', 'First')],
        p2: [session('s3', 'Third')],
      },
      resultsByProjectId: {
        p1: [
          {projectId: 'p1', sessionId: 's1', source: 'title'},
          {projectId: 'p1', sessionId: 's2', source: 'prompt', turnIndex: 4},
        ],
        p2: [],
      },
    });

    expect(sections).toHaveLength(1);
    expect(sections[0].project.projectId).toBe('p1');
    expect(sections[0].rows.map(row => row.session.sessionId)).toEqual(['s2', 's1']);
    expect(sections[0].rows[0].result).toEqual({projectId: 'p1', sessionId: 's2', source: 'prompt', turnIndex: 4});
  });

  test('splits title highlight segments case-insensitively', () => {
    expect(splitSessionSearchTitleHighlight('Deploy deployer', 'dep')).toEqual([
      {text: 'Dep', match: true},
      {text: 'loy ', match: false},
      {text: 'dep', match: true},
      {text: 'loyer', match: false},
    ]);
  });
});
