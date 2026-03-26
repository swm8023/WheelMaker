import { RegistryClient } from './observeClient';
import type { RegistryEnvelope, RegistryFsEntry, RegistryProject } from '../types/observe';

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
    const resp = await this.client.request({
      method: 'project.list',
      payload: {},
    });
    const payload = (resp.payload ?? {}) as { projects?: RegistryProject[] };
    return (payload.projects ?? []).filter(project => !!project.projectId);
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

  close(): void {
    this.client.close();
  }
}

export const createRegistryRepository = (): RegistryRepository => {
  return new RegistryRepository(new RegistryClient());
};

export type RegistryResponse<TPayload> = RegistryEnvelope<TPayload>;
export { RegistryRepository as ObserveRepository };
export const createObserveRepository = createRegistryRepository;
export type ObserveResponse<TPayload> = RegistryResponse<TPayload>;
