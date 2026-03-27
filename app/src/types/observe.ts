export type RegistryMessageType = 'request' | 'response' | 'error' | 'event';

export interface RegistryEnvelope<TPayload = unknown> {
  version: string;
  requestId?: string;
  type: RegistryMessageType;
  method?: string;
  projectId?: string;
  payload?: TPayload;
  error?: {
    code?: string;
    message?: string;
    details?: unknown;
  };
}

export interface RegistryProject {
  projectId: string;
  name: string;
  online?: boolean;
}

export interface RegistryFsEntry {
  name: string;
  path: string;
  kind: 'dir' | 'file';
  size?: number;
  mtime?: string;
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

export type ObserveMessageType = RegistryMessageType;
export type ObserveEnvelope<TPayload = unknown> = RegistryEnvelope<TPayload>;
export type ObserveProject = RegistryProject;
export type ObserveFsEntry = RegistryFsEntry;
export type ObserveGitCommit = RegistryGitCommit;
export type ObserveGitCommitFile = RegistryGitCommitFile;
export type ObserveGitFileDiff = RegistryGitFileDiff;
