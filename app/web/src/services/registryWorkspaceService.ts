import {createRegistryRepository, type RegistryRepository} from './registryRepository';
import {RegistryRequestError} from './registryClient';
import type {RegistryDebugSink} from './registryClient';
import type {RegistryDebugConnection} from '../debug/registryDebug';
import {LocalHubReadManager, type LocalHubReadStatus} from './localHubReadManager';
import type {
  RegistryEnvelope,
  RegistryFsInfo,
  RegistryFsEntry,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryGitStatus,
  RegistryHub,
  RegistryNpmCommandResponse,
  RegistryPortRelayEnablePayload,
  RegistryPortRelaySnapshot,
  RegistryProject,
  RegistryProjectListResponse,
  RegistrySessionAttachmentCancelPayload,
  RegistrySessionAttachmentCancelResponse,
  RegistrySessionAttachmentChunkPayload,
  RegistrySessionAttachmentChunkResponse,
  RegistrySessionAttachmentDeletePayload,
  RegistrySessionAttachmentDeleteResponse,
  RegistrySessionAttachmentFinishPayload,
  RegistrySessionAttachmentFinishResponse,
  RegistrySessionAttachmentStartPayload,
  RegistrySessionAttachmentStartResponse,
  RegistrySessionContentBlock,
  RegistrySessionConfigOption,
  RegistrySessionMessage,
  RegistrySessionReadResponse,
  RegistrySessionSearchResponse,
  RegistrySessionSearchStatusResponse,
  RegistryResumableSession,
  RegistrySessionSummary,
  RegistrySkillCommandResponse,
  RegistrySkillInstallPayload,
  RegistrySkillScopePayload,
  RegistrySyncCheckPayload,
  RegistrySyncCheckResponse,
  RegistryTokenScanResult,
  RegistryWheelMakerUpdateResponse,
  RegistryWorkingTreeFileDiff,
} from '../types/registry';

export type WorkspaceSession = {
  projects: RegistryProject[];
  hubs: RegistryHub[];
  selectedProjectId: string;
  fileEntries: RegistryFsEntry[];
};

export type RegistryWorkspaceServiceOptions = {
  createRepository?: (debugSink?: RegistryDebugSink, debugConnection?: RegistryDebugConnection) => RegistryRepository;
  localHubReadManager?: LocalHubReadManager;
};

export class RegistryWorkspaceService {
  private repository: RegistryRepository | null = null;
  private session: WorkspaceSession | null = null;
  private token = '';
  private eventListeners = new Set<(event: RegistryEnvelope) => void>();
  private closeListeners = new Set<() => void>();
  private unsubscribeRepositoryEvent: (() => void) | null = null;
  private unsubscribeRepositoryClose: (() => void) | null = null;
  private readonly createRepository: (debugSink?: RegistryDebugSink, debugConnection?: RegistryDebugConnection) => RegistryRepository;
  private readonly localHubReadManager: LocalHubReadManager;

  constructor(private readonly debugSink?: RegistryDebugSink, options: RegistryWorkspaceServiceOptions = {}) {
    this.createRepository = options.createRepository ?? createRegistryRepository;
    this.localHubReadManager = options.localHubReadManager ?? new LocalHubReadManager({
      createRepository: () => this.createRepository(this.debugSink, 'Local'),
      debugSink,
    });
  }

