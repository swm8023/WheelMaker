import { RegistryClient } from './registryClient';
import type {
  RegistryEnvelope,
  RegistryFsInfo,
  RegistryFsListResponse,
  RegistryFsReadResponse,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryGitStatus,
  RegistryProject,
  RegistryProjectAgentProfile,
  RegistryResumableSession,
  RegistrySessionConfigOption,
  RegistrySessionConfigOptionValue,
  RegistrySessionCommand,
  RegistrySessionMessage,
  RegistrySessionMessageEventPayload,
  RegistrySessionReadResponse,
  RegistrySessionSummary,
  RegistrySyncCheckPayload,
  RegistrySyncCheckResponse,
  RegistryTokenProvider,
  RegistryDeepSeekTokenStats,
  RegistryTokenScanResult,
  RegistryWorkingTreeFileDiff,
} from '../types/registry';

export class RegistryRepository {
  constructor(private readonly client: RegistryClient) {}

  private normalizeSessionConfigOptionValue(raw: unknown): RegistrySessionConfigOptionValue | null {
    if (!raw || typeof raw !== 'object') {
      return null;
    }
    const input = raw as Record<string, unknown>;
    const value = typeof input.value === 'string' ? input.value.trim() : '';
    if (!value) {
      return null;
    }
    return {
      value,
      name: typeof input.name === 'string' ? input.name : undefined,
      description: typeof input.description === 'string' ? input.description : undefined,
    };
  }

  private normalizeSessionConfigOption(raw: unknown): RegistrySessionConfigOption | null {
    if (!raw || typeof raw !== 'object') {
      return null;
    }
    const input = raw as Record<string, unknown>;
    const id = typeof input.id === 'string' ? input.id.trim() : '';
    if (!id) {
      return null;
    }
    const options = Array.isArray(input.options)
      ? input.options
          .map(item => this.normalizeSessionConfigOptionValue(item))
          .filter((item): item is RegistrySessionConfigOptionValue => !!item)
      : undefined;
    return {
      id,
      name: typeof input.name === 'string' ? input.name : undefined,
      description: typeof input.description === 'string' ? input.description : undefined,
      category: typeof input.category === 'string' ? input.category : undefined,
      type: typeof input.type === 'string' ? input.type : undefined,
      currentValue: typeof input.currentValue === 'string' ? input.currentValue : undefined,
      options,
    };
  }

  private normalizeSessionCommand(raw: unknown): RegistrySessionCommand | null {
    if (!raw || typeof raw !== 'object') {
      return null;
    }
    const input = raw as Record<string, unknown>;
    const nameRaw = typeof input.name === 'string' ? input.name.trim() : '';
    if (!nameRaw) {
      return null;
    }
    const name = nameRaw.startsWith('/') ? nameRaw : `/${nameRaw}`;
    return {
      name,
      description: typeof input.description === 'string' ? input.description : undefined,
    };
  }
  private normalizeSessionSummary(raw: unknown): RegistrySessionSummary | null {
    if (!raw || typeof raw !== 'object') {
      return null;
    }
    const input = raw as Record<string, unknown>;
    const sessionId = typeof input.sessionId === 'string' ? input.sessionId.trim() : '';
    if (!sessionId) {
      return null;
    }
    return {
      sessionId,
      title: typeof input.title === 'string' ? input.title : '',
      preview: typeof input.preview === 'string' ? input.preview : '',
      updatedAt: typeof input.updatedAt === 'string' ? input.updatedAt : '',
      messageCount: typeof input.messageCount === 'number' && Number.isFinite(input.messageCount) ? input.messageCount : 0,
      unreadCount: typeof input.unreadCount === 'number' && Number.isFinite(input.unreadCount) ? input.unreadCount : undefined,
      agentType: typeof input.agentType === 'string' ? input.agentType : undefined,
      latestTurnIndex: typeof input.latestTurnIndex === 'number' && Number.isFinite(input.latestTurnIndex)
        ? Math.max(0, Math.trunc(input.latestTurnIndex))
        : undefined,
      running: input.running === true,
      lastDoneTurnIndex: typeof input.lastDoneTurnIndex === 'number' && Number.isFinite(input.lastDoneTurnIndex)
        ? Math.max(0, Math.trunc(input.lastDoneTurnIndex))
        : undefined,
      lastDoneSuccess: typeof input.lastDoneSuccess === 'boolean' ? input.lastDoneSuccess : undefined,
      lastReadTurnIndex: typeof input.lastReadTurnIndex === 'number' && Number.isFinite(input.lastReadTurnIndex)
        ? Math.max(0, Math.trunc(input.lastReadTurnIndex))
        : undefined,
      configOptions: Array.isArray(input.configOptions)
        ? input.configOptions
            .map(item => this.normalizeSessionConfigOption(item))
            .filter((item): item is RegistrySessionConfigOption => !!item)
        : undefined,
      commands: Array.isArray(input.commands)
        ? input.commands
            .map(item => this.normalizeSessionCommand(item))
            .filter((item): item is RegistrySessionCommand => !!item)
        : undefined,
    };
  }

