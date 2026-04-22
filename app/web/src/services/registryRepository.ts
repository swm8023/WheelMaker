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
  RegistrySessionPrompt,
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
    const syncSubIndex = typeof input.syncSubIndex === 'number' && Number.isFinite(input.syncSubIndex)
      ? input.syncSubIndex
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
      syncSubIndex,
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

  private extractTextFromACPContent(content: unknown): string {
    if (typeof content === 'string') {
      return content.trim();
    }
    if (!Array.isArray(content)) {
      return '';
    }
    const chunks: string[] = [];
    for (const item of content) {
      if (!item || typeof item !== 'object') continue;
      const entry = item as Record<string, unknown>;
      if (typeof entry.text === 'string' && entry.text.trim()) {
        chunks.push(entry.text.trim());
      }
    }
    return chunks.join('\n').trim();
  }

  private normalizeSessionWireMessage(raw: unknown, fallbackSessionId: string): RegistrySessionMessage | null {
    if (!raw || typeof raw !== 'object') {
      return null;
    }
    const input = raw as Record<string, unknown>;
    if (typeof input.messageId === 'string' && input.messageId.trim().length > 0) {
      return this.normalizeSessionMessage(input, fallbackSessionId);
    }

    const sessionId = typeof input.sessionId === 'string' && input.sessionId.trim().length > 0
      ? input.sessionId.trim()
      : fallbackSessionId;
    if (!sessionId) {
      return null;
    }
    const turnId = typeof input.turnId === 'string' ? input.turnId.trim() : '';
    const promptIndex = typeof input.promptIndex === 'number' && Number.isFinite(input.promptIndex)
      ? Math.trunc(input.promptIndex)
      : 0;
    const turnIndex = typeof input.turnIndex === 'number' && Number.isFinite(input.turnIndex)
      ? Math.trunc(input.turnIndex)
      : 0;
    const updateIndex = typeof input.updateIndex === 'number' && Number.isFinite(input.updateIndex)
      ? Math.trunc(input.updateIndex)
      : 0;
    const content = typeof input.content === 'string' ? input.content.trim() : '';
    const messageId = turnId || `${sessionId}:${promptIndex}:${turnIndex}:${updateIndex}`;
    const now = new Date().toISOString();
    const out: RegistrySessionMessage = {
      messageId,
      sessionId,
      syncIndex: promptIndex > 0 ? promptIndex : undefined,
      syncSubIndex: turnIndex > 0 ? turnIndex : (updateIndex > 0 ? updateIndex : undefined),
      role: 'assistant',
      kind: 'message',
      text: '',
      status: 'done',
      createdAt: now,
      updatedAt: now,
    };
    if (content === '') {
      return out;
    }

    try {
      const doc = JSON.parse(content) as Record<string, unknown>;
      const method = typeof doc.method === 'string' ? doc.method.trim() : '';
      const payloadDoc = (doc.payload && typeof doc.payload === 'object')
        ? (doc.payload as Record<string, unknown>)
        : undefined;
      const params = (doc.params && typeof doc.params === 'object')
        ? (doc.params as Record<string, unknown>)
        : undefined;
      const result = (doc.result && typeof doc.result === 'object')
        ? (doc.result as Record<string, unknown>)
        : undefined;

      if (typeof doc.id === 'number' && Number.isFinite(doc.id)) {
        out.requestId = doc.id;
      }
      if (payloadDoc) {
        out.role = payloadDoc.role === 'user' || payloadDoc.role === 'assistant' || payloadDoc.role === 'system'
          ? payloadDoc.role
          : out.role;
        out.kind = payloadDoc.kind === 'text' || payloadDoc.kind === 'image' || payloadDoc.kind === 'thought' || payloadDoc.kind === 'tool' || payloadDoc.kind === 'permission' || payloadDoc.kind === 'prompt_result' || payloadDoc.kind === 'message'
          ? payloadDoc.kind
          : out.kind;
        out.status = payloadDoc.status === 'streaming' || payloadDoc.status === 'done' || payloadDoc.status === 'needs_action'
          ? payloadDoc.status
          : out.status;
        if (typeof payloadDoc.text === 'string') {
          out.text = payloadDoc.text;
        }
        if (typeof payloadDoc.requestId === 'number' && Number.isFinite(payloadDoc.requestId)) {
          out.requestId = payloadDoc.requestId;
        }
        if (Array.isArray(payloadDoc.blocks)) {
          out.blocks = payloadDoc.blocks as RegistrySessionMessage['blocks'];
        }
        if (Array.isArray(payloadDoc.options)) {
          out.options = payloadDoc.options as RegistrySessionMessage['options'];
        }
      }

      if (method === 'session.prompt') {
        const promptBlocks = Array.isArray(params?.prompt) ? params.prompt : [];
        out.role = 'user';
        if (!out.text) {
          out.text = this.extractTextFromACPContent(promptBlocks);
        }
        if (promptBlocks.length > 0) {
          out.blocks = promptBlocks as RegistrySessionMessage['blocks'];
        }
        const stopReason = (result?.stopReason && typeof result.stopReason === 'string')
          ? result.stopReason.trim()
          : '';
        if (!out.text && stopReason) {
          out.text = stopReason;
        }
      } else if (method === 'session.update') {
        const update = (params?.update && typeof params.update === 'object')
          ? (params.update as Record<string, unknown>)
          : undefined;
        const updateMethod = typeof update?.sessionUpdate === 'string' ? update.sessionUpdate.trim() : '';
        const updateText = this.extractTextFromACPContent(update?.content);
        if (updateMethod === 'user_message_chunk') {
          out.role = 'user';
          out.kind = 'message';
        } else if (updateMethod === 'agent_message_chunk') {
          out.role = 'assistant';
          out.kind = 'message';
        } else if (updateMethod === 'agent_thought_chunk') {
          out.role = 'assistant';
          out.kind = 'thought';
        } else if (updateMethod === 'tool_call' || updateMethod === 'tool_call_update') {
          out.role = 'system';
          out.kind = 'tool';
        }
        if (!out.text) {
          out.text = updateText;
        }
      } else if (method === 'request_permission') {
        const toolCall = (params?.toolCall && typeof params.toolCall === 'object')
          ? (params.toolCall as Record<string, unknown>)
          : undefined;
        const outcome = (result?.outcome && typeof result.outcome === 'object')
          ? (result.outcome as Record<string, unknown>)
          : undefined;
        out.role = 'system';
        out.kind = 'permission';
        if (!out.text && typeof toolCall?.title === 'string') {
          out.text = toolCall.title;
        }
        if (Array.isArray(params?.options) && (!out.options || out.options.length === 0)) {
          out.options = params.options as RegistrySessionMessage['options'];
        }
        if (typeof outcome?.outcome === 'string') {
          out.status = outcome.outcome === 'streaming' || outcome.outcome === 'done' || outcome.outcome === 'needs_action'
            ? outcome.outcome
            : out.status;
        } else {
          out.status = 'needs_action';
        }
      }
    } catch {
      out.text = content;
    }
    return out;
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
    afterSubIndex: number,
    method: 'session.read',
  ): Promise<RegistrySessionReadResponse> {
    const resp = await this.client.request({
      method,
      projectId,
      payload: afterIndex > 0 || afterSubIndex > 0 ? {sessionId, afterIndex, afterSubIndex} : {sessionId},
      timeoutMs: 15000,
    });
    const payload = (resp.payload ?? {}) as {
      session?: unknown;
      prompts?: unknown[];
      messages?: unknown[];
      lastIndex?: number;
      lastSubIndex?: number;
      lastPromptIndex?: number;
      lastPromptUpdateIndex?: number;
    };
    const normalizedSession = this.normalizeSessionSummary(payload.session) ?? {
      sessionId,
      title: sessionId,
      preview: '',
      updatedAt: '',
      messageCount: 0,
    };

    if (Array.isArray(payload.messages) && !Array.isArray(payload.prompts)) {
      const normalizedMessages = payload.messages
        .map(item => this.normalizeSessionWireMessage(item, normalizedSession.sessionId))
        .filter((item): item is RegistrySessionMessage => !!item);
      const maxSyncIndex = normalizedMessages.reduce((max, message) => Math.max(max, message.syncIndex ?? 0), 0);
      return {
        session: normalizedSession,
        prompts: [],
        messages: normalizedMessages,
        lastIndex: typeof payload.lastIndex === 'number' ? payload.lastIndex : maxSyncIndex,
        lastSubIndex: typeof payload.lastSubIndex === 'number' ? payload.lastSubIndex : 0,
      };
    }

    const normalizedPrompts: RegistrySessionPrompt[] = (Array.isArray(payload.prompts) ? payload.prompts : [])
      .map(raw => raw as Record<string, unknown>)
      .map(prompt => ({
        messageId: typeof prompt?.messageId === 'string' ? prompt.messageId : (typeof prompt?.promptId === 'string' ? prompt.promptId : ''),
        promptId: typeof prompt?.promptId === 'string' ? prompt.promptId : '',
        sessionId: typeof prompt?.sessionId === 'string' && prompt.sessionId.trim().length > 0 ? prompt.sessionId : normalizedSession.sessionId,
        promptIndex: Number(prompt?.promptIndex ?? 0),
        updateIndex: Number(prompt?.updateIndex ?? 0),
        title: typeof prompt?.title === 'string' ? prompt.title : '',
        stopReason: typeof prompt?.stopReason === 'string' ? prompt.stopReason : undefined,
        status: typeof prompt?.status === 'string' ? prompt.status : 'done',
        updatedAt: typeof prompt?.updatedAt === 'string' ? prompt.updatedAt : '',
        turns: (Array.isArray(prompt?.turns) ? prompt.turns : [])
          .map(turnRaw => turnRaw as Record<string, unknown>)
          .map(turn => ({
            turnId: typeof turn?.turnId === 'string' ? turn.turnId : '',
            promptIndex: Number(turn?.promptIndex ?? prompt?.promptIndex ?? 0),
            turnIndex: Number(turn?.turnIndex ?? 0),
            updateIndex: Number(turn?.updateIndex ?? 0),
            role: turn?.role as RegistrySessionPrompt['turns'][number]['role'],
            kind: turn?.kind as RegistrySessionPrompt['turns'][number]['kind'],
            text: typeof turn?.text === 'string' ? turn.text : undefined,
            status: turn?.status as RegistrySessionPrompt['turns'][number]['status'],
            requestId: typeof turn?.requestId === 'number' && Number.isFinite(turn.requestId) ? turn.requestId : undefined,
            toolCallId: typeof turn?.toolCallId === 'string' ? turn.toolCallId : undefined,
            blocks: Array.isArray(turn?.blocks) ? (turn.blocks as RegistrySessionMessage['blocks']) : undefined,
            options: Array.isArray(turn?.options) ? (turn.options as RegistrySessionMessage['options']) : undefined,
          }))
          .filter(turn => !!turn.turnId && turn.promptIndex > 0 && turn.turnIndex > 0 && turn.updateIndex > 0),
      }))
      .filter(prompt => !!prompt.messageId && !!prompt.promptId && prompt.promptIndex > 0 && prompt.updateIndex > 0)
      .sort((a, b) => a.promptIndex - b.promptIndex);

    const flattenedPromptMessages = normalizedPrompts.flatMap(prompt =>
      prompt.turns.map(turn => ({
        messageId: turn.turnId,
        sessionId: prompt.sessionId,
        syncIndex: prompt.promptIndex,
        syncSubIndex: turn.updateIndex,
        role: turn.role,
        kind: turn.kind,
        text: turn.text,
        status: turn.status,
        createdAt: prompt.updatedAt,
        updatedAt: prompt.updatedAt,
        requestId: turn.requestId,
        blocks: turn.blocks,
        options: turn.options,
      })),
    );
    const normalizedMessages = ((Array.isArray(payload.messages) && payload.messages.length > 0) ? payload.messages : flattenedPromptMessages)
      .map(item => this.normalizeSessionMessage(item, normalizedSession.sessionId))
      .filter((item): item is RegistrySessionMessage => !!item);
    const maxSyncIndex = normalizedMessages.reduce((max, message) => Math.max(max, message.syncIndex ?? 0), 0);
    const maxPromptIndex = normalizedPrompts.reduce((max, prompt) => Math.max(max, prompt.promptIndex), 0);
    const lastIndex = typeof payload.lastPromptIndex === 'number'
      ? payload.lastPromptIndex
      : (typeof payload.lastIndex === 'number' ? payload.lastIndex : Math.max(maxPromptIndex, maxSyncIndex));
    const lastSubIndex = typeof payload.lastPromptUpdateIndex === 'number'
      ? payload.lastPromptUpdateIndex
      : (typeof payload.lastSubIndex === 'number' ? payload.lastSubIndex : 0);
    return {
      session: normalizedSession,
      prompts: normalizedPrompts,
      messages: normalizedMessages,
      lastIndex,
      lastSubIndex,
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

  async readSession(projectId: string, sessionId: string, afterIndex = 0, afterSubIndex = 0): Promise<RegistrySessionReadResponse> {
    return this.readSessionByMethod(projectId, sessionId, afterIndex, afterSubIndex, 'session.read');
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
