  async connect(wsUrl: string, token: string): Promise<WorkspaceSession> {
    const repository = this.createRepository(this.debugSink, 'Remote');
    const normalizedToken = token.trim();
    try {
      await repository.initialize(wsUrl, normalizedToken);
      const previousRepository = this.repository;
      this.bindRepository(repository);
      const snapshot = await this.listProjectSnapshotWithRetry(repository);
      this.token = normalizedToken;
      await this.localHubReadManager.refresh(snapshot, normalizedToken);
      const {selectedProjectId, fileEntries} = snapshot.projects.length > 0
        ? await this.selectFirstReachableProject(repository, snapshot.projects)
        : {selectedProjectId: '', fileEntries: []};
      previousRepository?.close();
      this.repository = repository;
      this.session = {...snapshot, selectedProjectId, fileEntries};
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

  private async listProjectSnapshotWithRetry(repository: RegistryRepository): Promise<RegistryProjectListResponse> {
    const retryDelaysMs = [0, 400, 900];
    let lastSnapshot: RegistryProjectListResponse = {projects: [], hubs: []};
    for (let i = 0; i < retryDelaysMs.length; i += 1) {
      if (retryDelaysMs[i] > 0) {
        await new Promise(resolve => {
          setTimeout(resolve, retryDelaysMs[i]);
        });
      }
      const snapshot = await repository.listProjectSnapshot();
      lastSnapshot = snapshot;
      if (snapshot.projects.length > 0 || snapshot.hubs.length > 0) return snapshot;
    }
    return lastSnapshot;
  }

  private async selectFirstReachableProject(
    repository: RegistryRepository,
    projects: RegistryProject[],
  ): Promise<{selectedProjectId: string; fileEntries: RegistryFsEntry[]}> {
    let lastError: unknown = null;
    for (const project of projects) {
      if (!project.projectId) continue;
      try {
        const readRepository = this.localHubReadManager.readRepositoryForProject(project.projectId, repository);
        const fileList = await readRepository.listFiles(project.projectId, '.');
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
    this.localHubReadManager.closeAll();
  }

  getSession(): WorkspaceSession | null {
    return this.session;
  }

  async selectProject(projectId: string): Promise<WorkspaceSession> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    const repository = this.readRepositoryForProject(projectId);
    const fileEntries = (await repository.listFiles(projectId, '.')).entries ?? [];
    this.session = {...this.session, selectedProjectId: projectId, fileEntries};
    return this.session;
  }

  async selectProjectLightweight(projectId: string): Promise<WorkspaceSession> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    if (!this.session.projects.some(project => project.projectId === projectId)) {
      throw new Error('Project is no longer available');
    }
    this.session = {...this.session, selectedProjectId: projectId};
    return this.session;
  }

  async listDirectory(path: string, knownHash?: string): Promise<{entries: RegistryFsEntry[]; hash?: string; notModified: boolean}> {
    if (!this.session || !this.repository) {
      return {entries: [], hash: '', notModified: false};
    }
    if (!this.session.selectedProjectId) {
      return {entries: [], hash: '', notModified: false};
    }
    const repository = this.readRepositoryForProject(this.session.selectedProjectId);
    const result = await repository.listFiles(this.session.selectedProjectId, path || '.', knownHash);
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
    return this.getProjectFileInfo(this.session.selectedProjectId, path);
  }

  async getProjectFileInfo(projectId: string, path: string): Promise<RegistryFsInfo> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.readRepositoryForProject(projectId).getFileInfo(projectId, path);
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
    return this.readProjectFile(path, this.session.selectedProjectId, options);
  }

  async readProjectFile(path: string, projectId: string, options?: {knownHash?: string}): Promise<{
    content: string;
    hash?: string;
    notModified: boolean;
    total?: number;
    isBinary?: boolean;
  }> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    const repository = this.readRepositoryForProject(projectId);
    const result = await repository.readFile(projectId, path, options);
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
    return this.readRepositoryForProject(this.session.selectedProjectId).gitLog(this.session.selectedProjectId, ref, '', 50, refs);
  }

  async listGitBranches(): Promise<{current: string; branches: string[]; remoteBranches: string[]}> {
    if (!this.session || !this.repository) {
      return {current: '', branches: [], remoteBranches: []};
    }
    return this.readRepositoryForProject(this.session.selectedProjectId).gitBranches(this.session.selectedProjectId);
  }

