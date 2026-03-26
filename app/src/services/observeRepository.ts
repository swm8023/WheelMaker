import { ObserveClient } from './observeClient';
import type { ObserveEnvelope, ObserveFsEntry, ObserveProject } from '../types/observe';

export class ObserveRepository {
  constructor(private readonly client: ObserveClient) {}

  async initialize(url: string, token?: string): Promise<void> {
    await this.client.connect(url);
    await this.client.hello();
    if (token && token.trim().length > 0) {
      await this.client.auth(token.trim());
    }
  }

  async listProjects(): Promise<ObserveProject[]> {
    const resp = await this.client.request({
      method: 'project.list',
      payload: {},
    });
    const payload = (resp.payload ?? {}) as { projects?: ObserveProject[] };
    return (payload.projects ?? []).filter(project => !!project.projectId);
  }

  async listFiles(projectId: string, path = '.'): Promise<ObserveFsEntry[]> {
    const resp = await this.client.request({
      method: 'fs.list',
      projectId,
      payload: { path, cursor: '', limit: 200 },
    });
    const payload = (resp.payload ?? {}) as { entries?: ObserveFsEntry[] };
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

export const createObserveRepository = (): ObserveRepository => {
  return new ObserveRepository(new ObserveClient());
};

export type ObserveResponse<TPayload> = ObserveEnvelope<TPayload>;