  private normalizeSessionWireMessage(raw: unknown): RegistrySessionMessage | null {
    if (!raw || typeof raw !== 'object') {
      return null;
    }
    const input = raw as Record<string, unknown>;
    const sessionId = typeof input.sessionId === 'string' ? input.sessionId.trim() : '';
    if (!sessionId) {
      return null;
    }
    const turnIndex = typeof input.turnIndex === 'number' && Number.isFinite(input.turnIndex)
      ? Math.trunc(input.turnIndex)
      : 0;
    if (turnIndex <= 0) {
      return null;
    }
    const finished = input.finished === true;
    const content = typeof input.content === 'string' ? input.content.trim() : '';
    if (content === '') {
      return null;
    }
    try {
      const doc = JSON.parse(content) as Record<string, unknown>;
      const method = typeof doc.method === 'string' ? doc.method.trim() : '';
      const param =
        doc.param != null && typeof doc.param === 'object' && !Array.isArray(doc.param)
          ? (doc.param as Record<string, unknown>)
          : {};
      // Skip Claude command system messages.
      if (method === 'user_message_chunk') {
        const text = typeof param.text === 'string' ? param.text : '';
        if (
          /^<(command-name|command-message|command-args|local-command-caveat|local-command-stdout)[\s>]/.test(text)
        ) {
          return null;
        }
      }
      return { sessionId, turnIndex, method, param, finished };
    } catch {
      return {
        sessionId,
        turnIndex,
        method: 'system',
        param: { text: content },
        finished,
      };
    }
  }

  private async listSessionsByMethod(projectId: string, method: 'session.list'): Promise<RegistrySessionSummary[]> {
    const resp = await this.client.request({
      method,
      projectId,
      payload: {},
      timeoutMs: 15000,
    });
    const payload = (resp.payload ?? {}) as {sessions?: unknown[]};
    return (payload.sessions ?? [])
      .map(item => this.normalizeSessionSummary(item))
      .filter((item): item is RegistrySessionSummary => !!item);
  }

  private async readSessionByMethod(
    projectId: string,
    sessionId: string,
    afterTurnIndex: number,
    method: 'session.read',
  ): Promise<RegistrySessionReadResponse> {
    const resp = await this.client.request({
      method,
      projectId,
      payload: afterTurnIndex > 0 ? {sessionId, afterTurnIndex} : {sessionId},
      timeoutMs: 15000,
    });
    const payload = (resp.payload ?? {}) as {
      session?: unknown;
      latestTurnIndex?: unknown;
      turns?: unknown[];
    };
    const latestTurnIndex = typeof payload.latestTurnIndex === 'number' && Number.isFinite(payload.latestTurnIndex)
      ? Math.max(0, Math.trunc(payload.latestTurnIndex))
      : 0;
    const normalizedSession = this.normalizeSessionSummary(payload.session);
    if (normalizedSession) {
      normalizedSession.latestTurnIndex = latestTurnIndex;
    }

    const normalizedMessages: RegistrySessionMessage[] = (Array.isArray(payload.turns) ? payload.turns : [])
      .map(item => this.normalizeSessionWireMessage(item))
      .filter((item): item is RegistrySessionMessage => !!item)
      .sort((a, b) => {
        return (a.turnIndex ?? 0) - (b.turnIndex ?? 0);
      });

    return {
      ...(normalizedSession ? {session: normalizedSession} : {}),
      messages: normalizedMessages,
      latestTurnIndex,
    };
  }
  async initialize(url: string, token?: string): Promise<void> {
    await this.client.connect(url);
    await this.client.connectInit({
      clientName: 'wheelmaker-web',
      clientVersion: '0.1.0',
      protocolVersion: '2.2',
      role: 'client',
      token: token?.trim() ?? '',
    });
  }

