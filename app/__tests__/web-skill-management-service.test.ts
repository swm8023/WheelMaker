import {RegistryRepository} from '../web/src/services/registryRepository';
import type {RegistryClient} from '../web/src/services/registryClient';

describe('skill management registry service', () => {
  test('sends cmd.skills scan with hubId and bounded timeout', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, hubId: 'hub-a', hubSkills: {scope: 'hub', skills: []}, projects: []},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.scanSkills('hub-a');

    expect(client.request).toHaveBeenCalledWith({
      method: 'cmd.skills',
      payload: {action: 'scan', hubId: 'hub-a'},
      timeoutMs: 60000,
    });
  });

  test('sends cmd.skills source list with controlled source payload', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, hubId: 'hub-a', source: 'mattpocock/skills', candidates: []},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.listSkillsSource('hub-a', 'mattpocock/skills');

    expect(client.request).toHaveBeenCalledWith({
      method: 'cmd.skills',
      payload: {action: 'list', hubId: 'hub-a', source: 'mattpocock/skills'},
      timeoutMs: 60000,
    });
  });

  test('sends install uninstall and update without paths or raw args', async () => {
    const client = {
      request: jest.fn().mockResolvedValue({
        type: 'response',
        payload: {ok: true, hubId: 'hub-a', skills: []},
      }),
    } as unknown as RegistryClient;
    const repository = new RegistryRepository(client);

    await repository.installSkills({
      hubId: 'hub-a',
      scope: 'project',
      projectName: 'WheelMaker',
      source: 'mattpocock/skills',
      skills: ['tdd'],
    });
    await repository.uninstallSkills({hubId: 'hub-a', scope: 'hub', skills: ['tdd']});
    await repository.updateSkills({hubId: 'hub-a', scope: 'project', projectName: 'WheelMaker'});
    await repository.updateSkills({hubId: 'hub-a', scope: 'hub', includeProjects: true});

    expect(client.request).toHaveBeenNthCalledWith(1, {
      method: 'cmd.skills',
      payload: {
        action: 'install',
        hubId: 'hub-a',
        scope: 'project',
        projectName: 'WheelMaker',
        source: 'mattpocock/skills',
        skills: ['tdd'],
      },
      timeoutMs: 60000,
    });
    expect(client.request).toHaveBeenNthCalledWith(2, {
      method: 'cmd.skills',
      payload: {action: 'uninstall', hubId: 'hub-a', scope: 'hub', skills: ['tdd']},
      timeoutMs: 60000,
    });
    expect(client.request).toHaveBeenNthCalledWith(3, {
      method: 'cmd.skills',
      payload: {action: 'update', hubId: 'hub-a', scope: 'project', projectName: 'WheelMaker'},
      timeoutMs: 60000,
    });
    expect(client.request).toHaveBeenNthCalledWith(4, {
      method: 'cmd.skills',
      payload: {action: 'update', hubId: 'hub-a', scope: 'hub', includeProjects: true},
      timeoutMs: 60000,
    });
  });
});
