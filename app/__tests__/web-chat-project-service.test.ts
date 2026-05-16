import { RegistryWorkspaceService } from '../web/src/services/registryWorkspaceService';

describe('registry workspace project-scoped chat service methods', () => {
  test('delegates read/send/config to the explicitly selected chat project', async () => {
    const service = new RegistryWorkspaceService();
    const repository = {
      readSession: jest.fn().mockResolvedValue({ messages: [], latestTurnIndex: 0 }),
      sendSessionMessage: jest.fn().mockResolvedValue({ ok: true, sessionId: 's1' }),
      setSessionConfig: jest.fn().mockResolvedValue({ ok: true, sessionId: 's1', configOptions: [] }),
    };

    Object.assign(service as unknown as { repository: unknown; session: unknown }, {
      repository,
      session: {
        projects: [],
        selectedProjectId: 'workspace-project',
        fileEntries: [],
      },
    });

    await (service as any).readProjectSession('chat-project', 's1', 7);
    await (service as any).sendProjectSessionMessage('chat-project', {
      sessionId: 's1',
      text: 'hello',
    });
    await (service as any).setProjectSessionConfig('chat-project', {
      sessionId: 's1',
      configId: 'model',
      value: 'x',
    });

    expect(repository.readSession).toHaveBeenCalledWith('chat-project', 's1', 7);
    expect(repository.sendSessionMessage).toHaveBeenCalledWith('chat-project', {
      sessionId: 's1',
      text: 'hello',
    });
    expect(repository.setSessionConfig).toHaveBeenCalledWith('chat-project', {
      sessionId: 's1',
      configId: 'model',
      value: 'x',
    });
    expect(repository.readSession).not.toHaveBeenCalledWith('workspace-project', 's1', 7);
  });
});
