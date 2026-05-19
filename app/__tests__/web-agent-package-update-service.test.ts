import {RegistryRepository} from '../web/src/services/registryRepository';
import type {RegistryClient} from '../web/src/services/registryClient';

describe('agent package update registry service', () => {
  test('sends cmd.npm scan with hubId and 60 second timeout', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {
          ok: true,
          updatedAt: '2026-05-19T10:00:00Z',
          hub: {hubId: 'hub-b', online: true, packages: []},
          task: null,
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

  test('sends cmd.npm write and query actions with controlled payloads', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, accepted: true, task: null},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.installNpmPackage('hub-a', '@openai/codex', 'latest');
    await repository.uninstallNpmPackage('hub-a', '@zed-industries/claude-agent-acp');
    await repository.queryNpmPackageTask('hub-a');

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
    expect(client.request).toHaveBeenNthCalledWith(3, {
      method: 'cmd.npm',
      payload: {action: 'query', hubId: 'hub-a'},
    });
  });
});
