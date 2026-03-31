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
  changedPaths?: string[];
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
