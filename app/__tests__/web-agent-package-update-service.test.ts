import {RegistryRepository} from '../web/src/services/registryRepository';
import type {RegistryClient} from '../web/src/services/registryClient';

describe('agent package update registry service', () => {
  test('reads project.list hubs without depending on online state', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {
          projects: [{projectId: 'hub-b:app', name: 'app', online: true, path: '/app'}],
          hubs: [{hubId: 'hub-b', online: true}, {hubId: ' '}],
        },
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    const result = await repository.listProjectSnapshot();

    expect(result.projects).toEqual([
      expect.objectContaining({projectId: 'hub-b:app', hubId: 'hub-b'}),
    ]);
    expect(result.hubs).toEqual([{hubId: 'hub-b'}]);
    expect(client.request).toHaveBeenCalledWith({
      method: 'project.list',
      payload: {},
    });
  });

  test('sends cmd.npm scan with hubId and 60 second timeout', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {
          ok: true,
          updatedAt: '2026-05-19T10:00:00Z',
          hub: {hubId: 'hub-b', packages: []},
          operation: null,
        },
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    const result = await repository.scanNpmPackages('hub-b');

    expect(result.ok).toBe(true);
    expect(client.request).toHaveBeenCalledWith({
      method: 'cmd.npm',
      payload: {action: 'scan', hubId: 'hub-b'},
      timeoutMs: 60000,
    });
  });

  test('sends cmd.npm write actions with controlled payloads', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, accepted: true, operation: null},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.installNpmPackage('hub-a', '@openai/codex', 'latest');
    await repository.uninstallNpmPackage('hub-a', '@zed-industries/claude-agent-acp');

    expect(client.request).toHaveBeenNthCalledWith(1, {
      method: 'cmd.npm',
      payload: {
        action: 'install',
        hubId: 'hub-a',
        packageName: '@openai/codex',
        version: 'latest',
      },
    });
    expect(client.request).toHaveBeenNthCalledWith(2, {
      method: 'cmd.npm',
      payload: {
        action: 'uninstall',
        hubId: 'hub-a',
        packageName: '@zed-industries/claude-agent-acp',
      },
    });
    expect(client.request).toHaveBeenCalledTimes(2);
  });

  test('sends cmd.update query and update-publish with controlled payloads', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, status: 'update_pending', pendingSignal: true},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.queryWheelMakerUpdate('hub-a');
    await repository.requestWheelMakerUpdatePublish('hub-a');

    expect(client.request).toHaveBeenNthCalledWith(1, {
      method: 'cmd.update',
      payload: {action: 'query', hubId: 'hub-a'},
      timeoutMs: 60000,
    });
    expect(client.request).toHaveBeenNthCalledWith(2, {
      method: 'cmd.update',
      payload: {action: 'update-publish', hubId: 'hub-a'},
    });
  });
});
