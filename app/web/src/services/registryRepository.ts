import { RegistryClient, type RegistryDebugSink } from './registryClient';
import type {RegistryDebugConnection} from '../debug/registryDebug';
import {
  decodeSessionTurnToMessage,
  normalizeSessionReadPayload,
} from '../chat/chatWire';
import type {
  RegistryEnvelope,
  RegistryFsInfo,
  RegistryFsListResponse,
  RegistryFsReadResponse,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitFileDiff,
  RegistryGitStatus,
  RegistryHub,
  RegistryLocalReadCandidate,
  RegistryNpmCommandResponse,
  RegistryNpmHubSnapshot,
  RegistryNpmPackage,
  RegistryProject,
  RegistryProjectAgentProfile,
  RegistryProjectListResponse,
  RegistryPortRelayEnablePayload,
  RegistryPortRelaySnapshot,
  RegistryResumableSession,
  RegistrySessionConfigOption,
  RegistrySessionConfigOptionValue,
  RegistrySessionCommand,
  RegistrySessionMessage,
  RegistrySessionMessageEventPayload,
  RegistrySessionReadResponse,
  RegistrySessionSummary,
  RegistrySessionTurn,
  RegistrySkillCommandResponse,
  RegistrySkillInstallPayload,
  RegistrySkillScopePayload,
  RegistrySyncCheckPayload,
  RegistrySyncCheckResponse,
  RegistryTokenProvider,
  RegistryDeepSeekTokenStats,
  RegistryTokenScanResult,
  RegistryWheelMakerUpdateResponse,
  RegistryWorkingTreeFileDiff,
} from '../types/registry';

export type LocalReadProofResponse = {
  endpointId?: string;
  nonce?: string;
  signature?: string;
  proofPublicKey?: string;
  proofFingerprint?: string;
};

export type LocalReadProofVerifier = (input: {
  candidate: RegistryLocalReadCandidate;
  response: LocalReadProofResponse;
  nonce: string;
}) => Promise<boolean>;

export type LocalReadInitOptions = {
  createNonce?: () => string;
  verifyProof?: LocalReadProofVerifier;
};

function normalizeAgentType(agentType: unknown): string | undefined {
  if (typeof agentType !== 'string') {
    return undefined;
  }
  const normalized = agentType.trim();
  if (!normalized) {
    return undefined;
  }
  return normalized;
}

function normalizeAgentTypes(agentTypes: unknown): string[] {
  if (!Array.isArray(agentTypes)) {
    return [];
  }
  return agentTypes
    .map(item => normalizeAgentType(item))
    .filter((item): item is string => !!item);
}

function normalizeNpmPackage(raw: unknown): RegistryNpmPackage | null {
  if (!raw || typeof raw !== 'object') {
    return null;
  }
  const input = raw as Record<string, unknown>;
  const packageName = typeof input.packageName === 'string' ? input.packageName : '';
  if (!packageName) {
    return null;
  }
  return {
    packageName,
    displayName: typeof input.displayName === 'string' ? input.displayName : packageName,
    agentTypes: normalizeAgentTypes(input.agentTypes),
    kind: input.kind === 'deprecated' ? 'deprecated' : 'runtime',
    installed: input.installed === true,
    installedVersion: typeof input.installedVersion === 'string' ? input.installedVersion : '',
    latestVersion: typeof input.latestVersion === 'string' ? input.latestVersion : '',
    status: typeof input.status === 'string' ? input.status as RegistryNpmPackage['status'] : 'latest_unknown',
    error: typeof input.error === 'string' ? input.error : '',
    canInstall: input.canInstall === true,
    canUpdate: input.canUpdate === true,
    canUninstall: input.canUninstall === true,
  };
}

function normalizeNpmHubSnapshot(raw: unknown, hubId: string): RegistryNpmHubSnapshot | undefined {
  if (!raw || typeof raw !== 'object') {
    return undefined;
  }
  const input = raw as Record<string, unknown>;
  const packages = Array.isArray(input.packages)
    ? input.packages.map(item => normalizeNpmPackage(item)).filter((item): item is RegistryNpmPackage => !!item)
    : [];
  return {
    hubId: typeof input.hubId === 'string' && input.hubId ? input.hubId : hubId,
    nodeVersion: typeof input.nodeVersion === 'string' ? input.nodeVersion : '',
    npmVersion: typeof input.npmVersion === 'string' ? input.npmVersion : '',
    npmPrefix: typeof input.npmPrefix === 'string' ? input.npmPrefix : '',
    warning: typeof input.warning === 'string' ? input.warning : '',
    error: typeof input.error === 'string' ? input.error : '',
    packages,
  };
}

