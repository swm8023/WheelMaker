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

export interface RegistrySessionTurn {
  turnIndex: number;
  content: string;
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
  running?: boolean;
  lastDoneTurnIndex?: number;
  lastDoneSuccess?: boolean;
  lastReadTurnIndex?: number;
  configOptions?: RegistrySessionConfigOption[];
  commands?: RegistrySessionCommand[];
}



export interface RegistrySessionReadResponse {
  sessionId: string;
  session?: RegistrySessionSummary;
  turns: RegistrySessionTurn[];
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

export type RegistryNpmPackageStatus =
  | 'checking_latest'
  | 'not_installed'
  | 'up_to_date'
  | 'update_available'
  | 'latest_unknown'
  | 'checking_failed'
  | 'deprecated'
  | 'installing'
  | 'updating'
  | 'uninstalling'
  | 'running'
  | 'succeeded'
  | 'failed';

export interface RegistryNpmPackage {
  packageName: string;
  displayName: string;
  agentTypes: string[];
  kind: 'runtime' | 'deprecated';
  installed: boolean;
  installedVersion: string;
  latestVersion: string;
  status: RegistryNpmPackageStatus;
  error: string;
  canInstall: boolean;
  canUpdate: boolean;
  canUninstall: boolean;
}

export interface RegistryNpmHubSnapshot {
  hubId: string;
  nodeVersion: string;
  npmVersion: string;
  npmPrefix: string;
  warning: string;
  error: string;
  packages: RegistryNpmPackage[];
}

export interface RegistryNpmOperation {
  running: boolean;
  action: 'scan_latest' | 'install' | 'install_many' | 'uninstall' | string;
  packageName: string;
  packageNames?: string[];
  version: string;
  status: RegistryNpmPackageStatus;
  startedAt: string;
  finishedAt: string;
  exitCode: number | null;
  errorSummary: string;
  message?: string;
}

export interface RegistryNpmCommandResponse {
  ok: boolean;
  accepted?: boolean;
  updatedAt?: string;
  hub?: RegistryNpmHubSnapshot;
  operation: RegistryNpmOperation | null;
}

export type RegistryWheelMakerUpdateStatus =
  | 'up_to_date'
  | 'update_available'
  | 'update_pending'
  | 'not_published'
  | 'checking_failed'
  | 'ahead_of_remote'
  | 'diverged';

export interface RegistryWheelMakerRelease {
  schemaVersion: number;
  repo: string;
  branch: string;
  remote: string;
  sha: string;
  publishedAt: string;
}

export interface RegistryWheelMakerGitSnapshot {
  branch: string;
  remote: string;
  currentSha: string;
  latestSha: string;
  currentCommittedAt?: string;
  latestCommittedAt?: string;
  behindCount: number;
  aheadCount: number;
  dirty: boolean;
}

export interface RegistryWheelMakerUpdateResponse {
  ok: boolean;
  accepted?: boolean;
  requestedAt?: string;
  status: RegistryWheelMakerUpdateStatus | string;
  hubId: string;
  release?: RegistryWheelMakerRelease;
  git?: RegistryWheelMakerGitSnapshot;
  pendingSignal: boolean;
  canUpdatePublish: boolean;
  error?: string;
}

export type RegistrySkillScope = 'hub' | 'project';

export interface RegistrySkillSnapshot {
  name: string;
  path?: string;
  category: string;
  categoryKey: string;
  managed?: boolean;
  agents?: string[];
}

export interface RegistrySkillScopeSnapshot {
  scope: RegistrySkillScope;
  skills: RegistrySkillSnapshot[];
}

export interface RegistrySkillProjectSnapshot {
  projectName: string;
  projectId?: string;
  online: boolean;
  path?: string;
  skills: RegistrySkillSnapshot[];
  error?: string;
}

export interface RegistrySkillSourceCandidate {
  name: string;
  description?: string;
  category: string;
  categoryKey: string;
}

export interface RegistrySkillOperation {
  running: boolean;
  action: 'install' | 'uninstall' | 'update' | string;
  scope?: RegistrySkillScope;
  projectName?: string;
  source?: string;
  skills?: string[];
  includeProjects?: boolean;
  status: 'running' | 'succeeded' | 'failed' | string;
  startedAt: string;
  finishedAt?: string;
  exitCode: number | null;
  errorSummary?: string;
  message?: string;
}

export interface RegistrySkillCommandResponse {
  ok: boolean;
  accepted?: boolean;
  hubId: string;
  updatedAt?: string;
  source?: string;
  scope?: RegistrySkillScope;
  projectName?: string;
  hubSkills?: RegistrySkillScopeSnapshot;
  projects?: RegistrySkillProjectSnapshot[];
  skills?: RegistrySkillSnapshot[];
  candidates?: RegistrySkillSourceCandidate[];
  operation?: RegistrySkillOperation | null;
  message?: string;
  errorSummary?: string;
}

export interface RegistrySkillInstallPayload {
  hubId: string;
  scope: RegistrySkillScope;
  projectName?: string;
  source: string;
  skills: string[];
}

export interface RegistrySkillScopePayload {
  hubId: string;
  scope: RegistrySkillScope;
  projectName?: string;
  skills?: string[];
  includeProjects?: boolean;
}

export interface RegistrySessionMessageEventPayload {
  sessionId: string;
  turn: RegistrySessionTurn;
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

export interface RegistryLocalReadCandidate {
  endpointId: string;
  url: string;
  proofPublicKey: string;
  proofFingerprint: string;
}

export interface RegistryHub {
  hubId: string;
  localRead?: RegistryLocalReadCandidate;
}

export interface RegistryProjectListResponse {
  projects: RegistryProject[];
  hubs: RegistryHub[];
}

export type RegistryPortRelayStatus = 'Disabled' | 'Opening' | 'Up' | 'Error';

export interface RegistryPortRelaySnapshot {
  ok: boolean;
  enabled: boolean;
  status: RegistryPortRelayStatus;
  listenPort?: number;
  hubId?: string;
  targetHost?: string;
  targetPort?: number;
  relayUrl?: string;
  accessCodeGeneration?: number;
  tunnelConnectedAt?: string;
  error?: string;
}

export interface RegistryPortRelayEnablePayload {
  listenPort: number;
  hubId: string;
  targetHost: string;
  targetPort: number;
  accessCode: string;
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
  role: 'client' | 'hub' | 'local_read';
  hubId?: string;
  token: string;
  ts?: number;
  nonce?: string;
};


