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

export interface RegistrySessionContentBlock {
  type: 'text' | 'image';
  text?: string;
  mimeType?: string;
  data?: string;
}

export interface RegistrySessionMessage {
  sessionId: string;
  turnIndex: number;
  method: string;
  param: Record<string, unknown>;
  finished: boolean;
}

export interface RegistrySessionPlanEntry {
  content: string;
  status?: string;
}



export interface RegistrySessionConfigOptionValue {
  value: string;
  name?: string;
  description?: string;
}

export interface RegistrySessionConfigOption {
  id: string;
  name?: string;
  description?: string;
  category?: string;
  type?: string;
  currentValue?: string;
  options?: RegistrySessionConfigOptionValue[];
}

export interface RegistrySessionCommand {
  name: string;
  description?: string;
}

export interface RegistrySessionSummary {
  sessionId: string;
  title: string;
  preview: string;
  updatedAt: string;
  messageCount: number;
  unreadCount?: number;
  agentType?: string;
  latestTurnIndex?: number;
  configOptions?: RegistrySessionConfigOption[];
  commands?: RegistrySessionCommand[];
}



export interface RegistrySessionReadResponse {
  session?: RegistrySessionSummary;
  messages: RegistrySessionMessage[];
  latestTurnIndex: number;
}

export interface RegistryTokenProvider {
  id: string;
  name: string;
  authMode: string;
}

export interface RegistryDeepSeekBalanceInfo {
  currency: string;
  totalBalance: string;
  grantedBalance: string;
  toppedUpBalance: string;
}

export interface RegistryDeepSeekBalanceView {
  isAvailable: boolean;
  items: RegistryDeepSeekBalanceInfo[];
}

export interface RegistryDeepSeekUsageRow {
  bucket: string;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  cost: number;
}

export interface RegistryDeepSeekUsageView {
  rangeType: 'day' | 'month';
  month: string;
  rows: RegistryDeepSeekUsageRow[];
}

export interface RegistryDeepSeekTokenStats {
  ok: boolean;
  provider: string;
  rangeType: 'day' | 'month';
  month: string;
  updatedAt: string;
  balance: RegistryDeepSeekBalanceView;
  usage: RegistryDeepSeekUsageView;
  usageUnavailable: boolean;
  usageMessage?: string;
}

export interface RegistryTokenProviderAccount {
  id: string;
  alias: string;
  displayName: string;
  source: string;
  status: 'ok' | 'error';
  message?: string;
  email?: string;
  plan?: string;
  fiveHourLimit?: string;
  weeklyLimit?: string;
  premiumRequestsUsed?: number;
  premiumRequestsRemaining?: number;
  premiumRequestsMonth?: string;
  balance: RegistryDeepSeekBalanceView;
  usage: RegistryDeepSeekUsageView;
  usageUnavailable: boolean;
  usageMessage?: string;
  updatedAt?: string;
}

export interface RegistryTokenScanProvider {
  id: string;
  name: string;
  accounts: RegistryTokenProviderAccount[];
}

export interface RegistryTokenScanResult {
  ok: boolean;
  updatedAt: string;
  providers: RegistryTokenScanProvider[];
}

export interface RegistrySessionMessageEventPayload {
  sessionId: string;
  turnIndex: number;
  content: string;
  finished?: boolean;
}

export type RegistryChatContentBlock = RegistrySessionContentBlock;
export type RegistryChatMessage = RegistrySessionMessage;
export type RegistryChatSession = RegistrySessionSummary;
export type RegistryChatSessionReadResponse = RegistrySessionReadResponse;
export type RegistryChatMessageEventPayload = RegistrySessionMessageEventPayload;

export interface RegistryResumableSession {
  sessionId: string;
  title: string;
  preview?: string;
  updatedAt: string;
  messageCount: number;
  cwd: string;
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

export interface RegistryProjectAgentProfile {
  name: string;
  skills?: string[];
}

export interface RegistryProject {
  projectId: string;
  name: string;
  online: boolean;
  path: string;
  hubId?: string;
  agent?: string;
  agents?: string[];
  agentProfiles?: RegistryProjectAgentProfile[];
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
  returned?: number;
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