  async listProjects(): Promise<RegistryProject[]> {
    const resp = await this.client.request({
      method: 'project.list',
      payload: {},
    });
    const payload = (resp.payload ?? {}) as { projects?: RegistryProject[] };
    return (payload.projects ?? [])
      .filter(project => !!project.projectId)
      .map(project => ({
        ...project,
        agent: typeof project.agent === 'string'
          ? project.agent.trim()
          : undefined,
        agents: Array.isArray(project.agents)
          ? project.agents
              .filter((item): item is string => typeof item === 'string')
              .map(item => item.trim())
              .filter(item => item.length > 0)
          : undefined,
        agentProfiles: Array.isArray(project.agentProfiles)
          ? project.agentProfiles
              .map((item): RegistryProjectAgentProfile | null => {
                const name = typeof item?.name === 'string' ? item.name.trim() : '';
                if (!name) {
                  return null;
                }
                const skills = Array.isArray(item.skills)
                  ? item.skills
                      .filter((skill): skill is string => typeof skill === 'string')
                      .map(skill => skill.trim())
                      .filter(skill => skill.length > 0)
                  : [];
                return { name, skills };
              })
              .filter((item): item is RegistryProjectAgentProfile => !!item)
          : undefined,
        hubId: project.hubId || project.projectId.split(':', 1)[0] || '',
      }));
  }

  async syncCheck(projectId: string, payload: RegistrySyncCheckPayload): Promise<RegistrySyncCheckResponse> {
    const resp = await this.client.request({
      method: 'project.syncCheck',
      projectId,
      payload,
    });
    const body = (resp.payload ?? {}) as Partial<RegistrySyncCheckResponse>;
    return {
      projectRev: body.projectRev ?? '',
      gitRev: body.gitRev ?? '',
      worktreeRev: body.worktreeRev ?? '',
      staleDomains: Array.isArray(body.staleDomains) ? body.staleDomains.filter(item => typeof item === 'string') : [],
    };
  }

  async listFiles(projectId: string, path = '.', knownHash?: string): Promise<RegistryFsListResponse> {
    const resp = await this.client.request({
      method: 'fs.list',
      projectId,
      payload: knownHash ? {path, knownHash} : {path},
      timeoutMs: 20000,
    });
    const payload = (resp.payload ?? {}) as RegistryFsListResponse;
    const basePath = payload.path ?? path;
    const joinPath = (parent: string, name: string): string => {
      const cleanParent = (parent || '.').replace(/\\/g, '/');
      if (cleanParent === '.' || cleanParent === '') return name;
      return `${cleanParent.replace(/\/+$/, '')}/${name}`;
    };
    const entries = (payload.entries ?? [])
      .filter(entry => !!entry?.name)
      .map(entry => ({
        ...entry,
        path: entry.path && entry.path.trim().length > 0 ? entry.path : joinPath(basePath, entry.name),
      }));
    return {
      path: payload.path ?? path,
      hash: payload.hash,
      notModified: payload.notModified ?? false,
      entries,
    };
  }

  async getFileInfo(projectId: string, path: string): Promise<RegistryFsInfo> {
    const resp = await this.client.request({
      method: 'fs.info',
      projectId,
      payload: {path},
    });
    const payload = (resp.payload ?? {}) as RegistryFsInfo;
    const tabSize = typeof payload.tabSize === 'number' && Number.isFinite(payload.tabSize)
      ? Math.max(1, Math.min(12, Math.trunc(payload.tabSize)))
      : undefined;
    return {
      path: payload.path ?? path,
      kind: payload.kind ?? 'file',
      size: payload.size ?? 0,
      isBinary: payload.isBinary ?? false,
      mimeType: payload.mimeType ?? '',
      totalLines: payload.totalLines ?? 0,
      tabSize,
      entryCount: payload.entryCount ?? 0,
      hash: payload.hash ?? '',
    };
  }

  async readFile(
    projectId: string,
    path: string,
    options?: {knownHash?: string},
  ): Promise<RegistryFsReadResponse> {
    const resp = await this.client.request({
      method: 'fs.read',
      projectId,
      payload: {
        path,
        ...(options?.knownHash ? {knownHash: options.knownHash} : {}),
      },
    });
    const payload = (resp.payload ?? {}) as RegistryFsReadResponse;
    return {
      path: payload.path ?? path,
      hash: payload.hash,
      notModified: payload.notModified ?? false,
      isBinary: payload.isBinary ?? false,
      mimeType: payload.mimeType ?? '',
      encoding: payload.encoding ?? 'utf-8',
      content: payload.content ?? '',
      size: payload.size ?? 0,
      total: payload.total ?? 0,
      returned: payload.returned ?? 0,
    };
  }