  async listGitCommitFiles(sha: string): Promise<RegistryGitCommitFile[]> {
    if (!this.session || !this.repository) return [];
    return this.readRepositoryForProject(this.session.selectedProjectId).gitCommitFiles(this.session.selectedProjectId, sha);
  }

  async readGitFileDiff(sha: string, path: string): Promise<RegistryGitFileDiff> {
    if (!this.session || !this.repository) {
      return {sha, path, isBinary: false, diff: '', truncated: false};
    }
    return this.readRepositoryForProject(this.session.selectedProjectId).gitCommitFileDiff(this.session.selectedProjectId, sha, path, 3);
  }

  async getGitStatus(): Promise<RegistryGitStatus> {
    if (!this.session || !this.repository) {
      return {dirty: false, worktreeRev: '', staged: [], unstaged: [], untracked: []};
    }
    return this.readRepositoryForProject(this.session.selectedProjectId).gitStatus(this.session.selectedProjectId);
  }

  async readWorkingTreeFileDiff(
    path: string,
    scope: 'staged' | 'unstaged' | 'untracked' = 'unstaged',
  ): Promise<RegistryWorkingTreeFileDiff> {
    if (!this.session || !this.repository) {
      return {path, scope, isBinary: false, diff: '', truncated: false};
    }
    return this.readRepositoryForProject(this.session.selectedProjectId).gitWorkingTreeFileDiff(this.session.selectedProjectId, path, scope, 3);
  }

  async syncCheck(payload: RegistrySyncCheckPayload): Promise<RegistrySyncCheckResponse> {
    if (!this.session || !this.repository) {
      return {projectRev: '', gitRev: '', worktreeRev: '', staleDomains: []};
    }
    return this.readRepositoryForProject(this.session.selectedProjectId).syncCheck(this.session.selectedProjectId, payload);
  }

  async listProjects(): Promise<RegistryProject[]> {
    return (await this.listProjectSnapshot()).projects;
  }

  async listProjectSnapshot(): Promise<RegistryProjectListResponse> {
    if (!this.repository) {
      return {projects: this.session?.projects ?? [], hubs: this.session?.hubs ?? []};
    }
    const snapshot = await this.repository.listProjectSnapshot();
    await this.localHubReadManager.refresh(snapshot, this.token);
    if (this.session) {
      this.session = {...this.session, projects: snapshot.projects, hubs: snapshot.hubs};
    }
    return snapshot;
  }

  setLocalHubReadEnabled(enabled: boolean): void {
    this.localHubReadManager.setEnabled(enabled);
    if (enabled && this.session) {
      void this.localHubReadManager.refresh(this.session, this.token);
    }
  }

  getLocalHubReadStatuses(hubs: RegistryHub[] = this.session?.hubs ?? []): Record<string, LocalHubReadStatus> {
    return this.localHubReadManager.getHubStatuses(hubs);
  }

  private readRepositoryForProject(projectId: string): RegistryRepository {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.localHubReadManager.readRepositoryForProject(projectId, this.repository);
  }

  async listSessions(): Promise<RegistrySessionSummary[]> {
    if (!this.session || !this.repository) {
      return [];
    }
    return this.repository.listSessions(this.session.selectedProjectId);
  }

  async listProjectSessions(projectId: string): Promise<RegistrySessionSummary[]> {
    if (!this.repository) {
      return [];
    }
    return this.repository.listSessions(projectId);
  }

  async readSession(sessionId: string, afterTurnIndex = 0): Promise<RegistrySessionReadResponse> {
    if (!this.session || !this.repository) {
      return {
        sessionId: '',
        turns: [],
        messages: [],
        latestTurnIndex: 0,
      };
    }
    return this.repository.readSession(this.session.selectedProjectId, sessionId, afterTurnIndex);
  }

  async readProjectSession(projectId: string, sessionId: string, afterTurnIndex = 0): Promise<RegistrySessionReadResponse> {
    if (!this.repository) {
      return {
        sessionId: '',
        turns: [],
        messages: [],
        latestTurnIndex: 0,
      };
    }
    return this.repository.readSession(projectId, sessionId, afterTurnIndex);
  }

