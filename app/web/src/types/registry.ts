export type RegistryMessageType = 'request' | 'response' | 'error' | 'event';

export interface RegistryErrorPayload {
  code?: string;
  message?: string;
  details?: unknown;
}

export interface RegistryEnvelope<TPayload = unknown> {
  requestId?: number;
  type: RegistryMessageType;
  method?: string;
  projectId?: string;
  payload?: TPayload;
}

export interface RegistryProjectEventPayload {
  projectRev?: string;
  gitRev?: string;
  worktreeRev?: string;
  changedDomains?: string[];
  changedPaths?: string[];
}

export type RegistryChatMessageRole = 'user' | 'assistant' | 'system';
export type RegistryChatMessageKind = 'text' | 'image' | 'thought' | 'tool' | 'permission' | 'prompt_result' | 'message';
export type RegistryChatMessageStatus = 'streaming' | 'done' | 'needs_action';

export interface RegistryChatContentBlock {
  type: 'text' | 'image';
  text?: string;
  mimeType?: string;
  data?: string;
}

export interface RegistryChatPermissionOption {
  optionId: string;
  name: string;
  kind: string;
}

export interface RegistryChatMessage {
  messageId: string;
  chatId: string;
  sessionId?: string;
  role: RegistryChatMessageRole;
  kind: RegistryChatMessageKind;
  text: string;
  status: RegistryChatMessageStatus;
  createdAt: string;
  updatedAt: string;
  requestId?: number;
  blocks?: RegistryChatContentBlock[];
  options?: RegistryChatPermissionOption[];
}

export interface RegistryChatSession {
  chatId: string;
  sessionId?: string;
  title: string;
  preview: string;
  updatedAt: string;
  messageCount: number;
}

export interface RegistryChatSessionReadResponse {
  session: RegistryChatSession;
  messages: RegistryChatMessage[];
}

export interface RegistryChatMessageEventPayload {
  session: RegistryChatSession;
  message: RegistryChatMessage;
}

export interface RegistryGitWorkspaceChangedPayload {
  gitRev?: string;
  worktreeRev?: string;
  headSha?: string;
  dirty?: boolean;
}

export interface RegistrySyncCheckPayload {
  knownProjectRev?: string;
  knownGitRev?: string;
  knownWorktreeRev?: string;
}

export interface RegistrySyncCheckResponse {
  projectRev: string;
  gitRev: string;
  worktreeRev: string;
  staleDomains: string[];
}

export interface RegistryProjectGitState {
  branch: string;
  headSha: string;
  dirty: boolean;
  gitRev: string;
  worktreeRev: string;
}

export interface RegistryProject {
  projectId: string;
  name: string;
  online: boolean;
  path: string;
  hubId?: string;
  agent?: string;
  imType?: string;
  projectRev?: string;
  git?: RegistryProjectGitState;
}

export interface RegistryFsEntry {
  name: string;
  path: string;
  kind: 'dir' | 'file';
  size?: number;
  mtime?: string;
}

export interface RegistryFsListResponse {
  path: string;
  hash?: string;
  notModified: boolean;
  entries?: RegistryFsEntry[];
}

export interface RegistryFsInfo {
  path: string;
  kind: 'file' | 'dir';
  size?: number;
  isBinary?: boolean;
  mimeType?: string;
  totalLines?: number;
  tabSize?: number;
  entryCount?: number;
  hash?: string;
}

export interface RegistryFsReadResponse {
  path: string;
  hash?: string;
  notModified: boolean;
  isBinary?: boolean;
  mimeType?: string;
  encoding?: string;
  content?: string | null;
  size?: number;
  total?: number;
  offset?: number;
  returned?: number;
  hasMore?: boolean;
}

export interface RegistryGitCommit {
  sha: string;
  author: string;
  email: string;
  time: string;
  title: string;
}

export interface RegistryGitCommitFile {
  path: string;
  status: string;
  additions: number;
  deletions: number;
}

export interface RegistryGitFileDiff {
  sha: string;
  path: string;
  isBinary: boolean;
  diff: string;
  truncated: boolean;
}

export interface RegistryGitStatusEntry {
  path: string;
  status: string;
}

export interface RegistryGitStatus {
  dirty: boolean;
  worktreeRev: string;
  staged: RegistryGitStatusEntry[];
  unstaged: RegistryGitStatusEntry[];
  untracked: RegistryGitStatusEntry[];
}

export interface RegistryWorkingTreeFileDiff {
  path: string;
  scope: 'staged' | 'unstaged' | 'untracked';
  isBinary: boolean;
  diff: string;
  truncated: boolean;
}

export type RegistryConnectInitPayload = {
  clientName: string;
  clientVersion: string;
  protocolVersion: string;
  role: 'client' | 'hub';
  hubId?: string;
  token: string;
  ts?: number;
  nonce?: string;
};
