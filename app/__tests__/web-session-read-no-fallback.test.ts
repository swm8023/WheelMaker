import {RegistryRepository} from '../web/src/services/registryRepository';
import type {RegistryClient} from '../web/src/services/registryClient';

describe('registry session.read', () => {
  test('does not synthesize a fallback session when server omits session metadata', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {
          latestTurnIndex: 7,
          turns: [
            {
              turnIndex: 4,
              content: JSON.stringify({method: 'agent_message_chunk', param: {text: 'missing session id'}}),
              finished: true,
            },
          ],
        },
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    const result = await repository.readSession('project-1', 'sess-1', 3);

    expect(result.session).toBeUndefined();
    expect(result.messages).toEqual([]);
    expect(result.latestTurnIndex).toBe(7);
  });
});
