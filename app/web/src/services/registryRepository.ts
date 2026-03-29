import { RegistryClient } from './registryClient';
import type {
  RegistryEnvelope,
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryProject,
} from '../types/registry';

export class RegistryRepository {
  constructor(private readonly client: RegistryClient) {}

  async initialize(url: string, token?: string): Promise<void> {
    await this.client.connect(url);
    await this.client.hello();
    if (token && token.trim().length > 0) {
      await this.client.auth(token.trim());
    }
  }

  async listProjects(): Promise<RegistryProject[]> {
    type FullProjectItem = {
      projectId: string;
      name: string;
      cwd?: string;
      online?: boolean;
    };
    type HubProjectItem = {
      id?: string;
      name?: string;
      path?: string;
    };
    type HubSnapshot = {
      hubId: string;
      projects?: HubProjectItem[];
    };

    let baseProjects: RegistryProject[] = [];
    try {
      const fullResp = await this.client.request({
        method: 'project.listFull',
        payload: {},
      });
      const fullPayload = (fullResp.payload ?? {}) as { projects?: FullProjectItem[] };
      baseProjects = (fullPayload.projects ?? [])
        .filter(project => !!project.projectId)
        .map(project => ({
          projectId: project.projectId,
          name: project.name,
          online: project.online,
          path: project.cwd ?? '',
        }));
    } catch {
      const resp = await this.client.request({
        method: 'project.list',
        payload: {},
      });
      const payload = (resp.payload ?? {}) as { projects?: RegistryProject[] };
      baseProjects = (payload.projects ?? []).filter(project => !!project.projectId);
    }

    try {
      const hubResp = await this.client.request({
        method: 'registry.listProjects',
        payload: {},
      });
      const hubPayload = (hubResp.payload ?? {}) as { hubs?: HubSnapshot[] };
      const hubIndex = new Map<string, {hubId: string; path?: string}>();
      for (const hub of hubPayload.hubs ?? []) {
        for (const project of hub.projects ?? []) {
          const id = (project.id ?? '').trim();
          const name = (project.name ?? '').trim();
          if (id) {
            hubIndex.set(id, {hubId: hub.hubId, path: project.path});
          }
          if (name) {
            hubIndex.set(name, {hubId: hub.hubId, path: project.path});
          }
        }
      }
      return baseProjects.map(project => {
        const match = hubIndex.get(project.projectId) ?? hubIndex.get(project.name);
        return {
          ...project,
          path: project.path || match?.path || '',
          hubId: match?.hubId || '',
        };
      });
    } catch {
      return baseProjects;
    }
  }

  async listFiles(projectId: string, path = '.'): Promise<RegistryFsEntry[]> {
    const resp = await this.client.request({
      method: 'fs.list',
      projectId,
      payload: { path, cursor: '', limit: 200 },
    });
    const payload = (resp.payload ?? {}) as { entries?: RegistryFsEntry[] };
    return (payload.entries ?? []).filter(entry => !!entry.path && !!entry.name);
  }

  async readFile(projectId: string, path: string): Promise<string> {
    const resp = await this.client.request({
      method: 'fs.read',
      projectId,
      payload: { path, offset: 0, limit: 65536 },
    });
    const payload = (resp.payload ?? {}) as { content?: string };
    return payload.content ?? '';
  }

  async gitLog(projectId: string, ref = 'HEAD', cursor = '', limit = 50): Promise<RegistryGitCommit[]> {
    const resp = await this.client.request({
      method: 'git.log',
      projectId,
      payload: { ref, cursor, limit },
      timeoutMs: 30000,
    });
    const payload = (resp.payload ?? {}) as { commits?: RegistryGitCommit[] };
    return (payload.commits ?? []).filter(commit => !!commit.sha);
  }

  async gitBranches(projectId: string): Promise<{current: string; branches: string[]}> {
    const resp = await this.client.request({
      method: 'git.branches',
      projectId,
      payload: {},
      timeoutMs: 20000,
    });
    const payload = (resp.payload ?? {}) as {current?: string; branches?: string[]};
    return {
      current: payload.current ?? '',
      branches: payload.branches ?? [],
    };
  }

  async gitCommitFiles(projectId: string, sha: string): Promise<RegistryGitCommitFile[]> {
    const resp = await this.client.request({
      method: 'git.commit.files',
      projectId,
      payload: { sha },
      timeoutMs: 20000,
    });
    const payload = (resp.payload ?? {}) as { files?: RegistryGitCommitFile[] };
    return (payload.files ?? []).filter(file => !!file.path);
  }

  async gitCommitFileDiff(
    projectId: string,
    sha: string,
    path: string,
    contextLines = 3,
  ): Promise<RegistryGitFileDiff> {
    const resp = await this.client.request({
      method: 'git.commit.fileDiff',
      projectId,
      payload: { sha, path, contextLines },
      timeoutMs: 30000,
    });
    const payload = (resp.payload ?? {}) as RegistryGitFileDiff;
    return {
      sha: payload.sha ?? sha,
      path: payload.path ?? path,
      isBinary: payload.isBinary ?? false,
      diff: payload.diff ?? '',
      truncated: payload.truncated ?? false,
    };
  }

  close(): void {
    this.client.close();
  }
}

export const createRegistryRepository = (): RegistryRepository => {
  return new RegistryRepository(new RegistryClient());
};

export type RegistryResponse<TPayload> = RegistryEnvelope<TPayload>;
