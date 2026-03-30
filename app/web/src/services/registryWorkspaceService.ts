import {createRegistryRepository, type RegistryRepository} from './registryRepository';
import type {
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryProject,
} from '../types/registry';

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
      const projects = await this.listProjectsWithRetry(repository);
      if (projects.length === 0) {
        throw new Error('No projects available. Please ensure at least one project is online and retry.');
      }
      const {selectedProjectId, fileEntries} = await this.selectFirstReachableProject(repository, projects);
      this.repository?.close();
      this.repository = repository;
      this.session = {projects, selectedProjectId, fileEntries};
      return this.session;
    } catch (error) {
      repository.close();
      throw error;
    }
  }

  private async listProjectsWithRetry(repository: RegistryRepository): Promise<RegistryProject[]> {
    const retryDelaysMs = [0, 400, 900];
    for (let i = 0; i < retryDelaysMs.length; i += 1) {
      if (retryDelaysMs[i] > 0) {
        await new Promise(resolve => {
          setTimeout(resolve, retryDelaysMs[i]);
        });
      }
      const projects = await repository.listProjects();
      if (projects.length > 0) return projects;
    }
    return [];
  }

  private async selectFirstReachableProject(
    repository: RegistryRepository,
    projects: RegistryProject[],
  ): Promise<{selectedProjectId: string; fileEntries: RegistryFsEntry[]}> {
    let lastError: unknown = null;
    for (const project of projects) {
      if (!project.projectId) continue;
      try {
        const fileEntries = await repository.listFiles(project.projectId, '.');
        return {selectedProjectId: project.projectId, fileEntries};
      } catch (error) {
        lastError = error;
        const message = error instanceof Error ? error.message.toLowerCase() : String(error).toLowerCase();
        const offline = message.includes('project not found or hub offline');
        if (!offline) {
          throw error;
        }
      }
    }
    if (lastError instanceof Error) {
      throw new Error(`No reachable projects. Last error: ${lastError.message}`);
    }
    throw new Error('No reachable projects. All listed projects appear offline.');
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
