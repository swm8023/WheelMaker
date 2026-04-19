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
  RegistrySessionMessage,
  RegistrySessionMessageEventPayload,
  RegistrySessionReadResponse,
  RegistrySessionSummary,
  RegistrySyncCheckPayload,
  RegistrySyncCheckResponse,
  RegistryWorkingTreeFileDiff,
} from '../types/registry';

export class RegistryRepository {
  constructor(private readonly client: RegistryClient) {}

  private normalizeSessionSummary(raw: unknown): RegistrySessionSummary | null {
    if (!raw || typeof raw !== 'object') {
      return null;
    }
    const input = raw as Record<string, unknown>;
    const sessionId = typeof input.sessionId === 'string'
      ? input.sessionId.trim()
      : (typeof input.chatId === 'string' ? input.chatId.trim() : '');
    if (!sessionId) {
      return null;
    }
    return {
      sessionId,
      title: typeof input.title === 'string' ? input.title : sessionId,
      preview: typeof input.preview === 'string' ? input.preview : '',
      updatedAt: typeof input.updatedAt === 'string' ? input.updatedAt : '',
      messageCount: typeof input.messageCount === 'number' && Number.isFinite(input.messageCount) ? input.messageCount : 0,
      unreadCount: typeof input.unreadCount === 'number' && Number.isFinite(input.unreadCount) ? input.unreadCount : undefined,
      agent: typeof input.agent === 'string' ? input.agent : undefined,
      status: typeof input.status === 'string' ? input.status : undefined,
    };
  }

  private normalizeSessionMessage(raw: unknown, fallbackSessionId: string): RegistrySessionMessage | null {
    if (!raw || typeof raw !== 'object') {
      return null;
    }
    const input = raw as Record<string, unknown>;
    const messageId = typeof input.messageId === 'string' ? input.messageId : '';
    if (!messageId) {
      return null;
    }
    const sessionId = typeof input.sessionId === 'string'
      ? input.sessionId
      : (typeof input.chatId === 'string' ? input.chatId : fallbackSessionId);
    const syncIndex = typeof input.syncIndex === 'number' && Number.isFinite(input.syncIndex)
      ? input.syncIndex
      : undefined;
    const role = input.role === 'user' || input.role === 'assistant' || input.role === 'system'
      ? input.role
      : 'assistant';
    const kind = input.kind === 'text' || input.kind === 'image' || input.kind === 'thought' || input.kind === 'tool' || input.kind === 'permission' || input.kind === 'prompt_result' || input.kind === 'message'
      ? input.kind
      : 'text';
    const status = input.status === 'streaming' || input.status === 'done' || input.status === 'needs_action'
      ? input.status
      : 'done';
    return {
      messageId,
      sessionId,
      syncIndex,
      role,
      kind,
      text: typeof input.text === 'string' ? input.text : '',
      status,
      createdAt: typeof input.createdAt === 'string' ? input.createdAt : '',
      updatedAt: typeof input.updatedAt === 'string' ? input.updatedAt : '',
      requestId: typeof input.requestId === 'number' && Number.isFinite(input.requestId) ? input.requestId : undefined,
      blocks: Array.isArray(input.blocks) ? input.blocks as RegistrySessionMessage['blocks'] : undefined,
      options: Array.isArray(input.options) ? input.options as RegistrySessionMessage['options'] : undefined,
    };
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
    afterIndex: number,
    method: 'session.read',
  ): Promise<RegistrySessionReadResponse> {
    const resp = await this.client.request({
      method,
      projectId,
      payload: afterIndex > 0 ? {sessionId, afterIndex} : {sessionId},
      timeoutMs: 15000,
    });
    const payload = (resp.payload ?? {}) as {
      session?: unknown;
      messages?: unknown[];
      lastIndex?: number;
    };
    const normalizedSession = this.normalizeSessionSummary(payload.session) ?? {
      sessionId,
      title: sessionId,
      preview: '',
      updatedAt: '',
      messageCount: 0,
    };
    const normalizedMessages = (payload.messages ?? [])
      .map(item => this.normalizeSessionMessage(item, normalizedSession.sessionId))
      .filter((item): item is RegistrySessionMessage => !!item);
    const maxSyncIndex = normalizedMessages.reduce((max, message) => Math.max(max, message.syncIndex ?? 0), 0);
    return {
      session: normalizedSession,
      messages: normalizedMessages,
      lastIndex: typeof payload.lastIndex === 'number' ? payload.lastIndex : maxSyncIndex,
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

  async readSession(projectId: string, sessionId: string, afterIndex = 0): Promise<RegistrySessionReadResponse> {
    return this.readSessionByMethod(projectId, sessionId, afterIndex, 'session.read');
  }

  async createSession(projectId: string, title?: string): Promise<{ok: boolean; session: RegistrySessionSummary}> {
    const resp = await this.client.request({
      method: 'session.new',
      projectId,
      payload: title?.trim() ? {title: title.trim()} : {},
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

  async markSessionRead(projectId: string, sessionId: string): Promise<{ok: boolean}> {
    const resp = await this.client.request({
      method: 'session.markRead',
      projectId,
      payload: {sessionId},
      timeoutMs: 15000,
    });
    const body = (resp.payload ?? {}) as {ok?: boolean};
    return {
      ok: body.ok ?? false,
    };
  }

  async respondToSessionPermission(projectId: string, payload: {sessionId: string; requestId: number; optionId: string}): Promise<{ok: boolean}> {
    const resp = await this.client.request({
      method: 'chat.permission.respond',
      projectId,
      payload: {
        chatId: payload.sessionId,
        requestId: payload.requestId,
        optionId: payload.optionId,
      },
      timeoutMs: 15000,
    });
    const body = (resp.payload ?? {}) as {ok?: boolean};
    return {
      ok: body.ok ?? false,
    };
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








