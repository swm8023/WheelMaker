import {createRegistryRepository, type RegistryRepository} from './observeRepository';
import type {
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryProject,
} from '../types/observe';

export type WorkspaceSession = {
  projects: RegistryProject[];
  selectedProjectId: string;
  fileEntries: RegistryFsEntry[];
};

export class RegistryWorkspaceService {
  private repository: RegistryRepository | null = null;
  private session: WorkspaceSession | null = null;

  async connect(wsUrl: string, token: string): Promise<WorkspaceSession> {
    const repository = createRegistryRepository();
    try {
      await repository.initialize(wsUrl, token);
      const projects = await repository.listProjects();
      if (projects.length === 0) {
        throw new Error('no projects available');
      }
      const selectedProjectId = projects[0].projectId;
      const fileEntries = await repository.listFiles(selectedProjectId, '.');
      this.repository?.close();
      this.repository = repository;
      this.session = {projects, selectedProjectId, fileEntries};
      return this.session;
    } catch (error) {
      repository.close();
      throw error;
    }
  }

  close(): void {
    this.repository?.close();
    this.repository = null;
    this.session = null;
  }

  getSession(): WorkspaceSession | null {
    return this.session;
  }

  async selectProject(projectId: string): Promise<WorkspaceSession> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    const fileEntries = await this.repository.listFiles(projectId, '.');
    this.session = {...this.session, selectedProjectId: projectId, fileEntries};
    return this.session;
  }

  async listDirectory(path: string): Promise<RegistryFsEntry[]> {
    if (!this.session || !this.repository) return [];
    return this.repository.listFiles(this.session.selectedProjectId, path || '.');
  }

  async readFile(path: string): Promise<string> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.readFile(this.session.selectedProjectId, path);
  }

  async listGitCommits(ref = 'HEAD'): Promise<RegistryGitCommit[]> {
    if (!this.session || !this.repository) return [];
    return this.repository.gitLog(this.session.selectedProjectId, ref, '', 50);
  }

  async listGitBranches(): Promise<{current: string; branches: string[]}> {
    if (!this.session || !this.repository) {
      return {current: '', branches: []};
    }
    return this.repository.gitBranches(this.session.selectedProjectId);
  }

  async listGitCommitFiles(sha: string): Promise<RegistryGitCommitFile[]> {
    if (!this.session || !this.repository) return [];
    return this.repository.gitCommitFiles(this.session.selectedProjectId, sha);
  }

  async readGitFileDiff(sha: string, path: string): Promise<RegistryGitFileDiff> {
    if (!this.session || !this.repository) {
      return {sha, path, isBinary: false, diff: '', truncated: false};
    }
    return this.repository.gitCommitFileDiff(this.session.selectedProjectId, sha, path, 3);
  }
}
