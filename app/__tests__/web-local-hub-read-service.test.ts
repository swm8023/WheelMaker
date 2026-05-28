import {LocalHubReadManager} from '../web/src/services/localHubReadManager';
import {RegistryRepository, type LocalReadProofVerifier} from '../web/src/services/registryRepository';
import {RegistryWorkspaceService} from '../web/src/services/registryWorkspaceService';
import type {RegistryEnvelope, RegistryLocalReadCandidate, RegistryProjectListResponse} from '../web/src/types/registry';

function makeCandidate(overrides: Partial<RegistryLocalReadCandidate> = {}): RegistryLocalReadCandidate {
  return {
    endpointId: 'endpoint-1',
    url: 'ws://127.0.0.1:53123/ws',
    proofPublicKey: 'public-key',
    proofFingerprint: 'sha256:fingerprint',
    ...overrides,
  };
}

describe('local hub read service routing', () => {
  test('normalizes local read candidate metadata from project.list hubs', async () => {
    const fakeClient = {
      request: jest.fn(async () => ({
        payload: {
          projects: [
            {projectId: 'hub-a:proj1', name: 'proj1', path: 'D:/proj1', online: true},
          ],
          hubs: [
            {
              hubId: ' hub-a ',
              localRead: {
                endpointId: ' endpoint-1 ',
                url: ' ws://127.0.0.1:53123/ws ',
                proofPublicKey: ' public-key ',
                proofFingerprint: ' sha256:fingerprint ',
              },
            },
          ],
        },
      })),
    };
    const repository = new RegistryRepository(fakeClient as never);

    const snapshot = await repository.listProjectSnapshot();

    expect(snapshot.hubs).toEqual([
      {
        hubId: 'hub-a',
        localRead: makeCandidate(),
      },
    ]);
  });

  test('does not send token until local read proof verifies', async () => {
    const calls: Array<{method: string; payload?: unknown}> = [];
    const verifyProof: LocalReadProofVerifier = jest.fn(async () => true);
    const fakeClient = {
      connect: jest.fn(async () => {
        calls.push({method: 'connect'});
      }),
      request: jest.fn(async (args: {method: string; payload: unknown}) => {
        calls.push({method: args.method, payload: args.payload});
        return {
          payload: {
            endpointId: 'endpoint-1',
            nonce: 'nonce-1',
            signature: 'signature',
            proofPublicKey: 'public-key',
            proofFingerprint: 'sha256:fingerprint',
          },
        } as RegistryEnvelope;
      }),
      connectInit: jest.fn(async (payload: unknown) => {
        calls.push({method: 'connect.init', payload});
      }),
      close: jest.fn(),
    };
    const repository = new RegistryRepository(fakeClient as never);

    await repository.initializeLocalRead('ws://127.0.0.1:53123/ws', 'secret-token', 'hub-a', makeCandidate(), {
      createNonce: () => 'nonce-1',
      verifyProof,
    });

    expect(calls.map(call => call.method)).toEqual(['connect', 'local_read.proof', 'connect.init']);
    expect(calls[1].payload).toEqual({endpointId: 'endpoint-1', nonce: 'nonce-1'});
    expect(JSON.stringify(calls[1].payload)).not.toContain('secret-token');
    expect(calls[2].payload).toMatchObject({
      role: 'local_read',
      hubId: 'hub-a',
      token: 'secret-token',
      protocolVersion: '2.3',
    });
  });

  test('routes read-safe project calls through matched local repository while sessions stay remote', async () => {
    const localFileError = new Error('local file failure');
    const remoteRepository = {
      initialize: jest.fn(async () => undefined),
      listProjectSnapshot: jest.fn(async () => ({
        projects: [{projectId: 'hub-a:proj1', name: 'proj1', path: 'D:/proj1', online: true, hubId: 'hub-a'}],
        hubs: [{hubId: 'hub-a', localRead: makeCandidate()}],
      })),
      listFiles: jest.fn(async () => ({entries: [{name: 'remote.txt', path: 'remote.txt', kind: 'file'}], notModified: false})),
      gitStatus: jest.fn(async () => ({dirty: false, worktreeRev: 'remote', staged: [], unstaged: [], untracked: []})),
      syncCheck: jest.fn(async () => ({projectRev: 'remote', gitRev: 'remote', worktreeRev: 'remote', staleDomains: []})),
      listSessions: jest.fn(async () => [{sessionId: 'remote-session', title: 'Remote', preview: '', updatedAt: '', messageCount: 0}]),
      onEvent: jest.fn(() => () => undefined),
      onClose: jest.fn(() => () => undefined),
      close: jest.fn(),
    };
    const localRepository = {
      initializeLocalRead: jest.fn(async () => undefined),
      listProjectSnapshot: jest.fn(async (): Promise<RegistryProjectListResponse> => ({
        projects: [{projectId: 'hub-a:proj1', name: 'proj1', path: 'D:/proj1', online: true, hubId: 'hub-a'}],
        hubs: [{hubId: 'hub-a', localRead: makeCandidate()}],
      })),
      listFiles: jest.fn(async () => ({entries: [{name: 'local.txt', path: 'local.txt', kind: 'file'}], notModified: false})),
      gitStatus: jest.fn(async () => ({dirty: true, worktreeRev: 'local', staged: [], unstaged: [], untracked: []})),
      syncCheck: jest.fn(async () => ({projectRev: 'local', gitRev: 'local', worktreeRev: 'local', staleDomains: ['git']})),
      close: jest.fn(),
    };
    const service = new RegistryWorkspaceService(undefined, {
      createRepository: jest.fn()
        .mockReturnValueOnce(remoteRepository)
        .mockReturnValue(localRepository),
      localHubReadManager: new LocalHubReadManager({
        createRepository: () => localRepository as never,
        verifyProof: jest.fn(async () => true),
      }),
    });

    const session = await service.connect('ws://registry.example/ws', 'secret-token');
    await new Promise(resolve => setTimeout(resolve, 0));
    const status = await service.getGitStatus();
    const sync = await service.syncCheck({});
    const sessions = await service.listSessions();

    expect(session.fileEntries).toEqual([{name: 'remote.txt', path: 'remote.txt', kind: 'file'}]);
    expect(status.worktreeRev).toBe('local');
    expect(sync.gitRev).toBe('local');
    expect(sessions[0].sessionId).toBe('remote-session');
    expect(remoteRepository.gitStatus).not.toHaveBeenCalled();
    expect(remoteRepository.listSessions).toHaveBeenCalled();

    localRepository.listFiles.mockRejectedValueOnce(localFileError);
    await expect(service.listDirectory('.')).rejects.toThrow('local file failure');
    expect(remoteRepository.listFiles).toHaveBeenCalledTimes(1);
  });

  test('does not wait for local read refresh before returning the registry session', async () => {
    let releaseRefresh: (() => void) | null = null;
    const refreshStarted = jest.fn();
    const remoteRepository = {
      initialize: jest.fn(async () => undefined),
      listProjectSnapshot: jest.fn(async () => ({
        projects: [{projectId: 'hub-a:proj1', name: 'proj1', path: 'D:/proj1', online: true, hubId: 'hub-a'}],
        hubs: [{hubId: 'hub-a', localRead: makeCandidate()}],
      })),
      listFiles: jest.fn(async () => ({entries: [{name: 'remote.txt', path: 'remote.txt', kind: 'file'}], notModified: false})),
      onEvent: jest.fn(() => () => undefined),
      onClose: jest.fn(() => () => undefined),
      close: jest.fn(),
    };
    const localHubReadManager = {
      refresh: jest.fn(async () => {
        refreshStarted();
        await new Promise<void>(resolve => {
          releaseRefresh = resolve;
        });
      }),
      readRepositoryForProject: jest.fn((_projectId: string, repository: unknown) => repository),
      closeAll: jest.fn(),
      setEnabled: jest.fn(),
      getHubStatuses: jest.fn(() => ({})),
    };
    const service = new RegistryWorkspaceService(undefined, {
      createRepository: jest.fn().mockReturnValue(remoteRepository),
      localHubReadManager: localHubReadManager as never,
    });

    const connectPromise = service.connect('ws://registry.example/ws', 'secret-token');
    const raceResult = await Promise.race([
      connectPromise,
      new Promise(resolve => setTimeout(() => resolve('blocked'), 20)),
    ]);
    releaseRefresh?.();
    const session = await connectPromise;

    expect(raceResult).not.toBe('blocked');
    expect(refreshStarted).toHaveBeenCalled();
    expect(session.fileEntries).toEqual([{name: 'remote.txt', path: 'remote.txt', kind: 'file'}]);
  });
});
