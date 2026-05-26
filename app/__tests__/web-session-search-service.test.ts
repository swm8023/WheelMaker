import {RegistryRepository} from '../web/src/services/registryRepository';
import type {RegistryClient} from '../web/src/services/registryClient';

describe('session search registry service', () => {
  test('sends start query and cancel actions through session.search', async () => {
    const client = {
      request: jest.fn(async (args: {payload?: {action?: string}}) => {
        if (args.payload?.action === 'query') {
          return {
            type: 'response',
            payload: {
              searchId: 'search-1',
              done: true,
              results: [
                {projectId: 'proj1', sessionId: 'sess-title', source: 'title', turnIndex: 99},
                {projectId: 'proj1', sessionId: 'sess-prompt', source: 'prompt', turnIndex: 7},
                {projectId: 'proj1', sessionId: '', source: 'prompt', turnIndex: 8},
                {projectId: 'proj1', sessionId: 'bad-source', source: 'preview', turnIndex: 9},
              ],
              errors: [
                {projectId: 'proj1', sessionId: 'sess-bad', message: 'read failed'},
                {projectId: '', message: 'ignored'},
              ],
            },
          };
        }
        return {
          type: 'response',
          payload: {
            searchId: 'search-1',
            done: args.payload?.action === 'cancel',
          },
        };
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    const start = await repository.startSessionSearch('proj1', 'search-1', 'Deploy');
    const query = await repository.querySessionSearch('proj1', 'search-1');
    const cancel = await repository.cancelSessionSearch('proj1', 'search-1');

    expect(start).toEqual({searchId: 'search-1', done: false});
    expect(query).toEqual({
      searchId: 'search-1',
      done: true,
      results: [
        {projectId: 'proj1', sessionId: 'sess-title', source: 'title'},
        {projectId: 'proj1', sessionId: 'sess-prompt', source: 'prompt', turnIndex: 7},
      ],
      errors: [
        {projectId: 'proj1', sessionId: 'sess-bad', message: 'read failed'},
      ],
    });
    expect(cancel).toEqual({searchId: 'search-1', done: true});
    expect(client.request).toHaveBeenNthCalledWith(1, {
      method: 'session.search',
      projectId: 'proj1',
      payload: {action: 'start', searchId: 'search-1', query: 'Deploy'},
      timeoutMs: 15000,
    });
    expect(client.request).toHaveBeenNthCalledWith(2, {
      method: 'session.search',
      projectId: 'proj1',
      payload: {action: 'query', searchId: 'search-1'},
      timeoutMs: 15000,
    });
    expect(client.request).toHaveBeenNthCalledWith(3, {
      method: 'session.search',
      projectId: 'proj1',
      payload: {action: 'cancel', searchId: 'search-1'},
      timeoutMs: 15000,
    });
  });
});
