import type {RegistryDebugSink} from './registryClient';
import {createRegistryRepository, type LocalReadProofVerifier, type RegistryRepository} from './registryRepository';
import type {RegistryHub, RegistryProjectListResponse} from '../types/registry';

export type LocalHubReadStatus = 'Local' | 'Remote';

export type LocalHubReadManagerOptions = {
  createRepository?: () => RegistryRepository;
  debugSink?: RegistryDebugSink;
  verifyProof?: LocalReadProofVerifier;
};

type HubReadState = {
  repository: RegistryRepository | null;
  endpointId: string;
  projectIds: Set<string>;
  status: LocalHubReadStatus;
  reason: string;
};

function projectHubId(projectId: string): string {
  const splitAt = projectId.indexOf(':');
  return splitAt > 0 ? projectId.slice(0, splitAt) : '';
}

export class LocalHubReadManager {
  private enabled = true;
  private readonly hubs = new Map<string, HubReadState>();
  private readonly createRepository: () => RegistryRepository;
  private readonly verifyProof?: LocalReadProofVerifier;

  constructor(options: LocalHubReadManagerOptions = {}) {
    this.createRepository = options.createRepository ?? (() => createRegistryRepository(options.debugSink));
    this.verifyProof = options.verifyProof;
  }

  setEnabled(enabled: boolean): void {
    if (this.enabled === enabled) {
      return;
    }
    this.enabled = enabled;
    if (!enabled) {
      this.closeAll();
    }
  }

  isEnabled(): boolean {
    return this.enabled;
  }

  async refresh(snapshot: RegistryProjectListResponse, token: string): Promise<void> {
    if (!this.enabled) {
      this.closeAll();
      return;
    }
    const activeHubIds = new Set(snapshot.hubs.map(hub => hub.hubId).filter(Boolean));
    for (const hubId of Array.from(this.hubs.keys())) {
      if (!activeHubIds.has(hubId)) {
        this.closeHub(hubId);
      }
    }
    for (const hub of snapshot.hubs) {
      await this.refreshHub(hub, token);
    }
  }

  readRepositoryForProject(projectId: string, remoteRepository: RegistryRepository): RegistryRepository {
    if (!this.enabled) {
      return remoteRepository;
    }
    const hubId = projectHubId(projectId);
    if (!hubId) {
      return remoteRepository;
    }
    const state = this.hubs.get(hubId);
    if (state?.status !== 'Local' || !state.repository || !state.projectIds.has(projectId)) {
      return remoteRepository;
    }
    return state.repository;
  }

  getHubStatus(hubId: string): LocalHubReadStatus {
    if (!this.enabled) {
      return 'Remote';
    }
    return this.hubs.get(hubId)?.status ?? 'Remote';
  }

  getHubStatuses(hubs: RegistryHub[]): Record<string, LocalHubReadStatus> {
    const out: Record<string, LocalHubReadStatus> = {};
    for (const hub of hubs) {
      if (!hub.hubId) continue;
      out[hub.hubId] = this.getHubStatus(hub.hubId);
    }
    return out;
  }

  closeAll(): void {
    for (const hubId of Array.from(this.hubs.keys())) {
      this.closeHub(hubId);
    }
  }

  private async refreshHub(hub: RegistryHub, token: string): Promise<void> {
    const hubId = (hub.hubId || '').trim();
    if (!hubId) {
      return;
    }
    const candidate = hub.localRead;
    if (!candidate) {
      this.closeHub(hubId);
      this.hubs.set(hubId, {
        repository: null,
        endpointId: '',
        projectIds: new Set(),
        status: 'Remote',
        reason: 'no local read candidate',
      });
      return;
    }

    const existing = this.hubs.get(hubId);
    if (existing?.status === 'Local' && existing.endpointId === candidate.endpointId && existing.repository) {
      return;
    }
    this.closeHub(hubId);

    const repository = this.createRepository();
    try {
      await repository.initializeLocalRead(candidate.url, token, hubId, candidate, {
        verifyProof: this.verifyProof,
      });
      const localSnapshot = await repository.listProjectSnapshot();
      const projectIds = new Set(
        localSnapshot.projects
          .map(project => project.projectId)
          .filter(projectId => projectId.startsWith(`${hubId}:`)),
      );
      this.hubs.set(hubId, {
        repository,
        endpointId: candidate.endpointId,
        projectIds,
        status: 'Local',
        reason: '',
      });
    } catch (error) {
      repository.close();
      this.hubs.set(hubId, {
        repository: null,
        endpointId: candidate.endpointId,
        projectIds: new Set(),
        status: 'Remote',
        reason: error instanceof Error ? error.message : String(error),
      });
    }
  }

  private closeHub(hubId: string): void {
    const existing = this.hubs.get(hubId);
    existing?.repository?.close();
    this.hubs.delete(hubId);
  }
}