function normalizeNpmCommandResponse(raw: unknown, hubId: string): RegistryNpmCommandResponse {
  const input = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {};
  return {
    ok: input.ok === true,
    accepted: input.accepted === true ? true : undefined,
    updatedAt: typeof input.updatedAt === 'string' ? input.updatedAt : undefined,
    hub: normalizeNpmHubSnapshot(input.hub, hubId),
    operation: (input.operation ?? null) as RegistryNpmCommandResponse['operation'],
  };
}

function base64ToArrayBuffer(value: string): ArrayBuffer {
  const binary = globalThis.atob(value);
  const buffer = new ArrayBuffer(binary.length);
  const bytes = new Uint8Array(buffer);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return buffer;
}

function createLocalReadNonce(): string {
  const crypto = globalThis.crypto;
  if (!crypto?.getRandomValues) {
    throw new Error('local read proof requires WebCrypto random values');
  }
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes)
    .map(byte => byte.toString(16).padStart(2, '0'))
    .join('');
}

export async function verifyLocalReadProof(input: {
  candidate: RegistryLocalReadCandidate;
  response: LocalReadProofResponse;
  nonce: string;
}): Promise<boolean> {
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) {
    return false;
  }
  if (
    input.response.endpointId !== input.candidate.endpointId ||
    input.response.nonce !== input.nonce ||
    input.response.proofPublicKey !== input.candidate.proofPublicKey ||
    input.response.proofFingerprint !== input.candidate.proofFingerprint ||
    !input.response.signature
  ) {
    return false;
  }
  try {
    const key = await subtle.importKey(
      'raw',
      base64ToArrayBuffer(input.candidate.proofPublicKey),
      {name: 'Ed25519'} as AlgorithmIdentifier,
      false,
      ['verify'],
    );
    const data = new TextEncoder().encode(`${input.candidate.endpointId}\n${input.nonce}`);
    return await subtle.verify(
      {name: 'Ed25519'} as AlgorithmIdentifier,
      key,
      base64ToArrayBuffer(input.response.signature),
      data,
    );
  } catch {
    return false;
  }
}

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
      agentType: normalizeAgentType(input.agentType),
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
      sessionId?: unknown;
      session?: unknown;
      latestTurnIndex?: unknown;
      turns?: unknown[];
    };
    const latestTurnIndex = typeof payload.latestTurnIndex === 'number' && Number.isFinite(payload.latestTurnIndex)
      ? Math.max(0, Math.trunc(payload.latestTurnIndex))
      : 0;
    const normalized = normalizeSessionReadPayload(
      payload,
      sessionId,
      raw => this.normalizeSessionSummary(raw),
    );
    if (!normalized) {
      return {
        sessionId: '',
        turns: [],
        messages: [],
        latestTurnIndex,
      };
    }
    if (normalized.session) {
      normalized.session.latestTurnIndex = normalized.latestTurnIndex;
    }
    const normalizedTurns: RegistrySessionTurn[] = normalized.turns;
    const normalizedMessages: RegistrySessionMessage[] = normalizedTurns
      .map(turn => decodeSessionTurnToMessage(normalized.sessionId, turn))
      .filter((item): item is RegistrySessionMessage => !!item);

    return {
      sessionId: normalized.sessionId,
      turns: normalizedTurns,
      ...(normalized.session ? {session: normalized.session} : {}),
      messages: normalizedMessages,
      latestTurnIndex: normalized.latestTurnIndex,
    };
  }
  async initialize(url: string, token?: string): Promise<void> {
    await this.client.connect(url);
    await this.client.connectInit({
      clientName: 'wheelmaker-web',
      clientVersion: '0.1.0',
      protocolVersion: '2.3',
      role: 'client',
      token: token?.trim() ?? '',
    });
  }

  async initializeLocalRead(
    url: string,
    token: string,
    hubId: string,
    candidate: RegistryLocalReadCandidate,
    options: LocalReadInitOptions = {},
  ): Promise<void> {
    await this.client.connect(url);
    const nonce = options.createNonce?.() ?? createLocalReadNonce();
    const proofResp = await this.client.request({
      method: 'local_read.proof',
      payload: {
        endpointId: candidate.endpointId,
        nonce,
      },
    });
    const proofPayload = (proofResp.payload ?? {}) as LocalReadProofResponse;
    const verify = options.verifyProof ?? verifyLocalReadProof;
    const verified = await verify({candidate, response: proofPayload, nonce});
    if (!verified) {
      this.client.close();
      throw new Error('local read proof verification failed');
    }
    await this.client.connectInit({
      clientName: 'wheelmaker-web',
      clientVersion: '0.1.0',
      protocolVersion: '2.3',
      role: 'local_read',
      hubId,
      token: token.trim(),
    });
  }

  async listProjectSnapshot(): Promise<RegistryProjectListResponse> {
    const resp = await this.client.request({
      method: 'project.list',
      payload: {},
    });
    const payload = (resp.payload ?? {}) as { projects?: RegistryProject[]; hubs?: RegistryHub[] };
    const projects = (payload.projects ?? [])
      .filter(project => !!project.projectId)
      .map(project => ({
        ...project,
        agent: normalizeAgentType(project.agent),
        agents: Array.isArray(project.agents)
          ? project.agents
              .filter((item): item is string => typeof item === 'string')
              .map(item => normalizeAgentType(item))
              .filter((item): item is string => !!item)
          : undefined,
        agentProfiles: Array.isArray(project.agentProfiles)
          ? project.agentProfiles
              .map((item): RegistryProjectAgentProfile | null => {
                const name = normalizeAgentType(item?.name) ?? '';
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
    const seenHubIds = new Set<string>();
    const normalizeLocalRead = (raw: unknown): RegistryLocalReadCandidate | undefined => {
      if (!raw || typeof raw !== 'object') {
        return undefined;
      }
      const input = raw as Record<string, unknown>;
      const endpointId = typeof input.endpointId === 'string' ? input.endpointId.trim() : '';
      const url = typeof input.url === 'string' ? input.url.trim() : '';
      const proofPublicKey = typeof input.proofPublicKey === 'string' ? input.proofPublicKey.trim() : '';
      const proofFingerprint = typeof input.proofFingerprint === 'string' ? input.proofFingerprint.trim() : '';
      if (!endpointId || !url || !proofPublicKey || !proofFingerprint) {
        return undefined;
      }
      return {endpointId, url, proofPublicKey, proofFingerprint};
    };
    const hubs = (payload.hubs ?? [])
      .map((hub): RegistryHub => {
        const localRead = normalizeLocalRead((hub as {localRead?: unknown})?.localRead);
        return {
          hubId: typeof hub?.hubId === 'string' ? hub.hubId.trim() : '',
          ...(localRead ? {localRead} : {}),
        };
      })
      .filter(hub => {
        if (!hub.hubId || seenHubIds.has(hub.hubId)) {
          return false;
        }
        seenHubIds.add(hub.hubId);
        return true;
      });
    return {projects, hubs};
  }

  async listProjects(): Promise<RegistryProject[]> {
    return (await this.listProjectSnapshot()).projects;
  }

  async getPortRelayStatus(): Promise<RegistryPortRelaySnapshot> {
    const resp = await this.client.request({
      method: 'relay.status',
      payload: {},
    });
    return (resp.payload ?? {ok: true, enabled: false, status: 'Disabled'}) as RegistryPortRelaySnapshot;
  }

  async enablePortRelay(payload: RegistryPortRelayEnablePayload): Promise<RegistryPortRelaySnapshot> {
    const resp = await this.client.request({
      method: 'relay.enable',
      payload,
      timeoutMs: 15000,
    });
    return (resp.payload ?? {}) as RegistryPortRelaySnapshot;
  }

  async disablePortRelay(): Promise<RegistryPortRelaySnapshot> {
    const resp = await this.client.request({
      method: 'relay.disable',
      payload: {},
    });
    return (resp.payload ?? {ok: true, enabled: false, status: 'Disabled'}) as RegistryPortRelaySnapshot;
  }

  async regeneratePortRelayAccessCode(accessCode: string): Promise<RegistryPortRelaySnapshot> {
    const resp = await this.client.request({
      method: 'relay.regenerateAccessCode',
      payload: {accessCode},
    });
    return (resp.payload ?? {}) as RegistryPortRelaySnapshot;
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
    agentType = normalizeAgentType(agentType) ?? agentType.trim();
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

  async cancelSession(projectId: string, sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    const resp = await this.client.request({
      method: 'session.cancel',
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

  async archiveSession(projectId: string, sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    const resp = await this.client.request({
      method: 'session.archive',
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

  async deleteSession(projectId: string, sessionId: string): Promise<{ok: boolean; sessionId: string}> {
    const resp = await this.client.request({
      method: 'session.delete',
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

  async renameSession(projectId: string, sessionId: string, title: string): Promise<{ok: boolean; sessionId: string; session: RegistrySessionSummary}> {
    const resp = await this.client.request({
      method: 'session.rename',
      projectId,
      payload: {sessionId, title},
      timeoutMs: 15000,
    });
    const body = (resp.payload ?? {}) as {ok?: boolean; sessionId?: string; session?: unknown};
    const session = this.normalizeSessionSummary(body.session) ?? {
      sessionId,
      title,
      preview: '',
      updatedAt: '',
      messageCount: 0,
    };
    return {
      ok: body.ok ?? false,
      sessionId: body.sessionId ?? session.sessionId ?? sessionId,
      session,
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
    agentType = normalizeAgentType(agentType) ?? agentType.trim();
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
    agentType = normalizeAgentType(agentType) ?? agentType.trim();
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

  async scanNpmPackages(hubId: string): Promise<RegistryNpmCommandResponse> {
    const resp = await this.client.request({
      method: 'cmd.npm',
      payload: {action: 'scan', hubId},
      timeoutMs: 60000,
    });
    return normalizeNpmCommandResponse(resp.payload, hubId);
  }

  async installNpmPackage(hubId: string, packageName: string, version = 'latest'): Promise<RegistryNpmCommandResponse> {
    const resp = await this.client.request({
      method: 'cmd.npm',
      payload: {
        action: 'install',
        hubId,
        packageName,
        version,
      },
    });
    return normalizeNpmCommandResponse(resp.payload, hubId);
  }

  async uninstallNpmPackage(hubId: string, packageName: string): Promise<RegistryNpmCommandResponse> {
    const resp = await this.client.request({
      method: 'cmd.npm',
      payload: {
        action: 'uninstall',
        hubId,
        packageName,
      },
    });
    return normalizeNpmCommandResponse(resp.payload, hubId);
  }

  async queryWheelMakerUpdate(hubId: string): Promise<RegistryWheelMakerUpdateResponse> {
    const resp = await this.client.request({
      method: 'cmd.update',
      payload: {action: 'query', hubId},
      timeoutMs: 60000,
    });
    return (resp.payload ?? {}) as RegistryWheelMakerUpdateResponse;
  }

  async requestWheelMakerUpdatePublish(hubId: string): Promise<RegistryWheelMakerUpdateResponse> {
    const resp = await this.client.request({
      method: 'cmd.update',
      payload: {action: 'update-publish', hubId},
    });
    return (resp.payload ?? {}) as RegistryWheelMakerUpdateResponse;
  }

  async scanSkills(hubId: string): Promise<RegistrySkillCommandResponse> {
    const resp = await this.client.request({
      method: 'cmd.skills',
      payload: {action: 'scan', hubId},
      timeoutMs: 60000,
    });
    return (resp.payload ?? {}) as RegistrySkillCommandResponse;
  }

  async listSkillsSource(hubId: string, source: string): Promise<RegistrySkillCommandResponse> {
    const resp = await this.client.request({
      method: 'cmd.skills',
      payload: {action: 'list', hubId, source},
      timeoutMs: 60000,
    });
    return (resp.payload ?? {}) as RegistrySkillCommandResponse;
  }

  async installSkills(payload: RegistrySkillInstallPayload): Promise<RegistrySkillCommandResponse> {
    const resp = await this.client.request({
      method: 'cmd.skills',
      payload: {action: 'install', ...payload},
      timeoutMs: 60000,
    });
    return (resp.payload ?? {}) as RegistrySkillCommandResponse;
  }

  async uninstallSkills(payload: RegistrySkillScopePayload): Promise<RegistrySkillCommandResponse> {
    const resp = await this.client.request({
      method: 'cmd.skills',
      payload: {action: 'uninstall', ...payload},
      timeoutMs: 60000,
    });
    return (resp.payload ?? {}) as RegistrySkillCommandResponse;
  }

  async updateSkills(payload: RegistrySkillScopePayload): Promise<RegistrySkillCommandResponse> {
    const resp = await this.client.request({
      method: 'cmd.skills',
      payload: {action: 'update', ...payload},
      timeoutMs: 60000,
    });
    return (resp.payload ?? {}) as RegistrySkillCommandResponse;
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

export const createRegistryRepository = (
  debugSink?: RegistryDebugSink,
  debugConnection: RegistryDebugConnection = 'Remote',
): RegistryRepository => {
  return new RegistryRepository(new RegistryClient(8000, debugSink, debugConnection));
};

export type RegistryResponse<TPayload> = RegistryEnvelope<TPayload>;








