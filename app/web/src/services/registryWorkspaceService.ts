import {createRegistryRepository, type RegistryRepository} from './registryRepository';
import {RegistryRequestError} from './registryClient';
import type {
  RegistryEnvelope,
  RegistryFsInfo,
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryGitStatus,
  RegistryProject,
  RegistrySessionConfigOption,
  RegistrySessionMessage,
  RegistrySessionReadResponse,
  RegistryResumableSession,
  RegistrySessionSummary,
  RegistrySyncCheckPayload,
  RegistrySyncCheckResponse,
  RegistryTokenScanResult,
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
  private eventListeners = new Set<(event: RegistryEnvelope) => void>();
  private closeListeners = new Set<() => void>();
  private unsubscribeRepositoryEvent: (() => void) | null = null;
  private unsubscribeRepositoryClose: (() => void) | null = null;

  async connect(wsUrl: string, token: string): Promise<WorkspaceSession> {
    const repository = createRegistryRepository();
    try {
      await repository.initialize(wsUrl, token);
      const previousRepository = this.repository;
      this.bindRepository(repository);
      const projects = await this.listProjectsWithRetry(repository);
      if (projects.length === 0) {
        throw new Error('No projects available. Please ensure at least one project is online and retry.');
      }
      const {selectedProjectId, fileEntries} = await this.selectFirstReachableProject(repository, projects);
      previousRepository?.close();
      this.repository = repository;
      this.session = {projects, selectedProjectId, fileEntries};
      return this.session;
    } catch (error) {
      this.unbindRepository();
      repository.close();
      throw error;
    }
  }

  private bindRepository(repository: RegistryRepository): void {
    this.unbindRepository();
    this.unsubscribeRepositoryEvent = repository.onEvent(event => {
      this.eventListeners.forEach(listener => listener(event));
    });
    this.unsubscribeRepositoryClose = repository.onClose(() => {
      this.closeListeners.forEach(listener => listener());
    });
  }

  private unbindRepository(): void {
    this.unsubscribeRepositoryEvent?.();
    this.unsubscribeRepositoryEvent = null;
    this.unsubscribeRepositoryClose?.();
    this.unsubscribeRepositoryClose = null;
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

  async readFile(path: string, options?: {knownHash?: string}): Promise<{
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

  async listGitCommits(ref = 'HEAD', refs: string[] = []): Promise<RegistryGitCommit[]> {
    if (!this.session || !this.repository) return [];
    return this.repository.gitLog(this.session.selectedProjectId, ref, '', 50, refs);
  }

  async listGitBranches(): Promise<{current: string; branches: string[]; remoteBranches: string[]}> {
    if (!this.session || !this.repository) {
      return {current: '', branches: [], remoteBranches: []};
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

  async listSessions(): Promise<RegistrySessionSummary[]> {
    if (!this.session || !this.repository) {
      return [];
    }
    return this.repository.listSessions(this.session.selectedProjectId);
  }

  async readSession(sessionId: string, promptIndex = 0, turnIndex = 0): Promise<{session: RegistrySessionSummary; prompts: RegistrySessionReadResponse['prompts']; messages: RegistrySessionMessage[]}> {
    if (!this.session || !this.repository) {
      return {
        session: {sessionId, title: sessionId, preview: '', updatedAt: '', messageCount: 0, latestTurnIndex: 0},
        prompts: [],
        messages: [],
      };
    }
    return this.repository.readSession(this.session.selectedProjectId, sessionId, promptIndex, turnIndex);
  }

  async createSession(agentType: string, title?: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.createSession(this.session.selectedProjectId, agentType, title);
  }

  async sendSessionMessage(payload: {sessionId: string; text?: string; blocks?: unknown[]}): Promise<{ok: boolean; sessionId: string}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.sendSessionMessage(this.session.selectedProjectId, payload);
  }

  async deleteSession(sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.deleteSession(this.session.selectedProjectId, sessionId);
  }

  async listResumableSessions(agentType: string): Promise<RegistryResumableSession[]> {
    if (!this.session || !this.repository) {
      return [];
    }
    return this.repository.listResumableSessions(this.session.selectedProjectId, agentType);
  }

  async importResumedSession(agentType: string, sessionId: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.importResumedSession(this.session.selectedProjectId, agentType, sessionId);
  }

  async reloadSession(sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.reloadSession(this.session.selectedProjectId, sessionId);
  }

  async setSessionConfig(payload: {sessionId: string; configId: string; value: string}): Promise<{ok: boolean; sessionId: string; configOptions: RegistrySessionConfigOption[]}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.setSessionConfig(this.session.selectedProjectId, payload);
  }

  async scanTokenStats(projectId?: string): Promise<RegistryTokenScanResult> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    const targetProjectId = (projectId || '').trim() || this.session.selectedProjectId;
    return this.repository.scanTokenStats(targetProjectId);
  }

  onEvent(listener: (event: RegistryEnvelope) => void): () => void {
    this.eventListeners.add(listener);
    return () => {
      this.eventListeners.delete(listener);
    };
  }

  onClose(listener: () => void): () => void {
    this.closeListeners.add(listener);
    return () => {
      this.closeListeners.delete(listener);
    };
  }
}








