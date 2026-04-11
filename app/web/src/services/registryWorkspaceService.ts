import {createRegistryRepository, type RegistryRepository} from './registryRepository';
import {RegistryRequestError} from './registryClient';
import type {
  RegistryChatMessage,
  RegistryChatSession,
  RegistryEnvelope,
  RegistryFsInfo,
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryGitStatus,
  RegistryProject,
  RegistrySyncCheckPayload,
  RegistrySyncCheckResponse,
  RegistryWorkingTreeFileDiff,
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
        const fileList = await repository.listFiles(project.projectId, '.');
        return {selectedProjectId: project.projectId, fileEntries: fileList.entries ?? []};
      } catch (error) {
        lastError = error;
        const offline =
          error instanceof RegistryRequestError &&
          (error.code === 'NOT_FOUND' || error.code === 'UNAVAILABLE');
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
    const fileEntries = (await this.repository.listFiles(projectId, '.')).entries ?? [];
    this.session = {...this.session, selectedProjectId: projectId, fileEntries};
    return this.session;
  }

  async listDirectory(path: string, knownHash?: string): Promise<{entries: RegistryFsEntry[]; hash?: string; notModified: boolean}> {
    if (!this.session || !this.repository) {
      return {entries: [], hash: '', notModified: false};
    }
    const result = await this.repository.listFiles(this.session.selectedProjectId, path || '.', knownHash);
    return {
      entries: result.entries ?? [],
      hash: result.hash,
      notModified: result.notModified,
    };
  }

  async getFileInfo(path: string): Promise<RegistryFsInfo> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.getFileInfo(this.session.selectedProjectId, path);
  }

  async readFile(path: string, options?: {knownHash?: string; offset?: number; count?: number}): Promise<{
    content: string;
    hash?: string;
    notModified: boolean;
    total?: number;
    isBinary?: boolean;
  }> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    const result = await this.repository.readFile(this.session.selectedProjectId, path, options);
    return {
      content: typeof result.content === 'string' ? result.content : '',
      hash: result.hash,
      notModified: result.notModified,
      total: result.total,
      isBinary: result.isBinary,
    };
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

  async getGitStatus(): Promise<RegistryGitStatus> {
    if (!this.session || !this.repository) {
      return {dirty: false, worktreeRev: '', staged: [], unstaged: [], untracked: []};
    }
    return this.repository.gitStatus(this.session.selectedProjectId);
  }

  async readWorkingTreeFileDiff(
    path: string,
    scope: 'staged' | 'unstaged' | 'untracked' = 'unstaged',
  ): Promise<RegistryWorkingTreeFileDiff> {
    if (!this.session || !this.repository) {
      return {path, scope, isBinary: false, diff: '', truncated: false};
    }
    return this.repository.gitWorkingTreeFileDiff(this.session.selectedProjectId, path, scope, 3);
  }

  async syncCheck(payload: RegistrySyncCheckPayload): Promise<RegistrySyncCheckResponse> {
    if (!this.session || !this.repository) {
      return {projectRev: '', gitRev: '', worktreeRev: '', staleDomains: []};
    }
    return this.repository.syncCheck(this.session.selectedProjectId, payload);
  }

  async listProjects(): Promise<RegistryProject[]> {
    if (!this.repository) {
      return this.session?.projects ?? [];
    }
    const projects = await this.repository.listProjects();
    if (this.session) {
      this.session = {...this.session, projects};
    }
    return projects;
  }

  async listChatSessions(): Promise<RegistryChatSession[]> {
    if (!this.session || !this.repository) {
      return [];
    }
    return this.repository.listChatSessions(this.session.selectedProjectId);
  }

  async readChatSession(chatId: string): Promise<{session: RegistryChatSession; messages: RegistryChatMessage[]}> {
    if (!this.session || !this.repository) {
      return {
        session: {chatId, title: chatId, preview: '', updatedAt: '', messageCount: 0},
        messages: [],
      };
    }
    return this.repository.readChatSession(this.session.selectedProjectId, chatId);
  }

  async sendChatMessage(payload: {chatId: string; text?: string; blocks?: unknown[]}): Promise<{ok: boolean; chatId: string}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.sendChatMessage(this.session.selectedProjectId, payload);
  }

  async respondToChatPermission(payload: {chatId: string; requestId: number; optionId: string}): Promise<{ok: boolean}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.respondToChatPermission(this.session.selectedProjectId, payload);
  }

  onEvent(listener: (event: RegistryEnvelope) => void): () => void {
    if (!this.repository) {
      return () => undefined;
    }
    return this.repository.onEvent(listener);
  }

  onClose(listener: () => void): () => void {
    if (!this.repository) {
      return () => undefined;
    }
    return this.repository.onClose(listener);
  }
}
