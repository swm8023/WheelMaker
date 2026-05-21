import {
  createRegistryDebugStore,
  extractRegistryDebugSessionIds,
  filterRegistryDebugRecords,
  formatRegistryDebugTime,
  resolveRegistryDebugScope,
} from '../web/src/debug/registryDebug';

describe('registry debug records', () => {
  test('extracts session ids from common registry payload shapes', () => {
    const ids = extractRegistryDebugSessionIds({
      sessionId: 'top-level',
      payload: {
        sessionId: 'payload-session',
        session: {sessionId: 'summary-session'},
        turn: {sessionId: 'turn-session'},
        sessions: [
          {sessionId: 'list-a'},
          {sessionId: 'list-b'},
          {sessionId: 'list-a'},
        ],
      },
    });

    expect(ids).toEqual([
      'top-level',
      'payload-session',
      'summary-session',
      'turn-session',
      'list-a',
      'list-b',
    ]);
  });

  test('formats local time at millisecond precision', () => {
    const timestamp = new Date(2026, 4, 19, 1, 2, 3, 4).getTime();

    expect(formatRegistryDebugTime(timestamp)).toBe('01:02:03.004');
  });

  test('resolves method families into debug scopes', () => {
    expect(resolveRegistryDebugScope('session.send', 'request')).toBe('session.*');
    expect(resolveRegistryDebugScope('fs.read', 'response')).toBe('fs.*');
    expect(resolveRegistryDebugScope('git.status', 'event')).toBe('git.*');
    expect(resolveRegistryDebugScope('project.list', 'request')).toBe('project.*');
    expect(resolveRegistryDebugScope('token.scan', 'response')).toBe('token.*');
    expect(resolveRegistryDebugScope('ping', 'request')).toBe('ping');
    expect(resolveRegistryDebugScope(undefined, 'connect_open')).toBe('lifecycle');
    expect(resolveRegistryDebugScope(undefined, 'parse_error')).toBe('parse_error');
    expect(resolveRegistryDebugScope(undefined, 'event')).toBe('unknown');
  });

  test('records, correlates, filters, and clears debug entries', () => {
    const store = createRegistryDebugStore(() => 1000);
    store.setEnabled(true);

    store.recordOutbound({
      envelope: {
        requestId: 7,
        type: 'request',
        method: 'session.send',
        projectId: 'project-a',
        payload: {sessionId: 'sess-a', text: 'hello'},
      },
      raw: '{"requestId":7}',
    });
    store.recordInboundEnvelope({
      envelope: {
        requestId: 7,
        type: 'response',
        payload: {ok: true},
      },
      raw: '{"requestId":7,"type":"response"}',
      timestamp: 1123,
    });
    store.recordInboundEnvelope({
      envelope: {
        type: 'event',
        method: 'session.list',
        projectId: 'project-a',
        payload: {
          sessions: [
            {sessionId: 'sess-a'},
            {sessionId: 'sess-b'},
          ],
        },
      },
      raw: '{"type":"event"}',
      timestamp: 1200,
    });

    const records = store.getRecords();
    expect(records).toHaveLength(3);
    expect(records[0]).toMatchObject({
      direction: 'out',
      phase: 'request',
      scope: 'session.*',
      connection: 'Remote',
      method: 'session.send',
      requestId: 7,
      projectId: 'project-a',
      sessionIds: ['sess-a'],
      raw: '{"requestId":7}',
    });
    expect(records[1]).toMatchObject({
      direction: 'in',
      phase: 'response',
      scope: 'session.*',
      connection: 'Remote',
      method: 'session.send',
      requestId: 7,
      projectId: 'project-a',
      sessionIds: ['sess-a'],
      durationMs: 123,
    });
    expect(records[2]).toMatchObject({
      phase: 'event',
      scope: 'session.*',
      connection: 'Remote',
      sessionIds: ['sess-a', 'sess-b'],
      multiSession: true,
    });

    store.recordOutbound({
      envelope: {
        requestId: 8,
        type: 'request',
        method: 'fs.read',
        projectId: 'project-a',
        payload: {path: 'README.md'},
      },
      raw: '{"requestId":8}',
      connection: 'Local',
      timestamp: 1250,
    });
    const recordsWithFs = store.getRecords();
    expect(recordsWithFs[3]).toMatchObject({
      scope: 'fs.*',
      connection: 'Local',
      method: 'fs.read',
      sessionIds: [],
    });

    expect(filterRegistryDebugRecords(recordsWithFs, {
      selectedScope: 'session.*',
      selectedSessionId: 'sess-a',
      includeMultiSessionRecords: false,
    }).map(record => record.id)).toEqual([
      recordsWithFs[0].id,
      recordsWithFs[1].id,
    ]);
    expect(filterRegistryDebugRecords(recordsWithFs, {
      selectedScope: 'session.*',
      selectedSessionId: 'sess-a',
      includeMultiSessionRecords: true,
    }).map(record => record.id)).toEqual([
      recordsWithFs[0].id,
      recordsWithFs[1].id,
      recordsWithFs[2].id,
    ]);
    expect(filterRegistryDebugRecords(recordsWithFs, {
      selectedScope: 'fs.*',
      selectedSessionId: 'All',
      includeMultiSessionRecords: false,
    }).map(record => record.id)).toEqual([recordsWithFs[3].id]);
    expect(filterRegistryDebugRecords(recordsWithFs, {
      selectedScope: 'All',
      selectedSessionId: 'All',
      includeMultiSessionRecords: false,
    })).toHaveLength(4);

    store.clear();
    expect(store.getRecords()).toEqual([]);

    store.recordInboundEnvelope({
      envelope: {
        requestId: 7,
        type: 'response',
        payload: {ok: true},
      },
      raw: '{}',
      timestamp: 1300,
    });
    expect(store.getRecords()[0].sessionIds).toEqual([]);
  });

  test('does not record while disabled and skips connect init while enabled', () => {
    const store = createRegistryDebugStore(() => 1000);

    store.recordOutbound({
      envelope: {
        requestId: 1,
        type: 'request',
        method: 'session.list',
        payload: {},
      },
      raw: '{}',
    });
    expect(store.getRecords()).toEqual([]);

    store.setEnabled(true);
    store.recordOutbound({
      envelope: {
        requestId: 2,
        type: 'request',
        method: 'connect.init',
        payload: {token: 'secret-token'},
      },
      raw: '{"method":"connect.init"}',
    });
    expect(store.getRecords()).toEqual([]);
  });
});