  async gitLog(
    projectId: string,
    ref = 'HEAD',
    cursor = '',
    limit = 50,
    refs: string[] = [],
  ): Promise<RegistryGitCommit[]> {
    const normalizedRefs = refs
      .map(item => item.trim())
      .filter(item => item.length > 0);
    const resp = await this.client.request({
      method: 'git.log',
      projectId,
      payload: normalizedRefs.length > 0
        ? {ref, refs: normalizedRefs, cursor, limit}
        : {ref, cursor, limit},
      timeoutMs: 30000,
    });
    const payload = (resp.payload ?? {}) as { commits?: RegistryGitCommit[] };
    return (payload.commits ?? []).filter(commit => !!commit.sha);
  }

  async gitBranches(projectId: string): Promise<{current: string; branches: string[]; remoteBranches: string[]}> {
    const resp = await this.client.request({
      method: 'git.refs',
      projectId,
      payload: {},
      timeoutMs: 20000,
    });
    const payload = (resp.payload ?? {}) as {current?: string; branches?: string[]; remoteBranches?: string[]};
    return {
      current: payload.current ?? '',
      branches: payload.branches ?? [],
      remoteBranches: payload.remoteBranches ?? [],
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

  async gitStatus(projectId: string): Promise<RegistryGitStatus> {
    const resp = await this.client.request({
      method: 'git.status',
      projectId,
      payload: {},
      timeoutMs: 20000,
    });
    const payload = (resp.payload ?? {}) as Partial<RegistryGitStatus>;
    return {
      dirty: payload.dirty ?? false,
      worktreeRev: payload.worktreeRev ?? '',
      staged: payload.staged ?? [],
      unstaged: payload.unstaged ?? [],
      untracked: payload.untracked ?? [],
    };
  }

  async gitWorkingTreeFileDiff(
    projectId: string,
    path: string,
    scope: 'staged' | 'unstaged' | 'untracked' = 'unstaged',
    contextLines = 3,
  ): Promise<RegistryWorkingTreeFileDiff> {
    const resp = await this.client.request({
      method: 'git.workingTree.fileDiff',
      projectId,
      payload: {path, scope, contextLines},
      timeoutMs: 30000,
    });
    const payload = (resp.payload ?? {}) as Partial<RegistryWorkingTreeFileDiff>;
    return {
      path: payload.path ?? path,
      scope: payload.scope ?? scope,
      isBinary: payload.isBinary ?? false,
      diff: payload.diff ?? '',
      truncated: payload.truncated ?? false,
    };
  }

  async listSessions(projectId: string): Promise<RegistrySessionSummary[]> {
    return this.listSessionsByMethod(projectId, 'session.list');
  }

  async readSession(projectId: string, sessionId: string, afterTurnIndex = 0): Promise<RegistrySessionReadResponse> {
    return this.readSessionByMethod(projectId, sessionId, afterTurnIndex, 'session.read');
  }

  async markSessionRead(
    projectId: string,
    sessionId: string,
    lastReadTurnIndex: number,
  ): Promise<{ok: boolean; session?: RegistrySessionSummary}> {
    const cursor = Number.isFinite(lastReadTurnIndex)
      ? Math.max(0, Math.trunc(lastReadTurnIndex))
      : 0;
    const resp = await this.client.request({
      method: 'session.markRead',
      projectId,
      payload: {
        sessionId,
        lastReadTurnIndex: cursor,
      },
      timeoutMs: 15000,
    });
    const body = (resp.payload ?? {}) as {ok?: boolean; session?: unknown};
    return {
      ok: body.ok ?? false,
      session: this.normalizeSessionSummary(body.session) ?? undefined,
    };
  }

  async createSession(projectId: string, agentType: string, title?: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
    const resp = await this.client.request({
      method: 'session.new',
      projectId,
      payload: title?.trim() ? {agentType, title: title.trim()} : {agentType},
      timeoutMs: 15000,
    });
    const body = (resp.payload ?? {}) as {ok?: boolean; session?: RegistrySessionSummary};
    return {
      ok: body.ok ?? false,
      session: body.session ?? {
        sessionId: '',
        title: title?.trim() || '',
        preview: '',
        updatedAt: '',
        messageCount: 0,
        agentType,
      },
    };
  }

  async sendSessionMessage(projectId: string, payload: {sessionId: string; text?: string; blocks?: unknown[]}): Promise<{ok: boolean; sessionId: string}> {
    const resp = await this.client.request({
      method: 'session.send',
      projectId,
      payload,
      timeoutMs: 30000,
    });
    const body = (resp.payload ?? {}) as {ok?: boolean; sessionId?: string};
    return {
      ok: body.ok ?? false,
      sessionId: body.sessionId ?? payload.sessionId,
    };
  }

  async deleteSession(projectId: string, sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    const resp = await this.client.request({
      method: 'session.delete',
      projectId,
      payload: {sessionId},
      timeoutMs: 15000,
    });
    const body = (resp.payload ?? {}) as {ok?: boolean; sessionId?: string};
    return {
      ok: body.ok ?? false,
      sessionId: body.sessionId ?? sessionId,
    };
  }

  async setSessionConfig(
    projectId: string,
    payload: {sessionId: string; configId: string; value: string},
  ): Promise<{ok: boolean; sessionId: string; configOptions: RegistrySessionConfigOption[]}> {
    const resp = await this.client.request({
      method: 'session.setConfig',
      projectId,
      payload,
      timeoutMs: 15000,
    });
    const body = (resp.payload ?? {}) as {
      ok?: boolean;
      sessionId?: string;
      configOptions?: unknown[];
    };
    const configOptions = (Array.isArray(body.configOptions) ? body.configOptions : [])
      .map(item => this.normalizeSessionConfigOption(item))
      .filter((item): item is RegistrySessionConfigOption => !!item);
    return {
      ok: body.ok ?? false,
      sessionId: body.sessionId ?? payload.sessionId,
      configOptions,
    };
  }

  async listResumableSessions(projectId: string, agentType: string): Promise<RegistryResumableSession[]> {
    const resp = await this.client.request({
      method: 'session.resume.list',
      projectId,
      payload: {agentType},
      timeoutMs: 15000,
    });
    const payload = (resp.payload ?? {}) as {sessions?: RegistryResumableSession[]};
    return payload.sessions ?? [];
  }

  async importResumedSession(projectId: string, agentType: string, sessionId: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
    const resp = await this.client.request({
      method: 'session.resume.import',
      projectId,
      payload: {agentType, sessionId},
      timeoutMs: 15000,
    });
    const body = (resp.payload ?? {}) as {ok?: boolean; session?: RegistrySessionSummary};
    return {
      ok: body.ok ?? false,
      session: body.session ?? {
        sessionId,
        title: '',
        preview: '',
        updatedAt: '',
        messageCount: 0,
        agentType,
      },
    };
  }

  async reloadSession(projectId: string, sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    const resp = await this.client.request({
      method: 'session.reload',
      projectId,
      payload: {sessionId},
      timeoutMs: 30000,
    });
    const body = (resp.payload ?? {}) as {ok?: boolean; sessionId?: string};
    return {
      ok: body.ok ?? false,
      sessionId: body.sessionId ?? sessionId,
    };
  }

  async listTokenProviders(projectId: string): Promise<RegistryTokenProvider[]> {
    const resp = await this.client.request({
      method: 'session.token.providers',
      projectId,
      payload: {},
      timeoutMs: 15000,
    });
    const payload = (resp.payload ?? {}) as {providers?: unknown[]};
    return (Array.isArray(payload.providers) ? payload.providers : [])
      .map(item => {
        if (!item || typeof item !== 'object') return null;
        const value = item as Record<string, unknown>;
        const id = typeof value.id === 'string' ? value.id.trim() : '';
        if (!id) return null;
        return {
          id,
          name: typeof value.name === 'string' ? value.name : id,
          authMode: typeof value.authMode === 'string' ? value.authMode : 'api_key',
        } as RegistryTokenProvider;
      })
      .filter((item): item is RegistryTokenProvider => !!item);
  }

  async fetchDeepSeekTokenStats(
    projectId: string,
    payload: {apiKey: string; rangeType?: 'day' | 'month'; month?: string},
  ): Promise<RegistryDeepSeekTokenStats> {
    const resp = await this.client.request({
      method: 'session.token.deepseek.stats',
      projectId,
      payload,
      timeoutMs: 30000,
    });
    return (resp.payload ?? {}) as RegistryDeepSeekTokenStats;
  }

  async scanTokenStats(projectId: string): Promise<RegistryTokenScanResult> {
    const resp = await this.client.request({
      method: 'session.token.scan',
      projectId,
      payload: {},
      timeoutMs: 45000,
    });
    return (resp.payload ?? {}) as RegistryTokenScanResult;
  }

  close(): void {
    this.client.close();
  }

  onEvent(
    listener: (
      event:
        | RegistryEnvelope<RegistrySessionMessageEventPayload>
        | RegistryEnvelope<Record<string, never>>,
    ) => void,
  ): () => void {
    return this.client.onEvent(listener as (event: RegistryEnvelope) => void);
  }

  onClose(listener: () => void): () => void {
    return this.client.onClose(listener);
  }
}

export const createRegistryRepository = (): RegistryRepository => {
  return new RegistryRepository(new RegistryClient());
};

export type RegistryResponse<TPayload> = RegistryEnvelope<TPayload>;