  async startProjectSessionSearch(projectId: string, searchId: string, query: string): Promise<RegistrySessionSearchStatusResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.startSessionSearch(projectId, searchId, query);
  }

  async queryProjectSessionSearch(projectId: string, searchId: string): Promise<RegistrySessionSearchResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.querySessionSearch(projectId, searchId);
  }

  async cancelProjectSessionSearch(projectId: string, searchId: string): Promise<RegistrySessionSearchStatusResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.cancelSessionSearch(projectId, searchId);
  }

  async markSessionRead(sessionId: string, lastReadTurnIndex: number): Promise<{ok: boolean; session?: RegistrySessionSummary}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.markSessionRead(this.session.selectedProjectId, sessionId, lastReadTurnIndex);
  }

  async markProjectSessionRead(
    projectId: string,
    sessionId: string,
    lastReadTurnIndex: number,
  ): Promise<{ok: boolean; session?: RegistrySessionSummary}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.markSessionRead(projectId, sessionId, lastReadTurnIndex);
  }

  async createSession(agentType: string, title?: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.createSession(this.session.selectedProjectId, agentType, title);
  }

  async createProjectSession(projectId: string, agentType: string, title?: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.createSession(projectId, agentType, title);
  }

  async sendSessionMessage(payload: {sessionId: string; text?: string; blocks?: RegistrySessionContentBlock[]}): Promise<{ok: boolean; sessionId: string}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.sendSessionMessage(this.session.selectedProjectId, payload);
  }

  async sendProjectSessionMessage(projectId: string, payload: {sessionId: string; text?: string; blocks?: RegistrySessionContentBlock[]}): Promise<{ok: boolean; sessionId: string}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.sendSessionMessage(projectId, payload);
  }

  async startProjectSessionAttachment(
    projectId: string,
    payload: RegistrySessionAttachmentStartPayload,
  ): Promise<RegistrySessionAttachmentStartResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.startSessionAttachment(projectId, payload);
  }

  async uploadProjectSessionAttachmentChunk(
    projectId: string,
    payload: RegistrySessionAttachmentChunkPayload,
  ): Promise<RegistrySessionAttachmentChunkResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.uploadSessionAttachmentChunk(projectId, payload);
  }

  async finishProjectSessionAttachment(
    projectId: string,
    payload: RegistrySessionAttachmentFinishPayload,
  ): Promise<RegistrySessionAttachmentFinishResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.finishSessionAttachment(projectId, payload);
  }

  async cancelProjectSessionAttachment(
    projectId: string,
    payload: RegistrySessionAttachmentCancelPayload,
  ): Promise<RegistrySessionAttachmentCancelResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.cancelSessionAttachment(projectId, payload);
  }

  async deleteProjectSessionAttachment(
    projectId: string,
    payload: RegistrySessionAttachmentDeletePayload,
  ): Promise<RegistrySessionAttachmentDeleteResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.deleteSessionAttachment(projectId, payload);
  }

  async cancelProjectSession(projectId: string, sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.cancelSession(projectId, sessionId);
  }

  async archiveSession(sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.archiveSession(this.session.selectedProjectId, sessionId);
  }

  async archiveProjectSession(projectId: string, sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.archiveSession(projectId, sessionId);
  }

  async deleteProjectSession(projectId: string, sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.deleteSession(projectId, sessionId);
  }

  async renameSession(sessionId: string, title: string): Promise<{ok: boolean; sessionId: string; session: RegistrySessionSummary}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.renameSession(this.session.selectedProjectId, sessionId, title);
  }

  async renameProjectSession(projectId: string, sessionId: string, title: string): Promise<{ok: boolean; sessionId: string; session: RegistrySessionSummary}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.renameSession(projectId, sessionId, title);
  }

  async listResumableSessions(agentType: string): Promise<RegistryResumableSession[]> {
    if (!this.session || !this.repository) {
      return [];
    }
    return this.repository.listResumableSessions(this.session.selectedProjectId, agentType);
  }

  async listProjectResumableSessions(projectId: string, agentType: string): Promise<RegistryResumableSession[]> {
    if (!this.repository) {
      return [];
    }
    return this.repository.listResumableSessions(projectId, agentType);
  }

  async importResumedSession(agentType: string, sessionId: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.importResumedSession(this.session.selectedProjectId, agentType, sessionId);
  }

  async importProjectResumedSession(projectId: string, agentType: string, sessionId: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.importResumedSession(projectId, agentType, sessionId);
  }

  async reloadSession(sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.reloadSession(this.session.selectedProjectId, sessionId);
  }

  async reloadProjectSession(projectId: string, sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.reloadSession(projectId, sessionId);
  }

  async setSessionConfig(payload: {sessionId: string; configId: string; value: string}): Promise<{ok: boolean; sessionId: string; configOptions: RegistrySessionConfigOption[]}> {
    if (!this.session || !this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.setSessionConfig(this.session.selectedProjectId, payload);
  }

  async setProjectSessionConfig(projectId: string, payload: {sessionId: string; configId: string; value: string}): Promise<{ok: boolean; sessionId: string; configOptions: RegistrySessionConfigOption[]}> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.setSessionConfig(projectId, payload);
  }

  async scanTokenStats(hubId: string): Promise<RegistryTokenScanResult> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.scanTokenStats(hubId);
  }

  async scanNpmPackages(hubId: string): Promise<RegistryNpmCommandResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.scanNpmPackages(hubId);
  }

  async getPortRelayStatus(): Promise<RegistryPortRelaySnapshot> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.getPortRelayStatus();
  }

  async enablePortRelay(payload: RegistryPortRelayEnablePayload): Promise<RegistryPortRelaySnapshot> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.enablePortRelay(payload);
  }

  async disablePortRelay(): Promise<RegistryPortRelaySnapshot> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.disablePortRelay();
  }

  async regeneratePortRelayAccessCode(accessCode: string): Promise<RegistryPortRelaySnapshot> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.regeneratePortRelayAccessCode(accessCode);
  }

  async installNpmPackage(hubId: string, packageName: string, version = 'latest'): Promise<RegistryNpmCommandResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.installNpmPackage(hubId, packageName, version);
  }

  async installNpmPackages(hubId: string, packageNames: string[], version = 'latest'): Promise<RegistryNpmCommandResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.installNpmPackages(hubId, packageNames, version);
  }

  async uninstallNpmPackage(hubId: string, packageName: string): Promise<RegistryNpmCommandResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.uninstallNpmPackage(hubId, packageName);
  }

  async queryWheelMakerUpdate(hubId: string): Promise<RegistryWheelMakerUpdateResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.queryWheelMakerUpdate(hubId);
  }

  async requestWheelMakerUpdatePublish(hubId: string): Promise<RegistryWheelMakerUpdateResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.requestWheelMakerUpdatePublish(hubId);
  }

  async scanSkills(hubId: string): Promise<RegistrySkillCommandResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.scanSkills(hubId);
  }

  async listSkillsSource(hubId: string, source: string): Promise<RegistrySkillCommandResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.listSkillsSource(hubId, source);
  }

  async installSkills(payload: RegistrySkillInstallPayload): Promise<RegistrySkillCommandResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.installSkills(payload);
  }

  async uninstallSkills(payload: RegistrySkillScopePayload): Promise<RegistrySkillCommandResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.uninstallSkills(payload);
  }

  async updateSkills(payload: RegistrySkillScopePayload): Promise<RegistrySkillCommandResponse> {
    if (!this.repository) {
      throw new Error('session is not ready');
    }
    return this.repository.updateSkills(payload);
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








