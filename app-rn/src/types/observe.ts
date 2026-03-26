export type ObserveMessageType = 'request' | 'response' | 'error' | 'event';

export interface ObserveEnvelope<TPayload = unknown> {
  version: string;
  requestId?: string;
  type: ObserveMessageType;
  method?: string;
  projectId?: string;
  payload?: TPayload;
  error?: {
    code?: string;
    message?: string;
    details?: unknown;
  };
}

export interface ObserveProject {
  projectId: string;
  name: string;
  online?: boolean;
}

export interface ObserveFsEntry {
  name: string;
  path: string;
  kind: 'dir' | 'file';
  size?: number;
  mtime?: string;
}
