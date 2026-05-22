import {RegistryRepository} from '../web/src/services/registryRepository';
import type {RegistryClient} from '../web/src/services/registryClient';

describe('port relay registry service', () => {
  test('sends relay.status without project scope', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, enabled: false, status: 'Disabled'},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    const result = await repository.getPortRelayStatus();

    expect(result.status).toBe('Disabled');
    expect(client.request).toHaveBeenCalledWith({
      method: 'relay.status',
      payload: {},
    });
  });

  test('sends relay.enable with explicit hub target and access code', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {
          ok: true,
          enabled: true,
          status: 'Opening',
          listenPort: 28810,
          hubId: 'hub-a',
          targetHost: '127.0.0.1',
          targetPort: 12345,
          relayUrl: 'http://127.0.0.1:28810/',
          accessCodeGeneration: 2,
        },
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    const result = await repository.enablePortRelay({
      listenPort: 28810,
      hubId: 'hub-a',
      targetHost: '127.0.0.1',
      targetPort: 12345,
      accessCode: '483921',
    });

    expect(result.enabled).toBe(true);
    expect(client.request).toHaveBeenCalledWith({
      method: 'relay.enable',
      payload: {
        listenPort: 28810,
        hubId: 'hub-a',
        targetHost: '127.0.0.1',
        targetPort: 12345,
        accessCode: '483921',
      },
      timeoutMs: 15000,
    });
  });

  test('sends relay.disable and regenerateAccessCode', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, enabled: false, status: 'Disabled'},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.disablePortRelay();
    await repository.regeneratePortRelayAccessCode('111222');

    expect(client.request).toHaveBeenNthCalledWith(1, {
      method: 'relay.disable',
      payload: {},
    });
    expect(client.request).toHaveBeenNthCalledWith(2, {
      method: 'relay.regenerateAccessCode',
      payload: {accessCode: '111222'},
    });
  });
});
