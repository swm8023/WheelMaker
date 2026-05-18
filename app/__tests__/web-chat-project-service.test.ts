import { RegistryWorkspaceService } from '../web/src/services/registryWorkspaceService';

describe('registry workspace project-scoped chat service methods', () => {
  test('delegates read/send/config to the explicitly selected chat project', async () => {
    const service = new RegistryWorkspaceService();
    const repository = {
      readSession: jest.fn().mockResolvedValue({ messages: [], latestTurnIndex: 0 }),
      sendSessionMessage: jest.fn().mockResolvedValue({ ok: true, sessionId: 's1' }),
      cancelSession: jest.fn().mockResolvedValue({ ok: true, sessionId: 's1' }),
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
    await (service as any).cancelProjectSession('chat-project', 's1');
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
    expect(repository.cancelSession).toHaveBeenCalledWith('chat-project', 's1');
    expect(repository.setSessionConfig).toHaveBeenCalledWith('chat-project', {
      sessionId: 's1',
      configId: 'model',
      value: 'x',
    });
    expect(repository.readSession).not.toHaveBeenCalledWith('workspace-project', 's1', 7);
  });

  test('lightweight project selection updates selected project without listing files', async () => {
    const service = new RegistryWorkspaceService();
    const repository = {
      listFiles: jest.fn().mockResolvedValue({ entries: [{ name: 'root', path: 'root', kind: 'file' }] }),
    };

    Object.assign(service as unknown as { repository: unknown; session: unknown }, {
      repository,
      session: {
        projects: [
          { projectId: 'p1', name: 'One', online: true, path: '/one' },
          { projectId: 'p2', name: 'Two', online: true, path: '/two' },
        ],
        selectedProjectId: 'p1',
        fileEntries: [{ name: 'old', path: 'old', kind: 'file' }],
      },
    });

    const session = await (service as any).selectProjectLightweight('p2');

    expect(session.selectedProjectId).toBe('p2');
    expect(session.fileEntries).toEqual([{ name: 'old', path: 'old', kind: 'file' }]);
    expect(repository.listFiles).not.toHaveBeenCalled();
  });

  test('lightweight project selection rejects unknown projects', async () => {
    const service = new RegistryWorkspaceService();

    Object.assign(service as unknown as { repository: unknown; session: unknown }, {
      repository: { listFiles: jest.fn() },
      session: {
        projects: [{ projectId: 'p1', name: 'One', online: true, path: '/one' }],
        selectedProjectId: 'p1',
        fileEntries: [],
      },
    });

    await expect((service as any).selectProjectLightweight('missing')).rejects.toThrow(
      'Project is no longer available',
    );
  });
});
