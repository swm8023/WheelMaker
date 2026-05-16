import React, { useCallback, useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import setiThemeJson from '@codingame/monaco-vscode-theme-seti-default-extension/resources/vs-seti-icon-theme.json';
import setiFontUrl from '@codingame/monaco-vscode-theme-seti-default-extension/resources/seti.woff';
import '@vscode/codicons/dist/codicon.css';
import '@fontsource/ibm-plex-sans/400.css';
import '@fontsource/ibm-plex-sans/500.css';
import '@fontsource/ibm-plex-sans/600.css';
import '@fontsource/jetbrains-mono/400.css';
import ReactMarkdown, { type Components } from 'react-markdown';
import rehypeKatex from 'rehype-katex';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import mermaid from 'mermaid';
import 'katex/dist/katex.min.css';

declare const require: (id: string) => any;

import { getDefaultRegistryAddress, toRegistryWsUrl } from './runtime';
import { initializePWAFoundation } from './pwa';
import { ResponsiveShell } from './shell/ResponsiveShell';
import {
  getLatestSessionReadCursor,
  isFinishedChatMessage,
  needsPromptTurnRefresh,
  reconcileCachedSessionReadCursor,
  reconcileSessionReadMessages,
  replaceSessionMessages,
  shouldRequestSessionReadForIncomingTurn,
} from './chatSync';
import { compareUpdatedAtDesc, formatPromptDurationMs } from './sessionTime';
import {
  isChatSessionRunningMessage,
  resolveChatSessionVisualState as resolveChatSessionVisualStateValue,
  type ChatSessionVisualState,
} from './chatSessionState';
import {
  chatSessionKeyFromParts,
  encodeChatSessionKey,
  type ChatSessionKey,
} from './chat/chatSessionKey';
import {
  createLatestTurnWindow,
  expandTurnWindowEarlier,
  expandTurnWindowLater,
  followLatestTurnWindow,
  sliceTurnsForWindow,
  type ChatTurnWindow,
} from './chat/chatTurnWindow';
import { buildPromptDoneCopyRange } from './chat/chatCopyRange';
import { resolvePromptTurnStatus, type ChatPromptStatus } from './chat/chatPromptStatus';
import { RegistryWorkspaceService } from './services/registryWorkspaceService';
import { sortProjectsByPin, togglePinnedProjectId } from './services/projectNavigation';
import { resolveLayoutMode } from './services/responsiveLayout';
import {
  buildTokenStatCards,
  type TokenProviderSectionView,
  type TokenStatCardView,
} from './tokenStatsView';
import {
  CODE_FONT_OPTIONS,
  CODE_THEME_OPTIONS,
  CODE_THEME_OPTION_GROUPS,
  DEFAULT_CODE_FONT,
  DEFAULT_CODE_FONT_SIZE,
  DEFAULT_CODE_LINE_HEIGHT,
  DEFAULT_CODE_TAB_SIZE,
  DEFAULT_CODE_THEME,
  isCodeFontId,
  isCodeThemeId,
  renderShikiDiffHtml,
  renderShikiHtml,
  resolveCodeFontFamily,
  type CodeFontId,
  type CodeThemeId,
  type DiffRenderLine,
} from './services/shikiRenderer';
import { WorkspaceController } from './services/workspaceController';
import { WorkspaceStore } from './services/workspaceStore';
import {
  DESKTOP_SIDEBAR_WIDTH_DEFAULT,
  DESKTOP_SIDEBAR_WIDTH_MAX,
  DESKTOP_SIDEBAR_WIDTH_MIN,
  createWorkspaceUiState,
  sanitizeDesktopSidebarWidth,
  workspaceUiReducer,
  type WorkspaceUiStateValue,
} from './services/workspaceUiState';
import type { PersistedFloatingControlSlot } from './services/workspacePersistence';
import type {
  RegistryChatContentBlock,
  RegistryChatMessage,
  RegistryChatMessageEventPayload,
  RegistryChatSession,
  RegistryResumableSession,
  RegistrySessionContentBlock,
  RegistrySessionConfigOption,
  RegistryFsEntry,
  RegistryFsInfo,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitStatus,
  RegistryProject,
  RegistryTokenScanResult,
} from './types/registry';
import './styles.css';

type Tab = 'chat' | 'file' | 'git';
type ThemeMode = 'dark' | 'light';
type DirEntries = Record<string, RegistryFsEntry[]>;
type GitDiffSource = 'commit' | 'worktree';
type ChatAttachment = {
  id: string;
  name: string;
  mimeType: string;
  data: string;
};
type WideProjectActionMenuState = {
  projectId: string;
  kind: 'new' | 'resume';
  phase: 'agents' | 'sessions';
  agentType: string;
};
type MobileProjectActionMenuState = WideProjectActionMenuState;
type ProjectSessionActionMenuState = {
  projectId: string;
  sessionId: string;
};
type SettingsDetailView = 'tokenStats' | null;
type ChatComposerDraft = {
  text: string;
  attachments: ChatAttachment[];
};
type PendingChatPrompt = {
  sessionId: string;
  blocks: RegistryChatContentBlock[];
  createdAt: string;
  turnIndex: number;
};
type ChatSlashCommandOption = {
  name: string;
  description?: string;
};
type WorkingTreeFileEntry = {
  path: string;
  status: string;
  scope: 'staged' | 'unstaged' | 'untracked';
};
type SessionFlagMap = Record<string, true>;
type FloatingDragState = {
  active: boolean;
  pressing: boolean;
  pointerId: number;
  originY: number;
  startTop: number;
  currentTop: number;
  cooldownUntil: number;
};
type DesktopSidebarResizeState = {
  pointerId: number;
  originX: number;
  startWidth: number;
  currentWidth: number;
};
type SetiThemeSection = {
  file: string;
  fileExtensions?: Record<string, string>;
  fileNames?: Record<string, string>;
};
type SetiIconDefinition = {
  fontCharacter?: string;
  fontColor?: string;
};
type SetiTheme = {
  iconDefinitions: Record<string, SetiIconDefinition>;
  file: string;
  fileExtensions?: Record<string, string>;
  fileNames?: Record<string, string>;
  light?: SetiThemeSection;
};
type SetiResolvedIcon = {
  glyph: string;
  color: string;
};
type GitDiffChange =
  | { type: 'insert'; content: string; lineNumber: number }
  | { type: 'delete'; content: string; lineNumber: number }
  | {
      type: 'normal';
      content: string;
      oldLineNumber: number;
      newLineNumber: number;
    };
type GitDiffFile = {
  hunks?: Array<{
    changes?: GitDiffChange[];
  }>;
};
type GitDiffParser = {
  parse: (source: string) => GitDiffFile[];
};

const WORKING_TREE_COMMIT_ID = '__WORKING_TREE__';
const LARGE_FILE_CONFIRM_BYTES = 2 * 1024 * 1024;
const gitdiffParser = require('gitdiff-parser') as GitDiffParser;

type ThinkingBlockProps = {
  content: string;
  isStreaming: boolean;
};

function ThinkingBlock({ content, isStreaming }: ThinkingBlockProps) {
  const [expanded, setExpanded] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);
  const [contentHeight, setContentHeight] = useState(0);

  useEffect(() => {
    if (contentRef.current) {
      setContentHeight(contentRef.current.scrollHeight);
    }
  }, [content, expanded]);

  // Auto-collapse when streaming finishes
  const wasStreamingRef = useRef(isStreaming);
  useEffect(() => {
    if (wasStreamingRef.current && !isStreaming) {
      setExpanded(false);
    }
    wasStreamingRef.current = isStreaming;
  }, [isStreaming]);

  const summaryText = useMemo(() => {
    if (isStreaming) return '';
    const firstLine = content.split('\n').find(l => l.trim().length > 0) || '';
    return firstLine.length > 120 ? firstLine.slice(0, 120) + '…' : firstLine;
  }, [content, isStreaming]);

  return (
    <div
      className={`thinking-block ${isStreaming ? 'streaming' : 'done'} ${
        expanded ? 'expanded' : ''
      }`}
    >
      <button
        className="thinking-header"
        onClick={() => !isStreaming && setExpanded(v => !v)}
        disabled={isStreaming}
        aria-expanded={expanded}
      >
        <span className="thinking-icon codicon codicon-sparkle" />
        {isStreaming ? (
          <span className="thinking-title streaming-text">
            Thinking
            <span className="thinking-dots">
              <span>.</span>
              <span>.</span>
              <span>.</span>
            </span>
          </span>
        ) : (
          <span className="thinking-title summary-text">{summaryText}</span>
        )}
        {!isStreaming && (
          <span
            className={`thinking-chevron codicon ${
              expanded ? 'codicon-chevron-up' : 'codicon-chevron-down'
            }`}
          />
        )}
      </button>
      <div
        className="thinking-body"
        style={{ maxHeight: expanded ? contentHeight + 16 : 0 }}
      >
        <div className="thinking-content" ref={contentRef}>
          {content}
        </div>
      </div>
    </div>
  );
}

const pwaFoundation = initializePWAFoundation();
const service = new RegistryWorkspaceService();
const workspaceStore = new WorkspaceStore();
const workspaceController = new WorkspaceController(service, workspaceStore);
const setiTheme = setiThemeJson as SetiTheme;
const VS_CODE_EDITOR_FONT_FAMILY = "Consolas, 'Courier New', monospace";
const MAX_AUTO_RENDER_DIFF_CHARS = 200000;
const CODE_FONT_SIZE_OPTIONS = [12, 13, 14, 15, 16] as const;
const CODE_LINE_HEIGHT_OPTIONS = [1.35, 1.45, 1.5, 1.6, 1.7] as const;
const CODE_TAB_SIZE_OPTIONS = [2, 4, 8] as const;
const RECONNECT_RETRY_DELAY_MS = 1000;
const RECONNECT_GRACE_PERIOD_MS = 30_000;
const CHAT_NEW_DRAFT_SESSION_KEY = '__new__';
const CHAT_DRAFT_KEY_PROJECT_FALLBACK = '__no_project__';
const CHAT_AUTO_SCROLL_BOTTOM_THRESHOLD = 80;
const CHAT_CONFIG_PRIORITY_IDS = ['mode', 'model', 'effort'] as const;
const CHAT_CONFIG_PRIORITY_MATCHERS = ['mode', 'model', 'effort', 'thought'] as const;
const CHAT_CONFIG_INLINE_LIMIT = 3;
const WIDE_PROJECT_SESSION_LIMIT = 5;
const PROJECT_PIN_LONG_PRESS_MS = 450;
const PROJECT_SESSION_LONG_PRESS_MS = 450;
const DESKTOP_SIDEBAR_VIEWPORT_MAX_RATIO = 0.45;
const FLOATING_CONTROL_SLOT_ORDER = ['upper', 'upper-middle', 'center', 'lower-middle'] as const;
const EMPTY_CHAT_COMPOSER_DRAFT: ChatComposerDraft = { text: '', attachments: [] };
let mermaidRenderSequence = 0;

function nextMermaidRenderId(): string {
  mermaidRenderSequence += 1;
  return `wm-mermaid-${mermaidRenderSequence}`;
}

function buildChatDraftKey(activeProjectId: string, sessionId: string): string {
  const projectKey = activeProjectId.trim() || CHAT_DRAFT_KEY_PROJECT_FALLBACK;
  const sessionKey = sessionId.trim() || CHAT_NEW_DRAFT_SESSION_KEY;
  return `${projectKey}:${sessionKey}`;
}

function isChatScrolledNearBottom(container: HTMLElement): boolean {
  const distance = container.scrollHeight - container.clientHeight - container.scrollTop;
  return Math.max(0, distance) <= CHAT_AUTO_SCROLL_BOTTOM_THRESHOLD;
}

function floatingControlSlotRatio(slot: PersistedFloatingControlSlot): number {
  switch (slot) {
    case 'upper':
      return 0.26;
    case 'center':
      return 0.5;
    case 'lower-middle':
      return 0.68;
    default:
      return 0.4;
  }
}

function clampFloatingTop(top: number, minTop: number, maxTop: number): number {
  return Math.min(maxTop, Math.max(minTop, top));
}

function nearestFloatingSlot(
  top: number,
  slotTops: Array<{ slot: PersistedFloatingControlSlot; top: number }>,
): PersistedFloatingControlSlot {
  return slotTops.reduce((best, entry) =>
    Math.abs(entry.top - top) < Math.abs(best.top - top) ? entry : best,
  ).slot;
}


function tagVariantClass(prefix: string, value: string): string {
  const normalized = value.trim().toLowerCase();
  let hash = 0;
  for (let index = 0; index < normalized.length; index += 1) {
    hash = (hash * 31 + normalized.charCodeAt(index)) >>> 0;
  }
  return `${prefix}-${hash % 8}`;
}


function normalizeChatSlashCommandName(name: string): string {
  const normalized = name.trim();
  if (!normalized) {
    return '';
  }
  return normalized.startsWith('/') ? normalized : `/${normalized}`;
}

function normalizeChatSlashCommands(skills?: string[]): ChatSlashCommandOption[] {
  const merged = new Map<string, ChatSlashCommandOption>();
  for (const skill of skills ?? []) {
    const normalizedSkill = (skill || '').trim();
    if (!normalizedSkill) {
      continue;
    }
    const name = normalizeChatSlashCommandName(normalizedSkill);
    if (!name) {
      continue;
    }
    const key = name.toLowerCase();
    if (merged.has(key)) {
      continue;
    }
    merged.set(key, { name });
  }
  return Array.from(merged.values()).sort((left, right) => left.name.localeCompare(right.name));
}

function parseChatSlashQuery(text: string): string | null {
  const leadingTrimmed = text.trimStart();
  if (!leadingTrimmed.startsWith('/')) {
    return null;
  }
  const firstToken = leadingTrimmed.split(/\s+/, 1)[0] || '';
  if (!firstToken.startsWith('/')) {
    return null;
  }
  if (leadingTrimmed.length > firstToken.length) {
    return null;
  }
  return firstToken.slice(1).toLowerCase();
}

function filterChatSlashCommands(
  options: ChatSlashCommandOption[],
  query: string | null,
): ChatSlashCommandOption[] {
  if (query === null) {
    return [];
  }
  if (!query) {
    return options;
  }
  return options.filter(option =>
    option.name.toLowerCase().includes(query) ||
    (option.description || '').toLowerCase().includes(query),
  );
}

function sortChatSessions(items: RegistryChatSession[]): RegistryChatSession[] {
  return [...items].sort((a, b) => compareUpdatedAtDesc(a.updatedAt || '', b.updatedAt || ''));
}

function mergeChatSession(
  list: RegistryChatSession[],
  next: Partial<RegistryChatSession> & {sessionId: string},
): RegistryChatSession[] {
  const existing = list.find(item => item.sessionId === next.sessionId);
  const merged: RegistryChatSession = {
    sessionId: next.sessionId,
    title: next.title ?? existing?.title ?? '',
    preview: next.preview ?? existing?.preview ?? '',
    updatedAt: next.updatedAt ?? existing?.updatedAt ?? '',
    messageCount: next.messageCount ?? existing?.messageCount ?? 0,
    unreadCount: next.unreadCount ?? existing?.unreadCount,
    agentType: next.agentType ?? existing?.agentType,
    latestTurnIndex: next.latestTurnIndex ?? existing?.latestTurnIndex,
    running: next.running ?? existing?.running,
    lastDoneTurnIndex: next.lastDoneTurnIndex ?? existing?.lastDoneTurnIndex,
    lastDoneSuccess: next.lastDoneSuccess ?? existing?.lastDoneSuccess,
    lastReadTurnIndex: next.lastReadTurnIndex ?? existing?.lastReadTurnIndex,
    configOptions:
      next.configOptions ??
      (existing?.configOptions
        ? [...existing.configOptions]
        : undefined),
    commands:
      next.commands ??
      (existing?.commands
        ? [...existing.commands]
        : undefined),
  };
  const filtered = list.filter(item => item.sessionId !== next.sessionId);
  return sortChatSessions([merged, ...filtered]);
}

function mergeProjectSessionMap(
  map: Record<string, RegistryChatSession[]>,
  projectId: string,
  session: Partial<RegistryChatSession> & {sessionId: string},
): Record<string, RegistryChatSession[]> {
  if (!projectId || !session.sessionId) {
    return map;
  }
  return {
    ...map,
    [projectId]: mergeChatSession(map[projectId] ?? [], session),
  };
}

function addSessionFlag(flags: SessionFlagMap, sessionId: string): SessionFlagMap {
  if (!sessionId || flags[sessionId]) {
    return flags;
  }
  return {
    ...flags,
    [sessionId]: true,
  };
}

function removeSessionFlag(flags: SessionFlagMap, sessionId: string): SessionFlagMap {
  if (!sessionId || !flags[sessionId]) {
    return flags;
  }
  const next = {
    ...flags,
  };
  delete next[sessionId];
  return next;
}


function upsertChatMessage(
  list: RegistryChatMessage[],
  next: RegistryChatMessage,
): RegistryChatMessage[] {
  const key = chatMessageDomKey(next);
  const index = list.findIndex(
    item => chatMessageDomKey(item) === key,
  );
  if (index < 0) {
    return [...list, next].sort((a, b) => {
      return (a.turnIndex ?? 0) - (b.turnIndex ?? 0);
    });
  }
  const copy = [...list];
  copy[index] = next;
  return copy;
}

function chatMessageDomKey(message: Pick<RegistryChatMessage, 'sessionId' | 'turnIndex'>): string {
  return `${message.sessionId}:${message.turnIndex}`;
}

function nextPromptTurnIndex(messages: RegistryChatMessage[]): number {
  return Math.max(
    0,
    ...messages.map(message => Math.max(0, Math.trunc(message.turnIndex ?? 0))),
  ) + 1;
}

function buildPendingPromptMessage(prompt: PendingChatPrompt): RegistryChatMessage {
  return {
    sessionId: prompt.sessionId,
    turnIndex: prompt.turnIndex,
    method: 'prompt_request',
    param: {
      contentBlocks: prompt.blocks,
      createdAt: prompt.createdAt,
    },
    finished: false,
  };
}

function isPromptStartMessage(message: RegistryChatMessage): boolean {
  return message.method === 'prompt_request' || message.method === 'user_message_chunk';
}

// -- Message accessor helpers (all derived from method + param) --

function msgRole(method: string): string {
  switch (method) {
    case 'prompt_request':
    case 'user_message_chunk':
      return 'user';
    case 'prompt_done':
    case 'tool_call':
    case 'system':
      return 'system';
    default:
      return 'assistant';
  }
}

function msgKind(method: string): string {
  switch (method) {
    case 'prompt_done':
      return 'prompt_result';
    case 'agent_thought_chunk':
      return 'thought';
    case 'tool_call':
      return 'tool';
    case 'agent_plan':
      return 'plan';
    default:
      return 'message';
  }
}

function extractTextFromACPContent(content: unknown): string {
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

function extractTextFromIMParam(param: unknown): string {
  if (typeof param === 'string') {
    return param.trim();
  }
  if (Array.isArray(param)) {
    const chunks = param
      .map(item => {
        if (!item || typeof item !== 'object') return '';
        const entry = item as Record<string, unknown>;
        return typeof entry.content === 'string' ? entry.content.trim() : '';
      })
      .filter(Boolean);
    return chunks.join('\n').trim();
  }
  if (!param || typeof param !== 'object') {
    return '';
  }
  const input = param as Record<string, unknown>;
  if (typeof input.text === 'string') {
    return input.text.trim();
  }
  if (typeof input.output === 'string') {
    return input.output.trim();
  }
  if (typeof input.cmd === 'string') {
    return input.cmd.trim();
  }
  if (Array.isArray(input.contentBlocks)) {
    return extractTextFromACPContent(input.contentBlocks);
  }
  return '';
}

function msgText(method: string, param: Record<string, unknown>): string {
  if (method === 'prompt_request') {
    const blocks = Array.isArray(param.contentBlocks) ? param.contentBlocks : [];
    return extractTextFromACPContent(blocks);
  }
  if (method === 'prompt_done') {
    return typeof param.stopReason === 'string' ? param.stopReason : '';
  }
  return extractTextFromIMParam(param);
}

function msgBlocks(
  method: string,
  param: Record<string, unknown>,
): RegistrySessionContentBlock[] {
  if (Array.isArray(param.contentBlocks)) {
    return param.contentBlocks as RegistrySessionContentBlock[];
  }
  if (method === 'prompt_request') {
    return [];
  }
  return [];
}

function msgPlanEntries(
  method: string,
  param: Record<string, unknown>,
): { content: string; status?: string }[] {
  if (method !== 'agent_plan' || !Array.isArray(param)) {
    return [];
  }
  const entries: { content: string; status?: string }[] = [];
  for (const item of param as unknown[]) {
    if (!item || typeof item !== 'object') continue;
    const entry = item as Record<string, unknown>;
    const content = typeof entry.content === 'string' ? entry.content.trim() : '';
    if (!content) continue;
    const status = typeof entry.status === 'string' ? entry.status.trim() : '';
    entries.push(status ? { content, status } : { content });
  }
  return entries;
}

function chatConfigPriority(option: RegistrySessionConfigOption): number {
  const id = (option.id || '').trim().toLowerCase();
  const label = (option.name || '').trim().toLowerCase();
  const exactRank = CHAT_CONFIG_PRIORITY_IDS.findIndex(item => item === id);
  if (exactRank >= 0) {
    return exactRank;
  }
  const fuzzyRank = CHAT_CONFIG_PRIORITY_MATCHERS.findIndex(
    item => id.includes(item) || label.includes(item),
  );
  if (fuzzyRank >= 0) {
    return CHAT_CONFIG_PRIORITY_IDS.length + fuzzyRank;
  }
  return 99;
}

function chatConfigCurrentValue(option: RegistrySessionConfigOption): string {
  const optionValues = option.options ?? [];
  return option.currentValue || optionValues[0]?.value || '';
}

function chatConfigCurrentLabel(option: RegistrySessionConfigOption): string {
  const currentValue = chatConfigCurrentValue(option);
  const optionValues = option.options ?? [];
  const currentOption = optionValues.find(item => item.value === currentValue);
  return currentOption?.name || currentValue || option.name || option.id;
}

function chatConfigIconClass(option: RegistrySessionConfigOption): string {
  const key = `${option.id || ''} ${option.name || ''}`.toLowerCase();
  if (key.includes('model')) {
    return 'codicon-symbol-class';
  }
  if (key.includes('effort') || key.includes('thought')) {
    return 'codicon-pulse';
  }
  if (key.includes('mode') || key.includes('permission')) {
    return 'codicon-shield';
  }
  return 'codicon-settings-gear';
}

function decodeSessionMessageFromEventPayload(
  payload: RegistryChatMessageEventPayload,
): RegistryChatMessage | null {
  const sessionId = typeof payload.sessionId === 'string' ? payload.sessionId.trim() : '';
  const content = typeof payload.content === 'string' ? payload.content.trim() : '';
  const turnIndex = Number(payload.turnIndex ?? 0);
  const finished = payload.finished === true;
  if (!sessionId || turnIndex <= 0) {
    return null;
  }
  if (!content) {
    return null;
  }
  try {
    const doc = JSON.parse(content) as Record<string, unknown>;
    const method = typeof doc.method === 'string' ? doc.method.trim() : '';
    const param =
      doc.param != null && typeof doc.param === 'object' && !Array.isArray(doc.param)
        ? (doc.param as Record<string, unknown>)
        : {};
    // Skip Claude command system messages (<command-name>, <local-command-*, etc.)
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
    // Unparseable content: store as system message
    return {
      sessionId,
      turnIndex,
      method: 'system',
      param: { text: content },
      finished,
    };
  }
}

const CollapsibleThought = React.memo(function CollapsibleThought({
  text,
  markdownComponents,
  markdownUrlTransform,
}: {
  text: string;
  markdownComponents: Components;
  markdownUrlTransform: (value: string) => string;
}) {
  const [open, setOpen] = React.useState(false);
  const firstLine = (text || '')
    .split('\n')
    .map(line => line.trim())
    .find(Boolean) || '';

  return (
    <div className={`chat-thought-block${open ? ' chat-thought-open' : ''}`}>
      <div
        className="chat-thought-header"
        onClick={() => setOpen(!open)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setOpen(!open); } }}
      >
        <span className="codicon codicon-chevron-right chat-thought-chevron" />
        <span className="codicon codicon-lightbulb" />
        {!open && firstLine ? (
          <span className="chat-thought-preview">{firstLine}</span>
        ) : null}
      </div>
      {open ? (
        <div className="chat-thought-content">
          <ReactMarkdown
            remarkPlugins={[remarkGfm, remarkMath]}
            urlTransform={markdownUrlTransform}
            rehypePlugins={[rehypeKatex]}
            components={markdownComponents}
          >
            {text}
          </ReactMarkdown>
        </div>
      ) : null}
    </div>
  );
});

function isPlanEntryCompleted(status?: string): boolean {
  const value = (status || '').trim().toLowerCase();
  return value === 'completed' || value === 'done' || value === 'success';
}

function groupImageBlocks(msgs: RegistryChatMessage[]): RegistrySessionContentBlock[] {
  const blocks: RegistrySessionContentBlock[] = [];
  for (const m of msgs) {
    for (const b of msgBlocks(m.method, m.param)) {
      if (b.type === 'image' && b.data) {
        blocks.push(b);
      }
    }
  }
  return blocks;
}

async function writeTextToClipboard(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', 'true');
  textarea.style.position = 'fixed';
  textarea.style.top = '-1000px';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  try {
    document.execCommand('copy');
  } finally {
    document.body.removeChild(textarea);
  }
}

type ChatTurnViewProps = {
  message: RegistryChatMessage;
  promptRequest?: RegistryChatMessage;
  promptStatus?: ChatPromptStatus;
  hideToolCalls: boolean;
  markdownComponents: Components;
  markdownUrlTransform: (value: string) => string;
  copyDisabled?: boolean;
  onCopyPromptDone?: () => void;
};

const ChatTurnView = React.memo(function ChatTurnView({
  message,
  promptRequest,
  promptStatus = null,
  hideToolCalls,
  markdownComponents,
  markdownUrlTransform,
  copyDisabled = true,
  onCopyPromptDone,
}: ChatTurnViewProps) {
  const text = msgText(message.method, message.param).trim();
  const kind = msgKind(message.method);

  if (message.method === 'prompt_request' || message.method === 'user_message_chunk') {
    const imageBlocks = groupImageBlocks([message]);
    return (
      <div className="chat-prompt-group">
        {text || promptStatus ? (
          <div className="chat-prompt-user-row">
            {text ? (
              <div className="chat-prompt-user">{text}</div>
            ) : null}
            {promptStatus === 'responding' ? (
              <span className="chat-prompt-status chat-prompt-status-responding" title="Responding">
                <span className="chat-prompt-status-dots" aria-hidden="true">
                  <span>.</span>
                  <span>.</span>
                  <span>.</span>
                </span>
              </span>
            ) : null}
          </div>
        ) : null}
        {imageBlocks.length > 0 ? (
          <div className="chat-image-strip">
            {imageBlocks.map((block, index) => (
              <img
                key={`${message.sessionId}:${message.turnIndex}:img:${index}`}
                className="chat-inline-image"
                src={`data:${block.mimeType || 'image/png'};base64,${block.data}`}
                alt="chat attachment"
              />
            ))}
          </div>
        ) : null}
      </div>
    );
  }

  if (message.method === 'prompt_done') {
    const completedAt = typeof message.param.completedAt === 'string' ? Date.parse(message.param.completedAt) : NaN;
    const createdAt = typeof promptRequest?.param.createdAt === 'string' ? Date.parse(promptRequest.param.createdAt) : NaN;
    const durationMs = Number.isFinite(completedAt) && Number.isFinite(createdAt) && completedAt >= createdAt
      ? completedAt - createdAt
      : 0;
    const modelName = typeof promptRequest?.param.modelName === 'string'
      ? promptRequest.param.modelName
      : '';
    const failed = text.toLowerCase() === 'failed';
    return (
      <div className="chat-prompt-separator">
        <hr />
        <span className="chat-prompt-separator-label">
          By {modelName || 'unknown'}
          {durationMs > 0 ? ` · ${formatPromptDurationMs(durationMs)}` : ''}
        </span>
        <div className="chat-prompt-actions" aria-label="Prompt actions">
          <button
            type="button"
            className="chat-prompt-action-button"
            onClick={() => onCopyPromptDone?.()}
            disabled={copyDisabled}
            title="Copy response"
            aria-label="Copy response markdown"
          >
            <span className="codicon codicon-copy" />
          </button>
        </div>
        {failed ? (
          <div className="chat-prompt-failure-line">
            Response failed.
          </div>
        ) : null}
      </div>
    );
  }

  if (hideToolCalls && kind === 'tool') {
    return null;
  }
  if (kind === 'tool') {
    return (
      <div className="chat-tool-line" title={text}>
        <span className="codicon codicon-tools" />
        <span>{text}</span>
      </div>
    );
  }
  if (kind === 'thought') {
    return (
      <CollapsibleThought
        text={text}
        markdownComponents={markdownComponents}
        markdownUrlTransform={markdownUrlTransform}
      />
    );
  }
  if (kind === 'plan') {
    let planEntries = msgPlanEntries(message.method, message.param);
    if (planEntries.length === 0 && text) {
      planEntries = text
        .split('\n')
        .map(line => line.trim())
        .filter(Boolean)
        .map(content => ({ content }));
    }
    if (planEntries.length === 0) {
      return null;
    }
    return (
      <div className="chat-plan-block">
        <div className="chat-plan-title">
          <span className="codicon codicon-checklist" />
          <span>Plan</span>
        </div>
        <ul className="chat-plan-list">
          {planEntries.map((item, index) => {
            const done = isPlanEntryCompleted(item.status);
            return (
              <li
                key={`${message.sessionId}:${message.turnIndex}:plan:${index}`}
                className={done ? 'done' : ''}
              >
                <span className="chat-plan-marker">{done ? '✓' : '○'}</span>
                <span>{item.content}</span>
              </li>
            );
          })}
        </ul>
      </div>
    );
  }
  if (!text) {
    return null;
  }
  return (
    <div className="chat-main-message">
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkMath]}
        urlTransform={markdownUrlTransform}
        rehypePlugins={[rehypeKatex]}
        components={markdownComponents}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
});

function shouldRenderChatTurn(
  message: RegistryChatMessage,
  hideToolCalls: boolean,
  promptStatus: ChatPromptStatus,
): boolean {
  const text = msgText(message.method, message.param).trim();
  if (message.method === 'prompt_request' || message.method === 'user_message_chunk') {
    return !!text || !!promptStatus || groupImageBlocks([message]).length > 0;
  }
  if (message.method === 'prompt_done') {
    return true;
  }
  const kind = msgKind(message.method);
  if (kind === 'tool') {
    return !hideToolCalls && !!text;
  }
  if (kind === 'thought') {
    return !!text;
  }
  if (kind === 'plan') {
    return msgPlanEntries(message.method, message.param).length > 0 || !!text;
  }
  return !!text;
}

function formatChatTimestamp(value: string): string {
  if (!value) return '';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return '';
  return parsed.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function clampCodeFontSize(value: number): number {
  return Math.min(
    20,
    Math.max(11, Number.isFinite(value) ? value : DEFAULT_CODE_FONT_SIZE),
  );
}

function clampCodeLineHeight(value: number): number {
  return Math.min(
    2,
    Math.max(1.2, Number.isFinite(value) ? value : DEFAULT_CODE_LINE_HEIGHT),
  );
}

function clampCodeTabSize(value: number): number {
  return Math.min(
    8,
    Math.max(1, Number.isFinite(value) ? value : DEFAULT_CODE_TAB_SIZE),
  );
}

function sortEntries(entries: RegistryFsEntry[]): RegistryFsEntry[] {
  return [...entries].sort((a, b) => {
    if (a.kind === 'dir' && b.kind !== 'dir') return -1;
    if (a.kind !== 'dir' && b.kind === 'dir') return 1;
    return a.name.localeCompare(b.name);
  });
}

function getFileExtension(path: string): string {
  const match = /\.([a-z0-9]+)$/i.exec(path);
  return match ? match[1].toLowerCase() : '';
}

function isMarkdownPath(path: string): boolean {
  const ext = getFileExtension(path);
  return ext === 'md' || ext === 'markdown';
}

function detectCodeLanguage(path: string): string {
  const ext = getFileExtension(path);
  switch (ext) {
    case 'ts':
      return 'typescript';
    case 'tsx':
      return 'tsx';
    case 'js':
    case 'cjs':
    case 'mjs':
      return 'javascript';
    case 'jsx':
      return 'jsx';
    case 'json':
      return 'json';
    case 'go':
      return 'go';
    case 'c':
      return 'c';
    case 'cc':
    case 'cpp':
    case 'cxx':
    case 'h':
    case 'hh':
    case 'hpp':
      return 'cpp';
    case 'rs':
      return 'rust';
    case 'sh':
    case 'bash':
      return 'shellscript';
    case 'ps1':
    case 'psm1':
      return 'powershell';
    case 'py':
      return 'python';
    case 'yml':
    case 'yaml':
      return 'yaml';
    case 'md':
    case 'markdown':
      return 'markdown';
    case 'diff':
    case 'patch':
      return 'diff';
    case 'html':
      return 'markup';
    default:
      return 'clike';
  }
}

function inferImageMimeType(path: string): string {
  const ext = getFileExtension(path);
  switch (ext) {
    case 'svg':
      return 'image/svg+xml';
    case 'png':
      return 'image/png';
    case 'jpg':
    case 'jpeg':
      return 'image/jpeg';
    case 'gif':
      return 'image/gif';
    case 'webp':
      return 'image/webp';
    case 'bmp':
      return 'image/bmp';
    case 'ico':
      return 'image/x-icon';
    case 'avif':
      return 'image/avif';
    default:
      return '';
  }
}

function isImageFile(path: string, mimeType?: string): boolean {
  const normalizedMime = (mimeType || '').trim().toLowerCase();
  if (normalizedMime.startsWith('image/')) {
    return true;
  }
  return inferImageMimeType(path) !== '';
}

function encodeUtf8ToBase64(value: string): string {
  try {
    if (typeof TextEncoder !== 'undefined') {
      const bytes = new TextEncoder().encode(value);
      let binary = '';
      for (let i = 0; i < bytes.length; i += 1) {
        binary += String.fromCharCode(bytes[i]);
      }
      return btoa(binary);
    }
  } catch {
    // fallback below
  }
  return btoa(unescape(encodeURIComponent(value)));
}

function buildImageDataUrl(params: {
  content: string;
  path: string;
  mimeType?: string;
  isBinary?: boolean;
}): string {
  const { content, path, mimeType, isBinary } = params;
  if (!content) {
    return '';
  }
  const inferredMime = inferImageMimeType(path);
  const normalizedMime = inferredMime || (mimeType || '').trim() || 'image/png';
  if (isBinary) {
    return `data:${normalizedMime};base64,${content}`;
  }
  return `data:${normalizedMime};base64,${encodeUtf8ToBase64(content)}`;
}
function parseTrailingLineNumber(value: string): number | null {
  const input = value.trim();
  if (!input) return null;
  const hashMatch = /#L(\d+)(?:C\d+)?$/i.exec(input);
  if (hashMatch) {
    const line = Number.parseInt(hashMatch[1], 10);
    return Number.isFinite(line) && line > 0 ? line : null;
  }
  const suffixMatch = /:(\d+)(?::\d+)?$/.exec(input);
  if (suffixMatch) {
    const line = Number.parseInt(suffixMatch[1], 10);
    return Number.isFinite(line) && line > 0 ? line : null;
  }
  return null;
}

function collectReactText(node: React.ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') {
    return String(node);
  }
  if (Array.isArray(node)) {
    return node.map(item => collectReactText(item)).join('');
  }
  if (React.isValidElement(node)) {
    return collectReactText((node.props as { children?: React.ReactNode }).children);
  }
  return '';
}
function isLoopbackHost(host: string): boolean {
  const v = host.trim().toLowerCase();
  return v === '127.0.0.1' || v === 'localhost' || v === '::1' || v === '[::1]';
}

function isLoopbackAddress(address: string): boolean {
  const input = address.trim();
  if (!input) return false;
  if (/^wss?:\/\//i.test(input)) {
    try {
      const url = new URL(input);
      return isLoopbackHost(url.hostname);
    } catch {
      return false;
    }
  }
  if (/^https?:\/\//i.test(input)) {
    try {
      const url = new URL(input);
      return isLoopbackHost(url.hostname);
    } catch {
      return false;
    }
  }
  const host = input.split('/')[0].split(':')[0];
  return isLoopbackHost(host);
}

function resolveInitialRegistryAddress(
  savedAddress: string,
  defaultAddress: string,
): string {
  const pageHost = window.location.hostname;
  if (!isLoopbackHost(pageHost) && isLoopbackAddress(savedAddress)) {
    return defaultAddress;
  }
  return savedAddress || defaultAddress;
}

function readSafeAreaTopInset(): number {
  const value = window
    .getComputedStyle(document.documentElement)
    .getPropertyValue('--wm-safe-area-top');
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

type UnifiedDiffRow = {
  kind: 'context' | 'added' | 'removed' | 'separator';
  oldLineNumber: number | null;
  newLineNumber: number | null;
  text: string;
  separator?: 'hunk' | 'file';
};

function pushUnifiedDiffSeparator(
  rows: UnifiedDiffRow[],
  separator: 'hunk' | 'file',
  text: string,
): void {
  if (rows.length === 0) return;
  const last = rows[rows.length - 1];
  if (last.kind === 'separator') return;
  rows.push({
    kind: 'separator',
    oldLineNumber: null,
    newLineNumber: null,
    text,
    separator,
  });
}

function parseUnifiedDiffRows(content: string): UnifiedDiffRow[] {
  const rows: UnifiedDiffRow[] = [];
  try {
    const files = gitdiffParser.parse(content);
    for (let fileIndex = 0; fileIndex < files.length; fileIndex += 1) {
      const file = files[fileIndex];
      const hunks = file.hunks || [];
      if (hunks.length === 0) continue;

      if (rows.length > 0) {
        pushUnifiedDiffSeparator(rows, 'file', '... next file ...');
      }

      for (let hunkIndex = 0; hunkIndex < hunks.length; hunkIndex += 1) {
        const hunk = hunks[hunkIndex];
        if (hunkIndex > 0) {
          pushUnifiedDiffSeparator(rows, 'hunk', '... skipped unchanged lines ...');
        }
        for (const change of hunk.changes || []) {
          if (change.type === 'insert') {
            rows.push({
              kind: 'added',
              oldLineNumber: null,
              newLineNumber: change.lineNumber,
              text: change.content,
            });
            continue;
          }
          if (change.type === 'delete') {
            rows.push({
              kind: 'removed',
              oldLineNumber: change.lineNumber,
              newLineNumber: null,
              text: change.content,
            });
            continue;
          }
          rows.push({
            kind: 'context',
            oldLineNumber: change.oldLineNumber,
            newLineNumber: change.newLineNumber,
            text: change.content,
          });
        }
      }
    }
  } catch {
    // fall through to fallback parser for non-standard diff snippets
  }

  if (rows.length > 0) {
    return rows;
  }

  // Fallback for non-standard patches: keep a readable inline rendering.
  const lines = content.split('\n');
  for (const raw of lines) {
    if (raw.startsWith('+')) {
      rows.push({
        kind: 'added',
        oldLineNumber: null,
        newLineNumber: null,
        text: raw.slice(1),
      });
      continue;
    }
    if (raw.startsWith('-')) {
      rows.push({
        kind: 'removed',
        oldLineNumber: null,
        newLineNumber: null,
        text: raw.slice(1),
      });
      continue;
    }
    rows.push({
      kind: 'context',
      oldLineNumber: null,
      newLineNumber: null,
      text: raw.startsWith(' ') ? raw.slice(1) : raw,
    });
  }
  return rows;
}

function buildInlineDiffRenderLines(rows: UnifiedDiffRow[]): DiffRenderLine[] {
  return rows.map(row => {
    if (row.kind === 'separator') {
      return {
        code: row.text,
        lineNumber: null,
        oldLineNumber: null,
        newLineNumber: null,
        kind: 'empty',
        separator: row.separator ?? 'hunk',
      };
    }
    return {
      code: row.text,
      lineNumber: row.newLineNumber ?? row.oldLineNumber,
      oldLineNumber: row.oldLineNumber,
      newLineNumber: row.newLineNumber,
      kind: row.kind,
    };
  });
}

function toSetiGlyph(fontCharacter?: string): string {
  if (!fontCharacter) return '?';
  const hex = fontCharacter.replace('\\', '');
  const code = Number.parseInt(hex, 16);
  if (Number.isNaN(code)) return '?';
  return String.fromCodePoint(code);
}

function resolveSetiIcon(name: string, mode: ThemeMode): SetiResolvedIcon {
  const section: SetiThemeSection =
    mode === 'light' && setiTheme.light
      ? {
          file: setiTheme.light.file,
          fileExtensions: setiTheme.light.fileExtensions,
          fileNames: setiTheme.light.fileNames,
        }
      : {
          file: setiTheme.file,
          fileExtensions: setiTheme.fileExtensions,
          fileNames: setiTheme.fileNames,
        };

  const lowerName = name.toLowerCase();
  let iconId = section.file;

  if (section.fileNames?.[lowerName]) {
    iconId = section.fileNames[lowerName];
  } else if (section.fileExtensions) {
    const parts = lowerName.split('.');
    for (let i = 0; i < parts.length; i += 1) {
      const candidate = parts.slice(i).join('.');
      if (section.fileExtensions[candidate]) {
        iconId = section.fileExtensions[candidate];
        break;
      }
    }
  }

  const definition =
    setiTheme.iconDefinitions[iconId] ??
    setiTheme.iconDefinitions[section.file] ??
    {};
  return {
    glyph: toSetiGlyph(definition.fontCharacter),
    color: definition.fontColor ?? '#d4d7d6',
  };
}

function setiFontFaceCss(): string {
  return `@font-face { font-family: 'wm-seti'; src: url('${setiFontUrl}') format('woff'); font-weight: normal; font-style: normal; }`;
}

function buildWorkingTreeFiles(
  status: RegistryGitStatus,
): WorkingTreeFileEntry[] {
  const rows: WorkingTreeFileEntry[] = [];
  for (const item of status.unstaged ?? []) {
    if (!item.path) continue;
    rows.push({ path: item.path, status: item.status, scope: 'unstaged' });
  }
  for (const item of status.staged ?? []) {
    if (!item.path) continue;
    rows.push({ path: item.path, status: item.status, scope: 'staged' });
  }
  for (const item of status.untracked ?? []) {
    if (!item.path) continue;
    rows.push({
      path: item.path,
      status: item.status || 'U',
      scope: 'untracked',
    });
  }
  return rows;
}

function isHeavyGeneratedDiffPath(path: string): boolean {
  const normalized = (path || '').replace(/\\/g, '/').toLowerCase();
  return (
    normalized.endsWith('/dist/bundle.js') ||
    normalized.endsWith('/dist/bundle.js.map') ||
    normalized.endsWith('/dist/index.html')
  );
}

function pickPreferredPath<T extends { path: string }>(items: T[]): string {
  if (items.length === 0) return '';
  const preferred = items.find(item => !isHeavyGeneratedDiffPath(item.path));
  return (preferred ?? items[0]).path;
}

function normalizeGitBranches(branches: string[], current: string): string[] {
  const merged: string[] = [];
  if (current.trim()) {
    merged.push(current.trim());
  }
  for (const branch of branches) {
    const trimmed = branch.trim();
    if (!trimmed) continue;
    merged.push(trimmed);
  }
  return [...new Set(merged)];
}

function pickGitSelectedBranches(
  previous: string[],
  available: string[],
  current: string,
): string[] {
  const validPrevious = previous.filter(item => available.includes(item));
  if (validPrevious.length > 0) {
    return validPrevious;
  }
  const normalizedCurrent = current.trim();
  if (normalizedCurrent && available.includes(normalizedCurrent)) {
    return [normalizedCurrent];
  }
  if (available.length > 0) {
    return [available[0]];
  }
  return [];
}

function splitPathForDisplay(path: string): {
  fileName: string;
  parentPath: string;
} {
  const normalized = (path || '').replaceAll('\\', '/');
  const separator = normalized.lastIndexOf('/');
  if (separator < 0) {
    return { fileName: normalized, parentPath: '' };
  }
  return {
    fileName: normalized.slice(separator + 1),
    parentPath: normalized.slice(0, separator),
  };
}

function formatGitCommitDateTime(value: string): string {
  if (!value) return '';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString();
}

function formatRelativeTime(value: string): string {
  if (!value) return 'just now';
  const parsed = new Date(value);
  const ts = parsed.getTime();
  if (Number.isNaN(ts)) return 'just now';
  const deltaMs = Date.now() - ts;
  const deltaMin = Math.max(0, Math.floor(deltaMs / 60000));
  if (deltaMin < 1) return 'just now';
  if (deltaMin < 60) return `${deltaMin}m ago`;
  const deltaHour = Math.floor(deltaMin / 60);
  if (deltaHour < 24) return `${deltaHour}h ago`;
  const deltaDay = Math.floor(deltaHour / 24);
  if (deltaDay < 30) return `${deltaDay}d ago`;
  const deltaMonth = Math.floor(deltaDay / 30);
  if (deltaMonth < 12) return `${deltaMonth}mo ago`;
  const deltaYear = Math.floor(deltaMonth / 12);
  return `${deltaYear}y ago`;
}

function formatCompactRelativeAge(value: string): string {
  if (!value) return '0m';
  const parsed = new Date(value);
  const ts = parsed.getTime();
  if (Number.isNaN(ts)) return '0m';
  const deltaMs = Math.max(0, Date.now() - ts);
  const deltaMin = Math.floor(deltaMs / 60000);
  if (deltaMin < 60) return `${Math.max(0, deltaMin)}m`;
  const deltaHour = Math.floor(deltaMin / 60);
  if (deltaHour < 24) return `${deltaHour}h`;
  const deltaDay = Math.floor(deltaHour / 24);
  if (deltaDay < 30) return `${deltaDay}d`;
  const deltaMonth = Math.floor(deltaDay / 30);
  if (deltaMonth < 12) return `${deltaMonth}mo`;
  const deltaYear = Math.floor(deltaMonth / 12);
  return `${deltaYear}y`;
}

type ShikiCodeBlockProps = {
  content: string;
  language: string;
  wrap: boolean;
  lineNumbers: boolean;
  themeMode: ThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
};

function ShikiCodeBlock({
  content,
  language,
  wrap,
  lineNumbers,
  themeMode,
  codeTheme,
  codeFont,
  codeFontSize,
  codeLineHeight,
  codeTabSize,
}: ShikiCodeBlockProps) {
  const [html, setHtml] = useState('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const nextHtml = await renderShikiHtml({
        code: content,
        language,
        themeMode,
        codeTheme,
        codeFont,
        codeFontSize,
        codeLineHeight,
        codeTabSize,
        wrap,
        lineNumbers,
        mode: 'block',
      });
      if (!cancelled) {
        setHtml(nextHtml);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [
    content,
    language,
    themeMode,
    codeTheme,
    codeFont,
    codeFontSize,
    codeLineHeight,
    codeTabSize,
    wrap,
    lineNumbers,
  ]);

  return (
    <div
      className={`code-wrap ${wrap ? 'wrap' : 'nowrap'}`}
      dangerouslySetInnerHTML={{ __html: html || '<pre><code> </code></pre>' }}
    />
  );
}

type MermaidBlockProps = {
  content: string;
  themeMode: ThemeMode;
};

function MermaidBlock({ content, themeMode }: MermaidBlockProps) {
  const [svg, setSvg] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    const source = content.trim();
    if (!source) {
      setSvg('');
      setError('Empty mermaid diagram');
      return () => {
        cancelled = true;
      };
    }

    setSvg('');
    setError('');

    (async () => {
      try {
        mermaid.initialize({
          startOnLoad: false,
          securityLevel: 'strict',
          theme: themeMode === 'light' ? 'default' : 'dark',
        });
        const renderId = nextMermaidRenderId();
        const { svg: nextSvg } = await mermaid.render(renderId, source);
        if (!cancelled) {
          setSvg(nextSvg || '');
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err));
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [content, themeMode]);

  if (error) {
    return <div className="mermaid-error">{error}</div>;
  }

  if (!svg) {
    return <div className="muted block">Rendering mermaid diagram...</div>;
  }

  return (
    <div className="mermaid-block" dangerouslySetInnerHTML={{ __html: svg }} />
  );
}

type MarkdownPreviewProps = {
  content: string;
  themeMode: ThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
  wrap: boolean;
  lineNumbers: boolean;
};

const markdownPreRenderer: NonNullable<Components['pre']> = ({ children }) => (
  <>{children}</>
);

const markdownCodeRenderer = ({
  className,
  children,
  themeMode,
  codeTheme,
  codeFont,
  codeFontSize,
  codeLineHeight,
  codeTabSize,
  wrap,
  lineNumbers,
}: {
  className?: string;
  children?: React.ReactNode;
  themeMode: ThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
  wrap: boolean;
  lineNumbers: boolean;
}) => {
  const languageMatch = /language-([\w-]+)/.exec(className || '');
  const language = (languageMatch?.[1] || '').toLowerCase();
  const codeText = String(children ?? '').replace(/\n$/, '');

  if (language === "mermaid") {
    return <MermaidBlock content={codeText} themeMode={themeMode} />;
  }

  if (language || codeText.includes('\n')) {
    return (
      <ShikiCodeBlock
        content={codeText}
        language={language || 'text'}
        wrap={wrap}
        lineNumbers={lineNumbers}
        themeMode={themeMode}
        codeTheme={codeTheme}
        codeFont={codeFont}
        codeFontSize={codeFontSize}
        codeLineHeight={codeLineHeight}
        codeTabSize={codeTabSize}
      />
    );
  }

  return <code className={className}>{children}</code>;
};

const markdownPreviewPropsEqual = (
  prev: MarkdownPreviewProps,
  next: MarkdownPreviewProps,
) =>
  prev.content === next.content &&
  prev.themeMode === next.themeMode &&
  prev.codeTheme === next.codeTheme &&
  prev.codeFont === next.codeFont &&
  prev.codeFontSize === next.codeFontSize &&
  prev.codeLineHeight === next.codeLineHeight &&
  prev.codeTabSize === next.codeTabSize &&
  prev.wrap === next.wrap &&
  prev.lineNumbers === next.lineNumbers;

const MarkdownPreview = React.memo(function MarkdownPreview({
  content,
  themeMode,
  codeTheme,
  codeFont,
  codeFontSize,
  codeLineHeight,
  codeTabSize,
  wrap,
  lineNumbers,
}: MarkdownPreviewProps) {
  const markdownComponents = useMemo<Components>(
    () => ({
      pre: markdownPreRenderer,
      code: ({ className, children }) =>
        markdownCodeRenderer({
          className,
          children,
          themeMode,
          codeTheme,
          codeFont,
          codeFontSize,
          codeLineHeight,
          codeTabSize,
          wrap,
          lineNumbers,
        }),
    }),
    [
      themeMode,
      codeTheme,
      codeFont,
      codeFontSize,
      codeLineHeight,
      codeTabSize,
      wrap,
      lineNumbers,
    ],
  );

  return (
    <div className="markdown-preview">
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeKatex]}
        components={markdownComponents}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}, markdownPreviewPropsEqual);
type ShikiDiffPaneProps = {
  content: string;
  language: string;
  wrap: boolean;
  lineNumbers: boolean;
  themeMode: ThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontFamily: string;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
};

function ShikiDiffPane({
  content,
  language,
  wrap,
  lineNumbers,
  themeMode,
  codeTheme,
  codeFont,
  codeFontFamily,
  codeFontSize,
  codeLineHeight,
  codeTabSize,
}: ShikiDiffPaneProps) {
  const [diffHtml, setDiffHtml] = useState('');
  const rows = useMemo(() => parseUnifiedDiffRows(content), [content]);
  const lines = useMemo(() => buildInlineDiffRenderLines(rows), [rows]);

  useEffect(() => {
    let cancelled = false;
    if (rows.length === 0) {
      setDiffHtml('');
      return () => {
        cancelled = true;
      };
    }

    setDiffHtml('');
    (async () => {
      const nextDiffHtml = await renderShikiDiffHtml({
        lines,
        language,
        themeMode,
        codeTheme,
        codeFont,
        codeFontSize,
        codeLineHeight,
        codeTabSize,
        wrap,
        lineNumbers,
      });
      if (!cancelled) {
        setDiffHtml(nextDiffHtml);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [
    lines,
    rows.length,
    language,
    themeMode,
    codeTheme,
    codeFont,
    codeFontSize,
    codeLineHeight,
    codeTabSize,
    wrap,
    lineNumbers,
  ]);

  if (rows.length === 0)
    return <div className="muted block">No diff hunks available</div>;

  const diffStyle = {
    fontFamily: codeFontFamily || VS_CODE_EDITOR_FONT_FAMILY,
    fontSize: `${codeFontSize}px`,
    lineHeight: String(codeLineHeight),
    tabSize: String(codeTabSize),
  };

  return (
    <div className={`code-wrap diff-wrap ${wrap ? 'wrap' : 'nowrap'}`}>
      <div
        className={`diff-inline ${wrap ? 'wrap' : 'nowrap'}`}
        style={diffStyle}
        dangerouslySetInnerHTML={{__html: diffHtml || '<pre><code> </code></pre>'}}
      />
    </div>
  );
}
function App() {
  const defaultRegistryAddress = useMemo(() => getDefaultRegistryAddress(), []);
  const persistedGlobal = useMemo(
    () => workspaceStore.getGlobalState(defaultRegistryAddress),
    [defaultRegistryAddress],
  );
  const initialRegistryAddress = resolveInitialRegistryAddress(
    persistedGlobal.address || '',
    defaultRegistryAddress,
  );
  const [connected, setConnected] = useState(false);
  const [address, setAddress] = useState(initialRegistryAddress);
  const addressRef = useRef(initialRegistryAddress);
  const [token, setToken] = useState(persistedGlobal.token || '');
  const tokenRef = useRef(persistedGlobal.token || '');
  const [error, setError] = useState('');
  const [autoConnecting, setAutoConnecting] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const autoConnectTriedRef = useRef(false);

  const [themeMode, setThemeMode] = useState<ThemeMode>(
    persistedGlobal.themeMode === 'light' ? 'light' : 'dark',
  );
  const [codeTheme, setCodeTheme] = useState<CodeThemeId>(
    typeof persistedGlobal.codeTheme === 'string' &&
      isCodeThemeId(persistedGlobal.codeTheme)
      ? persistedGlobal.codeTheme
      : DEFAULT_CODE_THEME,
  );
  const [codeFont, setCodeFont] = useState<CodeFontId>(
    typeof persistedGlobal.codeFont === 'string' &&
      isCodeFontId(persistedGlobal.codeFont)
      ? persistedGlobal.codeFont
      : DEFAULT_CODE_FONT,
  );
  const [codeFontSize, setCodeFontSize] = useState<number>(
    clampCodeFontSize(Number(persistedGlobal.codeFontSize)),
  );
  const [codeLineHeight, setCodeLineHeight] = useState<number>(
    clampCodeLineHeight(Number(persistedGlobal.codeLineHeight)),
  );
  const [codeTabSize, setCodeTabSize] = useState<number>(
    clampCodeTabSize(Number(persistedGlobal.codeTabSize)),
  );
  const [wrapLines, setWrapLines] = useState(!!persistedGlobal.wrapLines);
  const [showLineNumbers, setShowLineNumbers] = useState(
    typeof persistedGlobal.showLineNumbers === 'boolean'
      ? persistedGlobal.showLineNumbers
      : true,
  );
  const [hideToolCalls, setHideToolCalls] = useState(
    typeof persistedGlobal.hideToolCalls === 'boolean'
      ? persistedGlobal.hideToolCalls
      : false,
  );
  const codeFontFamily = useMemo(
    () => resolveCodeFontFamily(codeFont),
    [codeFont],
  );
  const setiFontCss = useMemo(() => setiFontFaceCss(), []);
  const resolveFileIcon = (name: string) => resolveSetiIcon(name, themeMode);

  const [windowWidth, setWindowWidth] = useState<number>(window.innerWidth);
  const [windowHeight, setWindowHeight] = useState<number>(window.innerHeight);
  const [safeAreaTopInset, setSafeAreaTopInset] = useState<number>(() => readSafeAreaTopInset());
  const layoutMode = resolveLayoutMode(windowWidth);
  const isWide = layoutMode === 'desktop';
  const supportsChatClipboardImages = useMemo(() => {
    const userAgent = window.navigator.userAgent || '';
    const platform = window.navigator.platform || '';
    if (/iPad|iPhone|iPod/i.test(userAgent)) {
      return false;
    }
    if (
      /Macintosh/i.test(userAgent) &&
      (window.navigator.maxTouchPoints ?? 0) > 1
    ) {
      return false;
    }
    if (/Win/i.test(platform) || /Windows NT/i.test(userAgent)) {
      return true;
    }
    if (/Mac/i.test(platform) || /Macintosh/i.test(userAgent)) {
      return true;
    }
    if (/Linux/i.test(platform) || /X11|Linux x86_64|Linux i686/i.test(userAgent)) {
      return true;
    }
    return false;
  }, []);
  const isWindowsPlatform = useMemo(
    () => /windows/i.test(window.navigator.userAgent),
    [],
  );

  const [workspaceUiState, dispatchWorkspaceUi] = useReducer(
    workspaceUiReducer,
    persistedGlobal,
    globalState =>
      createWorkspaceUiState({
        tab: globalState.tab ?? 'file',
        collapsedProjectIds: globalState.collapsedProjectIds ?? globalState.desktopCollapsedProjectIds ?? [],
        desktopSidebarWidth: globalState.desktopSidebarWidth,
        pinnedProjectIds: globalState.pinnedProjectIds ?? [],
        floatingControlSlot: globalState.floatingControlSlot ?? 'upper-middle',
      }),
  );
  const tab = workspaceUiState.shared.tab as Tab;
  const floatingControlSlot = workspaceUiState.mobile.floatingControlSlot;
  const floatingDragState = workspaceUiState.transient.floatingDragState as FloatingDragState | null;
  const floatingKeyboardOffset = workspaceUiState.transient.floatingKeyboardOffset;
  const sidebarCollapsed = workspaceUiState.desktop.sidebarCollapsed;
  const desktopSidebarWidth = workspaceUiState.desktop.sidebarWidth;
  const collapsedProjectIds = workspaceUiState.shared.collapsedProjectIds;
  const pinnedProjectIds = workspaceUiState.shared.pinnedProjectIds;
  const drawerOpen = workspaceUiState.mobile.drawerOpen;
  const sidebarSettingsOpen = workspaceUiState.shared.settingsOpen;
  const chatConfigOverflowOpen = workspaceUiState.mobile.chatConfigOverflowOpen;
  const chatKeyboardInset = workspaceUiState.transient.chatKeyboardInset;
  const tabRef = useRef<Tab>(tab);
  const floatingDragStateRef = useRef<FloatingDragState | null>(null);
  const [floatingControlStackHeight, setFloatingControlStackHeight] = useState(184);
  const floatingLongPressTimerRef = useRef<number | null>(null);
  const floatingCooldownTimerRef = useRef<number | null>(null);
  const floatingClickCooldownUntilRef = useRef(0);
  const floatingIgnoreLostCaptureRef = useRef(false);
  const floatingControlStackRef = useRef<HTMLDivElement | null>(null);
  const desktopSidebarResizeRef = useRef<DesktopSidebarResizeState | null>(null);
  const projectPinLongPressTimerRef = useRef<number | null>(null);
  const projectPinLongPressTargetRef = useRef('');
  const projectSessionLongPressTimerRef = useRef<number | null>(null);
  const projectSessionLongPressTargetRef = useRef('');
  const layoutModeRef = useRef(layoutMode);
  const setTab = useCallback((next: WorkspaceUiStateValue<Tab>) => {
    dispatchWorkspaceUi({ type: 'shared/setTab', next });
  }, []);
  const setFloatingControlSlot = useCallback(
    (next: WorkspaceUiStateValue<PersistedFloatingControlSlot>) => {
      dispatchWorkspaceUi({ type: 'mobile/setFloatingControlSlot', next });
    },
    [],
  );
  const setFloatingDragState = useCallback(
    (next: WorkspaceUiStateValue<FloatingDragState | null>) => {
      dispatchWorkspaceUi({ type: 'transient/setFloatingDragState', next });
    },
    [],
  );
  const setFloatingKeyboardOffset = useCallback((next: WorkspaceUiStateValue<number>) => {
    dispatchWorkspaceUi({ type: 'transient/setFloatingKeyboardOffset', next });
  }, []);
  const setSidebarCollapsed = useCallback((next: WorkspaceUiStateValue<boolean>) => {
    dispatchWorkspaceUi({ type: 'desktop/setSidebarCollapsed', next });
  }, []);
  const setDesktopSidebarWidth = useCallback((next: WorkspaceUiStateValue<number>) => {
    dispatchWorkspaceUi({ type: 'desktop/setSidebarWidth', next });
  }, []);
  const setCollapsedProjectIds = useCallback((next: WorkspaceUiStateValue<string[]>) => {
    dispatchWorkspaceUi({ type: 'shared/setCollapsedProjectIds', next });
  }, []);
  const setPinnedProjectIds = useCallback((next: WorkspaceUiStateValue<string[]>) => {
    dispatchWorkspaceUi({ type: 'shared/setPinnedProjectIds', next });
  }, []);
  const setDrawerOpen = useCallback((next: WorkspaceUiStateValue<boolean>) => {
    dispatchWorkspaceUi({ type: 'mobile/setDrawerOpen', next });
  }, []);
  const setSidebarSettingsOpen = useCallback((next: WorkspaceUiStateValue<boolean>) => {
    dispatchWorkspaceUi({ type: 'shared/setSettingsOpen', next });
  }, []);
  const setChatConfigOverflowOpen = useCallback((next: WorkspaceUiStateValue<boolean>) => {
    dispatchWorkspaceUi({ type: 'mobile/setChatConfigOverflowOpen', next });
  }, []);
  const setChatKeyboardInset = useCallback((next: WorkspaceUiStateValue<number>) => {
    dispatchWorkspaceUi({ type: 'transient/setChatKeyboardInset', next });
  }, []);
  const [databasePanelOpen, setDatabasePanelOpen] = useState(false);
  const [databaseLoading, setDatabaseLoading] = useState(false);
  const [databaseError, setDatabaseError] = useState('');
  const [databaseDumpText, setDatabaseDumpText] = useState('');
  const [settingsDetailView, setSettingsDetailView] = useState<SettingsDetailView>(null);
  const [desktopSidebarResizing, setDesktopSidebarResizing] = useState(false);
  const [desktopSidebarDraftWidth, setDesktopSidebarDraftWidth] = useState<number | null>(null);
  const [tokenStatsLoading, setTokenStatsLoading] = useState(false);
  const [tokenStatsError, setTokenStatsError] = useState('');
  const [tokenStatsUpdatedAt, setTokenStatsUpdatedAt] = useState('');
  const [tokenStatsProviders, setTokenStatsProviders] = useState<TokenProviderSectionView[]>([]);

  const [projectMenuOpen, setProjectMenuOpen] = useState(false);

  const [projects, setProjects] = useState<RegistryProject[]>([]);
  const [projectId, setProjectId] = useState('');
  const projectIdRef = useRef('');
  const projectsRef = useRef<RegistryProject[]>([]);
  const currentProjectRef = useRef<RegistryProject | null>(null);
  const knownProjectRevRef = useRef('');
  const knownGitRevRef = useRef('');
  const knownWorktreeRevRef = useRef('');
  const [loadingProject, setLoadingProject] = useState(false);
  const [refreshingProject, setRefreshingProject] = useState(false);
  const [hasPendingProjectUpdates, setHasPendingProjectUpdates] = useState(false);

  const [dirEntries, setDirEntries] = useState<DirEntries>({ '.': [] });
  const [expandedDirs, setExpandedDirs] = useState<string[]>(['.']);
  const expandedDirsRef = useRef<string[]>(['.']);
  const [loadingDirs, setLoadingDirs] = useState<Record<string, boolean>>({});
  const [selectedFile, setSelectedFile] = useState('');
  const selectedFileRef = useRef('');
  const [pinnedFiles, setPinnedFiles] = useState<string[]>([]);
  const [fileContent, setFileContent] = useState('');
  const [fileInfo, setFileInfo] = useState<RegistryFsInfo | null>(null);
  const [fileLoading, setFileLoading] = useState(false);
  const [fileSearchQuery, setFileSearchQuery] = useState('');
  const [currentMatchIndex, setCurrentMatchIndex] = useState(0);
  const [gotoLineInput, setGotoLineInput] = useState('');
  const [pendingFileJump, setPendingFileJump] = useState<{
    path: string;
    line: number;
  } | null>(null);
  const [searchToolsOpen, setSearchToolsOpen] = useState(false);
  const [gotoToolsOpen, setGotoToolsOpen] = useState(false);
  const [markdownPreviewEnabled, setMarkdownPreviewEnabled] = useState(false);
  const fileScrollRef = useRef<HTMLDivElement | null>(null);
  const liveRefreshTimerRef = useRef<number | null>(null);
  const refreshInFlightRef = useRef(false);
  const reconnectTimerRef = useRef<number | null>(null);
  const reconnectStartedAtRef = useRef<number | null>(null);
  const connectInFlightRef = useRef(false);
  const supervisorManagedCloseRef = useRef(false);
  const dirHashRef = useRef<Record<string, string>>({});
  const fileHashRef = useRef<Record<string, string>>({});
  const fileCacheRef = useRef<Record<string, string>>({});
  const fileReadSeqRef = useRef(0);
  const fileScrollTopByPathRef = useRef<Record<string, number>>({});
  const skipNextSelectedFileAutoReadRef = useRef(false);
  const fileSideActionsRef = useRef<HTMLDivElement | null>(null);
  const commitPopoverRef = useRef<HTMLDivElement | null>(null);
  const gitBranchMenuRef = useRef<HTMLDivElement | null>(null);
  const gitSelectedBranchesRef = useRef<string[]>([]);
  const chatFileInputRef = useRef<HTMLInputElement | null>(null);
  const chatScrollRef = useRef<HTMLDivElement | null>(null);
  const chatAutoScrollFollowRef = useRef(true);
  const chatPointerScrollingRef = useRef(false);
  const chatLastScrollTopRef = useRef(0);
  const chatComposerTextareaRef = useRef<HTMLTextAreaElement | null>(null);
  const chatPromptButtonRef = useRef<HTMLButtonElement | null>(null);
  const chatSlashMenuRef = useRef<HTMLDivElement | null>(null);
  const chatConfigOptionsRef = useRef<HTMLDivElement | null>(null);
  const chatConfigOverflowRef = useRef<HTMLDivElement | null>(null);
  const wideProjectActionMenuRef = useRef<HTMLDivElement | null>(null);
  const projectSessionSentinelRefs = useRef<Record<string, HTMLDivElement | null>>({});
  const chatSelectedIdRef = useRef('');
  const selectedChatKeyRef = useRef<ChatSessionKey | null>(null);
  const chatSyncIndexRef = useRef<Record<string, number>>({});
  const chatSyncSubIndexRef = useRef<Record<string, number>>({});
  const chatMessageStoreRef = useRef<Record<string, RegistryChatMessage[]>>({});
  const chatMessagesRef = useRef<RegistryChatMessage[]>([]);
  const chatVisibleWindowRef = useRef<Record<string, ChatTurnWindow>>({});
  const notifiedChatMessageIdsRef = useRef<Set<string>>(new Set());
  const chatIndexFullRefreshInFlightRef = useRef(false);
  const chatIndexFullRefreshDirtyRef = useRef(false);
  const chatProjectRefreshInFlightRef = useRef<Record<string, boolean>>({});
  const chatProjectRefreshDirtyRef = useRef<Record<string, boolean>>({});
  const [chatSessions, setChatSessions] = useState<RegistryChatSession[]>([]);
  const [projectSessionsByProjectId, setProjectSessionsByProjectId] = useState<Record<string, RegistryChatSession[]>>({});
  const projectSessionsByProjectIdRef = useRef<Record<string, RegistryChatSession[]>>({});
  const [wideProjectVisibleCounts, setWideProjectVisibleCounts] = useState<Record<string, number>>({});
  const [wideProjectActionMenu, setWideProjectActionMenu] = useState<WideProjectActionMenuState | null>(null);
  const [mobileProjectActionMenu, setMobileProjectActionMenu] = useState<MobileProjectActionMenuState | null>(null);
  const [projectSessionActionMenu, setProjectSessionActionMenu] = useState<ProjectSessionActionMenuState | null>(null);
  const [mobileProjectSessionErrors, setMobileProjectSessionErrors] = useState<Record<string, string>>({});
  const [mobileProjectSessionsRefreshing, setMobileProjectSessionsRefreshing] = useState(false);
  const [selectedChatId, setSelectedChatId] = useState('');
  const [selectedChatKey, setSelectedChatKey] = useState<ChatSessionKey | null>(null);
  const [chatMessages, setChatMessages] = useState<RegistryChatMessage[]>([]);
  const [chatLoading, setChatLoading] = useState(false);
  const [chatSending, setChatSending] = useState(false);
  const [chatShowScrollToBottom, setChatShowScrollToBottom] = useState(false);
  const [chatDeletingSessionId, setChatDeletingSessionId] = useState('');
  const [chatReloadingSessionId, setChatReloadingSessionId] = useState('');
  const [chatRunningSessionFlags, setChatRunningSessionFlags] = useState<SessionFlagMap>({});
  const [chatCompletedUnopenedFlags, setChatCompletedUnopenedFlags] = useState<SessionFlagMap>({});
  const [chatConfigUpdatingKey, setChatConfigUpdatingKey] = useState('');
  const [chatComposerText, setChatComposerText] = useState('');
  const [chatAttachments, setChatAttachments] = useState<ChatAttachment[]>([]);
  const [chatAttachmentReadPending, setChatAttachmentReadPending] = useState(false);
  const [chatComposerDrafts, setChatComposerDrafts] = useState<Record<string, ChatComposerDraft>>({});
  const [chatPendingPromptsByKey, setChatPendingPromptsByKey] = useState<Record<string, PendingChatPrompt>>({});
  const chatComposerTextRef = useRef('');
  const chatAttachmentsRef = useRef<ChatAttachment[]>([]);
  const chatComposerDraftsRef = useRef<Record<string, ChatComposerDraft>>({});
  const chatPendingPromptsByKeyRef = useRef<Record<string, PendingChatPrompt>>({});
  const chatAttachmentReadCountRef = useRef<Record<string, number>>({});
  const chatDraftGenerationRef = useRef<Record<string, number>>({});
  const currentChatDraftKeyRef = useRef('');
  const chatAttachmentIdRef = useRef(0);
  const [chatPromptMenuOpen, setChatPromptMenuOpen] = useState(false);
  const [chatConfigMenuOptionId, setChatConfigMenuOptionId] = useState('');
  const [chatSlashActiveIndex, setChatSlashActiveIndex] = useState(0);
  const [resumeSessions, setResumeSessions] = useState<RegistryResumableSession[]>([]);
  const [resumeLoading, setResumeLoading] = useState(false);

  const selectedChatEncodedKey = useMemo(
    () => encodeChatSessionKey(selectedChatKey),
    [selectedChatKey],
  );

  const selectedChatSession = useMemo(
    () => {
      if (!selectedChatKey) {
        return undefined;
      }
      const sessions =
        projectSessionsByProjectId[selectedChatKey.projectId] ??
        (selectedChatKey.projectId === projectId ? chatSessions : []);
      return sessions.find(item => item.sessionId === selectedChatKey.sessionId);
    },
    [chatSessions, projectId, projectSessionsByProjectId, selectedChatKey],
  );

  const selectedChatConfigOptions = useMemo(() => {
    return selectedChatSession?.configOptions ?? [];
  }, [selectedChatSession]);

  const chatConfigDisplay = useMemo(() => {
    if (selectedChatConfigOptions.length === 0) {
      return {
        visible: selectedChatConfigOptions,
        overflow: [] as RegistrySessionConfigOption[],
      };
    }
    if (selectedChatConfigOptions.length <= CHAT_CONFIG_INLINE_LIMIT) {
      return {
        visible: selectedChatConfigOptions,
        overflow: [] as RegistrySessionConfigOption[],
      };
    }
    const prioritized = selectedChatConfigOptions
      .map((option, index) => ({ option, index, rank: chatConfigPriority(option) }))
      .sort((left, right) => {
        if (left.rank !== right.rank) {
          return left.rank - right.rank;
        }
        return left.index - right.index;
      });
    const visibleIds = new Set(
      prioritized.slice(0, CHAT_CONFIG_INLINE_LIMIT).map(item => item.option.id),
    );
    const visible: RegistrySessionConfigOption[] = [];
    const overflow: RegistrySessionConfigOption[] = [];
    for (const option of selectedChatConfigOptions) {
      if (visibleIds.has(option.id) && visible.length < CHAT_CONFIG_INLINE_LIMIT) {
        visible.push(option);
        visibleIds.delete(option.id);
      } else {
        overflow.push(option);
      }
    }
    return { visible, overflow };
  }, [selectedChatConfigOptions]);

  const chatSlashSkills = useMemo(() => {
    const currentProject = projects.find(item => item.projectId === projectId);
    const deduped = new Map<string, string>();
    for (const profile of currentProject?.agentProfiles ?? []) {
      for (const skill of profile.skills ?? []) {
        const normalized = (skill || '').trim();
        if (!normalized) {
          continue;
        }
        const key = normalized.toLowerCase();
        if (!deduped.has(key)) {
          deduped.set(key, normalized);
        }
      }
    }
    return Array.from(deduped.values()).sort((left, right) => left.localeCompare(right, undefined, { sensitivity: 'base' }));
  }, [projects, projectId]);

  const chatSlashCommands = useMemo(
    () => normalizeChatSlashCommands(chatSlashSkills),
    [chatSlashSkills],
  );

  const chatSlashQuery = useMemo(
    () => parseChatSlashQuery(chatComposerText),
    [chatComposerText],
  );

  const chatSlashCommandOptions = useMemo(
    () => filterChatSlashCommands(chatSlashCommands, chatSlashQuery),
    [chatSlashCommands, chatSlashQuery],
  );

  const chatSlashMenuOptions = useMemo(
    () => (chatPromptMenuOpen ? chatSlashCommands : chatSlashCommandOptions),
    [chatPromptMenuOpen, chatSlashCommands, chatSlashCommandOptions],
  );

  const chatSlashMenuVisible = chatSlashMenuOptions.length > 0;


  const currentChatDraftKey = useMemo(
    () => buildChatDraftKey(selectedChatKey?.projectId ?? projectId, selectedChatId),
    [projectId, selectedChatId, selectedChatKey?.projectId],
  );

  const buildChatRuntimeKey = (
    activeProjectId: string,
    sessionId: string,
  ): string => encodeChatSessionKey(chatSessionKeyFromParts(activeProjectId, sessionId));

  const setVisibleChatMessagesForRuntimeKey = useCallback((
    runtimeKey: string,
    fullMessages: RegistryChatMessage[],
    options?: { resetToLatest?: boolean; followLatest?: boolean },
  ) => {
    if (!runtimeKey) {
      chatMessagesRef.current = [];
      setChatMessages([]);
      return;
    }
    const currentWindow = chatVisibleWindowRef.current[runtimeKey];
    const nextWindow = options?.resetToLatest || !currentWindow
      ? createLatestTurnWindow(fullMessages)
      : options?.followLatest
        ? followLatestTurnWindow(fullMessages, currentWindow)
        : currentWindow;
    chatVisibleWindowRef.current[runtimeKey] = nextWindow;
    if (encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey) {
      const visibleMessages = sliceTurnsForWindow(fullMessages, nextWindow);
      chatMessagesRef.current = visibleMessages;
      setChatMessages(visibleMessages);
    }
  }, []);

  const applySelectedChatKey = (key: ChatSessionKey | null) => {
    selectedChatKeyRef.current = key;
    setSelectedChatKey(key);
    const sessionId = key?.sessionId ?? '';
    chatSelectedIdRef.current = sessionId;
    setSelectedChatId(sessionId);
  };

  const resizeChatComposerTextarea = useCallback(() => {
    const input = chatComposerTextareaRef.current;
    if (!input) {
      return;
    }
    input.style.height = '0px';
    const nextHeight = Math.max(36, Math.min(input.scrollHeight, 180));
    input.style.height = `${nextHeight}px`;
    input.style.overflowY = input.scrollHeight > 180 ? 'auto' : 'hidden';
  }, []);

  const updateChatFollowModeFromScroll = useCallback(
    (container = chatScrollRef.current) => {
      if (!container) {
        return;
      }
      const nearBottom = isChatScrolledNearBottom(container);
      const runtimeKey = encodeChatSessionKey(selectedChatKeyRef.current);
      const fullMessages = runtimeKey ? chatMessageStoreRef.current[runtimeKey] ?? [] : [];
      const currentWindow = runtimeKey ? chatVisibleWindowRef.current[runtimeKey] : undefined;
      const coversKnownLatest =
        !runtimeKey ||
        fullMessages.length === 0 ||
        !currentWindow ||
        currentWindow.endIndex >= fullMessages.length;
      const followsLatest = nearBottom && coversKnownLatest;
      chatAutoScrollFollowRef.current = followsLatest;
      setChatShowScrollToBottom(!followsLatest);
    },
    [],
  );

  const scrollChatToBottom = useCallback((force = false) => {
    if (!force && (!chatAutoScrollFollowRef.current || chatPointerScrollingRef.current)) {
      return;
    }
    window.requestAnimationFrame(() => {
      const container = chatScrollRef.current;
      if (!container) {
        return;
      }
      if (!force && (!chatAutoScrollFollowRef.current || chatPointerScrollingRef.current)) {
        return;
      }
      container.scrollTop = container.scrollHeight;
      chatAutoScrollFollowRef.current = true;
    });
  }, []);

  const forceChatScrollToBottom = useCallback(() => {
    const runtimeKey = encodeChatSessionKey(selectedChatKeyRef.current);
    if (runtimeKey) {
      setVisibleChatMessagesForRuntimeKey(
        runtimeKey,
        chatMessageStoreRef.current[runtimeKey] ?? [],
        {resetToLatest: true},
      );
    }
    chatAutoScrollFollowRef.current = true;
    setChatShowScrollToBottom(false);
    scrollChatToBottom(true);
  }, [scrollChatToBottom, setVisibleChatMessagesForRuntimeKey]);

  const visibleChatMessageElements = (container: HTMLElement): HTMLElement[] => {
    const containerRect = container.getBoundingClientRect();
    return Array.from(container.querySelectorAll<HTMLElement>('[data-chat-message-key]'))
      .filter(element => {
        const rect = element.getBoundingClientRect();
        return rect.bottom > containerRect.top && rect.top < containerRect.bottom;
      });
  };

  const chatScrollAnchor = (
    container: HTMLElement,
    direction: 'up' | 'down',
  ): { key: string; top: number } | null => {
    const visible = visibleChatMessageElements(container);
    const element = direction === 'up' ? visible[0] : visible[visible.length - 1];
    const key = element?.dataset.chatMessageKey ?? '';
    if (!element || !key) {
      return null;
    }
    return {
      key,
      top: element.getBoundingClientRect().top,
    };
  };

  const restoreChatScrollAnchor = (
    key: string,
    previousTop: number,
  ) => {
    window.requestAnimationFrame(() => {
      const container = chatScrollRef.current;
      if (!container) {
        return;
      }
      const nextElement = Array.from(container.querySelectorAll<HTMLElement>('[data-chat-message-key]'))
        .find(element => element.dataset.chatMessageKey === key);
      if (!nextElement) {
        return;
      }
      container.scrollTop += nextElement.getBoundingClientRect().top - previousTop;
    });
  };

  const updateSelectedChatWindowFromScroll = (container: HTMLElement, direction: 'up' | 'down') => {
    const runtimeKey = encodeChatSessionKey(selectedChatKeyRef.current);
    if (!runtimeKey) {
      return;
    }
    const fullMessages = chatMessageStoreRef.current[runtimeKey] ?? [];
    const currentWindow =
      chatVisibleWindowRef.current[runtimeKey] ??
      createLatestTurnWindow(fullMessages);
    const boundaryAnchor = chatScrollAnchor(container, direction);
    const boundaryKey = boundaryAnchor?.key ?? '';
    const boundaryIndex = boundaryKey
      ? chatMessagesRef.current.findIndex(message => chatMessageDomKey(message) === boundaryKey)
      : -1;
    const boundaryAbsoluteIndex = boundaryIndex >= 0
      ? currentWindow.startIndex + boundaryIndex
      : -1;
    const distanceToWindowEdge = boundaryAbsoluteIndex >= 0
      ? direction === 'up'
        ? boundaryAbsoluteIndex - currentWindow.startIndex
        : currentWindow.endIndex - boundaryAbsoluteIndex - 1
      : Number.POSITIVE_INFINITY;
    const nearPixelEdge = direction === 'up'
      ? container.scrollTop <= 120
      : isChatScrolledNearBottom(container);
    if (distanceToWindowEdge >= 25 && !nearPixelEdge) {
      return;
    }
    const expandedWindow = direction === 'up'
      ? expandTurnWindowEarlier(fullMessages, currentWindow)
      : expandTurnWindowLater(fullMessages, currentWindow);
    if (
      expandedWindow.startIndex === currentWindow.startIndex &&
      expandedWindow.endIndex === currentWindow.endIndex
    ) {
      return;
    }
    const anchor = boundaryAnchor ?? chatScrollAnchor(container, direction === 'up' ? 'down' : 'up');
    chatVisibleWindowRef.current[runtimeKey] = expandedWindow;
    const visibleMessages = sliceTurnsForWindow(fullMessages, expandedWindow);
    chatMessagesRef.current = visibleMessages;
    setChatMessages(visibleMessages);
    if (anchor) {
      restoreChatScrollAnchor(anchor.key, anchor.top);
    }
  };

  useEffect(() => {
    if (!chatSlashMenuVisible) {
      setChatSlashActiveIndex(0);
      return;
    }
    setChatSlashActiveIndex(prev => Math.max(0, Math.min(prev, chatSlashMenuOptions.length - 1)));
  }, [chatSlashMenuVisible, chatSlashMenuOptions]);

  useEffect(() => {
    if (!chatSlashMenuVisible) {
      return;
    }
    const menu = chatSlashMenuRef.current;
    if (!menu) {
      return;
    }
    const activeItem = menu.querySelector<HTMLElement>('.chat-slash-item.active');
    if (!activeItem) {
      return;
    }
    activeItem.scrollIntoView({ block: 'nearest' });
  }, [chatSlashMenuVisible, chatSlashActiveIndex, chatSlashMenuOptions]);

  const saveChatComposerDraft = useCallback(
    (draftKey: string, text: string, attachments: ChatAttachment[]) => {
      const normalizedKey = draftKey.trim();
      if (!normalizedKey) {
        return;
      }
      const prev = chatComposerDraftsRef.current;
      const existing = prev[normalizedKey] ?? EMPTY_CHAT_COMPOSER_DRAFT;
      const hasContent = text.length > 0 || attachments.length > 0;
      if (!hasContent) {
        if (!(normalizedKey in prev)) {
          return;
        }
        const next = { ...prev };
        delete next[normalizedKey];
        chatComposerDraftsRef.current = next;
        setChatComposerDrafts(next);
        return;
      }
      if (existing.text === text && existing.attachments === attachments) {
        return;
      }
      const next = {
        ...prev,
        [normalizedKey]: {
          text,
          attachments,
        },
      };
      chatComposerDraftsRef.current = next;
      setChatComposerDrafts(next);
    },
    [],
  );

  const updateChatComposerText = useCallback(
    (nextText: string) => {
      setChatComposerText(nextText);
      saveChatComposerDraft(
        currentChatDraftKeyRef.current,
        nextText,
        chatAttachmentsRef.current,
      );
    },
    [saveChatComposerDraft],
  );

  const applyChatSlashCommand = useCallback(
    (command: ChatSlashCommandOption) => {
      const next = `${command.name} `;
      setChatPromptMenuOpen(false);
      setChatConfigMenuOptionId('');
      setChatConfigOverflowOpen(false);
      updateChatComposerText(next);
      window.requestAnimationFrame(() => {
        const input = chatComposerTextareaRef.current;
        if (!input) {
          return;
        }
        input.focus();
        input.setSelectionRange(next.length, next.length);
      });
    },
    [setChatConfigOverflowOpen, updateChatComposerText],
  );

  const openChatPromptMenu = useCallback(() => {
    setChatConfigMenuOptionId('');
    setChatConfigOverflowOpen(false);
    setChatPromptMenuOpen(value => !value);
    window.requestAnimationFrame(() => {
      const target = chatComposerTextareaRef.current;
      if (!target) {
        return;
      }
      target.focus();
    });
  }, [setChatConfigOverflowOpen]);

  const getChatDraftGeneration = useCallback((draftKey: string) => {
    const normalizedDraftKey = draftKey.trim();
    if (!normalizedDraftKey) {
      return 0;
    }
    return chatDraftGenerationRef.current[normalizedDraftKey] ?? 0;
  }, []);

  const bumpChatDraftGeneration = useCallback(
    (draftKey: string) => {
      const normalizedDraftKey = draftKey.trim();
      if (!normalizedDraftKey) {
        return 0;
      }
      const nextGeneration = getChatDraftGeneration(normalizedDraftKey) + 1;
      chatDraftGenerationRef.current[normalizedDraftKey] = nextGeneration;
      return nextGeneration;
    },
    [getChatDraftGeneration],
  );

  const syncChatAttachmentReadPending = useCallback(() => {
    const normalizedDraftKey = currentChatDraftKeyRef.current.trim();
    if (!normalizedDraftKey) {
      setChatAttachmentReadPending(false);
      return;
    }
    setChatAttachmentReadPending(
      (chatAttachmentReadCountRef.current[normalizedDraftKey] ?? 0) > 0,
    );
  }, []);

  const beginChatAttachmentRead = useCallback(
    (draftKey: string) => {
      const normalizedDraftKey = draftKey.trim();
      if (!normalizedDraftKey) {
        return;
      }
      chatAttachmentReadCountRef.current[normalizedDraftKey] =
        (chatAttachmentReadCountRef.current[normalizedDraftKey] ?? 0) + 1;
      syncChatAttachmentReadPending();
    },
    [syncChatAttachmentReadPending],
  );

  const endChatAttachmentRead = useCallback(
    (draftKey: string) => {
      const normalizedDraftKey = draftKey.trim();
      if (!normalizedDraftKey) {
        return;
      }
      const nextCount = Math.max(
        0,
        (chatAttachmentReadCountRef.current[normalizedDraftKey] ?? 0) - 1,
      );
      if (nextCount === 0) {
        delete chatAttachmentReadCountRef.current[normalizedDraftKey];
      } else {
        chatAttachmentReadCountRef.current[normalizedDraftKey] = nextCount;
      }
      syncChatAttachmentReadPending();
    },
    [syncChatAttachmentReadPending],
  );

  const applyChatAttachments = useCallback(
    (
      updater: (current: ChatAttachment[]) => ChatAttachment[],
      draftKey = currentChatDraftKeyRef.current,
      expectedGeneration = getChatDraftGeneration(draftKey),
    ) => {
      const normalizedDraftKey = draftKey.trim();
      if (!normalizedDraftKey) {
        return;
      }
      if (expectedGeneration !== getChatDraftGeneration(normalizedDraftKey)) {
        return;
      }
      if (normalizedDraftKey === currentChatDraftKeyRef.current) {
        const next = updater(chatAttachmentsRef.current);
        if (next === chatAttachmentsRef.current) {
          return;
        }
        chatAttachmentsRef.current = next;
        setChatAttachments(next);
        saveChatComposerDraft(
          normalizedDraftKey,
          chatComposerTextRef.current,
          next,
        );
        if (next.length === 0 && chatFileInputRef.current) {
          chatFileInputRef.current.value = '';
        }
        return;
      }
      const currentDraft =
        chatComposerDraftsRef.current[normalizedDraftKey] ??
        EMPTY_CHAT_COMPOSER_DRAFT;
      const next = updater(currentDraft.attachments);
      if (next === currentDraft.attachments) {
        return;
      }
      saveChatComposerDraft(normalizedDraftKey, currentDraft.text, next);
    },
    [getChatDraftGeneration, saveChatComposerDraft],
  );

  const appendChatAttachments = useCallback(
    (
      nextAttachments: ChatAttachment[],
      draftKey = currentChatDraftKeyRef.current,
      expectedGeneration = getChatDraftGeneration(draftKey),
    ) => {
      if (nextAttachments.length === 0) {
        return;
      }
      applyChatAttachments(
        current => [...current, ...nextAttachments],
        draftKey,
        expectedGeneration,
      );
    },
    [applyChatAttachments, getChatDraftGeneration],
  );

  const removeChatAttachment = useCallback(
    (attachmentId: string) => {
      if (!attachmentId) {
        return;
      }
      applyChatAttachments(current => {
        const filtered = current.filter(attachment => attachment.id !== attachmentId);
        return filtered.length === current.length ? current : filtered;
      });
    },
    [applyChatAttachments],
  );

  const readChatAttachmentFile = useCallback(
    async (file: File, fallbackName: string): Promise<ChatAttachment> => {
      const dataUrl = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () =>
          resolve(typeof reader.result === 'string' ? reader.result : '');
        reader.onerror = () =>
          reject(reader.error ?? new Error('Failed to read image file'));
        reader.readAsDataURL(file);
      });
      const base64 = dataUrl.includes(',')
        ? dataUrl.slice(dataUrl.indexOf(',') + 1)
        : dataUrl;
      chatAttachmentIdRef.current += 1;
      return {
        id: `chat-attachment-${chatAttachmentIdRef.current}`,
        name: file.name || fallbackName,
        mimeType: file.type || 'image/png',
        data: base64,
      };
    },
    [],
  );

  useEffect(() => {
    currentChatDraftKeyRef.current = currentChatDraftKey;
    syncChatAttachmentReadPending();
  }, [currentChatDraftKey, syncChatAttachmentReadPending]);

  useEffect(() => {
    chatComposerTextRef.current = chatComposerText;
  }, [chatComposerText]);

  useEffect(() => {
    chatAttachmentsRef.current = chatAttachments;
  }, [chatAttachments]);

  useEffect(() => {
    chatComposerDraftsRef.current = chatComposerDrafts;
  }, [chatComposerDrafts]);

  useEffect(() => {
    chatPendingPromptsByKeyRef.current = chatPendingPromptsByKey;
  }, [chatPendingPromptsByKey]);

  useEffect(() => {
    chatMessagesRef.current = chatMessages;
  }, [chatMessages]);

  useEffect(() => {
    const draft =
      chatComposerDraftsRef.current[currentChatDraftKey] ??
      EMPTY_CHAT_COMPOSER_DRAFT;
    if (chatComposerTextRef.current !== draft.text) {
      setChatComposerText(draft.text);
    }
    if (chatAttachmentsRef.current !== draft.attachments) {
      chatAttachmentsRef.current = draft.attachments;
      setChatAttachments(draft.attachments);
    }
    if (draft.attachments.length === 0 && chatFileInputRef.current) {
      chatFileInputRef.current.value = '';
    }
  }, [currentChatDraftKey]);

  useEffect(() => {
    if (!selectedChatEncodedKey) {
      return;
    }
    setChatCompletedUnopenedFlags(prev => removeSessionFlag(prev, selectedChatEncodedKey));
  }, [selectedChatEncodedKey]);

  const [gitLoading, setGitLoading] = useState(false);
  const [gitError, setGitError] = useState('');
  const [gitCurrentBranch, setGitCurrentBranch] = useState('');
  const [gitBranches, setGitBranches] = useState<string[]>([]);
  const [gitSelectedBranches, setGitSelectedBranches] = useState<string[]>([]);
  const [gitBranchPickerOpen, setGitBranchPickerOpen] = useState(false);
  const [gitLoadedProjectId, setGitLoadedProjectId] = useState('');
  const [commits, setCommits] = useState<RegistryGitCommit[]>([]);
  const [selectedCommit, setSelectedCommit] = useState('');
  const [expandedCommitShas, setExpandedCommitShas] = useState<string[]>([]);
  const [commitFilesBySha, setCommitFilesBySha] = useState<
    Record<string, RegistryGitCommitFile[]>
  >({});
  const [workingTreeFiles, setWorkingTreeFiles] = useState<
    WorkingTreeFileEntry[]
  >([]);
  const [worktreeExpanded, setWorktreeExpanded] = useState(true);
  const [commitPopover, setCommitPopover] = useState<{
    commit: RegistryGitCommit;
    x: number;
    y: number;
    width: number;
  } | null>(null);
  const [selectedDiffSource, setSelectedDiffSource] =
    useState<GitDiffSource>('commit');
  const [selectedDiffScope, setSelectedDiffScope] = useState<
    'staged' | 'unstaged' | 'untracked'
  >('unstaged');
  const [selectedDiff, setSelectedDiff] = useState('');
  const [allowHeavyDiffLoad, setAllowHeavyDiffLoad] = useState(false);
  const [allowLargeDiffRender, setAllowLargeDiffRender] = useState(false);
  const [diffText, setDiffText] = useState('');
  const [diffLoading, setDiffLoading] = useState(false);
  const projectIdListKey = useMemo(
    () => projects.map(item => item.projectId).join('|'),
    [projects],
  );
  const sortedProjectItems = useMemo(() => sortProjectsByPin(projects, pinnedProjectIds), [projects, pinnedProjectIds]);
  const projectSessionCountKey = useMemo(
    () => Object.entries(projectSessionsByProjectId)
      .map(([entryProjectId, sessions]) => `${entryProjectId}:${sessions.length}`)
      .sort()
      .join('|'),
    [projectSessionsByProjectId],
  );

  const expandProjectSessionVisibleCount = useCallback((targetProjectId: string) => {
    const total = projectSessionsByProjectIdRef.current[targetProjectId]?.length ?? 0;
    if (total <= 0) {
      return;
    }
    setWideProjectVisibleCounts(prev => {
      const current = prev[targetProjectId] ?? WIDE_PROJECT_SESSION_LIMIT;
      if (current >= total) {
        return prev;
      }
      return {
        ...prev,
        [targetProjectId]: Math.min(total, current + WIDE_PROJECT_SESSION_LIMIT),
      };
    });
  }, []);

  useEffect(() => {
    if (typeof IntersectionObserver === 'undefined') {
      return;
    }
    const observer = new IntersectionObserver(
      entries => {
        for (const entry of entries) {
          if (!entry.isIntersecting) {
            continue;
          }
          const targetProjectId = (entry.target as HTMLElement).dataset.projectId || '';
          expandProjectSessionVisibleCount(targetProjectId);
        }
      },
      {root: null, rootMargin: '180px 0px'},
    );
    for (const element of Object.values(projectSessionSentinelRefs.current)) {
      if (element) {
        observer.observe(element);
      }
    }
    return () => observer.disconnect();
  }, [expandProjectSessionVisibleCount, projectIdListKey, projectSessionCountKey, wideProjectVisibleCounts]);

  useEffect(() => {
    projectIdRef.current = projectId;
  }, [projectId]);

  useEffect(() => {
    addressRef.current = address;
  }, [address]);

  useEffect(() => {
    tokenRef.current = token;
  }, [token]);

  useEffect(() => {
    chatSelectedIdRef.current = selectedChatId;
  }, [selectedChatId]);
  useEffect(() => {
    selectedChatKeyRef.current = selectedChatKey;
  }, [selectedChatKey]);
  useEffect(() => {
    projectSessionsByProjectIdRef.current = projectSessionsByProjectId;
  }, [projectSessionsByProjectId]);
  useEffect(() => {
    floatingDragStateRef.current = floatingDragState;
  }, [floatingDragState]);
  useEffect(() => {
    tabRef.current = tab;
    if (tab !== 'chat') {
      return;
    }
    const activeProjectId = projectId || projectIdRef.current;
    if (!connected || !activeProjectId) {
      return;
    }
    const preferredChatKey =
      selectedChatKeyRef.current ||
      workspaceStore.migrateSelectedChatSessionKey(activeProjectId);
    if (preferredChatKey && !selectedChatKeyRef.current) {
      applySelectedChatKey(preferredChatKey);
      hydrateChatSessionsFromCache(preferredChatKey.projectId, preferredChatKey.sessionId);
    }
    loadChatSessions(
      preferredChatKey?.projectId ?? activeProjectId,
      preferredChatKey?.sessionId ?? '',
    ).catch(() => undefined);
  }, [tab, connected, projectId]);

  useEffect(() => {
    if (!projectId) return;
    setProjectSessionsByProjectId(prev => ({
      ...prev,
      [projectId]: chatSessions,
    }));
  }, [projectId, chatSessions]);

  useEffect(() => {
    const shouldHydrateProjectSessionIndex =
      isWide || (!isWide && tab === 'chat' && drawerOpen);
    if (!shouldHydrateProjectSessionIndex || projects.length === 0) return;
    setProjectSessionsByProjectId(prev => {
      const next = {...prev};
      for (const projectItem of projects) {
        const cachedSessions = workspaceStore
          .hydrateChatSessions(projectItem.projectId)
          .map(entry => entry.session);
        if (cachedSessions.length > 0 || !next[projectItem.projectId]) {
          next[projectItem.projectId] = sortChatSessions(cachedSessions);
        }
      }
      return next;
    });
  }, [isWide, tab, drawerOpen, projectIdListKey]);

  useEffect(() => {
    if (!connected || !isWide || projects.length === 0) return;
    let cancelled = false;
    for (const projectItem of projects) {
      service
        .listProjectSessions(projectItem.projectId)
        .then(sessions => {
          if (cancelled) return;
          const sortedSessions = sortChatSessions(sessions);
          setProjectSessionsByProjectId(prev => ({
            ...prev,
            [projectItem.projectId]: sortedSessions,
          }));
          const cached = workspaceStore.hydrateChatSessions(projectItem.projectId);
          const cursorBySessionId: Record<string, {turnIndex: number}> = {};
          for (const entry of cached) {
            cursorBySessionId[entry.session.sessionId] = entry.cursor;
          }
          workspaceStore.replaceChatSessions(
            projectItem.projectId,
            sortedSessions,
            cursorBySessionId,
          );
        })
        .catch(() => undefined);
    }
    return () => {
      cancelled = true;
    };
  }, [connected, isWide, projectIdListKey]);

  useEffect(() => {
    if (!wideProjectActionMenu) return;
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && wideProjectActionMenuRef.current?.contains(target)) {
        return;
      }
      setWideProjectActionMenu(null);
    };
    window.addEventListener('pointerdown', handlePointerDown);
    return () => {
      window.removeEventListener('pointerdown', handlePointerDown);
    };
  }, [wideProjectActionMenu]);


  useLayoutEffect(() => {
    resizeChatComposerTextarea();
  }, [resizeChatComposerTextarea, chatComposerText, tab, selectedChatId, currentChatDraftKey]);

  useEffect(() => {
    if (tab !== 'chat') {
      return;
    }
    forceChatScrollToBottom();
  }, [tab, selectedChatId, forceChatScrollToBottom]);

  useEffect(() => {
    if (tab !== 'chat') {
      return;
    }
    resizeChatComposerTextarea();
    scrollChatToBottom();
  }, [tab, selectedChatId, chatMessages, chatPendingPromptsByKey, chatLoading, chatKeyboardInset, resizeChatComposerTextarea, scrollChatToBottom]);

  useEffect(() => {
    gitSelectedBranchesRef.current = gitSelectedBranches;
  }, [gitSelectedBranches]);

  useEffect(() => {
    setMarkdownPreviewEnabled(isMarkdownPath(selectedFile));
  }, [selectedFile]);
  useEffect(() => {
    setAllowHeavyDiffLoad(false);
    setAllowLargeDiffRender(false);
  }, [selectedDiff, selectedCommit, selectedDiffSource, selectedDiffScope]);

  useEffect(() => {
    const onResize = () => {
      setWindowWidth(window.innerWidth);
      setWindowHeight(window.innerHeight);
      setSafeAreaTopInset(readSafeAreaTopInset());
    };
    onResize();
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  useEffect(() => {
    if (layoutModeRef.current === layoutMode) {
      return;
    }
    dispatchWorkspaceUi({
      type: 'layout/modeChanged',
      from: layoutModeRef.current,
      to: layoutMode,
    });
    layoutModeRef.current = layoutMode;
  }, [layoutMode]);

  useLayoutEffect(() => {
    if (isWide) {
      return;
    }
    const nextFloatingHeight = floatingControlStackRef.current?.offsetHeight ?? 184;
    setFloatingControlStackHeight(prev => (prev === nextFloatingHeight ? prev : nextFloatingHeight));
  }, [
    isWide,
    windowWidth,
    projectId,
    projects.length,
    tab,
  ]);

  useEffect(() => {
    if (isWide || tab !== 'chat') {
      setChatKeyboardInset(0);
      return;
    }
    const viewport = window.visualViewport;
    if (!viewport) {
      return;
    }
    let raf = 0;
    const updateInset = () => {
      const bottomGap = Math.max(
        0,
        Math.round(window.innerHeight - (viewport.height + viewport.offsetTop)),
      );
      const nextInset = bottomGap >= 72 ? bottomGap : 0;
      setChatKeyboardInset(prev => (prev === nextInset ? prev : nextInset));
    };
    const scheduleUpdate = () => {
      if (raf) {
        window.cancelAnimationFrame(raf);
      }
      raf = window.requestAnimationFrame(() => {
        raf = 0;
        updateInset();
      });
    };
    updateInset();
    viewport.addEventListener('resize', scheduleUpdate);
    viewport.addEventListener('scroll', scheduleUpdate);
    window.addEventListener('orientationchange', scheduleUpdate);
    return () => {
      if (raf) {
        window.cancelAnimationFrame(raf);
      }
      viewport.removeEventListener('resize', scheduleUpdate);
      viewport.removeEventListener('scroll', scheduleUpdate);
      window.removeEventListener('orientationchange', scheduleUpdate);
    };
  }, [isWide, tab]);

  useEffect(() => {
    if (isWide || tab !== 'chat') {
      setFloatingKeyboardOffset(0);
      return;
    }
    const viewport = window.visualViewport;
    if (!viewport) {
      return;
    }
    let raf = 0;
    const updateOffset = () => {
      const bottomGap = Math.max(
        0,
        Math.round(window.innerHeight - (viewport.height + viewport.offsetTop)),
      );
      const nextOffset = bottomGap >= 72 ? bottomGap : 0;
      setFloatingKeyboardOffset(prev => (prev === nextOffset ? prev : nextOffset));
    };
    const scheduleUpdate = () => {
      if (raf) {
        window.cancelAnimationFrame(raf);
      }
      raf = window.requestAnimationFrame(() => {
        raf = 0;
        updateOffset();
      });
    };
    updateOffset();
    viewport.addEventListener('resize', scheduleUpdate);
    viewport.addEventListener('scroll', scheduleUpdate);
    window.addEventListener('orientationchange', scheduleUpdate);
    return () => {
      if (raf) {
        window.cancelAnimationFrame(raf);
      }
      viewport.removeEventListener('resize', scheduleUpdate);
      viewport.removeEventListener('scroll', scheduleUpdate);
      window.removeEventListener('orientationchange', scheduleUpdate);
    };
  }, [isWide, tab]);

  useEffect(() => {
    if (!isWide) {
      return;
    }
    if (floatingLongPressTimerRef.current !== null) {
      window.clearTimeout(floatingLongPressTimerRef.current);
      floatingLongPressTimerRef.current = null;
    }
    if (floatingCooldownTimerRef.current !== null) {
      window.clearTimeout(floatingCooldownTimerRef.current);
      floatingCooldownTimerRef.current = null;
    }
    floatingIgnoreLostCaptureRef.current = false;
    setFloatingDragState(null);
    setFloatingKeyboardOffset(0);
  }, [isWide]);

  useEffect(
    () => () => {
      if (floatingLongPressTimerRef.current !== null) {
        window.clearTimeout(floatingLongPressTimerRef.current);
        floatingLongPressTimerRef.current = null;
      }
      if (floatingCooldownTimerRef.current !== null) {
        window.clearTimeout(floatingCooldownTimerRef.current);
        floatingCooldownTimerRef.current = null;
      }
      floatingIgnoreLostCaptureRef.current = false;
    },
    [],
  );

  useEffect(() => {
    const onPointer = () => {
      setProjectMenuOpen(false);
    };
    window.addEventListener('pointerdown', onPointer);
    return () => window.removeEventListener('pointerdown', onPointer);
  }, []);

  useEffect(() => {
    const onPointerDown = (event: PointerEvent) => {
      if (!searchToolsOpen && !gotoToolsOpen) return;
      const container = fileSideActionsRef.current;
      if (!container) return;
      const target = event.target as Node | null;
      if (target && container.contains(target)) return;
      setSearchToolsOpen(false);
      setGotoToolsOpen(false);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [searchToolsOpen, gotoToolsOpen]);
  useEffect(() => {
    if (!commitPopover) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && commitPopoverRef.current?.contains(target)) return;
      setCommitPopover(null);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [commitPopover]);

  useEffect(() => {
    if (!gitBranchPickerOpen || !isWide) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && gitBranchMenuRef.current?.contains(target)) return;
      setGitBranchPickerOpen(false);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [gitBranchPickerOpen, isWide]);

  useEffect(() => {
    if (!chatPromptMenuOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (
        target &&
        (chatSlashMenuRef.current?.contains(target) ||
          chatPromptButtonRef.current?.contains(target))
      ) {
        return;
      }
      setChatPromptMenuOpen(false);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [chatPromptMenuOpen]);

  useEffect(() => {
    if (!chatConfigMenuOptionId) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && chatConfigOptionsRef.current?.contains(target)) return;
      setChatConfigMenuOptionId('');
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [chatConfigMenuOptionId]);

  useEffect(() => {
    if (!chatConfigOverflowOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && chatConfigOverflowRef.current?.contains(target)) return;
      setChatConfigOverflowOpen(false);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [chatConfigOverflowOpen, setChatConfigOverflowOpen]);

  useEffect(() => {
    if (chatConfigDisplay.overflow.length === 0) {
      setChatConfigOverflowOpen(false);
    }
  }, [chatConfigDisplay.overflow.length, selectedChatId, setChatConfigOverflowOpen]);

  useEffect(() => {
    if (!chatConfigMenuOptionId) return;
    if (!chatConfigDisplay.visible.some(option => option.id === chatConfigMenuOptionId)) {
      setChatConfigMenuOptionId('');
    }
  }, [chatConfigMenuOptionId, chatConfigDisplay.visible]);

  useEffect(() => {
    workspaceStore.rememberGlobalState({
      address,
      token,
      themeMode,
      codeTheme,
      codeFont,
      codeFontSize,
      codeLineHeight,
      codeTabSize,
      wrapLines,
      showLineNumbers,
      hideToolCalls,
      tab,
      selectedProjectId: projectId,
      floatingControlSlot,
      desktopSidebarWidth,
      collapsedProjectIds,
      pinnedProjectIds,
    });
  }, [
    address,
    token,
    themeMode,
    codeTheme,
    codeFont,
    codeFontSize,
    codeLineHeight,
    codeTabSize,
    wrapLines,
    showLineNumbers,
    hideToolCalls,
    tab,
    projectId,
    floatingControlSlot,
    desktopSidebarWidth,
    collapsedProjectIds,
    pinnedProjectIds,
  ]);

  useEffect(() => {
    if (!projectId) return;
    workspaceStore.rememberProjectSnapshot(projectId, {
      expandedDirs,
      selectedFile,
      pinnedFiles,
      gitCurrentBranch,
      commits,
      selectedCommit,
      commitFilesBySha,
      selectedDiff,
    });
  }, [
    projectId,
    expandedDirs,
    selectedFile,
    pinnedFiles,
    gitCurrentBranch,
    commits,
    selectedCommit,
    commitFilesBySha,
    selectedDiff,
  ]);

  const currentProjectName = useMemo(
    () =>
      projects.find(item => item.projectId === projectId)?.name ?? 'Project',
    [projectId, projects],
  );
  const currentProject = useMemo(
    () => projects.find(item => item.projectId === projectId) ?? null,
    [projectId, projects],
  );
  const currentProjectTitle = useMemo(
    () => (currentProject?.name || '').trim(),
    [currentProject],
  );
  useEffect(() => {
    const baseTitle = 'WheelMaker';
    const projectTitle = currentProjectTitle;
    document.title = projectTitle ? `${baseTitle} - ${projectTitle}` : baseTitle;
  }, [currentProjectTitle]);
  const project = currentProject;
  const breadcrumbProjectName = useMemo(
    () => (currentProjectName || '').trim() || 'Project',
    [currentProjectName],
  );
  const fileBreadcrumbLabel = useMemo(
    () => splitPathForDisplay(selectedFile).fileName || 'No Selected File',
    [selectedFile],
  );
  const chatBreadcrumbLabel = useMemo(
    () => (selectedChatSession?.title || '').trim() || 'No Selected Session',
    [selectedChatSession],
  );
  const gitBreadcrumbLabel = useMemo(
    () => splitPathForDisplay(selectedDiff).fileName || 'No Selected Diff',
    [selectedDiff],
  );
  const renderBreadcrumbTitle = useCallback(
    (label: string) => (
      <div className="breadcrumb-title">
        <span className="breadcrumb-project-name">{breadcrumbProjectName}</span>
        <span className="breadcrumb-separator" aria-hidden="true">
          &gt;
        </span>
        <span className="title-text breadcrumb-current" title={label}>
          {label}
        </span>
      </div>
    ),
    [breadcrumbProjectName],
  );
  const floatingBounds = useMemo(() => {
    if (isWide) {
      return { minTop: 0, maxTop: 0 };
    }
    const minTop = Math.max(safeAreaTopInset + 12, 56);
    const maxTop = Math.max(
      minTop,
      windowHeight - floatingKeyboardOffset - floatingControlStackHeight - 18,
    );
    return { minTop, maxTop };
  }, [
    isWide,
    safeAreaTopInset,
    windowHeight,
    floatingKeyboardOffset,
    floatingControlStackHeight,
  ]);
  const floatingSlotTops = useMemo(
    () =>
      FLOATING_CONTROL_SLOT_ORDER.map(slot => ({
        slot,
        top: clampFloatingTop(
          Math.round(
            floatingBounds.minTop +
              (floatingBounds.maxTop - floatingBounds.minTop) * floatingControlSlotRatio(slot),
          ),
          floatingBounds.minTop,
          floatingBounds.maxTop,
        ),
      })),
    [floatingBounds.maxTop, floatingBounds.minTop],
  );
  const floatingRestTop = useMemo(
    () =>
      floatingSlotTops.find(entry => entry.slot === floatingControlSlot)?.top ??
      floatingSlotTops[0]?.top ??
      floatingBounds.minTop,
    [floatingBounds.minTop, floatingControlSlot, floatingSlotTops],
  );
  const floatingControlTop = useMemo(() => {
    if (floatingDragState?.active) {
      return clampFloatingTop(
        floatingDragState.currentTop,
        floatingBounds.minTop,
        floatingBounds.maxTop,
      );
    }
    const keyboardShift = Math.min(
      floatingKeyboardOffset,
      Math.max(0, floatingRestTop - floatingBounds.minTop),
    );
    return clampFloatingTop(
      floatingRestTop - keyboardShift,
      floatingBounds.minTop,
      floatingBounds.maxTop,
    );
  }, [
    floatingBounds.maxTop,
    floatingBounds.minTop,
    floatingDragState,
    floatingKeyboardOffset,
    floatingRestTop,
  ]);
  const floatingNavIndex = tab === 'chat' ? 0 : tab === 'file' ? 1 : 2;
  const floatingNavIndicatorStyle = useMemo(
    () =>
      ({
        '--floating-nav-index': String(floatingNavIndex),
      }) as React.CSSProperties,
    [floatingNavIndex],
  );
  const floatingControlStackStyle = useMemo(
    () =>
      !isWide
        ? ({
            top: `${floatingControlTop}px`,
          } as const)
        : undefined,
    [floatingControlTop, isWide],
  );
  const floatingDragVisualState =
    floatingDragState?.active ? 'dragging' : floatingDragState?.pressing ? 'drag-ready' : 'idle';
  const clearFloatingLongPressTimer = useCallback(() => {
    if (floatingLongPressTimerRef.current !== null) {
      window.clearTimeout(floatingLongPressTimerRef.current);
      floatingLongPressTimerRef.current = null;
    }
  }, []);
  const clearFloatingCooldownTimer = useCallback(() => {
    if (floatingCooldownTimerRef.current !== null) {
      window.clearTimeout(floatingCooldownTimerRef.current);
      floatingCooldownTimerRef.current = null;
    }
  }, []);
  const clearFloatingCooldownState = useCallback((cooldownUntil: number) => {
    clearFloatingCooldownTimer();
    const remaining = cooldownUntil - Date.now();
    if (remaining <= 0) {
      setFloatingDragState(prev =>
        prev && !prev.active && !prev.pressing && prev.cooldownUntil <= Date.now()
          ? null
          : prev,
      );
      return;
    }
    floatingCooldownTimerRef.current = window.setTimeout(() => {
      setFloatingDragState(prev =>
        prev && !prev.active && !prev.pressing && prev.cooldownUntil <= Date.now()
          ? null
          : prev,
      );
      floatingCooldownTimerRef.current = null;
    }, remaining);
  }, [clearFloatingCooldownTimer]);
  const beginFloatingPress = useCallback(
    (event: React.PointerEvent<HTMLElement>) => {
      if (isWide || event.button !== 0) {
        return;
      }
      if (floatingClickCooldownUntilRef.current > Date.now()) {
        return;
      }
      clearFloatingLongPressTimer();
      floatingIgnoreLostCaptureRef.current = false;
      event.currentTarget.setPointerCapture(event.pointerId);
      const originY = event.clientY;
      setFloatingDragState({
        active: false,
        pressing: true,
        pointerId: event.pointerId,
        originY,
        startTop: floatingControlTop,
        currentTop: floatingControlTop,
        cooldownUntil: 0,
      });
      floatingLongPressTimerRef.current = window.setTimeout(() => {
        setFloatingDragState(prev =>
          prev && prev.pointerId === event.pointerId
            ? { ...prev, active: true, pressing: false }
            : prev,
        );
        floatingLongPressTimerRef.current = null;
      }, 350);
    },
    [clearFloatingLongPressTimer, floatingControlTop, isWide],
  );
  const handleFloatingControlButtonPointerDown = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      beginFloatingPress(event);
      event.stopPropagation();
    },
    [beginFloatingPress],
  );
  const handleFloatingPointerMove = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      const current = floatingDragStateRef.current;
      if (!current || current.pointerId !== event.pointerId) {
        return;
      }
      const deltaY = event.clientY - current.originY;
      if (!current.active) {
        if (Math.abs(deltaY) >= 10) {
          clearFloatingLongPressTimer();
          const cooldownUntil = Date.now() + 120;
          floatingClickCooldownUntilRef.current = cooldownUntil;
          setFloatingDragState({
            ...current,
            active: false,
            pressing: false,
            cooldownUntil,
          });
          clearFloatingCooldownState(cooldownUntil);
        }
        return;
      }
      event.preventDefault();
      setFloatingDragState({
        ...current,
        currentTop: clampFloatingTop(
          current.startTop + deltaY,
          floatingBounds.minTop,
          floatingBounds.maxTop,
        ),
      });
    },
    [
      clearFloatingCooldownState,
      clearFloatingLongPressTimer,
      floatingBounds.maxTop,
      floatingBounds.minTop,
    ],
  );
  const finishFloatingDrag = useCallback(
    (pointerId: number) => {
      const current = floatingDragStateRef.current;
      if (!current || current.pointerId !== pointerId) {
        return;
      }
      clearFloatingLongPressTimer();
      if (!current.active) {
        setFloatingDragState(null);
        return;
      }
      const snappedTop = clampFloatingTop(
        current.currentTop,
        floatingBounds.minTop,
        floatingBounds.maxTop,
      );
      const nextSlot = nearestFloatingSlot(snappedTop, floatingSlotTops);
      const cooldownUntil = Date.now() + 120;
      floatingClickCooldownUntilRef.current = cooldownUntil;
      setFloatingControlSlot(nextSlot);
      workspaceStore.rememberGlobalState({ floatingControlSlot: nextSlot });
      setFloatingDragState({
        ...current,
        active: false,
        pressing: false,
        currentTop: snappedTop,
        cooldownUntil,
      });
      clearFloatingCooldownState(cooldownUntil);
    },
    [
      clearFloatingCooldownState,
      clearFloatingLongPressTimer,
      floatingBounds.maxTop,
      floatingBounds.minTop,
      floatingSlotTops,
    ],
  );
  const cancelFloatingDrag = useCallback(
    (pointerId: number) => {
      const current = floatingDragStateRef.current;
      if (!current || current.pointerId !== pointerId) {
        return;
      }
      clearFloatingLongPressTimer();
      if (!current.active) {
        setFloatingDragState(null);
        return;
      }
      const cooldownUntil = Date.now() + 120;
      floatingClickCooldownUntilRef.current = cooldownUntil;
      setFloatingDragState({
        ...current,
        active: false,
        pressing: false,
        cooldownUntil,
      });
      clearFloatingCooldownState(cooldownUntil);
    },
    [clearFloatingCooldownState, clearFloatingLongPressTimer],
  );
  const handleFloatingNavSelect = useCallback((nextTab: Tab) => {
    if (floatingClickCooldownUntilRef.current > Date.now()) {
      return;
    }
    if (nextTab === tab) {
      if (nextTab === 'chat' && drawerOpen) {
        setDrawerOpen(false);
      }
      return;
    }
    setTab(nextTab);
    setDrawerOpen(false);
  }, [drawerOpen, tab, setDrawerOpen, setTab]);
  const handleFloatingDrawerToggle = useCallback(() => {
    if (floatingClickCooldownUntilRef.current > Date.now()) {
      return;
    }
    setDrawerOpen(value => !value);
  }, []);
  const handleDesktopActivitySelect = useCallback((nextTab: Tab) => {
    if (sidebarSettingsOpen) {
      setSidebarSettingsOpen(false);
      if (nextTab !== tab) {
        setTab(nextTab);
      }
      setSidebarCollapsed(false);
      return;
    }
    if (nextTab === tab) {
      setSidebarCollapsed(value => !value);
      return;
    }
    setTab(nextTab);
    setSidebarCollapsed(false);
  }, [sidebarSettingsOpen, tab, setSidebarSettingsOpen, setSidebarCollapsed, setTab]);
  const handleDesktopSettingsSelect = useCallback(() => {
    const nextOpen = !sidebarSettingsOpen;
    setSidebarSettingsOpen(nextOpen);
    if (nextOpen) {
      setSidebarCollapsed(false);
    }
  }, [sidebarSettingsOpen, setSidebarSettingsOpen, setSidebarCollapsed]);
  const clampDesktopSidebarWidthForViewport = useCallback((width: number) => {
    const viewportMax = windowWidth > 0
      ? Math.floor(windowWidth * DESKTOP_SIDEBAR_VIEWPORT_MAX_RATIO)
      : DESKTOP_SIDEBAR_WIDTH_MAX;
    const maxWidth = Math.max(
      DESKTOP_SIDEBAR_WIDTH_MIN,
      Math.min(DESKTOP_SIDEBAR_WIDTH_MAX, viewportMax),
    );
    return Math.min(
      maxWidth,
      Math.max(DESKTOP_SIDEBAR_WIDTH_MIN, Math.round(width)),
    );
  }, [windowWidth]);
  const effectiveDesktopSidebarWidth = useMemo(
    () => clampDesktopSidebarWidthForViewport(
      desktopSidebarDraftWidth ?? desktopSidebarWidth,
    ),
    [clampDesktopSidebarWidthForViewport, desktopSidebarDraftWidth, desktopSidebarWidth],
  );
  const commitDesktopSidebarResize = useCallback(() => {
    const resizeState = desktopSidebarResizeRef.current;
    if (resizeState) {
      setDesktopSidebarWidth(resizeState.currentWidth);
    }
    desktopSidebarResizeRef.current = null;
    setDesktopSidebarDraftWidth(null);
    setDesktopSidebarResizing(false);
  }, [setDesktopSidebarWidth]);
  const beginDesktopSidebarResize = useCallback((event: React.PointerEvent<HTMLButtonElement>) => {
    if (!isWide) return;
    event.preventDefault();
    event.stopPropagation();
    desktopSidebarResizeRef.current = {
      pointerId: event.pointerId,
      originX: event.clientX,
      startWidth: effectiveDesktopSidebarWidth,
      currentWidth: effectiveDesktopSidebarWidth,
    };
    setDesktopSidebarDraftWidth(effectiveDesktopSidebarWidth);
    setDesktopSidebarResizing(true);
    event.currentTarget.setPointerCapture(event.pointerId);
  }, [effectiveDesktopSidebarWidth, isWide]);
  const moveDesktopSidebarResize = useCallback((event: React.PointerEvent<HTMLButtonElement>) => {
    const resizeState = desktopSidebarResizeRef.current;
    if (!resizeState || resizeState.pointerId !== event.pointerId) {
      return;
    }
    event.preventDefault();
    const nextWidth = resizeState.startWidth + event.clientX - resizeState.originX;
    const clampedWidth = clampDesktopSidebarWidthForViewport(nextWidth);
    desktopSidebarResizeRef.current = {
      ...resizeState,
      currentWidth: clampedWidth,
    };
    setDesktopSidebarDraftWidth(clampedWidth);
  }, [clampDesktopSidebarWidthForViewport]);
  const finishDesktopSidebarResize = useCallback((event: React.PointerEvent<HTMLButtonElement>) => {
    const resizeState = desktopSidebarResizeRef.current;
    if (!resizeState || resizeState.pointerId !== event.pointerId) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    commitDesktopSidebarResize();
    try {
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
    } catch {
      // Pointer capture may already be released by the browser.
    }
  }, [commitDesktopSidebarResize]);
  const resetDesktopSidebarWidth = useCallback((event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    desktopSidebarResizeRef.current = null;
    setDesktopSidebarDraftWidth(null);
    setDesktopSidebarResizing(false);
    setDesktopSidebarWidth(
      sanitizeDesktopSidebarWidth(
        clampDesktopSidebarWidthForViewport(DESKTOP_SIDEBAR_WIDTH_DEFAULT),
      ),
    );
  }, [clampDesktopSidebarWidthForViewport, setDesktopSidebarWidth]);
  const getWideProjectAgents = useCallback(
    (projectItem: RegistryProject, sessions: RegistryChatSession[]): string[] => {
      const seen = new Set<string>();
      const agents: string[] = [];
      const append = (value?: string) => {
        const normalized = (value || '').trim();
        if (!normalized) return;
        const key = normalized.toLowerCase();
        if (seen.has(key)) return;
        seen.add(key);
        agents.push(normalized);
      };
      for (const item of projectItem.agents ?? []) {
        append(item);
      }
      append(projectItem.agent);
      for (const session of sessions) {
        append(session.agentType);
      }
      return agents;
    },
    [],
  );
  const toggleWideProjectCollapsed = useCallback(
    (targetProjectId: string) => {
      setWideProjectActionMenu(current =>
        current?.projectId === targetProjectId ? null : current,
      );
      setMobileProjectActionMenu(current =>
        current?.projectId === targetProjectId ? null : current,
      );
      setCollapsedProjectIds(current =>
        current.includes(targetProjectId)
          ? current.filter(item => item !== targetProjectId)
          : [...current, targetProjectId],
      );
    },
    [setCollapsedProjectIds],
  );
  const clearProjectPinLongPress = useCallback(() => {
    if (projectPinLongPressTimerRef.current !== null) {
      window.clearTimeout(projectPinLongPressTimerRef.current);
      projectPinLongPressTimerRef.current = null;
    }
  }, []);
  const togglePinnedProject = useCallback(
    (targetProjectId: string) => {
      setPinnedProjectIds(current => togglePinnedProjectId(current, targetProjectId));
    },
    [setPinnedProjectIds],
  );
  const startProjectPinLongPress = useCallback(
    (targetProjectId: string, event: React.PointerEvent<HTMLButtonElement>) => {
      if (event.pointerType === 'mouse' && event.button !== 0) {
        return;
      }
      clearProjectPinLongPress();
      projectPinLongPressTargetRef.current = '';
      const target = event.currentTarget;
      if (target.setPointerCapture) {
        try {
          target.setPointerCapture(event.pointerId);
        } catch {
          // Pointer capture is best-effort; the timer still covers normal press flows.
        }
      }
      projectPinLongPressTimerRef.current = window.setTimeout(() => {
        projectPinLongPressTimerRef.current = null;
        projectPinLongPressTargetRef.current = targetProjectId;
        togglePinnedProject(targetProjectId);
      }, PROJECT_PIN_LONG_PRESS_MS);
    },
    [clearProjectPinLongPress, togglePinnedProject],
  );
  const finishProjectPinLongPress = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      clearProjectPinLongPress();
      const target = event.currentTarget;
      if (target.hasPointerCapture?.(event.pointerId)) {
        try {
          target.releasePointerCapture(event.pointerId);
        } catch {
          // ignore
        }
      }
    },
    [clearProjectPinLongPress],
  );
  const consumeProjectPinLongPressClick = useCallback(
    (targetProjectId: string, event: React.MouseEvent<HTMLButtonElement>): boolean => {
      if (projectPinLongPressTargetRef.current !== targetProjectId) {
        return false;
      }
      projectPinLongPressTargetRef.current = '';
      event.preventDefault();
      event.stopPropagation();
      return true;
    },
    [],
  );
  useEffect(() => clearProjectPinLongPress, [clearProjectPinLongPress]);
  const clearProjectSessionLongPress = useCallback(() => {
    if (projectSessionLongPressTimerRef.current !== null) {
      window.clearTimeout(projectSessionLongPressTimerRef.current);
      projectSessionLongPressTimerRef.current = null;
    }
  }, []);
  const projectSessionActionKey = (targetProjectId: string, sessionId: string) =>
    `${targetProjectId}:${sessionId}`;
  const startProjectSessionLongPress = useCallback(
    (
      targetProjectId: string,
      sessionId: string,
      event: React.PointerEvent<HTMLButtonElement>,
    ) => {
      if (event.pointerType === 'mouse' && event.button !== 0) {
        return;
      }
      clearProjectSessionLongPress();
      projectSessionLongPressTargetRef.current = '';
      const target = event.currentTarget;
      if (target.setPointerCapture) {
        try {
          target.setPointerCapture(event.pointerId);
        } catch {
          // ignore
        }
      }
      projectSessionLongPressTimerRef.current = window.setTimeout(() => {
        projectSessionLongPressTimerRef.current = null;
        projectSessionLongPressTargetRef.current = projectSessionActionKey(
          targetProjectId,
          sessionId,
        );
        setProjectSessionActionMenu({projectId: targetProjectId, sessionId});
      }, PROJECT_SESSION_LONG_PRESS_MS);
    },
    [clearProjectSessionLongPress],
  );
  const finishProjectSessionLongPress = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      clearProjectSessionLongPress();
      const target = event.currentTarget;
      if (target.hasPointerCapture?.(event.pointerId)) {
        try {
          target.releasePointerCapture(event.pointerId);
        } catch {
          // ignore
        }
      }
    },
    [clearProjectSessionLongPress],
  );
  const consumeProjectSessionLongPressClick = useCallback(
    (
      targetProjectId: string,
      sessionId: string,
      event: React.MouseEvent<HTMLButtonElement>,
    ): boolean => {
      if (
        projectSessionLongPressTargetRef.current !==
        projectSessionActionKey(targetProjectId, sessionId)
      ) {
        return false;
      }
      projectSessionLongPressTargetRef.current = '';
      event.preventDefault();
      event.stopPropagation();
      return true;
    },
    [],
  );
  useEffect(() => clearProjectSessionLongPress, [clearProjectSessionLongPress]);
  useEffect(() => {
    if (!projectSessionActionMenu) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Element | null;
      if (target?.closest('.project-session-action-btn')) {
        return;
      }
      setProjectSessionActionMenu(null);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [projectSessionActionMenu]);
  currentProjectRef.current = currentProject;
  projectsRef.current = projects;
  expandedDirsRef.current = expandedDirs;
  selectedFileRef.current = selectedFile;

  useEffect(() => {
    knownProjectRevRef.current = currentProject?.projectRev ?? '';
    knownGitRevRef.current = currentProject?.git?.gitRev ?? '';
    if (currentProject?.git?.worktreeRev) {
      knownWorktreeRevRef.current = currentProject.git.worktreeRev;
    }
  }, [currentProject]);
  const worktreeActive = selectedDiffSource === 'worktree';

  const isExpanded = (path: string) => expandedDirs.includes(path);
  const selectedFileIsMarkdown = isMarkdownPath(selectedFile);
  const isSelectedFilePinned = selectedFile
    ? pinnedFiles.includes(selectedFile)
    : false;
  const hasPinnedFiles = pinnedFiles.length > 0;
  const fileLines = useMemo(() => fileContent.split('\n'), [fileContent]);
  const fileSearchMatches = useMemo(() => {
    const query = fileSearchQuery.trim().toLocaleLowerCase();
    if (!query) return [] as number[];
    const matches: number[] = [];
    for (let i = 0; i < fileLines.length; i += 1) {
      if (fileLines[i].toLocaleLowerCase().includes(query)) {
        matches.push(i + 1);
      }
    }
    return matches;
  }, [fileContent, fileLines, fileSearchQuery]);

  const applyHydratedProjectState = (
    hydrated: {
      projectId: string;
      dirEntries: Record<string, RegistryFsEntry[]>;
      expandedDirs: string[];
      selectedFile: string;
      pinnedFiles: string[];
      gitCurrentBranch: string;
      commits: RegistryGitCommit[];
      selectedCommit: string;
      commitFilesBySha: Record<string, RegistryGitCommitFile[]>;
      selectedDiff: string;
      cachedDiffText: string;
    },
    options?: {preserveFileView?: boolean},
  ) => {
    const preserveFileView =
      options?.preserveFileView === true &&
      hydrated.projectId === projectIdRef.current &&
      hydrated.selectedFile === selectedFileRef.current &&
      !!hydrated.selectedFile;

    if (!preserveFileView) {
      fileReadSeqRef.current += 1;
      fileHashRef.current = {};
      fileCacheRef.current = {};
    }
    expandedDirsRef.current = hydrated.expandedDirs;
    selectedFileRef.current = hydrated.selectedFile;
    setProjectId(hydrated.projectId);
    setDirEntries(hydrated.dirEntries);
    setExpandedDirs(hydrated.expandedDirs);
    setSelectedFile(hydrated.selectedFile);
    setPinnedFiles([]);
    setPinnedFiles(hydrated.pinnedFiles);
    if (!preserveFileView) {
      setFileContent('');
      setFileInfo(null);
    }
    setGitCurrentBranch(hydrated.gitCurrentBranch);
    setGitBranches([]);
    setGitSelectedBranches([]);
    gitSelectedBranchesRef.current = [];
    setGitBranchPickerOpen(false);
    setCommits(hydrated.commits);
    setSelectedCommit(hydrated.selectedCommit);
    setExpandedCommitShas(hydrated.selectedCommit ? [hydrated.selectedCommit] : []);
    setCommitFilesBySha(hydrated.commitFilesBySha);
    setWorktreeExpanded(true);
    setCommitPopover(null);
    setSelectedDiff(hydrated.selectedDiff);
    setDiffText(hydrated.cachedDiffText);
    setWorkingTreeFiles([]);
    setGitLoadedProjectId('');
    setProjectMenuOpen(false);
    setSidebarSettingsOpen(false);
    if (!isWide) setDrawerOpen(false);
  };

  const togglePinSelectedFile = () => {
    if (!selectedFile) return;
    setPinnedFiles(prev =>
      prev.includes(selectedFile)
        ? prev.filter(path => path !== selectedFile)
        : [...prev, selectedFile],
    );
  };

  useEffect(() => {
    if (fileSearchMatches.length === 0) {
      setCurrentMatchIndex(0);
      return;
    }
    setCurrentMatchIndex(prev => Math.min(prev, fileSearchMatches.length - 1));
  }, [fileSearchMatches.length]);

  useEffect(() => {
    if (!searchToolsOpen) return;
    const query = fileSearchQuery.trim();
    if (!query || fileSearchMatches.length === 0) return;
    setCurrentMatchIndex(0);
    window.requestAnimationFrame(() => {
      scrollToFileLine(fileSearchMatches[0]);
    });
  }, [fileSearchMatches, fileSearchQuery, searchToolsOpen]);

  useEffect(
    () => () => {
      if (liveRefreshTimerRef.current !== null) {
        window.clearTimeout(liveRefreshTimerRef.current);
      }
      if (reconnectTimerRef.current !== null) {
        window.clearTimeout(reconnectTimerRef.current);
      }
    },
    [],
  );

  const captureSelectedFileScrollPosition = () => {
    const path = selectedFileRef.current;
    const container = fileScrollRef.current;
    if (!path || !container) return;
    fileScrollTopByPathRef.current[path] = container.scrollTop;
  };

  const scheduleRestoreSelectedFileScroll = (path: string) => {
    const savedTop = fileScrollTopByPathRef.current[path];
    if (!Number.isFinite(savedTop)) return;

    const restoreOnNextFrame = (attempt: number) => {
      const container = fileScrollRef.current;
      if (!container) return;
      if (selectedFileRef.current !== path) return;

      const maxScrollTop = Math.max(
        0,
        container.scrollHeight - container.clientHeight,
      );
      if (maxScrollTop <= 0 && attempt < 8) {
        window.requestAnimationFrame(() => restoreOnNextFrame(attempt + 1));
        return;
      }
      container.scrollTop = Math.min(savedTop, maxScrollTop);
    };

    window.requestAnimationFrame(() => restoreOnNextFrame(0));
  };

  const scrollToFileLine = (line: number) => {
    const container = fileScrollRef.current;
    if (!container) return;
    const lineElement = container.querySelector(
      `.code-wrap [data-line-number="${line}"]`,
    ) as HTMLElement | null;
    if (lineElement) {
      const containerRect = container.getBoundingClientRect();
      const lineRect = lineElement.getBoundingClientRect();
      const delta =
        lineRect.top -
        containerRect.top -
        container.clientHeight / 2 +
        lineRect.height / 2;
      container.scrollTo({
        top: container.scrollTop + delta,
        behavior: 'smooth',
      });
    } else {
      const codeElement = container.querySelector(
        '.code-wrap pre code',
      ) as HTMLElement | null;
      const lineHeight = codeElement
        ? Number.parseFloat(window.getComputedStyle(codeElement).lineHeight) ||
          20
        : 20;
      container.scrollTo({
        top: Math.max(0, (line - 1) * lineHeight),
        behavior: 'smooth',
      });
    }
  };

  useEffect(() => {
    if (!pendingFileJump) return;
    if (tab !== 'file') return;
    if (selectedFileRef.current !== pendingFileJump.path) return;
    if (fileLoading) return;

    const targetPath = pendingFileJump.path;
    const targetLine = pendingFileJump.line;
    const maxAttempts = 16;
    const runScroll = (attempt: number) => {
      if (selectedFileRef.current !== targetPath) return;
      const container = fileScrollRef.current;
      if (!container) {
        if (attempt < maxAttempts) {
          window.requestAnimationFrame(() => runScroll(attempt + 1));
        }
        return;
      }

      const exactLineElement = container.querySelector(
        `.code-wrap [data-line-number="${targetLine}"]`,
      );
      if (!exactLineElement && attempt < maxAttempts) {
        window.requestAnimationFrame(() => runScroll(attempt + 1));
        return;
      }

      scrollToFileLine(targetLine);
      setPendingFileJump(current =>
        current && current.path === targetPath && current.line === targetLine
          ? null
          : current,
      );
    };

    window.requestAnimationFrame(() => runScroll(0));
  }, [pendingFileJump, tab, fileLoading, selectedFile, fileContent]);

  const navigateSearchMatch = (delta: 1 | -1) => {
    if (fileSearchMatches.length === 0) return;
    const next =
      (currentMatchIndex + delta + fileSearchMatches.length) %
      fileSearchMatches.length;
    setCurrentMatchIndex(next);
    scrollToFileLine(fileSearchMatches[next]);
  };

  const triggerGoToLine = () => {
    if (!selectedFile || fileLoading || !fileLines.length) return;
    const raw = gotoLineInput.trim();
    if (!raw) return;
    if (!/^\d+$/.test(raw)) {
      return;
    }
    const parsed = Number.parseInt(raw, 10);
    if (!Number.isFinite(parsed) || parsed < 1) {
      return;
    }
    const line = Math.max(1, Math.min(fileLines.length, parsed));
    setGotoLineInput(String(line));
    window.requestAnimationFrame(() => {
      scrollToFileLine(line);
    });
  };

  const loadDirectory = async (path: string) => {
    if (loadingDirs[path]) return;
    setLoadingDirs(prev => ({ ...prev, [path]: true }));
    try {
      const persistedCache = projectId
        ? workspaceStore.getCachedDirectory(projectId, path)
        : null;
      const knownHash =
        dirHashRef.current[path] || persistedCache?.hash || '';
      const result = await service.listDirectory(
        path,
        knownHash || undefined,
      );

      if (result.notModified) {
        const cachedEntries = persistedCache?.entries;
        if (Array.isArray(cachedEntries)) {
          setDirEntries(prev => ({ ...prev, [path]: sortEntries(cachedEntries) }));
        }
        if (result.hash) {
          dirHashRef.current[path] = result.hash;
          if (projectId && Array.isArray(cachedEntries)) {
            workspaceStore.cacheDirectory(projectId, path, result.hash, cachedEntries);
          }
        }
        return;
      }

      const entries = sortEntries(result.entries);
      setDirEntries(prev => ({ ...prev, [path]: entries }));
      const nextHash = result.hash || persistedCache?.hash || '';
      if (nextHash) {
        dirHashRef.current[path] = nextHash;
      }
      if (projectId) {
        workspaceStore.cacheDirectory(projectId, path, nextHash, entries);
      }
    } finally {
      setLoadingDirs(prev => {
        const next = { ...prev };
        delete next[path];
        return next;
      });
    }
  };

  const toggleDirectory = async (path: string) => {
    if (isExpanded(path)) {
      setExpandedDirs(prev => prev.filter(item => item !== path));
      return;
    }
    setExpandedDirs(prev => [...prev, path]);
    if (!dirEntries[path]) {
      try {
        await loadDirectory(path);
      } catch (err) {
        setExpandedDirs(prev => prev.filter(item => item !== path));
        const reason = err instanceof Error ? err.message : String(err);
        setError(`Failed to load directory "${path}": ${reason}`);
      }
    }
  };

  const readSelectedFile = async (path: string, options?: {restoreScroll?: boolean; silent?: boolean}) => {
    if (!path) return;
    const requestSeq = fileReadSeqRef.current + 1;
    fileReadSeqRef.current = requestSeq;
    const silentRead = options?.silent === true;
    if (!silentRead) {
      setFileLoading(true);
    }
    const shouldRestoreScroll = options?.restoreScroll === true;
    try {
      const info = await service.getFileInfo(path);
      if (requestSeq !== fileReadSeqRef.current) return;
      setFileInfo(info);
      const persistedFile = projectId
        ? workspaceStore.getCachedFile(projectId, path)
        : null;
      if (persistedFile?.content && !fileCacheRef.current[path]) {
        fileCacheRef.current[path] = persistedFile.content;
      }
      if (persistedFile?.hash && !fileHashRef.current[path]) {
        fileHashRef.current[path] = persistedFile.hash;
      }
      const knownHash = fileHashRef.current[path] || persistedFile?.hash || '';
      const isFirstLoad = !knownHash;
      if ((info.size ?? 0) > LARGE_FILE_CONFIRM_BYTES && isFirstLoad) {
        const sizeMB = ((info.size ?? 0) / (1024 * 1024)).toFixed(1);
        const confirmed = window.confirm(
          `This file is ${sizeMB} MB. Load full content now?`,
        );
        if (!confirmed) {
          setFileContent('');
          return;
        }
      }
      const result = await service.readFile(path, {
        knownHash: knownHash || undefined,
      });
      if (requestSeq !== fileReadSeqRef.current) return;
      if (result.notModified) {
        const cachedContent =
          fileCacheRef.current[path] ?? persistedFile?.content ?? '';
        setFileContent(typeof cachedContent === 'string' ? cachedContent : '');
        const nextHash = result.hash || knownHash;
        if (nextHash) {
          fileHashRef.current[path] = nextHash;
          if (projectId && typeof cachedContent === 'string') {
            workspaceStore.cacheFile(projectId, path, nextHash, cachedContent);
          }
        }
        if (shouldRestoreScroll) {
          scheduleRestoreSelectedFileScroll(path);
        }
        return;
      }
      setFileContent(result.content);
      fileCacheRef.current[path] = result.content;
      const nextHash = result.hash || knownHash;
      if (nextHash) {
        fileHashRef.current[path] = nextHash;
      }
      if (projectId) {
        workspaceStore.cacheFile(projectId, path, nextHash, result.content);
      }
      if (shouldRestoreScroll) {
        scheduleRestoreSelectedFileScroll(path);
      }
    } catch (err) {
      if (requestSeq !== fileReadSeqRef.current) return;
      if (!silentRead) {
        setFileInfo(null);
        setFileContent('');
      }
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (requestSeq === fileReadSeqRef.current && !silentRead) {
        setFileLoading(false);
      }
    }
  };


  useEffect(() => {
    if (!selectedFile) {
      fileReadSeqRef.current += 1;
      setFileLoading(false);
      setFileInfo(null);
      setFileContent('');
      return;
    }
    if (skipNextSelectedFileAutoReadRef.current) {
      skipNextSelectedFileAutoReadRef.current = false;
      return;
    }
    readSelectedFile(selectedFile).catch(() => undefined);
  }, [projectId, selectedFile]);

  const loadGit = async (preferredRefs?: string[]) => {
    const targetProjectId = projectId;
    if (!targetProjectId) return;
    setGitLoading(true);
    setGitError('');
    try {
      const [branchData, statusData] = await Promise.all([
        service.listGitBranches(),
        service.getGitStatus(),
      ]);
      const currentBranch = branchData.current || '';
      const availableBranches = normalizeGitBranches(
        branchData.branches ?? [],
        currentBranch,
      );
      const selectedBranches = pickGitSelectedBranches(
        preferredRefs ?? gitSelectedBranchesRef.current,
        availableBranches,
        currentBranch,
      );
      const commitData = await service.listGitCommits('HEAD', selectedBranches);

      setGitCurrentBranch(currentBranch);
      setGitBranches(availableBranches);
      setGitSelectedBranches(selectedBranches);
      gitSelectedBranchesRef.current = selectedBranches;
      const working = buildWorkingTreeFiles(statusData);
      setWorkingTreeFiles(working);
      knownWorktreeRevRef.current = statusData.worktreeRev ?? '';
      setCommits(commitData);
      setGitLoadedProjectId(targetProjectId);
      const firstCommit = commitData[0]?.sha ?? '';
      setSelectedCommit(prev => {
        if (prev && commitData.some(item => item.sha === prev)) {
          return prev;
        }
        return firstCommit;
      });
      setExpandedCommitShas(prev => {
        const expanded = prev.find(sha => commitData.some(item => item.sha === sha));
        if (expanded) return [expanded];
        return firstCommit ? [firstCommit] : [];
      });
      setWorktreeExpanded(working.length > 0);
      setCommitPopover(null);
      if (!selectedDiff) {
        if (working[0]) {
          const preferredPath = pickPreferredPath(working);
          const preferredFile =
            working.find(item => item.path === preferredPath) ?? working[0];
          setSelectedDiff(preferredFile.path);
          setSelectedDiffSource('worktree');
          setSelectedDiffScope(preferredFile.scope);
        } else if (firstCommit) {
          setSelectedDiffSource('commit');
        }
      }
    } catch (err) {
      setGitError(err instanceof Error ? err.message : String(err));
    } finally {
      setGitLoading(false);
    }
  };

  const toggleGitBranchSelection = (branch: string) => {
    const normalizedBranch = branch.trim();
    if (!normalizedBranch) return;
    const currentSelection = gitSelectedBranchesRef.current;
    const nextSelection = currentSelection.includes(normalizedBranch)
      ? currentSelection.filter(item => item !== normalizedBranch)
      : [...currentSelection, normalizedBranch];
    const fallbackBranch = gitCurrentBranch.trim() || normalizedBranch;
    const effectiveSelection =
      nextSelection.length > 0 ? nextSelection : [fallbackBranch];
    setGitSelectedBranches(effectiveSelection);
    gitSelectedBranchesRef.current = effectiveSelection;
    setGitLoadedProjectId('');
    loadGit(effectiveSelection).catch(err =>
      setGitError(err instanceof Error ? err.message : String(err)),
    );
  };

  const refreshGitStatusOnly = async () => {
    try {
      const statusData = await service.getGitStatus();
      setWorkingTreeFiles(buildWorkingTreeFiles(statusData));
      knownWorktreeRevRef.current = statusData.worktreeRev ?? '';
    } catch {
      // Keep existing UI state on transient status fetch failure.
    }
  };

  useEffect(() => {
    if (!connected || tab !== 'git') return;
    if (!projectId) return;
    if (gitLoading) return;
    if (gitLoadedProjectId === projectId && !gitError) return;
    loadGit().catch(() => undefined);
  }, [connected, tab, projectId, gitError, gitLoading, gitLoadedProjectId]);

  useEffect(() => {
    const run = async () => {
      if (!selectedCommit) return;
      if (commitFilesBySha[selectedCommit]) return;
      const files = await service.listGitCommitFiles(selectedCommit);
      setCommitFilesBySha(prev => ({ ...prev, [selectedCommit]: files }));
      if (!selectedDiff && files[0]) {
        setSelectedDiff(pickPreferredPath(files));
        setSelectedDiffSource('commit');
      }
    };
    run().catch(err =>
      setGitError(err instanceof Error ? err.message : String(err)),
    );
  }, [selectedCommit, commitFilesBySha, selectedDiff]);

  useEffect(() => {
    const run = async () => {
      if (!projectId || !selectedDiff) return;
      if (isHeavyGeneratedDiffPath(selectedDiff) && !allowHeavyDiffLoad) {
        setDiffText('');
        return;
      }
      const cacheScope =
        selectedDiffSource === 'worktree'
          ? `WORKTREE:${selectedDiffScope}`
          : selectedCommit;
      if (!cacheScope) return;
      const cachedDiff = workspaceStore.getCachedDiff(
        projectId,
        cacheScope,
        selectedDiff,
      );
      if (cachedDiff !== null) {
        setDiffText(cachedDiff);
        return;
      }
      setDiffLoading(true);
      try {
        const diff =
          selectedDiffSource === 'worktree'
            ? await service.readWorkingTreeFileDiff(
                selectedDiff,
                selectedDiffScope,
              )
            : await service.readGitFileDiff(selectedCommit, selectedDiff);
        setDiffText(diff.diff || '');
        workspaceStore.cacheDiff(
          projectId,
          cacheScope,
          selectedDiff,
          diff.diff || '',
          !!diff.isBinary,
          !!diff.truncated,
        );
      } catch (err) {
        setGitError(err instanceof Error ? err.message : String(err));
      } finally {
        setDiffLoading(false);
      }
    };
    run().catch(() => undefined);
  }, [
    projectId,
    selectedCommit,
    selectedDiff,
    selectedDiffSource,
    selectedDiffScope,
    allowHeavyDiffLoad,
  ]);

  const clearReconnectTimer = () => {
    if (reconnectTimerRef.current !== null) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  };

  const scheduleReconnectAttempt = () => {
    clearReconnectTimer();
    reconnectTimerRef.current = window.setTimeout(() => {
      reconnectTimerRef.current = null;
      setAutoConnecting(true);
      connect({ silentReconnect: true }).catch(() => undefined);
    }, RECONNECT_RETRY_DELAY_MS);
  };

  const clearChatRuntimeState = (preferredSelection = '') => {
    setChatMessages([]);
    setChatSessions([]);
    setChatRunningSessionFlags({});
    setChatCompletedUnopenedFlags({});
    applySelectedChatKey(
      preferredSelection
        ? chatSessionKeyFromParts(projectIdRef.current, preferredSelection)
        : null,
    );
    chatSyncIndexRef.current = {};
    chatSyncSubIndexRef.current = {};
    chatMessageStoreRef.current = {};
  };

  const hydrateChatSessionContentFromCache = (
    sessionId: string,
    activeProjectId = projectIdRef.current,
  ): RegistryChatMessage[] => {
    if (!activeProjectId || !sessionId) return [];
    const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
    const cached = workspaceStore.getCachedChatSessionContent(activeProjectId, sessionId);
    if (!cached) {
      const inMemoryMessages = chatMessageStoreRef.current[runtimeKey] ?? [];
      if (inMemoryMessages.length === 0) {
        chatSyncIndexRef.current[runtimeKey] = 0;
        chatSyncSubIndexRef.current[runtimeKey] = 0;
      } else {
        const cursor = getLatestSessionReadCursor(inMemoryMessages);
        chatSyncIndexRef.current[runtimeKey] = 0;
        chatSyncSubIndexRef.current[runtimeKey] = cursor.turnIndex;
      }
      return inMemoryMessages;
    }

    const cachedMessages = [...cached.messages];
    chatMessageStoreRef.current[runtimeKey] = cachedMessages;

    const cursor = reconcileCachedSessionReadCursor(
      {turnIndex: chatSyncSubIndexRef.current[runtimeKey] ?? 0},
      cachedMessages,
    );
    chatSyncIndexRef.current[runtimeKey] = 0;
    chatSyncSubIndexRef.current[runtimeKey] = cursor.turnIndex;
    return cachedMessages;
  };

  const hydrateChatSessionsFromCache = (
    activeProjectId = projectIdRef.current,
    preferredSelection = '',
  ) => {
    if (!activeProjectId) return;
    const cachedSessions = workspaceStore.hydrateChatSessions(activeProjectId);
    if (cachedSessions.length === 0) {
      return;
    }

    const sessionRows = cachedSessions.map(item => item.session);
    setChatSessions(sortChatSessions(sessionRows));
    setProjectSessionsByProjectId(prev => ({
      ...prev,
      [activeProjectId]: sortChatSessions(sessionRows),
    }));

    for (const cached of cachedSessions) {
      const sessionId = cached.session.sessionId;
      if (!sessionId) continue;
      const content = workspaceStore.getCachedChatSessionContent(activeProjectId, sessionId);
      const cursor = reconcileCachedSessionReadCursor(cached.cursor, content?.messages ?? []);
      const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
      chatSyncIndexRef.current[runtimeKey] = 0;
      chatSyncSubIndexRef.current[runtimeKey] = cursor.turnIndex;
    }

    const persistedSelection = workspaceStore.migrateSelectedChatSessionKey(activeProjectId);
    const currentSelection =
      preferredSelection ||
      (selectedChatKeyRef.current?.projectId === activeProjectId
        ? selectedChatKeyRef.current.sessionId
        : '') ||
      (persistedSelection?.projectId === activeProjectId
        ? persistedSelection.sessionId
        : '') ||
      workspaceStore.getSelectedChatSessionId(activeProjectId);
    if (!currentSelection) {
      setChatMessages([]);
      return;
    }
    applySelectedChatKey(chatSessionKeyFromParts(activeProjectId, currentSelection));
    const runtimeKey = buildChatRuntimeKey(activeProjectId, currentSelection);
    if (sessionRows.some(item => item.sessionId === currentSelection)) {
      const cachedMessages = hydrateChatSessionContentFromCache(currentSelection, activeProjectId);
      setVisibleChatMessagesForRuntimeKey(runtimeKey, cachedMessages, {resetToLatest: true});
      return;
    }
    const retainedMessages = hydrateChatSessionContentFromCache(currentSelection, activeProjectId);
    if (retainedMessages.length > 0) {
      setVisibleChatMessagesForRuntimeKey(runtimeKey, retainedMessages, {resetToLatest: true});
    }
  };

  const persistChatSessionsIndex = (activeProjectId = projectIdRef.current) => {
    if (!activeProjectId) return;
    const cursorBySessionId: Record<string, {turnIndex: number}> = {};
    for (const session of chatSessions) {
      const sessionId = session.sessionId;
      if (!sessionId) continue;
      const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
      cursorBySessionId[sessionId] = {
        turnIndex: chatSyncSubIndexRef.current[runtimeKey] ?? 0,
      };
    }
    workspaceStore.replaceChatSessions(activeProjectId, chatSessions, cursorBySessionId);
  };

  const persistChatSessionContent = (
    sessionId: string,
    activeProjectId = projectIdRef.current,
    session?: RegistryChatSession,
  ) => {
    if (!activeProjectId || !sessionId) return;
    const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
    const messages = chatMessageStoreRef.current[runtimeKey] ?? [];
    const cursor = {
      turnIndex: chatSyncSubIndexRef.current[runtimeKey] ?? 0,
    };
    const cacheableMessages = messages.filter(isFinishedChatMessage);
    workspaceStore.rememberChatSessionContent(activeProjectId, sessionId, cacheableMessages);
    const targetSession =
      session ??
      projectSessionsByProjectId[activeProjectId]?.find(item => item.sessionId === sessionId) ??
      chatSessions.find(item => item.sessionId === sessionId);
    if (targetSession) {
      workspaceStore.rememberChatSession(activeProjectId, targetSession, cursor);
    }
  };

  const rememberChatSessionSummary = (
    activeProjectId: string,
    session: Partial<RegistryChatSession> & {sessionId: string},
  ) => {
    if (!activeProjectId || !session.sessionId) return;
    setProjectSessionsByProjectId(prev => mergeProjectSessionMap(prev, activeProjectId, session));
    if (activeProjectId === projectIdRef.current) {
      setChatSessions(prev => mergeChatSession(prev, session));
    }
  };

  const markChatSessionRunning = (
    activeProjectId: string,
    sessionId: string,
    preview = '',
  ) => {
    if (!activeProjectId || !sessionId) return;
    const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
    setChatRunningSessionFlags(prev => addSessionFlag(prev, runtimeKey));
    setChatCompletedUnopenedFlags(prev => removeSessionFlag(prev, runtimeKey));
    rememberChatSessionSummary(activeProjectId, {
      sessionId,
      running: true,
      ...(preview
        ? {
            preview,
            updatedAt: new Date().toISOString(),
          }
        : {}),
    });
  };

  const clearChatSessionRunning = (
    activeProjectId: string,
    sessionId: string,
  ) => {
    if (!activeProjectId || !sessionId) return;
    const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
    setChatRunningSessionFlags(prev => removeSessionFlag(prev, runtimeKey));
    rememberChatSessionSummary(activeProjectId, {
      sessionId,
      running: false,
    });
  };

  const markChatSessionRead = async (
    activeProjectId: string,
    sessionId: string,
    lastReadTurnIndex: number,
  ) => {
    const cursor = Math.max(0, Math.trunc(lastReadTurnIndex));
    if (!activeProjectId || !sessionId || cursor <= 0) return;
    try {
      const result = await service.markProjectSessionRead(activeProjectId, sessionId, cursor);
      if (!result.ok) return;
      const sessionPatch = result.session ?? {sessionId, lastReadTurnIndex: cursor};
      rememberChatSessionSummary(activeProjectId, sessionPatch);
      const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
      setChatCompletedUnopenedFlags(prev => removeSessionFlag(prev, runtimeKey));
      if (result.session) {
        workspaceStore.rememberChatSession(activeProjectId, result.session, {
          turnIndex: chatSyncSubIndexRef.current[runtimeKey] ?? 0,
        });
      }
    } catch {
      // The next session.list/session.updated response will reconcile read state.
    }
  };

  const resolveSessionVisualState = (session: RegistryChatSession, activeProjectId = projectIdRef.current): ChatSessionVisualState => {
    const runtimeKey = buildChatRuntimeKey(activeProjectId, session.sessionId);
    return resolveChatSessionVisualStateValue(session, {
      running: !!chatRunningSessionFlags[runtimeKey],
      completedUnviewed: !!chatCompletedUnopenedFlags[runtimeKey],
    });
  };

  const renderSessionStateMarker = (session: RegistryChatSession, activeProjectId = projectIdRef.current) => {
    const state = resolveSessionVisualState(session, activeProjectId);
    const title =
      state === 'running'
        ? 'In progress'
        : state === 'failed-unviewed'
          ? 'Failed, click to view'
          : state === 'completed-unviewed'
            ? 'Completed, click to view'
            : undefined;
    return (
      <span className={`session-state-marker ${state}`} title={title}>
        {state === 'running' ? (
          <span className="codicon codicon-loading codicon-modifier-spin" />
        ) : state === 'completed-unviewed' || state === 'failed-unviewed' ? (
          <span className="session-state-dot" />
        ) : null}
      </span>
    );
  };

  const clearProjectSessionCache = (
    targetProjectId: string,
    sessionId: string,
  ) => {
    const runtimeKey = buildChatRuntimeKey(targetProjectId, sessionId);
    chatSyncIndexRef.current[runtimeKey] = 0;
    chatSyncSubIndexRef.current[runtimeKey] = 0;
    chatMessageStoreRef.current[runtimeKey] = [];
    workspaceStore.rememberChatSessionContent(targetProjectId, sessionId, []);
    const targetSession =
      projectSessionsByProjectId[targetProjectId]?.find(item => item.sessionId === sessionId) ??
      (targetProjectId === projectIdRef.current
        ? chatSessions.find(item => item.sessionId === sessionId)
        : undefined);
    if (targetSession) {
      workspaceStore.rememberChatSession(targetProjectId, targetSession, {turnIndex: 0});
    }
  };

  const loadChatSession = async (
    sessionId: string,
    activeProjectId = projectIdRef.current,
    options?: {
      incremental?: boolean;
      preserveUserSelection?: boolean;
      selectionSnapshot?: string;
      forceFull?: boolean;
    },
  ) => {
    if (!activeProjectId || !sessionId) return false;
    const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
    setChatLoading(true);
    try {
      const requestedIncremental = options?.forceFull
        ? false
        : (options?.incremental ?? true);
      // Snapshot existing messages BEFORE the await so the base is
      // consistent with the cursor. Live session.message events may
      // mutate chatMessageStoreRef during the network round-trip.
      const existingMessages = chatMessageStoreRef.current[runtimeKey] ?? [];
      const checkpointTurnIndex = requestedIncremental
        ? chatSyncSubIndexRef.current[runtimeKey] ?? 0
        : 0;
      const fallbackToFullRead =
        requestedIncremental &&
        existingMessages.length === 0 &&
        checkpointTurnIndex > 0;
      const useIncremental = requestedIncremental && !fallbackToFullRead;
      const result = await service.readProjectSession(
        activeProjectId,
        sessionId,
        useIncremental ? checkpointTurnIndex : 0,
      );
      const selectionSnapshot = options?.selectionSnapshot ?? '';
      const currentSelectedRuntimeKey = encodeChatSessionKey(selectedChatKeyRef.current);
      if (
        options?.preserveUserSelection &&
        selectionSnapshot &&
        currentSelectedRuntimeKey !== selectionSnapshot &&
        chatSelectedIdRef.current !== selectionSnapshot &&
        chatSelectedIdRef.current !== sessionId
      ) {
        return;
      }
      const resultSessionId = result.session?.sessionId || sessionId;

      let nextMessages: RegistryChatMessage[];
      if (useIncremental) {
        if (result.messages.length > 0) {
          nextMessages = replaceSessionMessages(
            existingMessages,
            result.messages,
            checkpointTurnIndex,
          );
        } else {
          nextMessages = existingMessages;
        }
      } else {
        nextMessages = result.messages;
      }

      // Reconcile: live session.message events may have landed in the store
      // during the await. Fold only post-request changes back in so old cache
      // entries cannot overwrite newer session.read results.
      const resultRuntimeKey = buildChatRuntimeKey(activeProjectId, resultSessionId);
      const fresh = chatMessageStoreRef.current[resultRuntimeKey] ?? [];
      nextMessages = reconcileSessionReadMessages(nextMessages, fresh, existingMessages);
      forgetPendingPromptIfResolved(resultRuntimeKey, nextMessages);

      chatMessageStoreRef.current[resultRuntimeKey] = nextMessages;
      const latestSyncCursor = getLatestSessionReadCursor(nextMessages);
      chatSyncIndexRef.current[resultRuntimeKey] = 0;
      chatSyncSubIndexRef.current[resultRuntimeKey] = latestSyncCursor.turnIndex;
      const resultSession = result.session;
      if (resultSession) {
        setProjectSessionsByProjectId(prev => mergeProjectSessionMap(prev, activeProjectId, resultSession));
        if (activeProjectId === projectIdRef.current) {
          setChatSessions(prev => mergeChatSession(prev, resultSession));
        }
      }
      const nextSelectedKey = chatSessionKeyFromParts(activeProjectId, resultSessionId);
      applySelectedChatKey(nextSelectedKey);
      workspaceStore.rememberSelectedChatSessionKey(nextSelectedKey);
      setVisibleChatMessagesForRuntimeKey(resultRuntimeKey, nextMessages, {resetToLatest: true});
      persistChatSessionContent(resultSessionId, activeProjectId, result.session);
      const knownSession =
        resultSession ??
        projectSessionsByProjectId[activeProjectId]?.find(item => item.sessionId === resultSessionId) ??
        chatSessions.find(item => item.sessionId === resultSessionId);
      if ((knownSession?.lastDoneTurnIndex ?? 0) > (knownSession?.lastReadTurnIndex ?? 0)) {
        markChatSessionRead(
          activeProjectId,
          resultSessionId,
          knownSession?.lastDoneTurnIndex ?? 0,
        ).catch(() => undefined);
      }
      return true;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return false;
    } finally {
      setChatLoading(false);
    }
  };

  const refreshSessionTurns = async (
    sessionId: string,
    activeProjectId = projectIdRef.current,
    selectionSnapshot = chatSelectedIdRef.current,
  ) => {
    if (!activeProjectId || !sessionId) return false;
    const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
    try {
      const existingMessages = chatMessageStoreRef.current[runtimeKey] ?? [];
      const checkpointTurnIndex = chatSyncSubIndexRef.current[runtimeKey] ?? 0;
      const result = await service.readProjectSession(activeProjectId, sessionId, checkpointTurnIndex);
      const currentSelectedRuntimeKey = encodeChatSessionKey(selectedChatKeyRef.current);
      if (selectionSnapshot && currentSelectedRuntimeKey !== selectionSnapshot && chatSelectedIdRef.current !== selectionSnapshot) {
        return false;
      }
      const resultSessionId = result.session?.sessionId || sessionId;
      const resultRuntimeKey = buildChatRuntimeKey(activeProjectId, resultSessionId);
      const fresh = chatMessageStoreRef.current[resultRuntimeKey] ?? [];
      let nextMessages = replaceSessionMessages(fresh, result.messages, checkpointTurnIndex);
      nextMessages = reconcileSessionReadMessages(nextMessages, fresh, existingMessages);
      forgetPendingPromptIfResolved(resultRuntimeKey, nextMessages);

      chatMessageStoreRef.current[resultRuntimeKey] = nextMessages;
      const latestSyncCursor = getLatestSessionReadCursor(nextMessages);
      chatSyncIndexRef.current[resultRuntimeKey] = 0;
      chatSyncSubIndexRef.current[resultRuntimeKey] = latestSyncCursor.turnIndex;
      const resultSession = result.session;
      if (resultSession) {
        setProjectSessionsByProjectId(prev => mergeProjectSessionMap(prev, activeProjectId, resultSession));
        if (activeProjectId === projectIdRef.current) {
          setChatSessions(prev => mergeChatSession(prev, resultSession));
        }
      }
      if (encodeChatSessionKey(selectedChatKeyRef.current) === resultRuntimeKey) {
        setVisibleChatMessagesForRuntimeKey(resultRuntimeKey, nextMessages);
      }
      persistChatSessionContent(resultSessionId, activeProjectId, result.session);
      return true;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return false;
    }
  };

  const loadChatSessions = async (
    activeProjectId = projectIdRef.current,
    preferredSelection = '',
  ) => {
    if (!activeProjectId) return;
    try {
      const sessions = sortChatSessions(await service.listProjectSessions(activeProjectId));
      const nextSessions = sessions;
      setProjectSessionsByProjectId(prev => ({
        ...prev,
        [activeProjectId]: nextSessions,
      }));
      setChatSessions(prev => {
        const byId = new Map(prev.map(item => [item.sessionId, item]));
        let next: RegistryChatSession[] = [];
        for (const item of nextSessions) {
          const existing = byId.get(item.sessionId);
          const session =
            existing &&
            (item.configOptions === undefined || item.commands === undefined)
              ? {
                  ...item,
                  configOptions: item.configOptions ?? existing.configOptions,
                  commands: item.commands ?? existing.commands,
                }
              : item;
          const merged = mergeChatSession(next, session);
          next = merged;
        }
        return next;
      });

      const cursorBySessionId: Record<string, {turnIndex: number}> = {};
      for (const session of nextSessions) {
        const sessionId = session.sessionId;
        if (!sessionId) continue;
        const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
        cursorBySessionId[sessionId] = {
          turnIndex: chatSyncSubIndexRef.current[runtimeKey] ?? 0,
        };
      }
      workspaceStore.replaceChatSessions(activeProjectId, nextSessions, cursorBySessionId);

      const persistedSelection = workspaceStore.migrateSelectedChatSessionKey(activeProjectId);
      let currentSelection =
        preferredSelection ||
        (selectedChatKeyRef.current?.projectId === activeProjectId
          ? selectedChatKeyRef.current.sessionId
          : '') ||
        (persistedSelection?.projectId === activeProjectId
          ? persistedSelection.sessionId
          : '') ||
        workspaceStore.getSelectedChatSessionId(activeProjectId) ||
        '';
      if (currentSelection && !nextSessions.some(session => session.sessionId === currentSelection)) {
        currentSelection = nextSessions[0]?.sessionId || '';
      }
      currentSelection = currentSelection || nextSessions[0]?.sessionId || '';
      if (!currentSelection) {
        applySelectedChatKey(null);
        setChatMessages([]);
        return;
      }
      const nextSelectedKey = chatSessionKeyFromParts(activeProjectId, currentSelection);
      applySelectedChatKey(nextSelectedKey);
      workspaceStore.rememberSelectedChatSessionKey(nextSelectedKey);
      const cachedSelection = hydrateChatSessionContentFromCache(currentSelection, activeProjectId);
      const runtimeKey = buildChatRuntimeKey(activeProjectId, currentSelection);
      setVisibleChatMessagesForRuntimeKey(
        runtimeKey,
        cachedSelection.length > 0
          ? cachedSelection
          : (chatMessageStoreRef.current[runtimeKey] ?? []),
        {resetToLatest: true},
      );
      loadChatSession(currentSelection, activeProjectId, {
        incremental: true,
        preserveUserSelection: true,
        selectionSnapshot: runtimeKey,
      }).catch(() => undefined);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };
  const resetChatComposer = () => {
    chatComposerTextRef.current = '';
    chatAttachmentsRef.current = [];
    bumpChatDraftGeneration(currentChatDraftKeyRef.current);
    setChatComposerText('');
    setChatAttachments([]);
    saveChatComposerDraft(currentChatDraftKeyRef.current, '', []);
    if (chatFileInputRef.current) {
      chatFileInputRef.current.value = '';
    }
  };

  const rememberPendingChatPrompt = (runtimeKey: string, prompt: PendingChatPrompt) => {
    if (!runtimeKey) return;
    const next = {
      ...chatPendingPromptsByKeyRef.current,
      [runtimeKey]: prompt,
    };
    chatPendingPromptsByKeyRef.current = next;
    setChatPendingPromptsByKey(next);
  };

  const forgetPendingChatPrompt = (runtimeKey: string) => {
    if (!runtimeKey || !chatPendingPromptsByKeyRef.current[runtimeKey]) return;
    const next = {
      ...chatPendingPromptsByKeyRef.current,
    };
    delete next[runtimeKey];
    chatPendingPromptsByKeyRef.current = next;
    setChatPendingPromptsByKey(next);
  };

  const movePendingChatPrompt = (
    fromRuntimeKey: string,
    toRuntimeKey: string,
    sessionId: string,
  ) => {
    if (!fromRuntimeKey || !toRuntimeKey || fromRuntimeKey === toRuntimeKey) return;
    const prompt = chatPendingPromptsByKeyRef.current[fromRuntimeKey];
    if (!prompt) return;
    const next = {
      ...chatPendingPromptsByKeyRef.current,
      [toRuntimeKey]: {
        ...prompt,
        sessionId,
      },
    };
    delete next[fromRuntimeKey];
    chatPendingPromptsByKeyRef.current = next;
    setChatPendingPromptsByKey(next);
  };

  const forgetPendingPromptIfResolved = (
    runtimeKey: string,
    messages: RegistryChatMessage[],
  ) => {
    const pendingPrompt = chatPendingPromptsByKeyRef.current[runtimeKey];
    if (!pendingPrompt) return;
    const resolved = messages.some(message =>
      message.sessionId === pendingPrompt.sessionId &&
      isPromptStartMessage(message) &&
      (message.turnIndex ?? 0) >= pendingPrompt.turnIndex,
    );
    if (resolved) {
      forgetPendingChatPrompt(runtimeKey);
    }
  };

  const resetProjectResumeState = () => {
    setResumeSessions([]);
    setResumeLoading(false);
  };

  const openWideProjectActionMenu = (
    targetProjectId: string,
    kind: 'new' | 'resume',
  ) => {
    resetProjectResumeState();
    setMobileProjectActionMenu(null);
    setWideProjectActionMenu({
      projectId: targetProjectId,
      kind,
      phase: 'agents',
      agentType: '',
    });
  };

  const openMobileProjectActionMenu = (
    targetProjectId: string,
    kind: 'new' | 'resume',
  ) => {
    resetProjectResumeState();
    setWideProjectActionMenu(null);
    setMobileProjectActionMenu(current =>
      current?.projectId === targetProjectId && current.kind === kind
        ? null
        : {
            projectId: targetProjectId,
            kind,
            phase: 'agents',
            agentType: '',
          },
    );
  };

  const removeProjectChatSessionFromState = (
    targetProjectId: string,
    sessionId: string,
  ) => {
    if (!targetProjectId || !sessionId) return;
    setProjectSessionsByProjectId(prev => ({
      ...prev,
      [targetProjectId]: (prev[targetProjectId] ?? []).filter(
        item => item.sessionId !== sessionId,
      ),
    }));
    if (targetProjectId === projectIdRef.current) {
      setChatSessions(prev => prev.filter(item => item.sessionId !== sessionId));
    }
    const runtimeKey = buildChatRuntimeKey(targetProjectId, sessionId);
    setChatRunningSessionFlags(prev => removeSessionFlag(prev, runtimeKey));
    setChatCompletedUnopenedFlags(prev => removeSessionFlag(prev, runtimeKey));
    if (encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey) {
        applySelectedChatKey(null);
        setChatMessages([]);
        workspaceStore.rememberSelectedChatSessionKey(null);
    }
      const nextMessageStore = {...chatMessageStoreRef.current};
      const nextSyncIndex = {...chatSyncIndexRef.current};
      const nextSyncSubIndex = {...chatSyncSubIndexRef.current};
      delete nextMessageStore[runtimeKey];
      delete nextSyncIndex[runtimeKey];
      delete nextSyncSubIndex[runtimeKey];
      chatMessageStoreRef.current = nextMessageStore;
      chatSyncIndexRef.current = nextSyncIndex;
      chatSyncSubIndexRef.current = nextSyncSubIndex;
    workspaceStore.deleteChatSession(targetProjectId, sessionId);
  };

  const handleDeleteProjectSession = async (targetProjectId: string, sessionId: string) => {
    const normalizedSessionId = sessionId.trim();
    if (!targetProjectId || !normalizedSessionId || chatDeletingSessionId) {
      return;
    }
    const confirmed = window.confirm(
      'Delete this session and its chat history? This action cannot be undone.',
    );
    if (!confirmed) {
      setProjectSessionActionMenu(null);
      return;
    }
    setChatDeletingSessionId(normalizedSessionId);
    try {
      const result = await service.deleteProjectSession(targetProjectId, normalizedSessionId);
      if (!result.ok) {
        throw new Error('session.delete returned ok=false');
      }
      removeProjectChatSessionFromState(
        targetProjectId,
        result.sessionId || normalizedSessionId,
      );
      setProjectSessionActionMenu(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setChatDeletingSessionId('');
    }
  };

  const handleReloadProjectSession = async (targetProjectId: string, sessionId: string) => {
    const normalizedSessionId = sessionId.trim();
    if (!targetProjectId || !normalizedSessionId || chatReloadingSessionId) {
      return;
    }
    setChatReloadingSessionId(normalizedSessionId);
    try {
      const result = await service.reloadProjectSession(targetProjectId, normalizedSessionId);
      if (!result.ok) {
        throw new Error('session.reload returned ok=false');
      }
      clearProjectSessionCache(targetProjectId, normalizedSessionId);
      const runtimeKey = buildChatRuntimeKey(targetProjectId, normalizedSessionId);
      if (
        encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey
      ) {
        setChatMessages([]);
        await loadChatSession(normalizedSessionId, targetProjectId, { forceFull: true });
      }
      setProjectSessionActionMenu(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setChatReloadingSessionId('');
    }
  };

  const sendChatMessage = async () => {
    if (chatAttachmentReadPending) {
      setError('Wait for images to finish loading.');
      return;
    }
    const trimmedText = chatComposerText.trim();
    if (!trimmedText && chatAttachments.length === 0) return;
    const blocks: RegistryChatContentBlock[] = [];
    if (trimmedText) {
      blocks.push({ type: 'text', text: trimmedText });
    }
    blocks.push(...chatAttachments.map(attachment => ({
      type: 'image',
      mimeType: attachment.mimeType,
      data: attachment.data,
    } satisfies RegistryChatContentBlock)));
    const firstAttachmentName = chatAttachments[0]?.name || '';
    const selectedKey = selectedChatKeyRef.current;
    if (!selectedKey) {
      setError('Select or create a chat session first.');
      return;
    }
    const selectedProjectId = selectedKey.projectId;
    const sessionId = selectedKey.sessionId;
    if (!sessionId) {
      setError('Select or create a chat session first.');
      return;
    }
    const runtimeKey = buildChatRuntimeKey(selectedProjectId, sessionId);
    const createdAt = new Date().toISOString();
    rememberPendingChatPrompt(runtimeKey, {
      sessionId,
      blocks: blocks.map(block => ({...block})),
      createdAt,
      turnIndex: nextPromptTurnIndex(chatMessageStoreRef.current[runtimeKey] ?? []),
    });

    // Clear UI immediately after capturing text — before any async work
    resetChatComposer();
    forceChatScrollToBottom();
    setChatSending(true);
    let optimisticRunningSessionId = '';
    try {
      optimisticRunningSessionId = sessionId;
      markChatSessionRunning(selectedProjectId, sessionId, trimmedText || firstAttachmentName);
      const result = await service.sendProjectSessionMessage(selectedProjectId, {
        sessionId,
        text: trimmedText,
        blocks,
      });
      const nextSessionId = result.sessionId || sessionId;
      if (nextSessionId !== sessionId) {
        movePendingChatPrompt(runtimeKey, buildChatRuntimeKey(selectedProjectId, nextSessionId), nextSessionId);
      }
      markChatSessionRunning(selectedProjectId, nextSessionId, trimmedText || firstAttachmentName);
      const nextSelectedKey = chatSessionKeyFromParts(selectedProjectId, nextSessionId);
      applySelectedChatKey(nextSelectedKey);
      workspaceStore.rememberSelectedChatSessionKey(nextSelectedKey);
      rememberChatSessionSummary(selectedProjectId, {
        sessionId: nextSessionId,
        preview: trimmedText || firstAttachmentName || '',
        updatedAt: new Date().toISOString(),
        messageCount: 0,
      });
    } catch (err) {
      if (optimisticRunningSessionId) {
        clearChatSessionRunning(selectedProjectId, optimisticRunningSessionId);
      }
      forgetPendingChatPrompt(runtimeKey);
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setChatSending(false);
    }
  };

  const applyChatSessionConfigOptions = (
    activeProjectId: string,
    sessionId: string,
    configOptions: RegistrySessionConfigOption[],
  ) => {
    if (!activeProjectId || !sessionId) return;
    setProjectSessionsByProjectId(prev => {
      const existing = prev[activeProjectId]?.find(item => item.sessionId === sessionId);
      return mergeProjectSessionMap(prev, activeProjectId, {
        ...(existing ?? {sessionId}),
        configOptions,
      });
    });
    if (activeProjectId === projectIdRef.current) {
      setChatSessions(prev => {
        const existing = prev.find(item => item.sessionId === sessionId);
        if (!existing) return prev;
        return mergeChatSession(prev, {
          ...existing,
          configOptions,
        });
      });
    }
  };

  const handleChatConfigOptionChange = async (
    option: RegistrySessionConfigOption,
    value: string,
  ) => {
    const selectedKey = selectedChatKeyRef.current;
    const sessionId = selectedKey?.sessionId.trim() ?? '';
    const configId = option.id.trim();
    const nextValue = value;
    if (!selectedKey || !sessionId || !configId || !nextValue || nextValue === option.currentValue) {
      return;
    }
    const updatingKey = `${encodeChatSessionKey(selectedKey)}:${configId}`;
    setChatConfigUpdatingKey(updatingKey);

    try {
      const result = await service.setProjectSessionConfig(selectedKey.projectId, {
        sessionId,
        configId,
        value: nextValue,
      });
      if (!result.ok) {
        throw new Error('session.setConfig returned ok=false');
      }
      if (result.configOptions.length > 0) {
        applyChatSessionConfigOptions(selectedKey.projectId, result.sessionId || sessionId, result.configOptions);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setError(message);
    } finally {
      setChatConfigUpdatingKey(prev => (prev === updatingKey ? '' : prev));
    }
  };

  const handleChatFileChange = async (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const files = Array.from(event.target.files ?? []);
    if (files.length === 0) {
      return;
    }
    const attachmentDraftKey = currentChatDraftKeyRef.current;
    const attachmentDraftGeneration = getChatDraftGeneration(attachmentDraftKey);
    beginChatAttachmentRead(attachmentDraftKey);
    let readError = '';
    try {
      const attachments = (
        await Promise.all(
          files.map(async (file, index) => {
            if (!(file.type || '').toLowerCase().startsWith('image/')) {
              return null;
            }
            try {
              return await readChatAttachmentFile(
                file,
                `selected-image-${index + 1}.png`,
              );
            } catch (err) {
              if (!readError) {
                readError = err instanceof Error ? err.message : String(err);
              }
              return null;
            }
          }),
        )
      ).filter((attachment): attachment is ChatAttachment => !!attachment);
      appendChatAttachments(
        attachments,
        attachmentDraftKey,
        attachmentDraftGeneration,
      );
      if (readError) {
        setError(readError);
      }
    } finally {
      endChatAttachmentRead(attachmentDraftKey);
    }
    event.target.value = '';
  };

  const connect = async ({
    silentReconnect = false,
  }: { silentReconnect?: boolean } = {}) => {
    if (connectInFlightRef.current) {
      return;
    }
    connectInFlightRef.current = true;
    const trimmedToken = tokenRef.current.trim();
    const nextAddress = addressRef.current.trim();
    const previousSelectedChatKey = selectedChatKeyRef.current;
    setError('');
    clearReconnectTimer();
    if (!silentReconnect) {
      reconnectStartedAtRef.current = null;
      setReconnecting(false);
    }
    try {
      const ws = toRegistryWsUrl(nextAddress);
      const result = await workspaceController.connect(ws, trimmedToken);
      const persistedSelectedChatKey = workspaceStore.migrateSelectedChatSessionKey(result.hydrated.projectId);
      const preferredSelectedChatKey =
        previousSelectedChatKey ||
        persistedSelectedChatKey ||
        chatSessionKeyFromParts(
          result.hydrated.projectId,
          workspaceStore.getSelectedChatSessionId(result.hydrated.projectId),
        );
      const preferredSelectedChatId = preferredSelectedChatKey?.sessionId ?? '';
      setProjects(result.projects);
      setHasPendingProjectUpdates(false);
      captureSelectedFileScrollPosition();
      dirHashRef.current = {};
      if (!silentReconnect) {
        fileHashRef.current = {};
        fileCacheRef.current = {};
      }
      applyHydratedProjectState(result.hydrated, {
        preserveFileView: silentReconnect,
      });
      const selectedFileToReload =
        result.hydrated.selectedFile || selectedFileRef.current;
      if (selectedFileToReload) {
        skipNextSelectedFileAutoReadRef.current = true;
        readSelectedFile(selectedFileToReload, { restoreScroll: true, silent: silentReconnect }).catch(() => undefined);
      }
      reconnectStartedAtRef.current = null;
      setReconnecting(false);
      setConnected(true);
      if (!silentReconnect) {
        clearChatRuntimeState();
        if (preferredSelectedChatKey) {
          applySelectedChatKey(preferredSelectedChatKey);
          hydrateChatSessionsFromCache(preferredSelectedChatKey.projectId, preferredSelectedChatKey.sessionId);
        } else {
          hydrateChatSessionsFromCache(result.hydrated.projectId, '');
        }
      }
      if (silentReconnect) {
        const shouldSyncSelectedSession =
          tabRef.current === 'chat' &&
          !!preferredSelectedChatKey;
        if (shouldSyncSelectedSession) {
          await loadChatSession(preferredSelectedChatKey.sessionId, preferredSelectedChatKey.projectId, {
            incremental: true,
            preserveUserSelection: true,
            selectionSnapshot: encodeChatSessionKey(preferredSelectedChatKey),
          });
        }
      } else if (tabRef.current === 'chat') {
        loadChatSessions(
          preferredSelectedChatKey?.projectId ?? result.hydrated.projectId,
          preferredSelectedChatId,
        ).catch(() => undefined);
      }
      await refreshChatIndex();
      workspaceController
        .validateExpandedDirectories(
          result.hydrated.projectId,
          result.rootEntries,
          result.hydrated.expandedDirs,
        )
        .then(validated => {
          if (projectIdRef.current !== result.hydrated.projectId) return;
          setDirEntries(validated.dirEntries);
          setExpandedDirs(validated.expandedDirs);
        })
        .catch(() => undefined);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      if (silentReconnect) {
        const reconnectStartedAt = reconnectStartedAtRef.current ?? Date.now();
        reconnectStartedAtRef.current = reconnectStartedAt;
        const elapsed = Date.now() - reconnectStartedAt;
        if (elapsed < RECONNECT_GRACE_PERIOD_MS) {
          setError('');
          setReconnecting(true);
          scheduleReconnectAttempt();
          return;
        }
        reconnectStartedAtRef.current = null;
        setReconnecting(false);
        setError(
          `Registry reconnect failed for ${Math.floor(
            RECONNECT_GRACE_PERIOD_MS / 15000,
          )}s. Please reconnect manually.`,
        );
        return;
      }
      setError(message);
    } finally {
      connectInFlightRef.current = false;
      setAutoConnecting(false);
    }
  };

  const disconnectForSupervisor = (
    reason: 'background' | 'offline' | 'stop',
  ) => {
    supervisorManagedCloseRef.current = true;
    clearReconnectTimer();
    reconnectStartedAtRef.current = null;
    const shouldKeepWorkspaceVisible =
      reason !== 'stop' && !!addressRef.current.trim() && !!projectIdRef.current;
    setReconnecting(shouldKeepWorkspaceVisible);
    setAutoConnecting(false);
    setConnected(false);
    if (reason !== 'stop') {
      setError('');
    }
    service.close();
  };

  const maybeNotifyChatMessage = (
    message: RegistryChatMessage,
    session?: RegistryChatSession,
    activeProjectId = '',
  ) => {
    const runtimeKey = buildChatRuntimeKey(activeProjectId, message.sessionId);
    const messageKey = `${runtimeKey}:${message.turnIndex}`;
    if (!message.sessionId || msgRole(message.method) === 'user') {
      return;
    }
    if (notifiedChatMessageIdsRef.current.has(messageKey)) {
      return;
    }
    const isVisible =
      typeof document !== 'undefined' && document.visibilityState === 'visible';
    if (isVisible && runtimeKey && encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey) {
      return;
    }

    const text = msgText(message.method, message.param).trim();
    const body = text
      ? text.length > 120
        ? `${text.slice(0, 120)}...`
        : text
      : 'New chat message';

    notifiedChatMessageIdsRef.current.add(messageKey);
    if (notifiedChatMessageIdsRef.current.size > 500) {
      const first = notifiedChatMessageIdsRef.current.values().next().value;
      if (first) {
        notifiedChatMessageIdsRef.current.delete(first);
      }
    }

    const title = session?.title?.trim()
      ? `Chat: ${session.title}`
      : 'WheelMaker Chat';
    pwaFoundation.pushDemo
      .showLocalNotification({ title, body, url: '/' })
      .catch(() => undefined);
  };

  useEffect(() => {
    const supervisor = pwaFoundation.createConnectionSupervisor({
      connect: async () => {
        const canSilentReconnect =
          !!addressRef.current.trim() && !!projectIdRef.current;
        if (!canSilentReconnect) {
          return;
        }
        await connect({ silentReconnect: true });
      },
      disconnect: reason => {
        disconnectForSupervisor(reason);
      },
    });
    supervisor.start();
    return () => {
      supervisor.stop();
    };
  }, []);

  useEffect(() => {
    if (connected || autoConnecting) return;
    if (autoConnectTriedRef.current) return;
    if (!address.trim()) return;
    autoConnectTriedRef.current = true;
    setAutoConnecting(true);
    connect().catch(() => {
      setAutoConnecting(false);
    });
  }, [address, autoConnecting, connected]);

  useEffect(() => {
    const activeProjectId = projectIdRef.current;
    if (!activeProjectId) return;
    persistChatSessionsIndex(activeProjectId);
  }, [chatSessions]);

  const mergeTokenProviders = useCallback(
    (entries: Array<{hubId: string; projectId: string; result: RegistryTokenScanResult}>): TokenProviderSectionView[] => {
      const sections = new Map<string, TokenProviderSectionView>();
      for (const entry of entries) {
        const providers = Array.isArray(entry.result.providers) ? entry.result.providers : [];
        for (const provider of providers) {
          const providerId = (provider.id || provider.name || 'unknown').trim().toLowerCase();
          if (!providerId) continue;
          const section = sections.get(providerId) ?? {
            id: providerId,
            name: provider.name || provider.id || providerId,
            accounts: [],
          };
          const accounts = Array.isArray(provider.accounts) ? provider.accounts : [];
          for (const account of accounts) {
            section.accounts.push({
              ...account,
              id: account.id || account.alias || account.displayName || 'account',
              hubId: entry.hubId,
              projectId: entry.projectId,
              providerId: section.id,
              providerName: section.name,
            });
          }
          sections.set(providerId, section);
        }
      }
      const merged = Array.from(sections.values());
      merged.forEach(section => {
        section.accounts.sort((left, right) => {
          const hubDiff = left.hubId.localeCompare(right.hubId);
          if (hubDiff !== 0) return hubDiff;
          return (left.alias || left.displayName || '').localeCompare(right.alias || right.displayName || '');
        });
      });
      merged.sort((left, right) => left.name.localeCompare(right.name));
      return merged;
    },
    [],
  );

  const tokenTagVariantClass = useCallback((scope: 'agent' | 'hub', value: string): string => {
    return scope === 'agent'
      ? tagVariantClass('token-stats-pill-agent', value)
      : tagVariantClass('token-stats-pill-hub', value);
  }, []);

  const tokenStatCards = useMemo(
    (): TokenStatCardView[] => buildTokenStatCards(tokenStatsProviders),
    [tokenStatsProviders],
  );

  const refreshTokenStats = useCallback(async () => {
    setTokenStatsLoading(true);
    setTokenStatsError('');
    try {
      const latestProjects = await service.listProjects();
      if (latestProjects.length > 0) {
        setProjects(latestProjects);
      }
      const onlineByHub = new Map<string, RegistryProject>();
      for (const project of latestProjects) {
        if (!project.online) continue;
        const hubId = (project.hubId || 'local').trim() || 'local';
        if (!onlineByHub.has(hubId)) {
          onlineByHub.set(hubId, project);
        }
      }
      if (onlineByHub.size === 0) {
        setTokenStatsProviders([]);
        setTokenStatsUpdatedAt('');
        setTokenStatsError('No online hubs available.');
        return;
      }
      const requests = Array.from(onlineByHub.entries()).map(async ([hubId, project]) => {
        const result = await service.scanTokenStats(project.projectId);
        return {hubId, projectId: project.projectId, result};
      });
      const responses = await Promise.all(requests);
      setTokenStatsProviders(mergeTokenProviders(responses));
      const latestUpdatedAt = responses
        .map(item => item.result.updatedAt || '')
        .sort((left, right) => right.localeCompare(left))[0] || '';
      setTokenStatsUpdatedAt(latestUpdatedAt);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setTokenStatsError(message);
    } finally {
      setTokenStatsLoading(false);
    }
  }, [mergeTokenProviders]);

  useEffect(() => {
    if (settingsDetailView !== 'tokenStats') {
      return;
    }
    refreshTokenStats().catch(() => undefined);
  }, [settingsDetailView, refreshTokenStats]);

  const formatDatabaseDump = (dump: Awaited<ReturnType<typeof workspaceStore.dumpDatabase>>): string => {
    return JSON.stringify(
      {
        wm_global_kv: dump.global,
        wm_project_state: dump.projects,
        wm_project_commits: dump.projectCommits,
        wm_chat_session_index: dump.chatSessionIndex,
        wm_chat_session_content: dump.chatSessionContent,
        wm_file_cache: dump.fileCache,
        wm_diff_cache: dump.diffCache,
        wm_meta: dump.meta,
        local_storage: dump.localStorage,
      },
      null,
      2,
    );
  };

  const openDatabasePanel = () => {
    setDatabasePanelOpen(true);
    setDatabaseLoading(true);
    setDatabaseError('');
    workspaceStore
      .dumpDatabase()
      .then(dump => {
        setDatabaseDumpText(formatDatabaseDump(dump));
      })
      .catch(err => {
        const message = err instanceof Error ? err.message : String(err);
        setDatabaseError(message);
      })
      .finally(() => {
        setDatabaseLoading(false);
      });
  };

  const exportDatabaseDump = () => {
    if (!databaseDumpText || databaseLoading || databaseError) {
      return;
    }
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    const fileName = `wheelmaker-local-db-${timestamp}.json`;
    const blob = new Blob([databaseDumpText], {
      type: 'application/json;charset=utf-8',
    });
    const objectUrl = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = objectUrl;
    link.download = fileName;
    link.click();
    URL.revokeObjectURL(objectUrl);
  };

  const clearLocalCache = () => {
    const confirmed = window.confirm(
      'Clear all local cache data except token and address?',
    );
    if (!confirmed) return;
    workspaceStore.clearLocalCachePreservingToken();
    window.location.reload();
  };

  const switchProject = async (nextProjectId: string) => {
    setLoadingProject(true);
    try {
      const result = await workspaceController.switchProject(nextProjectId);
      setProjects(result.projects);
      setHasPendingProjectUpdates(false);
      applyHydratedProjectState(result.hydrated);
      workspaceController
        .validateExpandedDirectories(
          result.hydrated.projectId,
          result.rootEntries,
          result.hydrated.expandedDirs,
        )
        .then(validated => {
          if (projectIdRef.current !== result.hydrated.projectId) return;
          setDirEntries(validated.dirEntries);
          setExpandedDirs(validated.expandedDirs);
        })
        .catch(() => undefined);
    } finally {
      setLoadingProject(false);
    }
  };

  const selectProjectChatSession = async (
    targetProjectId: string,
    sessionId: string,
    options?: {closeMobileDrawer?: boolean},
  ) => {
    if (!targetProjectId || !sessionId) return;
    const nextSelectedKey = chatSessionKeyFromParts(targetProjectId, sessionId);
    if (!nextSelectedKey) return;
    workspaceStore.rememberSelectedChatSessionKey(nextSelectedKey);
    setWideProjectActionMenu(null);
    setMobileProjectActionMenu(null);
    if (options?.closeMobileDrawer) {
      setDrawerOpen(false);
    }
    setTab('chat');
    applySelectedChatKey(nextSelectedKey);
    const runtimeKey = encodeChatSessionKey(nextSelectedKey);
    setChatCompletedUnopenedFlags(prev => removeSessionFlag(prev, runtimeKey));
    setChatMessages([]);
    setVisibleChatMessagesForRuntimeKey(
      runtimeKey,
      hydrateChatSessionContentFromCache(sessionId, targetProjectId),
      {resetToLatest: true},
    );
    await loadChatSession(sessionId, targetProjectId, {
      incremental: true,
      preserveUserSelection: true,
      selectionSnapshot: runtimeKey,
    });
  };

  const selectWideProjectSession = async (targetProjectId: string, sessionId: string) => {
    await selectProjectChatSession(targetProjectId, sessionId);
  };

  const handleProjectCreateSession = async (
    targetProjectId: string,
    agentType: string,
    options?: {closeMobileDrawer?: boolean},
  ) => {
    const normalizedAgentType = agentType.trim();
    if (!targetProjectId || !normalizedAgentType) {
      setError('No agent selected for new session');
      return;
    }
    try {
      const result = await service.createProjectSession(targetProjectId, agentType, '');
      if (!result.ok || !result.session.sessionId) {
        throw new Error('project session.create returned ok=false');
      }
      const session = result.session;
      workspaceStore.rememberChatSession(targetProjectId, session, {turnIndex: 0});
      setProjectSessionsByProjectId(prev => ({
        ...prev,
        [targetProjectId]: mergeChatSession(prev[targetProjectId] ?? [], session),
      }));
      const runtimeKey = buildChatRuntimeKey(targetProjectId, session.sessionId);
      chatMessageStoreRef.current[runtimeKey] = [];
      chatSyncIndexRef.current[runtimeKey] = 0;
      chatSyncSubIndexRef.current[runtimeKey] = 0;
      if (targetProjectId === projectIdRef.current) {
        setChatSessions(prev => mergeChatSession(prev, session));
      }
      await selectProjectChatSession(targetProjectId, session.sessionId, options);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const handleWideProjectCreateSession = async (targetProjectId: string, agentType: string) => {
    await handleProjectCreateSession(targetProjectId, agentType);
  };

  const handleMobileProjectCreateSession = async (targetProjectId: string, agentType: string) => {
    await handleProjectCreateSession(targetProjectId, agentType, {closeMobileDrawer: true});
  };

  const handleWideProjectResumeAgent = async (targetProjectId: string, agentType: string) => {
    const normalizedAgentType = agentType.trim();
    if (!targetProjectId || !normalizedAgentType) {
      setError('No agent selected for resume');
      return;
    }
    setWideProjectActionMenu({
      projectId: targetProjectId,
      kind: 'resume',
      phase: 'sessions',
      agentType: normalizedAgentType,
    });
    setResumeLoading(true);
    setResumeSessions([]);
    try {
      const sessions = await service.listProjectResumableSessions(targetProjectId, agentType);
      setResumeSessions(sessions);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setWideProjectActionMenu(null);
    } finally {
      setResumeLoading(false);
    }
  };

  const handleWideProjectResumeImport = async (targetProjectId: string, agentType: string, sessionId: string) => {
    if (!targetProjectId || !agentType || !sessionId) return;
    setResumeLoading(true);
    let importedSessionId = '';
    try {
      const imported = await service.importProjectResumedSession(targetProjectId, agentType, sessionId);
      if (!imported.ok || !imported.session.sessionId) {
        throw new Error('project session.resume.import returned ok=false');
      }
      importedSessionId = imported.session.sessionId;
      const session = imported.session;
      workspaceStore.rememberChatSession(targetProjectId, session, {turnIndex: 0});
      const selectedKey = chatSessionKeyFromParts(targetProjectId, importedSessionId);
      workspaceStore.rememberSelectedChatSessionKey(selectedKey);
      setResumeSessions(prev => prev.filter(item => item.sessionId !== sessionId));
      setProjectSessionsByProjectId(prev => ({
        ...prev,
        [targetProjectId]: mergeChatSession(prev[targetProjectId] ?? [], session),
      }));
      if (targetProjectId === projectIdRef.current) {
        setChatSessions(prev => mergeChatSession(prev, session));
      }
      const reloaded = await service.reloadProjectSession(targetProjectId, importedSessionId);
      if (!reloaded.ok) {
        throw new Error('project session.reload returned ok=false');
      }
      const runtimeKey = buildChatRuntimeKey(targetProjectId, importedSessionId);
      chatMessageStoreRef.current[runtimeKey] = [];
      chatSyncIndexRef.current[runtimeKey] = 0;
      chatSyncSubIndexRef.current[runtimeKey] = 0;
      setTab('chat');
      applySelectedChatKey(selectedKey);
      setChatMessages([]);
      const loaded = await loadChatSession(importedSessionId, targetProjectId, { forceFull: true });
      if (!loaded) {
        throw new Error('Failed to load resumed session history');
      }
      setWideProjectActionMenu(null);
    } catch (err) {
      if (importedSessionId) {
        setWideProjectActionMenu(null);
      }
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setResumeLoading(false);
    }
  };

  const handleMobileProjectResumeAgent = async (targetProjectId: string, agentType: string) => {
    const normalizedAgentType = agentType.trim();
    if (!targetProjectId || !normalizedAgentType) {
      setError('No agent selected for resume');
      return;
    }
    setMobileProjectActionMenu({
      projectId: targetProjectId,
      kind: 'resume',
      phase: 'sessions',
      agentType: normalizedAgentType,
    });
    setResumeLoading(true);
    setResumeSessions([]);
    try {
      const sessions = await service.listProjectResumableSessions(targetProjectId, agentType);
      setResumeSessions(sessions);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setMobileProjectActionMenu(null);
    } finally {
      setResumeLoading(false);
    }
  };

  const handleMobileProjectResumeImport = async (targetProjectId: string, agentType: string, sessionId: string) => {
    if (!targetProjectId || !agentType || !sessionId) return;
    setResumeLoading(true);
    let importedSessionId = '';
    try {
      const imported = await service.importProjectResumedSession(targetProjectId, agentType, sessionId);
      if (!imported.ok || !imported.session.sessionId) {
        throw new Error('project session.resume.import returned ok=false');
      }
      importedSessionId = imported.session.sessionId;
      const session = imported.session;
      workspaceStore.rememberChatSession(targetProjectId, session, {turnIndex: 0});
      workspaceStore.rememberSelectedChatSessionKey(chatSessionKeyFromParts(targetProjectId, importedSessionId));
      setResumeSessions(prev => prev.filter(item => item.sessionId !== sessionId));
      setProjectSessionsByProjectId(prev => ({
        ...prev,
        [targetProjectId]: mergeChatSession(prev[targetProjectId] ?? [], session),
      }));
      if (targetProjectId === projectIdRef.current) {
        setChatSessions(prev => mergeChatSession(prev, session));
      }
      const reloaded = await service.reloadProjectSession(targetProjectId, importedSessionId);
      if (!reloaded.ok) {
        throw new Error('project session.reload returned ok=false');
      }
      const runtimeKey = buildChatRuntimeKey(targetProjectId, importedSessionId);
      chatMessageStoreRef.current[runtimeKey] = [];
      chatSyncIndexRef.current[runtimeKey] = 0;
      chatSyncSubIndexRef.current[runtimeKey] = 0;
      await selectProjectChatSession(targetProjectId, importedSessionId, {closeMobileDrawer: true});
    } catch (err) {
      if (importedSessionId) {
        setMobileProjectActionMenu(null);
      }
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setResumeLoading(false);
    }
  };

  const renderProjectSessionActionStrip = (targetProjectId: string, sessionId: string) => {
    if (
      projectSessionActionMenu?.projectId !== targetProjectId ||
      projectSessionActionMenu.sessionId !== sessionId
    ) {
      return null;
    }
    return (
      <div className="project-session-action-strip">
        <button
          type="button"
          className="project-session-action-btn reload"
          title="Reload session"
          aria-label="Reload session"
          disabled={chatReloadingSessionId === sessionId}
          onPointerDown={event => event.stopPropagation()}
          onClick={event => {
            event.stopPropagation();
            handleReloadProjectSession(targetProjectId, sessionId).catch(() => undefined);
          }}
        >
          <span
            className={`codicon ${
              chatReloadingSessionId === sessionId
                ? 'codicon-loading codicon-modifier-spin'
                : 'codicon-refresh'
            }`}
          />
          <span className="project-session-action-label">Reload</span>
        </button>
        <button
          type="button"
          className="project-session-action-btn delete"
          title="Delete session"
          aria-label="Delete session"
          disabled={chatDeletingSessionId === sessionId}
          onPointerDown={event => event.stopPropagation()}
          onClick={event => {
            event.stopPropagation();
            handleDeleteProjectSession(targetProjectId, sessionId).catch(() => undefined);
          }}
        >
          <span
            className={`codicon ${
              chatDeletingSessionId === sessionId
                ? 'codicon-loading codicon-modifier-spin'
                : 'codicon-trash'
            }`}
          />
          <span className="project-session-action-label">Delete</span>
        </button>
      </div>
    );
  };

  const refreshProject = async (options?: {silent?: boolean}) => {
    if (!connected || !projectId) return;
    if (refreshInFlightRef.current) return;
    refreshInFlightRef.current = true;
    const silent = !!options?.silent;
    const latestProject = currentProjectRef.current;
    const latestExpandedDirs = expandedDirsRef.current;
    const latestSelectedFile = selectedFileRef.current;
    if (!silent) {
      setRefreshingProject(true);
    }
    try {
      const sync = await service.syncCheck({
        knownProjectRev: latestProject?.projectRev ?? '',
        knownGitRev: latestProject?.git?.gitRev ?? '',
        knownWorktreeRev: latestProject?.git?.worktreeRev ?? '',
      });
      const needsProjectOrFsRefresh = sync.staleDomains.some(
        domain => domain === 'fs' || domain === 'project',
      );
      if (sync.staleDomains.includes('project') || !latestProject) {
        setProjects(await service.listProjects());
      }
      if (needsProjectOrFsRefresh) {
        const validated = await workspaceController.refreshProject(projectId, [
          ...latestExpandedDirs,
        ]);
        setDirEntries(validated.dirEntries);
        setExpandedDirs(validated.expandedDirs);
        dirHashRef.current = {};
      }
      if (latestSelectedFile && needsProjectOrFsRefresh) {
        await readSelectedFile(latestSelectedFile);
      }
      if (
        sync.staleDomains.some(
          domain =>
            domain === 'git' || domain === 'worktree' || domain === 'project',
        )
      ) {
        await loadGit();
      }
      if (!silent) {
        setHasPendingProjectUpdates(false);
      }
    } finally {
      refreshInFlightRef.current = false;
      if (!silent) {
        setRefreshingProject(false);
      }
    }
  };

  const rememberProjectSessionList = (
    targetProjectId: string,
    sessions: RegistryChatSession[],
  ) => {
    const sortedSessions = sortChatSessions(sessions);
    setProjectSessionsByProjectId(prev => ({
      ...prev,
      [targetProjectId]: sortedSessions,
    }));
    if (targetProjectId === projectIdRef.current) {
      setChatSessions(sortedSessions);
    }
    const cached = workspaceStore.hydrateChatSessions(targetProjectId);
    const cursorBySessionId: Record<string, {turnIndex: number}> = {};
    for (const entry of cached) {
      const sessionId = entry.session.sessionId;
      if (!sessionId) continue;
      const runtimeKey = buildChatRuntimeKey(targetProjectId, sessionId);
      cursorBySessionId[sessionId] = {
        turnIndex: chatSyncSubIndexRef.current[runtimeKey] ?? entry.cursor.turnIndex,
      };
    }
    for (const session of sortedSessions) {
      const sessionId = session.sessionId;
      if (!sessionId || cursorBySessionId[sessionId]) continue;
      const runtimeKey = buildChatRuntimeKey(targetProjectId, sessionId);
      cursorBySessionId[sessionId] = {
        turnIndex: chatSyncSubIndexRef.current[runtimeKey] ?? 0,
      };
    }
    workspaceStore.replaceChatSessions(targetProjectId, sortedSessions, cursorBySessionId);
  };

  const refreshChatProjectSessions = async (targetProjectId: string) => {
    if ((!connected && !connectInFlightRef.current) || !targetProjectId) return;
    if (chatProjectRefreshInFlightRef.current[targetProjectId]) {
      chatProjectRefreshDirtyRef.current[targetProjectId] = true;
      return;
    }
    chatProjectRefreshInFlightRef.current[targetProjectId] = true;
    try {
      do {
        chatProjectRefreshDirtyRef.current[targetProjectId] = false;
        try {
          const sessions = await service.listProjectSessions(targetProjectId);
          rememberProjectSessionList(targetProjectId, sessions);
          setMobileProjectSessionErrors(prev => {
            if (!prev[targetProjectId]) return prev;
            const next = {...prev};
            delete next[targetProjectId];
            return next;
          });
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          setMobileProjectSessionErrors(prev => ({
            ...prev,
            [targetProjectId]: message || 'Failed to refresh sessions',
          }));
        }
      } while (chatProjectRefreshDirtyRef.current[targetProjectId]);
    } finally {
      chatProjectRefreshInFlightRef.current[targetProjectId] = false;
    }
  };

  const refreshChatIndex = async () => {
    if (!connected && !connectInFlightRef.current) return;
    if (chatIndexFullRefreshInFlightRef.current) {
      chatIndexFullRefreshDirtyRef.current = true;
      return;
    }
    chatIndexFullRefreshInFlightRef.current = true;
    setMobileProjectSessionsRefreshing(true);
    setRefreshingProject(true);
    try {
      do {
        chatIndexFullRefreshDirtyRef.current = false;
        const latestProjects = await service.listProjects();
        setProjects(latestProjects);
        setHasPendingProjectUpdates(false);
        await Promise.all(
          latestProjects.map(projectItem =>
            refreshChatProjectSessions(projectItem.projectId),
          ),
        );
      } while (chatIndexFullRefreshDirtyRef.current);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      chatIndexFullRefreshInFlightRef.current = false;
      setMobileProjectSessionsRefreshing(false);
      setRefreshingProject(false);
    }
  };

  const refreshMobileChatProjectSessions = async () => {
    await refreshChatIndex();
  };

  useEffect(() => {
    const unsubscribeEvent = service.onEvent(event => {
      const eventProjectId = event.projectId ?? '';
      if (
        event.method === 'project.online' ||
        event.method === 'project.offline'
      ) {
        setProjects(prev =>
          prev.map(item =>
            item.projectId === eventProjectId
              ? { ...item, online: event.method === 'project.online' }
              : item,
            ),
        );
        if (!eventProjectId || eventProjectId === projectIdRef.current) {
          setHasPendingProjectUpdates(true);
        }
      }
      if (event.method === 'session.updated') {
        if (!eventProjectId) {
          return;
        }
        if (!projectsRef.current.some(item => item.projectId === eventProjectId)) {
          refreshChatIndex().catch(() => undefined);
          return;
        }
        const payload = (event.payload ?? {}) as {
          session?: RegistryChatSession;
        };
        if (payload.session?.sessionId) {
          const runtimeKey = buildChatRuntimeKey(eventProjectId, payload.session.sessionId);
          rememberChatSessionSummary(eventProjectId, payload.session);
          workspaceStore.rememberChatSession(eventProjectId, payload.session, {
            turnIndex: chatSyncSubIndexRef.current[runtimeKey] ?? 0,
          });
        }
        return;
      }
      if (event.method === 'session.message') {
        if (!eventProjectId) {
          return;
        }
        if (!projectsRef.current.some(item => item.projectId === eventProjectId)) {
          refreshChatIndex().catch(() => undefined);
          return;
        }
        const payload = (event.payload ??
          {}) as RegistryChatMessageEventPayload;
        const message = decodeSessionMessageFromEventPayload(payload);
        if (!message) {
          return;
        }
        const sessionId = message.sessionId;
        const runtimeKey = buildChatRuntimeKey(eventProjectId, sessionId);
        const isSelectedSession = encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey;
        const knownProjectSessions = projectSessionsByProjectIdRef.current[eventProjectId] ?? [];
        const knownSession = knownProjectSessions.some(session => session.sessionId === sessionId);
        if (!knownSession && !isSelectedSession) {
          refreshChatProjectSessions(eventProjectId).catch(() => undefined);
          return;
        }
        const shouldRefreshCompletedPrompt = message.method === 'prompt_done';
        const shouldMarkSessionRunning = isChatSessionRunningMessage(message);
        const existingMessagesForSession = chatMessageStoreRef.current[runtimeKey] ?? [];
        if (shouldMarkSessionRunning) {
          setChatRunningSessionFlags(prev => addSessionFlag(prev, runtimeKey));
          setChatCompletedUnopenedFlags(prev => removeSessionFlag(prev, runtimeKey));
        }
        if (shouldRefreshCompletedPrompt) {
          setChatRunningSessionFlags(prev => removeSessionFlag(prev, runtimeKey));
          if (isSelectedSession) {
            setChatCompletedUnopenedFlags(prev => removeSessionFlag(prev, runtimeKey));
          } else {
            setChatCompletedUnopenedFlags(prev => addSessionFlag(prev, runtimeKey));
          }
        }
        const messageText = msgText(message.method, message.param);
        const completedTurnIndex = Math.max(0, Math.trunc(message.turnIndex ?? 0));
        const sessionStatePatch: Partial<RegistryChatSession> =
          shouldRefreshCompletedPrompt
            ? {
                running: false,
                lastDoneTurnIndex: completedTurnIndex,
                lastDoneSuccess: messageText.trim() !== 'failed',
                lastReadTurnIndex: isSelectedSession && completedTurnIndex > 0
                  ? completedTurnIndex
                  : undefined,
              }
            : shouldMarkSessionRunning
              ? {running: true}
              : {};
        const existingSession = knownProjectSessions.find(item => item.sessionId === sessionId);
        const sessionPatchForIndex = {
          sessionId,
          preview: messageText || existingSession?.preview || '',
          updatedAt: existingSession?.updatedAt || '',
          messageCount: existingSession?.messageCount ?? 0,
          unreadCount: existingSession?.unreadCount,
          agentType: existingSession?.agentType,
          ...sessionStatePatch,
        };
        const mergedSessionForCache = mergeChatSession(
          knownProjectSessions,
          sessionPatchForIndex,
        ).find(item => item.sessionId === sessionId);
        setProjectSessionsByProjectId(prev =>
          mergeProjectSessionMap(prev, eventProjectId, sessionPatchForIndex),
        );
        if (eventProjectId === projectIdRef.current) {
          setChatSessions(prev => mergeChatSession(prev, mergedSessionForCache ?? sessionPatchForIndex));
        }
        const readCursorForGap = shouldRequestSessionReadForIncomingTurn(
          {
            cursor: {
              turnIndex: chatSyncSubIndexRef.current[runtimeKey] ?? 0,
            },
            messages: existingMessagesForSession,
          },
          message,
        );
        if (readCursorForGap) {
          chatSyncIndexRef.current[runtimeKey] = 0;
          chatSyncSubIndexRef.current[runtimeKey] = readCursorForGap.turnIndex;
          if (isSelectedSession) {
            loadChatSession(sessionId, eventProjectId, {
              incremental: true,
              preserveUserSelection: true,
              selectionSnapshot: runtimeKey,
            }).catch(() => undefined);
          }
          return;
        }

        maybeNotifyChatMessage(message, mergedSessionForCache, eventProjectId);

        const merged = upsertChatMessage(
          existingMessagesForSession,
          message,
        );
        const latestSyncCursor = getLatestSessionReadCursor(merged);
        chatSyncIndexRef.current[runtimeKey] = 0;
        chatSyncSubIndexRef.current[runtimeKey] = latestSyncCursor.turnIndex;
        chatMessageStoreRef.current[runtimeKey] = merged;
        persistChatSessionContent(sessionId, eventProjectId, mergedSessionForCache);

        if (isSelectedSession) {
          setVisibleChatMessagesForRuntimeKey(runtimeKey, merged, {
            followLatest: chatAutoScrollFollowRef.current,
          });
        }
        if (message.method === 'prompt_request') {
          forgetPendingChatPrompt(runtimeKey);
        }
        if (shouldRefreshCompletedPrompt && isSelectedSession) {
          markChatSessionRead(
            eventProjectId,
            sessionId,
            message.turnIndex ?? 0,
          ).catch(() => undefined);
        }
        if (
          shouldRefreshCompletedPrompt &&
          isSelectedSession &&
          needsPromptTurnRefresh(merged, message)
        ) {
          refreshSessionTurns(
            sessionId,
            eventProjectId,
            runtimeKey,
          ).catch(() => undefined);
        }
      }
    });
    const unsubscribeClose = service.onClose(() => {
      setConnected(false);
      if (supervisorManagedCloseRef.current) {
        supervisorManagedCloseRef.current = false;
        return;
      }
      const canSilentReconnect =
        !!addressRef.current.trim() && !!projectIdRef.current;
      if (!canSilentReconnect) {
        reconnectStartedAtRef.current = null;
        setReconnecting(false);
        setError(
          'Registry connection closed. Reconnect to resume live updates.',
        );
        return;
      }
      if (reconnectStartedAtRef.current === null) {
        reconnectStartedAtRef.current = Date.now();
      }
      setError('');
      setReconnecting(true);
      scheduleReconnectAttempt();
    });

    return () => {
      unsubscribeEvent();
      unsubscribeClose();
    };
  }, []);

  useEffect(() => {
    if (!connected || !projectId || reconnecting) {
      return;
    }
    const timer = window.setInterval(() => {
      refreshProject({silent: true}).catch(() => undefined);
    }, 15000);
    return () => {
      window.clearInterval(timer);
    };
  }, [connected, projectId, reconnecting]);
  const renderFileTree = (path: string, depth: number): React.ReactNode => {
    const entries = dirEntries[path] ?? [];
    return entries.map(entry => {
      if (entry.kind === 'dir') {
        const expanded = isExpanded(entry.path);
        return (
          <div key={entry.path}>
            <div
              className="item"
              style={{ paddingLeft: 10 + depth * 14 }}
              onClick={() => {
                toggleDirectory(entry.path);
              }}
            >
              <span
                className={`caret codicon ${
                  expanded ? 'codicon-chevron-down' : 'codicon-chevron-right'
                }`}
              />
              <span
                className={`node-icon codicon ${
                  expanded ? 'codicon-folder-opened' : 'codicon-folder'
                }`}
              />
              <span className="label">{entry.name}</span>
              {loadingDirs[entry.path] ? (
                <span className="muted">...</span>
              ) : null}
            </div>
            {expanded ? renderFileTree(entry.path, depth + 1) : null}
          </div>
        );
      }

      const fileIcon = resolveFileIcon(entry.name);
      return (
        <div
          key={entry.path}
          className={`item ${selectedFile === entry.path ? 'selected' : ''}`}
          style={{ paddingLeft: 10 + depth * 14 }}
          onClick={() => {
            setSelectedFile(entry.path);
            if (!isWide) setDrawerOpen(false);
          }}
        >
          <span className="caret placeholder" aria-hidden="true" />
          <span
            className="node-icon seti-icon"
            style={{ color: fileIcon.color }}
          >
            <span className="seti-glyph">{fileIcon.glyph}</span>
          </span>
          <span className="label">{entry.name}</span>
        </div>
      );
    });
  };

  const renderSidebarMain = (showSectionTitle = true) => {
    if (tab === 'file') {
      return (
        <>
          {showSectionTitle ? <div className="section-title">EXPLORER</div> : null}
          <div className="list">{renderFileTree('.', 0)}</div>
        </>
      );
    }
    if (tab !== 'git') {
      return null;
    }

    const popoverFiles = commitPopover
      ? commitFilesBySha[commitPopover.commit.sha] ?? []
      : [];
    const popoverFileCount = popoverFiles.length;
    const popoverAdditions = popoverFiles.reduce(
      (sum, item) => sum + (item.additions || 0),
      0,
    );
    const popoverDeletions = popoverFiles.reduce(
      (sum, item) => sum + (item.deletions || 0),
      0,
    );
    const graphItemsCount =
      commits.length + (workingTreeFiles.length > 0 ? 1 : 0);
    const headCommitSha = commits[0]?.sha ?? '';
    const branchOptions =
      gitBranches.length > 0
        ? gitBranches
        : gitCurrentBranch
        ? [gitCurrentBranch]
        : [];
    const branchFilterLabel =
      gitSelectedBranches.length <= 1
        ? gitSelectedBranches[0] ?? gitCurrentBranch ?? 'branch'
        : `${gitSelectedBranches.length} branches`;
    return (
      <>
        <div className="section-title git-section-title">
          <span className="git-section-main">GRAPH</span>
          <span className="git-section-meta">{`${graphItemsCount} items`}</span>
          <div className="git-section-actions">
            <div className="git-branch-picker" ref={gitBranchMenuRef}>
              <button
                type="button"
                className={`git-section-btn git-branch-picker-btn ${
                  gitBranchPickerOpen ? 'open' : ''
                }`}
                onClick={() => setGitBranchPickerOpen(prev => !prev)}
                title="Select branches to display"
              >
                <span className="codicon codicon-git-branch" />
                <span className="git-branch-picker-btn-text">{branchFilterLabel}</span>
                <span className="codicon codicon-chevron-down" />
              </button>
              {gitBranchPickerOpen && isWide ? (
                <div className="git-branch-picker-menu">
                  {branchOptions.length === 0 ? (
                    <div className="git-branch-picker-empty">No branches</div>
                  ) : (
                    branchOptions.map(branch => {
                      const selected = gitSelectedBranches.includes(branch);
                      return (
                        <button
                          key={branch}
                          type="button"
                          className={`git-branch-picker-item ${
                            selected ? 'selected' : ''
                          }`}
                          onClick={() => toggleGitBranchSelection(branch)}
                        >
                          <span className="git-branch-picker-check" aria-hidden="true">
                            {selected ? '✓' : ''}
                          </span>
                          <span className="git-branch-picker-name">{branch}</span>
                          {branch === gitCurrentBranch ? (
                            <span className="git-branch-picker-current">current</span>
                          ) : null}
                        </button>
                      );
                    })
                  )}
                </div>
              ) : null}
            </div>
            <button
              type="button"
              className="git-section-btn"
              onClick={() => {
                setCommitPopover(null);
                loadGit().catch(() => undefined);
              }}
              title="Refresh git view"
            >
              <span className="codicon codicon-refresh" />
            </button>
          </div>
        </div>
        <div className="list half">
          {gitLoading ? (
            <div className="muted block">Loading commits...</div>
          ) : null}
          {gitError ? <div className="error block">{gitError}</div> : null}

          {workingTreeFiles.length > 0 ? (
            <>
              <div
                className={`item git-row git-worktree-row ${
                  worktreeActive ? 'selected' : ''
                }`}
                onClick={() => {
                  setSelectedDiffSource('worktree');
                  setExpandedCommitShas([]);
                  setCommitPopover(null);
                  setWorktreeExpanded(prev => !prev);
                  if (workingTreeFiles[0]) {
                    const preferredPath = pickPreferredPath(workingTreeFiles);
                    const preferredFile =
                      workingTreeFiles.find(
                        item => item.path === preferredPath,
                      ) ?? workingTreeFiles[0];
                    setSelectedDiff(preferredFile.path);
                    setSelectedDiffScope(preferredFile.scope);
                  }
                }}
              >
                <span className="git-graph-lane" aria-hidden="true">
                  <span className="git-graph-line" />
                  <span className={`git-graph-dot ${worktreeActive ? 'active' : ''}`} />
                </span>
                <span className="git-row-spacer" aria-hidden="true" />
                <span className="label git-commit-label">
                  <span className="git-commit-title">Working Tree</span>
                  <span className="git-commit-sha">
                    {`${workingTreeFiles.length} files`}
                  </span>
                </span>
              </div>
              {worktreeExpanded
                ? workingTreeFiles.map(file => {
                    const { fileName, parentPath } = splitPathForDisplay(file.path);
                    return (
                      <div
                        key={`${WORKING_TREE_COMMIT_ID}:${file.scope}:${file.path}`}
                        className={`item git-row git-file-row git-tree-child ${
                          selectedDiff === file.path &&
                          selectedDiffScope === file.scope &&
                          selectedDiffSource === 'worktree'
                            ? 'selected'
                            : ''
                        }`}
                        onClick={() => {
                          setSelectedDiff(file.path);
                          setSelectedDiffSource('worktree');
                          setSelectedDiffScope(file.scope);
                          if (!isWide) setDrawerOpen(false);
                        }}
                      >
                        <span className="git-graph-lane child" aria-hidden="true">
                          <span className="git-graph-line" />
                        </span>
                        <span className="git-row-spacer" aria-hidden="true" />
                        <span className={`status-tag status-git-${file.status}`}>
                          {file.status}
                        </span>
                        <span className="muted git-file-scope">{file.scope}</span>
                        <span className="label git-file-label">
                          <span className="git-file-name">{fileName || file.path}</span>
                          {parentPath ? (
                            <span className="git-file-path">{parentPath}</span>
                          ) : null}
                        </span>
                      </div>
                    );
                  })
                : null}
            </>
          ) : !gitLoading ? (
            <div className="muted block">No local changes</div>
          ) : null}

          {commits.map(commit => {
            const expanded = expandedCommitShas.includes(commit.sha);
            const selected = !worktreeActive && selectedCommit === commit.sha;
            const filesLoaded = Object.prototype.hasOwnProperty.call(
              commitFilesBySha,
              commit.sha,
            );
            const files = commitFilesBySha[commit.sha] ?? [];
            const showBranchTags =
              headCommitSha !== '' && commit.sha === headCommitSha;
            const inlineBranchTags = showBranchTags
              ? gitSelectedBranches.slice(0, 2)
              : [];
            return (
              <React.Fragment key={commit.sha}>
                <div
                  className={`item git-row git-commit-row ${
                    selected ? 'selected' : ''
                  }`}
                  onClick={event => {
                    const nextExpanded = !expanded;
                    const currentFiles = commitFilesBySha[commit.sha] ?? [];
                    setSelectedCommit(commit.sha);
                    setSelectedDiffSource('commit');
                    setSelectedDiffScope('unstaged');
                    setWorktreeExpanded(false);
                    setExpandedCommitShas(nextExpanded ? [commit.sha] : []);
                    if (nextExpanded) {
                      if (currentFiles[0]) {
                        setSelectedDiff(pickPreferredPath(currentFiles));
                      } else {
                        setSelectedDiff('');
                      }
                    }

                    const rect = (
                      event.currentTarget as HTMLDivElement
                    ).getBoundingClientRect();
                    const popoverHeight = 250;
                    const safePadding = 8;
                    let popoverWidth = Math.min(
                      460,
                      Math.max(320, Math.round(window.innerWidth * 0.42)),
                    );
                    let x = safePadding;
                    let y = safePadding;

                    if (isWide) {
                      const maxX = window.innerWidth - popoverWidth - safePadding;
                      const rightCandidate = rect.right + 12;
                      const leftCandidate = rect.left - popoverWidth - 12;
                      x = rightCandidate;
                      if (x > maxX) {
                        x = leftCandidate >= safePadding ? leftCandidate : maxX;
                      }
                      x = Math.max(safePadding, Math.min(maxX, x));
                      y = Math.max(
                        52,
                        Math.min(
                          window.innerHeight - popoverHeight - safePadding,
                          rect.top - 8,
                        ),
                      );
                    } else {
                      popoverWidth = Math.max(
                        260,
                        Math.min(
                          window.innerWidth - safePadding * 2,
                          Math.round(window.innerWidth * 0.92),
                        ),
                      );
                      x = Math.max(
                        safePadding,
                        Math.round((window.innerWidth - popoverWidth) / 2),
                      );
                      const clickMidY = rect.top + rect.height / 2;
                      const listPanel = (
                        event.currentTarget as HTMLDivElement
                      ).closest('.list');
                      const panelRect =
                        listPanel instanceof HTMLElement
                          ? listPanel.getBoundingClientRect()
                          : null;
                      const panelMidY = panelRect
                        ? panelRect.top + panelRect.height / 2
                        : window.innerHeight / 2;
                      const preferBelow = clickMidY <= panelMidY;
                      const topZoneY = panelRect
                        ? Math.max(52, Math.round(panelRect.top + 8))
                        : 52;
                      const bottomZoneY = panelRect
                        ? Math.min(
                            window.innerHeight - popoverHeight - safePadding,
                            Math.round(panelRect.bottom - popoverHeight - 8),
                          )
                        : window.innerHeight - popoverHeight - safePadding;
                      if (bottomZoneY <= topZoneY) {
                        y = Math.max(
                          52,
                          Math.min(
                            window.innerHeight - popoverHeight - safePadding,
                            topZoneY,
                          ),
                        );
                      } else {
                        y = preferBelow ? bottomZoneY : topZoneY;
                      }
                    }

                    setCommitPopover({ commit, x, y, width: popoverWidth });
                  }}
                >
                  <span className="git-graph-lane" aria-hidden="true">
                    <span className="git-graph-line" />
                    <span className={`git-graph-dot ${selected ? 'active' : ''}`} />
                  </span>
                  <span className="git-row-spacer" aria-hidden="true" />
                  <span className="label git-commit-label">
                    <span className="git-commit-title">
                      {commit.title || commit.sha.slice(0, 7)}
                    </span>
                    <span className="git-commit-meta">
                      {formatRelativeTime(commit.time)}
                    </span>
                    {inlineBranchTags.length > 0 ? (
                      <span className="git-commit-tags">
                        {inlineBranchTags.map(branch => (
                          <span key={`${commit.sha}:${branch}`} className="git-commit-tag">
                            {branch}
                          </span>
                        ))}
                        {gitSelectedBranches.length > inlineBranchTags.length ? (
                          <span className="git-commit-tag git-commit-tag-muted">
                            +{gitSelectedBranches.length - inlineBranchTags.length}
                          </span>
                        ) : null}
                      </span>
                    ) : null}
                  </span>
                </div>
                {expanded ? (
                  filesLoaded ? (
                    files.length > 0 ? (
                      files.map(file => {
                        const { fileName, parentPath } = splitPathForDisplay(file.path);
                        return (
                          <div
                            key={`${commit.sha}:${file.path}`}
                            className={`item git-row git-file-row git-tree-child ${
                              selectedDiffSource === 'commit' &&
                              selectedCommit === commit.sha &&
                              selectedDiff === file.path
                                ? 'selected'
                                : ''
                            }`}
                            onClick={() => {
                              setSelectedCommit(commit.sha);
                              setSelectedDiff(file.path);
                              setSelectedDiffSource('commit');
                              setSelectedDiffScope('unstaged');
                              if (!isWide) setDrawerOpen(false);
                            }}
                          >
                            <span className="git-graph-lane child" aria-hidden="true">
                              <span className="git-graph-line" />
                            </span>
                            <span className="git-row-spacer" aria-hidden="true" />
                            <span className={`status-tag status-git-${file.status}`}>
                              {file.status}
                            </span>
                            <span className="label git-file-label">
                              <span className="git-file-name">{fileName || file.path}</span>
                              {parentPath ? (
                                <span className="git-file-path">{parentPath}</span>
                              ) : null}
                            </span>
                          </div>
                        );
                      })
                    ) : (
                      <div className="item git-row git-file-row git-tree-child muted">
                        <span className="git-graph-lane child" aria-hidden="true">
                          <span className="git-graph-line" />
                        </span>
                        <span className="git-row-spacer" aria-hidden="true" />
                        <span className="label">No changed files</span>
                      </div>
                    )
                  ) : (
                    <div className="item git-row git-file-row git-tree-child muted">
                      <span className="git-graph-lane child" aria-hidden="true">
                        <span className="git-graph-line" />
                      </span>
                      <span className="git-row-spacer" aria-hidden="true" />
                      <span className="label">Loading files...</span>
                    </div>
                  )
                ) : null}
              </React.Fragment>
            );
          })}

          {commits.length === 0 && !gitLoading ? (
            <div className="muted block">No commits found</div>
          ) : null}
        </div>

        {commitPopover ? (
          <div
            ref={commitPopoverRef}
            className="git-commit-popover"
            style={{
              left: `${commitPopover.x}px`,
              top: `${commitPopover.y}px`,
              width: `${commitPopover.width}px`,
            }}
          >
            <div className="git-commit-popover-header">
              <div className="git-commit-popover-meta">
                <span className="git-commit-popover-avatar">
                  {(commitPopover.commit.author || 'U').slice(0, 1).toLowerCase()}
                </span>
                <span className="git-commit-popover-meta-line">
                  {commitPopover.commit.author || 'Unknown'}, {' '}
                  {formatRelativeTime(commitPopover.commit.time)}
                  {' '}({formatGitCommitDateTime(commitPopover.commit.time)})
                </span>
              </div>
              <button
                type="button"
                className="git-commit-popover-close"
                onClick={() => setCommitPopover(null)}
                aria-label="Close commit details"
              >
                <span className="codicon codicon-close" />
              </button>
            </div>
            <div className="git-commit-popover-body">
              <div className="git-commit-popover-title-text">
                {commitPopover.commit.title || '(no title)'}
              </div>
              <div className="git-commit-popover-stats">
                <span>{`${popoverFileCount} files changed,`}</span>
                <span className="insertions">{`${popoverAdditions} insertions(+)`}</span>
                <span className="deletions">{`${popoverDeletions} deletions(-)`}</span>
              </div>
              <div className="git-commit-popover-branches">
                {gitCurrentBranch ? (
                  <span className="git-branch-pill local">{gitCurrentBranch}</span>
                ) : null}
                {gitCurrentBranch ? (
                  <span className="git-branch-pill remote">
                    {`origin/${gitCurrentBranch}`}
                  </span>
                ) : null}
              </div>
              <div className="git-commit-popover-sha">
                <span className="codicon codicon-git-commit" />
                <code>{commitPopover.commit.sha}</code>
              </div>
            </div>
          </div>
        ) : null}

        {gitBranchPickerOpen && !isWide ? (
          <div className="git-branch-sheet-backdrop" onClick={() => setGitBranchPickerOpen(false)}>
            <div
              className="git-branch-sheet"
              onClick={event => event.stopPropagation()}
            >
              <div className="git-branch-sheet-header">
                <span>Select Branches</span>
                <button
                  type="button"
                  className="git-section-btn"
                  onClick={() => setGitBranchPickerOpen(false)}
                  aria-label="Close branch selector"
                >
                  <span className="codicon codicon-close" />
                </button>
              </div>
              <div className="git-branch-sheet-body">
                {branchOptions.length === 0 ? (
                  <div className="git-branch-picker-empty">No branches</div>
                ) : (
                  branchOptions.map(branch => {
                    const selected = gitSelectedBranches.includes(branch);
                    return (
                      <button
                        key={`sheet:${branch}`}
                        type="button"
                        className={`git-branch-picker-item ${
                          selected ? 'selected' : ''
                        }`}
                        onClick={() => toggleGitBranchSelection(branch)}
                      >
                        <span className="git-branch-picker-check" aria-hidden="true">
                          {selected ? '✓' : ''}
                        </span>
                        <span className="git-branch-picker-name">{branch}</span>
                        {branch === gitCurrentBranch ? (
                          <span className="git-branch-picker-current">current</span>
                        ) : null}
                      </button>
                    );
                  })
                )}
              </div>
            </div>
          </div>
        ) : null}
      </>
    );
  };

  const renderTokenStatsSettingsDetail = () => (
    <div className="settings-detail-page token-stats-page">
      <div className="settings-detail-header">
        <button
          type="button"
          className="mobile-settings-back settings-detail-back"
          onClick={() => setSettingsDetailView(null)}
          aria-label="Back to settings"
          title="Back"
        >
          <span className="codicon codicon-arrow-left" />
        </button>
        <div className="settings-detail-title">Token Stats</div>
        <button
          type="button"
          className="token-stats-refresh-btn token-stats-refresh-inline"
          onClick={() => {
            refreshTokenStats().catch(() => undefined);
          }}
          disabled={tokenStatsLoading}
        >
          {tokenStatsLoading ? 'Refreshing...' : 'Refresh'}
        </button>
      </div>
      {tokenStatsUpdatedAt ? (
        <div className="muted block">Updated: {tokenStatsUpdatedAt}</div>
      ) : null}
      {tokenStatsLoading ? (
        <div className="muted block">Scanning online hubs...</div>
      ) : null}
      {tokenStatsError ? (
        <div className="muted block token-stats-error">{tokenStatsError}</div>
      ) : null}
      {!tokenStatsLoading && tokenStatCards.length === 0 && !tokenStatsError ? (
        <div className="muted block">No token accounts discovered.</div>
      ) : null}
      <div className="token-stats-account-list token-stats-account-list-flat">
        {tokenStatCards.map(card => (
          <div key={card.id} className="token-stats-account-item token-stats-account-item-flat">
            <div className="token-stats-card-line token-stats-card-line-tags">
              <span className={`token-stats-pill ${tokenTagVariantClass('agent', card.agentTag)}`}>
                {card.agentTag}
              </span>
              {card.hubTags.map(hubTag => (
                <span key={hubTag} className={`token-stats-pill ${tokenTagVariantClass('hub', hubTag)}`}>
                  {hubTag}
                </span>
              ))}
            </div>
            <div className="token-stats-card-line token-stats-card-line-primary">
              <span className="token-stats-account-name">{card.accountName}</span>
            </div>
            {card.message ? (
              <div className="token-stats-account-error">{card.message}</div>
            ) : null}
            {card.secondaryLine ? (
              <div className="token-stats-card-line">{card.secondaryLine}</div>
            ) : null}
            {card.tertiaryLine ? (
              <div className="token-stats-card-line">{card.tertiaryLine}</div>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  );

  const renderSettingsContent = (showSectionTitle: boolean) => {
    if (settingsDetailView === 'tokenStats') {
      return renderTokenStatsSettingsDetail();
    }
    return (
    <>
      {showSectionTitle ? <div className="section-title">SETTINGS</div> : null}
      <div className="list settings-list">
        <label className="switch-row sidebar-setting-row">
          <span>Dark Mode</span>
          <input
            type="checkbox"
            checked={themeMode === 'dark'}
            onChange={e =>
              setThemeMode(e.target.checked ? 'dark' : 'light')
            }
          />
        </label>
        <label className="switch-row sidebar-setting-row">
          <span>Hide Tool Calls</span>
          <input
            type="checkbox"
            checked={hideToolCalls}
            onChange={e => setHideToolCalls(e.target.checked)}
          />
        </label>
        <label className="switch-row sidebar-setting-row">
          <span>Code Theme</span>
          <select
            className="sidebar-setting-select"
            value={codeTheme}
            onChange={event => {
              const next = event.target.value;
              if (isCodeThemeId(next)) setCodeTheme(next);
            }}
          >
            <option
              key={CODE_THEME_OPTIONS[0].id}
              value={CODE_THEME_OPTIONS[0].id}
            >
              {CODE_THEME_OPTIONS[0].label}
            </option>
            {CODE_THEME_OPTION_GROUPS.map(group => (
              <optgroup key={group.label} label={group.label}>
                {group.options.map(item => (
                  <option key={item.id} value={item.id}>
                    {item.label}
                  </option>
                ))}
              </optgroup>
            ))}
          </select>
        </label>
        <label className="switch-row sidebar-setting-row">
          <span>Code Font</span>
          <select
            className="sidebar-setting-select"
            value={codeFont}
            onChange={event => {
              const next = event.target.value;
              if (isCodeFontId(next)) setCodeFont(next);
            }}
          >
            {CODE_FONT_OPTIONS.map(item => (
              <option key={item.id} value={item.id}>
                {item.label}
              </option>
            ))}
          </select>
        </label>
        <label className="switch-row sidebar-setting-row">
          <span>Font Size</span>
          <select
            className="sidebar-setting-select"
            value={String(codeFontSize)}
            onChange={event => {
              setCodeFontSize(
                clampCodeFontSize(Number(event.target.value)),
              );
            }}
          >
            {CODE_FONT_SIZE_OPTIONS.map(size => (
              <option key={size} value={size}>
                {size}px
              </option>
            ))}
          </select>
        </label>
        <label className="switch-row sidebar-setting-row">
          <span>Line Height</span>
          <select
            className="sidebar-setting-select"
            value={String(codeLineHeight)}
            onChange={event => {
              setCodeLineHeight(
                clampCodeLineHeight(Number(event.target.value)),
              );
            }}
          >
            {CODE_LINE_HEIGHT_OPTIONS.map(v => (
              <option key={v} value={v}>
                {v}
              </option>
            ))}
          </select>
        </label>
        <label className="switch-row sidebar-setting-row">
          <span>Tab Size</span>
          <select
            className="sidebar-setting-select"
            value={String(codeTabSize)}
            onChange={event => {
              setCodeTabSize(
                clampCodeTabSize(Number(event.target.value)),
              );
            }}
          >
            {CODE_TAB_SIZE_OPTIONS.map(v => (
              <option key={v} value={v}>
                {v}
              </option>
            ))}
          </select>
        </label>

        <button
          type="button"
          className="settings-detail-row"
          onClick={() => {
            setTokenStatsError('');
            setSettingsDetailView('tokenStats');
          }}
        >
          <span>Token Stats</span>
          <span className="codicon codicon-chevron-right" />
        </button>
        <button
          type="button"
          className="sidebar-clear-cache-btn"
          onClick={openDatabasePanel}
        >
          Database
        </button>
        <button
          type="button"
          className="sidebar-clear-cache-btn"
          onClick={clearLocalCache}
        >
          Clear Local Cache (Keep Token)
        </button>
        {databasePanelOpen ? (
          <div className="database-panel">
            <div className="database-panel-header">
              <strong>Local Database</strong>
              <div className="database-panel-actions">
                <button
                  type="button"
                  className="git-section-btn"
                  onClick={exportDatabaseDump}
                  disabled={databaseLoading || !!databaseError || !databaseDumpText}
                  title="Export current database dump"
                >
                  Export
                </button>
                <button
                  type="button"
                  className="git-section-btn"
                  onClick={() => {
                    setDatabasePanelOpen(false);
                    setDatabaseError('');
                  }}
                  aria-label="Close database panel"
                >
                  <span className="codicon codicon-close" />
                </button>
              </div>
            </div>
            {databaseLoading ? (
              <div className="muted block">Loading database...</div>
            ) : null}
            {databaseError ? (
              <div className="error">Database error: {databaseError}</div>
            ) : null}
            {!databaseLoading && !databaseError ? (
              <pre className="database-dump">{databaseDumpText}</pre>
            ) : null}
          </div>
        ) : null}
      </div>
    </>
    );
  };

  const renderMobileChatSessionSheet = () => {
    return (
      <>
        <div className="mobile-chat-drawer-header">
          <button
            type="button"
            className="drawer-settings-icon-btn"
            onClick={() => {
              setProjectMenuOpen(false);
              setSettingsDetailView(null);
              setSidebarSettingsOpen(true);
            }}
            title="Open settings"
            aria-label="Open settings"
          >
            <span className="codicon codicon-settings-gear" />
          </button>
          <span className="mobile-chat-drawer-title">Chats</span>
          <button
            className={`header-btn refresh-btn drawer-project-refresh${hasPendingProjectUpdates && !mobileProjectSessionsRefreshing && !reconnecting ? ' has-update-badge' : ''}`}
            onClick={() => refreshMobileChatProjectSessions().catch(() => undefined)}
            title={reconnecting ? 'Reconnecting...' : 'Refresh chats'}
            disabled={mobileProjectSessionsRefreshing || reconnecting}
          >
            {mobileProjectSessionsRefreshing ? '...' : refreshButtonContent}
          </button>
        </div>
        <div className="mobile-project-session-nav">
          {projects.length === 0 ? (
            <div className="chat-empty-hint">No projects available.</div>
          ) : null}
          {sortedProjectItems.map(projectItem => {
            const targetProjectId = projectItem.projectId;
            const projectSessions = projectSessionsByProjectId[targetProjectId] ?? [];
            const visibleCount =
              wideProjectVisibleCounts[targetProjectId] ?? WIDE_PROJECT_SESSION_LIMIT;
            const visibleSessions = projectSessions.slice(0, visibleCount);
            const collapsed = collapsedProjectIds.includes(targetProjectId);
            const pinnedProject = pinnedProjectIds.includes(targetProjectId);
            const agents = getWideProjectAgents(projectItem, projectSessions);
            const actionMenuOpen = mobileProjectActionMenu?.projectId === targetProjectId;
            const activeMobileProjectActionMenu = actionMenuOpen
              ? mobileProjectActionMenu
              : null;
            const projectHub = projectItem.hubId || 'local';
            const projectHubVariant = tagVariantClass('wide-project-hub', projectItem.hubId || 'local');
            const sessionError = mobileProjectSessionErrors[targetProjectId] ?? '';
            return (
              <div
                key={`mobile-project:${targetProjectId}`}
                  className={`wide-project-section mobile-project-section${targetProjectId === projectId ? ' active' : ''}${pinnedProject ? ' pinned' : ''}${
                  collapsed ? ' collapsed' : ''
                }`}
              >
                <div className="wide-project-row mobile-project-row">
                  <button
                    type="button"
                    className="wide-project-toggle mobile-project-toggle"
                    onPointerDown={event => startProjectPinLongPress(targetProjectId, event)}
                    onPointerUp={finishProjectPinLongPress}
                    onPointerCancel={finishProjectPinLongPress}
                    onPointerLeave={finishProjectPinLongPress}
                    onContextMenu={event => event.preventDefault()}
                    onClick={event => {
                      if (consumeProjectPinLongPressClick(targetProjectId, event)) {
                        return;
                      }
                      toggleWideProjectCollapsed(targetProjectId);
                    }}
                    title={collapsed ? 'Expand project' : 'Collapse project'}
                    aria-expanded={!collapsed}
                  >
                    <span className="wide-project-folder-wrap">
                      <span
                        className={`codicon ${collapsed ? 'codicon-folder' : 'codicon-folder-opened'} wide-project-folder-icon ${projectHubVariant}`}
                      />
                      {pinnedProject ? (
                        <span className="codicon codicon-pinned wide-project-pin-badge" aria-hidden="true" />
                      ) : null}
                    </span>
                    <span className="wide-project-title-group">
                      <span className="wide-project-name" title={projectItem.name}>
                        {projectItem.name}
                      </span>
                      <span
                        className={`wide-project-hub-tag ${projectHubVariant}`}
                      >
                        <span className="wide-project-hub-dot" aria-hidden="true" />
                        <span className="wide-project-hub-label">{projectHub}</span>
                      </span>
                    </span>
                  </button>
                  <div className="wide-project-actions mobile-project-actions">
                    <button
                      type="button"
                      className="wide-project-action-btn"
                      title="New session"
                      aria-label={`New session in ${projectItem.name}`}
                      onPointerDown={event => event.stopPropagation()}
                      onClick={event => {
                        event.stopPropagation();
                        openMobileProjectActionMenu(targetProjectId, 'new');
                      }}
                    >
                      <span className="codicon codicon-add" />
                    </button>
                    <button
                      type="button"
                      className="wide-project-action-btn"
                      title="Resume session"
                      aria-label={`Resume session in ${projectItem.name}`}
                      onPointerDown={event => event.stopPropagation()}
                      onClick={event => {
                        event.stopPropagation();
                        openMobileProjectActionMenu(targetProjectId, 'resume');
                      }}
                    >
                      <span className="codicon codicon-history" />
                    </button>
                  </div>
                </div>
                {activeMobileProjectActionMenu ? (
                  <div className="mobile-project-action-panel">
                    <div className="wide-project-action-title">
                      <span
                        className={`codicon ${
                          activeMobileProjectActionMenu.kind === 'new'
                            ? 'codicon-add'
                            : 'codicon-history'
                        }`}
                      />
                      <span className="wide-project-action-title-copy">
                        <span className="wide-project-action-title-main">
                          {activeMobileProjectActionMenu.kind === 'new' ? 'New Session' : 'Resume Session'}
                        </span>
                        <span className="wide-project-action-title-sub">
                          {projectItem.name}
                        </span>
                      </span>
                    </div>
                    {activeMobileProjectActionMenu.phase === 'agents' ? (
                      <>
                        {agents.map(agentType => (
                          <button
                            key={`${targetProjectId}:mobile:${activeMobileProjectActionMenu.kind}:${agentType}`}
                            type="button"
                            className="wide-project-action-menu-item"
                            onClick={() => {
                              if (activeMobileProjectActionMenu.kind === 'new') {
                                handleMobileProjectCreateSession(
                                  targetProjectId,
                                  agentType,
                                ).catch(() => undefined);
                              } else {
                                handleMobileProjectResumeAgent(
                                  targetProjectId,
                                  agentType,
                                ).catch(() => undefined);
                              }
                            }}
                          >
                            <span className="codicon codicon-sparkle" />
                            <span>{agentType}</span>
                          </button>
                        ))}
                        {agents.length === 0 ? (
                          <div className="wide-project-action-empty">
                            No agents available.
                          </div>
                        ) : null}
                      </>
                    ) : (
                      <>
                        <button
                          type="button"
                          className="wide-project-action-back"
                          onClick={() => {
                            setResumeSessions([]);
                            setResumeLoading(false);
                            setMobileProjectActionMenu({
                              ...activeMobileProjectActionMenu,
                              phase: 'agents',
                              agentType: '',
                            });
                          }}
                        >
                          <span className="codicon codicon-arrow-left" />
                          <span>{activeMobileProjectActionMenu.agentType}</span>
                        </button>
                        {resumeLoading ? (
                          <div className="wide-project-action-empty">
                            Loading sessions...
                          </div>
                        ) : null}
                        {!resumeLoading
                          ? resumeSessions.map(session => (
                              <button
                                key={`${targetProjectId}:mobile-resume:${session.sessionId}`}
                                type="button"
                                className="wide-project-action-menu-item"
                                onClick={() => {
                                  handleMobileProjectResumeImport(
                                    targetProjectId,
                                    activeMobileProjectActionMenu.agentType,
                                    session.sessionId,
                                  ).catch(() => undefined);
                                }}
                              >
                                <span className="codicon codicon-history" />
                                <span>{session.title || session.sessionId}</span>
                              </button>
                            ))
                          : null}
                        {!resumeLoading && resumeSessions.length === 0 ? (
                          <div className="wide-project-action-empty">
                            No resumable sessions.
                          </div>
                        ) : null}
                      </>
                    )}
                  </div>
                ) : null}
                {sessionError ? (
                  <div className="mobile-project-session-error">
                    <span>Session refresh failed.</span>
                    <button
                      type="button"
                      onClick={() => refreshMobileChatProjectSessions().catch(() => undefined)}
                    >
                      Retry
                    </button>
                  </div>
                ) : null}
                {!collapsed ? (
                  <div className="wide-project-session-list mobile-project-session-list">
                    {visibleSessions.map(session => {
                      const sessionAgent = (session.agentType || '').trim();
                      const sessionActionsOpen =
                        projectSessionActionMenu?.projectId === targetProjectId &&
                        projectSessionActionMenu.sessionId === session.sessionId;
                      return (
                        <div
                          key={`${targetProjectId}:mobile-session:${session.sessionId}`}
                          className={`project-session-row-wrap${sessionActionsOpen ? ' actions-open' : ''}`}
                        >
                          <button
                            type="button"
                            className={`wide-session-row mobile-session-row${
                              selectedChatEncodedKey === buildChatRuntimeKey(targetProjectId, session.sessionId)
                                ? ' selected'
                                : ''
                            }`}
                            onPointerDown={event => startProjectSessionLongPress(targetProjectId, session.sessionId, event)}
                            onPointerUp={finishProjectSessionLongPress}
                            onPointerCancel={finishProjectSessionLongPress}
                            onPointerLeave={finishProjectSessionLongPress}
                            onContextMenu={event => event.preventDefault()}
                            onClick={event => {
                              if (consumeProjectSessionLongPressClick(targetProjectId, session.sessionId, event)) {
                                return;
                              }
                              selectProjectChatSession(
                                targetProjectId,
                                session.sessionId,
                                {closeMobileDrawer: true},
                              ).catch(() => undefined);
                            }}
                          >
                            {renderSessionStateMarker(session, targetProjectId)}
                            <span className="wide-session-title">
                              {session.title || session.sessionId}
                            </span>
                            {sessionAgent ? (
                              <span className={`wide-session-agent-tag ${tagVariantClass('wide-session-agent', sessionAgent)}`}>
                                {sessionAgent}
                              </span>
                            ) : null}
                            <span className="wide-session-time" title={session.updatedAt || ''}>
                              {formatCompactRelativeAge(session.updatedAt)}
                            </span>
                          </button>
                          {renderProjectSessionActionStrip(targetProjectId, session.sessionId)}
                        </div>
                      );
                    })}
                    {projectSessions.length > visibleSessions.length ? (
                      <div
                        ref={node => {
                          projectSessionSentinelRefs.current[targetProjectId] = node;
                        }}
                        className="wide-project-session-sentinel"
                        data-project-id={targetProjectId}
                        aria-hidden="true"
                      />
                    ) : null}
                    {projectSessions.length === 0 ? (
                      <div className="wide-project-empty">No sessions yet.</div>
                    ) : null}
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      </>
    );
  };

  const renderWideProjectSessionNav = () => {
    return (
      <div className="wide-project-session-nav">
        {projects.length === 0 ? (
          <div className="chat-empty-hint">No projects available.</div>
        ) : null}
        {sortedProjectItems.map(projectItem => {
          const targetProjectId = projectItem.projectId;
          const projectSessions = projectSessionsByProjectId[targetProjectId] ?? [];
          const visibleCount =
            wideProjectVisibleCounts[targetProjectId] ?? WIDE_PROJECT_SESSION_LIMIT;
          const visibleSessions = projectSessions.slice(0, visibleCount);
          const collapsed = collapsedProjectIds.includes(targetProjectId);
          const pinnedProject = pinnedProjectIds.includes(targetProjectId);
          const agents = getWideProjectAgents(projectItem, projectSessions);
          const actionMenuOpen = wideProjectActionMenu?.projectId === targetProjectId;
          const projectHub = projectItem.hubId || 'local';
          const projectHubVariant = tagVariantClass('wide-project-hub', projectItem.hubId || 'local');
          return (
            <div
              key={`wide-project:${targetProjectId}`}
              className={`wide-project-section${targetProjectId === projectId ? ' active' : ''}${pinnedProject ? ' pinned' : ''}${
                collapsed ? ' collapsed' : ''
              }`}
            >
              <div className="wide-project-row">
                <button
                  type="button"
                  className="wide-project-toggle"
                  onPointerDown={event => startProjectPinLongPress(targetProjectId, event)}
                  onPointerUp={finishProjectPinLongPress}
                  onPointerCancel={finishProjectPinLongPress}
                  onPointerLeave={finishProjectPinLongPress}
                  onContextMenu={event => event.preventDefault()}
                  onClick={event => {
                    if (consumeProjectPinLongPressClick(targetProjectId, event)) {
                      return;
                    }
                    toggleWideProjectCollapsed(targetProjectId);
                  }}
                  title={collapsed ? 'Expand project' : 'Collapse project'}
                  aria-expanded={!collapsed}
                >
                  <span className="wide-project-folder-wrap">
                    <span
                      className={`codicon ${collapsed ? 'codicon-folder' : 'codicon-folder-opened'} wide-project-folder-icon ${projectHubVariant}`}
                    />
                    {pinnedProject ? (
                      <span className="codicon codicon-pinned wide-project-pin-badge" aria-hidden="true" />
                    ) : null}
                  </span>
                  <span className="wide-project-title-group">
                    <span className="wide-project-name" title={projectItem.name}>
                      {projectItem.name}
                    </span>
                    <span
                      className={`wide-project-hub-tag ${projectHubVariant}`}
                    >
                      <span className="wide-project-hub-dot" aria-hidden="true" />
                      <span className="wide-project-hub-label">{projectHub}</span>
                    </span>
                  </span>
                </button>
                <div className="wide-project-actions">
                  <button
                    type="button"
                    className="wide-project-action-btn"
                    title="New session"
                    aria-label={`New session in ${projectItem.name}`}
                    onPointerDown={event => event.stopPropagation()}
                    onClick={event => {
                      event.stopPropagation();
                      openWideProjectActionMenu(targetProjectId, 'new');
                    }}
                  >
                    <span className="codicon codicon-add" />
                  </button>
                  <button
                    type="button"
                    className="wide-project-action-btn"
                    title="Resume session"
                    aria-label={`Resume session in ${projectItem.name}`}
                    onPointerDown={event => event.stopPropagation()}
                    onClick={event => {
                      event.stopPropagation();
                      openWideProjectActionMenu(targetProjectId, 'resume');
                    }}
                  >
                    <span className="codicon codicon-history" />
                  </button>
                </div>
                {actionMenuOpen ? (
                  <div
                    ref={wideProjectActionMenuRef}
                    className="wide-project-action-popover"
                  >
                    <div className="wide-project-action-title">
                      <span
                        className={`codicon ${
                          wideProjectActionMenu.kind === 'new'
                            ? 'codicon-add'
                            : 'codicon-history'
                        }`}
                      />
                      <span className="wide-project-action-title-copy">
                        <span className="wide-project-action-title-main">
                          {wideProjectActionMenu.kind === 'new' ? 'New Session' : 'Resume Session'}
                        </span>
                        <span className="wide-project-action-title-sub">
                          {projectItem.name}
                        </span>
                      </span>
                    </div>
                    {wideProjectActionMenu.phase === 'agents' ? (
                      <>
                        {agents.map(agentType => (
                          <button
                            key={`${targetProjectId}:${wideProjectActionMenu.kind}:${agentType}`}
                            type="button"
                            className="wide-project-action-menu-item"
                            onClick={() => {
                              if (wideProjectActionMenu.kind === 'new') {
                                handleWideProjectCreateSession(
                                  targetProjectId,
                                  agentType,
                                ).catch(() => undefined);
                              } else {
                                handleWideProjectResumeAgent(
                                  targetProjectId,
                                  agentType,
                                ).catch(() => undefined);
                              }
                            }}
                          >
                            <span className="codicon codicon-sparkle" />
                            <span>{agentType}</span>
                          </button>
                        ))}
                        {agents.length === 0 ? (
                          <div className="wide-project-action-empty">
                            No agents available.
                          </div>
                        ) : null}
                      </>
                    ) : (
                      <>
                        <button
                          type="button"
                          className="wide-project-action-back"
                          onClick={() => {
                            setResumeSessions([]);
                            setResumeLoading(false);
                            setWideProjectActionMenu({
                              ...wideProjectActionMenu,
                              phase: 'agents',
                              agentType: '',
                            });
                          }}
                        >
                          <span className="codicon codicon-arrow-left" />
                          <span>{wideProjectActionMenu.agentType}</span>
                        </button>
                        {resumeLoading ? (
                          <div className="wide-project-action-empty">
                            Loading sessions...
                          </div>
                        ) : null}
                        {!resumeLoading
                          ? resumeSessions.map(session => (
                              <button
                                key={`${targetProjectId}:resume:${session.sessionId}`}
                                type="button"
                                className="wide-project-action-menu-item"
                                onClick={() => {
                                  handleWideProjectResumeImport(
                                    targetProjectId,
                                    wideProjectActionMenu.agentType,
                                    session.sessionId,
                                  ).catch(() => undefined);
                                }}
                              >
                                <span className="codicon codicon-history" />
                                <span>{session.title || session.sessionId}</span>
                              </button>
                            ))
                          : null}
                        {!resumeLoading && resumeSessions.length === 0 ? (
                          <div className="wide-project-action-empty">
                            No resumable sessions.
                          </div>
                        ) : null}
                      </>
                    )}
                  </div>
                ) : null}
              </div>
              {!collapsed ? (
                <div className="wide-project-session-list">
                  {visibleSessions.map(session => {
                    const sessionAgent = (session.agentType || '').trim();
                    const sessionActionsOpen =
                      projectSessionActionMenu?.projectId === targetProjectId &&
                      projectSessionActionMenu.sessionId === session.sessionId;
                    return (
                      <div
                        key={`${targetProjectId}:${session.sessionId}`}
                        className={`project-session-row-wrap${sessionActionsOpen ? ' actions-open' : ''}`}
                      >
                        <button
                          type="button"
                          className={`wide-session-row${
                            selectedChatEncodedKey === buildChatRuntimeKey(targetProjectId, session.sessionId)
                              ? ' selected'
                              : ''
                          }`}
                          onPointerDown={event => startProjectSessionLongPress(targetProjectId, session.sessionId, event)}
                          onPointerUp={finishProjectSessionLongPress}
                          onPointerCancel={finishProjectSessionLongPress}
                          onPointerLeave={finishProjectSessionLongPress}
                          onContextMenu={event => event.preventDefault()}
                          onClick={event => {
                            if (consumeProjectSessionLongPressClick(targetProjectId, session.sessionId, event)) {
                              return;
                            }
                            selectWideProjectSession(
                              targetProjectId,
                              session.sessionId,
                            ).catch(() => undefined);
                          }}
                        >
                          {renderSessionStateMarker(session, targetProjectId)}
                          <span className="wide-session-title">
                            {session.title || session.sessionId}
                          </span>
                          {sessionAgent ? (
                            <span className={`wide-session-agent-tag ${tagVariantClass('wide-session-agent', sessionAgent)}`}>
                              {sessionAgent}
                            </span>
                          ) : null}
                          <span className="wide-session-time" title={session.updatedAt || ''}>
                            {formatCompactRelativeAge(session.updatedAt)}
                          </span>
                        </button>
                        {renderProjectSessionActionStrip(targetProjectId, session.sessionId)}
                      </div>
                    );
                  })}
                  {projectSessions.length > visibleSessions.length ? (
                    <div
                      ref={node => {
                        projectSessionSentinelRefs.current[targetProjectId] = node;
                      }}
                      className="wide-project-session-sentinel"
                      data-project-id={targetProjectId}
                      aria-hidden="true"
                    />
                  ) : null}
                  {projectSessions.length === 0 ? (
                    <div className="wide-project-empty">No sessions yet.</div>
                  ) : null}
                </div>
              ) : null}
            </div>
          );
        })}
      </div>
    );
  };

  const renderSidebar = () => {
    const mobileSidebarMain = !isWide
      ? tab === 'chat' && !isWide ? renderMobileChatSessionSheet() : renderSidebarMain()
      : null;
    const wideSidebarTitle = sidebarSettingsOpen
      ? 'SETTINGS'
      : tab === 'chat'
      ? 'CHAT'
      : tab === 'file'
      ? 'EXPLORER'
      : 'SOURCE CONTROL';
    const wideSidebarMain = sidebarSettingsOpen
      ? renderSettingsContent(false)
      : tab === 'chat' ? renderWideProjectSessionNav() : renderSidebarMain(false);

    return (
      <>
        {!isWide && tab !== 'chat' ? (
          <div className="drawer-project-header">
            <button
              type="button"
              className="drawer-settings-icon-btn"
              onClick={() => {
                setProjectMenuOpen(false);
                setSidebarSettingsOpen(true);
              }}
              title="Open settings"
              aria-label="Open settings"
            >
              <span className="codicon codicon-settings-gear" />
            </button>
            <div className="drawer-project-pill">
              <div
                className="project-wrap"
                onPointerDown={event => event.stopPropagation()}
              >
                <button
                  className="project-btn drawer-project-button"
                  onClick={() => setProjectMenuOpen(value => !value)}
                >
                  <span className="project-arrow codicon codicon-chevron-down" />
                  <span className="project-name" title={currentProjectName}>
                    {currentProjectName}
                  </span>
                  {loadingProject || refreshingProject || reconnecting ? (
                    <span className="muted">...</span>
                  ) : null}
                </button>
                {projectMenu}
              </div>
              <button
                className={`header-btn refresh-btn drawer-project-refresh${hasPendingProjectUpdates && !refreshingProject && !reconnecting ? ' has-update-badge' : ''}`}
                onClick={() => refreshProject().catch(() => undefined)}
                title={reconnecting ? 'Reconnecting...' : 'Refresh project'}
                disabled={refreshingProject || reconnecting}
              >
                {refreshButtonContent}
              </button>
            </div>
          </div>
        ) : null}
        {isWide ? (
          <div className="sidebar-title-row">
            <span className="sidebar-title-text">{wideSidebarTitle}</span>
          </div>
        ) : null}
        <div className="sidebar-scroll">
          {isWide ? wideSidebarMain : mobileSidebarMain}
        </div>
        {isWide ? (
          <button
            type="button"
            className={`desktop-sidebar-resize-handle${desktopSidebarResizing ? ' resizing' : ''}`}
            aria-label="Resize sidebar"
            title="Resize sidebar"
            onPointerDown={beginDesktopSidebarResize}
            onPointerMove={moveDesktopSidebarResize}
            onPointerUp={finishDesktopSidebarResize}
            onPointerCancel={finishDesktopSidebarResize}
            onLostPointerCapture={commitDesktopSidebarResize}
            onDoubleClick={resetDesktopSidebarWidth}
          />
        ) : null}
      </>
    );
  };

  const renderCodePane = (
    content: string,
    forceLineNumbers = false,
    languageHint = '',
  ) => {
    const numbersOn = forceLineNumbers || showLineNumbers;
    const language = languageHint || detectCodeLanguage(selectedFile);
    return (
      <ShikiCodeBlock
        content={content}
        language={language}
        wrap={wrapLines}
        lineNumbers={numbersOn}
        themeMode={themeMode}
        codeTheme={codeTheme}
        codeFont={codeFont}
        codeFontSize={codeFontSize}
        codeLineHeight={codeLineHeight}
        codeTabSize={codeTabSize}
      />
    );
  };

  const renderViewTools = () => (
    <>
      {selectedFileIsMarkdown ? (
        <button
          type="button"
          className={`view-tool markdown-preview-toggle ${
            markdownPreviewEnabled ? 'active' : ''
          }`}
          onClick={() => setMarkdownPreviewEnabled(value => !value)}
          title={
            markdownPreviewEnabled
              ? 'Switch to source mode'
              : 'Switch to markdown preview'
          }
          aria-label="Toggle markdown preview"
        >
          <span className="markdown-preview-toggle-text">MD</span>
        </button>
      ) : null}
      <button
        type="button"
        className={`view-tool ${wrapLines ? 'active' : ''}`}
        onClick={() => setWrapLines(value => !value)}
        title="Toggle wrap line"
        aria-label="Toggle wrap line"
      >
        <span className="codicon codicon-word-wrap view-tool-icon" />
      </button>
      <button
        type="button"
        className={`view-tool ${showLineNumbers ? 'active' : ''}`}
        onClick={() => setShowLineNumbers(value => !value)}
        title="Toggle line number"
        aria-label="Toggle line number"
      >
        <span className="codicon codicon-list-ordered view-tool-icon" />
      </button>
    </>
  );

  const renderDiffPane = (content: string) => {
    if (!content) return <div className="muted block">No diff available</div>;
    const shouldDelayLargeRender =
      !allowLargeDiffRender &&
      isHeavyGeneratedDiffPath(selectedDiff || '') &&
      content.length > MAX_AUTO_RENDER_DIFF_CHARS;
    if (shouldDelayLargeRender) {
      return (
        <div className="muted block">
          Large generated diff detected ({(content.length / 1024).toFixed(0)}{' '}
          KB). Click to render when needed.
          <div style={{ marginTop: 10 }}>
            <button
              type="button"
              className="button"
              onClick={() => setAllowLargeDiffRender(true)}
            >
              Render Diff
            </button>
          </div>
        </div>
      );
    }

    const language = detectCodeLanguage(selectedDiff || selectedFile);
    return (
      <ShikiDiffPane
        content={content}
        language={language}
        wrap={wrapLines}
        lineNumbers={showLineNumbers}
        themeMode={themeMode}
        codeTheme={codeTheme}
        codeFont={codeFont}
        codeFontFamily={codeFontFamily}
        codeFontSize={codeFontSize}
        codeLineHeight={codeLineHeight}
        codeTabSize={codeTabSize}
      />
    );
  };

  const resolveChatFileLink = (
    href: string,
  ): { path: string; line: number | null } | null => {
    const rawHref = href.trim();
    if (!rawHref) return null;
    const isWindowsDrivePath = /^\/?[a-zA-Z]:/.test(rawHref);

    const decodePath = (value: string) => {
      try {
        return decodeURIComponent(value);
      } catch {
        return value;
      }
    };

    let pathCandidate = rawHref;
    if (/^\/?[a-zA-Z]:[^\\/]/.test(pathCandidate)) {
      const hasLeadingSlash = pathCandidate.startsWith('/');
      const prefix = hasLeadingSlash
        ? pathCandidate.slice(0, 3)
        : pathCandidate.slice(0, 2);
      const suffix = hasLeadingSlash
        ? pathCandidate.slice(3)
        : pathCandidate.slice(2);
      pathCandidate = `${prefix}/${suffix}`;
    }
    if (/^file:\/\//i.test(rawHref)) {
      try {
        const parsed = new URL(rawHref);
        pathCandidate = `${parsed.hostname || ''}${decodePath(
          parsed.pathname,
        )}`;
      } catch {
        return null;
      }
    } else if (/^vscode:\/\//i.test(rawHref)) {
      try {
        const parsed = new URL(rawHref);
        if (parsed.hostname.toLowerCase() !== 'file') return null;
        pathCandidate = decodePath(parsed.pathname);
      } catch {
        return null;
      }
    } else if (isWindowsDrivePath) {
      pathCandidate = decodePath(rawHref);
    } else if (/^[a-z][a-z0-9+.-]*:/i.test(rawHref)) {
      return null;
    } else {
      pathCandidate = decodePath(rawHref);
    }

    const normalizeSlashes = (value: string) => value.replaceAll('\\', '/');
    let normalized = normalizeSlashes(pathCandidate.trim());
    if (!normalized) return null;

    let line: number | null = null;
    const hashMatch = /#L(\d+)(?:C\d+)?$/i.exec(normalized);
    if (hashMatch) {
      const parsedLine = Number.parseInt(hashMatch[1], 10);
      if (Number.isFinite(parsedLine) && parsedLine > 0) {
        line = parsedLine;
      }
      normalized = normalized.slice(0, hashMatch.index);
    }
    const suffixLineMatch = /:(\d+)(?::\d+)?$/.exec(normalized);
    if (suffixLineMatch) {
      const parsedLine = Number.parseInt(suffixLineMatch[1], 10);
      if (Number.isFinite(parsedLine) && parsedLine > 0) {
        line = parsedLine;
      }
      normalized = normalized.slice(0, suffixLineMatch.index);
    }
    normalized = normalized.trim();
    if (!normalized) return null;
    if (/^(\/\/|[a-z]+:\/\/)/i.test(normalized)) {
      return null;
    }

    const root = normalizeSlashes(currentProject?.path ?? '').replace(
      /\/+$/,
      '',
    );
    const rootLower = root.toLowerCase();
    let candidateLower = normalized.toLowerCase();
    if (root && candidateLower === rootLower) {
      return null;
    }

    if (/^\/[a-z]:\//i.test(normalized)) {
      normalized = normalized.slice(1);
      candidateLower = normalized.toLowerCase();
    }

    let resolvedPath = normalized;
    if (root) {
      const normalizedRootWithSlash = `${rootLower}/`;
      if (candidateLower.startsWith(normalizedRootWithSlash)) {
        resolvedPath = normalized.slice(root.length + 1);
      }
    }

    resolvedPath = resolvedPath
      .replace(/^\.\/+/, '')
      .replace(/^\/+/, '')
      .replace(/\/+/g, '/');
    if (!resolvedPath || resolvedPath.startsWith('../')) {
      return null;
    }

    return { path: resolvedPath, line };
  };
  const chatMarkdownUrlTransform = useCallback((value: string) => {
    const trimmed = value.trim();
    if (!trimmed) return '';
    if (/^(javascript|vbscript):/i.test(trimmed)) {
      return '';
    }
    return value;
  }, []);

  const chatMarkdownComponents = useMemo<Components>(
    () => ({
      pre: markdownPreRenderer,
      code: ({ className, children }) =>
        markdownCodeRenderer({
          className,
          children,
          themeMode,
          codeTheme,
          codeFont,
          codeFontSize,
          codeLineHeight,
          codeTabSize,
          wrap: true,
          lineNumbers: false,
        }),
      a: ({ href, children, ...rest }) => {
        const linkHref = typeof href === 'string' ? href : '';
        const targetFile = linkHref ? resolveChatFileLink(linkHref) : null;
        const isFileLink = !!targetFile;
        const isWindowsLocalPath = /^\/?[a-zA-Z]:/.test(linkHref.trim());
        const linkText = collectReactText(children);
        const textLine = parseTrailingLineNumber(linkText);
        const jumpLine = targetFile?.line ?? textLine;
        const fallbackHref = linkHref || '#';

        return (
          <a
            {...rest}
            href={fallbackHref}
            target={isFileLink ? undefined : '_blank'}
            rel={isFileLink ? undefined : 'noreferrer'}
            title={
              isFileLink && jumpLine
                ? `${targetFile.path}:${jumpLine}`
                : rest.title
            }
            onClick={event => {
              if (!targetFile) {
                if (isWindowsLocalPath) {
                  event.preventDefault();
                  setError(`Invalid file link: ${linkHref}`);
                }
                return;
              }
              event.preventDefault();
              if (jumpLine) {
                setPendingFileJump({ path: targetFile.path, line: jumpLine });
              } else {
                setPendingFileJump(null);
              }
              setTab('file');
              setSelectedFile(targetFile.path);
            }}
          >
            <>
              {children}
              {isFileLink && jumpLine && !textLine ? (
                <span className="chat-file-link-line">:{jumpLine}</span>
              ) : null}
            </>
          </a>
        );
      },
    }),
    [
      themeMode,
      codeTheme,
      codeFont,
      codeFontSize,
      codeLineHeight,
      codeTabSize,
      currentProject?.path,
    ],
  );

  const selectedFullChatMessages =
    selectedChatEncodedKey
      ? chatMessageStoreRef.current[selectedChatEncodedKey] ?? []
      : [];

  const findPromptRequestForDone = (doneTurnIndex: number): RegistryChatMessage | undefined => {
    const ordered = [...selectedFullChatMessages].sort((left, right) => (left.turnIndex ?? 0) - (right.turnIndex ?? 0));
    const doneIndex = ordered.findIndex(message => message.method === 'prompt_done' && (message.turnIndex ?? 0) === doneTurnIndex);
    if (doneIndex < 0) {
      return undefined;
    }
    for (let index = doneIndex - 1; index >= 0; index -= 1) {
      if (ordered[index].method === 'prompt_request') {
        return ordered[index];
      }
    }
    return undefined;
  };

  const copyPromptDoneMarkdown = async (doneTurnIndex: number) => {
    const result = buildPromptDoneCopyRange(selectedFullChatMessages, doneTurnIndex);
    if (!result.ok) {
      return;
    }
    await writeTextToClipboard(result.markdown);
  };

  const renderedChatTurns = chatMessages.map(message => {
    const doneTurnIndex = message.turnIndex ?? 0;
    const copyRange = message.method === 'prompt_done'
      ? buildPromptDoneCopyRange(selectedFullChatMessages, doneTurnIndex)
      : null;
    const promptStatus = isPromptStartMessage(message)
      ? resolvePromptTurnStatus(selectedFullChatMessages, message)
      : null;
    if (!shouldRenderChatTurn(message, hideToolCalls, promptStatus)) {
      return null;
    }
    return (
      <div
        key={`${selectedChatEncodedKey}:${message.turnIndex}:${message.method}`}
        data-chat-message-key={chatMessageDomKey(message)}
      >
        <ChatTurnView
          message={message}
          promptRequest={message.method === 'prompt_done' ? findPromptRequestForDone(doneTurnIndex) : undefined}
          promptStatus={promptStatus}
          hideToolCalls={hideToolCalls}
          markdownComponents={chatMarkdownComponents}
          markdownUrlTransform={chatMarkdownUrlTransform}
          copyDisabled={copyRange ? !copyRange.ok : true}
          onCopyPromptDone={
            message.method === 'prompt_done'
              ? () => copyPromptDoneMarkdown(doneTurnIndex).catch(() => undefined)
              : undefined
          }
        />
      </div>
    );
  });
  const selectedPendingPrompt = selectedChatEncodedKey
    ? chatPendingPromptsByKey[selectedChatEncodedKey]
    : undefined;
  const renderedPendingChatTurn = selectedPendingPrompt ? (
    <ChatTurnView
      key={`${selectedChatEncodedKey}:pending:${selectedPendingPrompt.createdAt}`}
      message={buildPendingPromptMessage(selectedPendingPrompt)}
      promptStatus="responding"
      hideToolCalls={hideToolCalls}
      markdownComponents={chatMarkdownComponents}
      markdownUrlTransform={chatMarkdownUrlTransform}
    />
  ) : null;
  const renderMain = () => {
    const heavyDiffDeferred =
      !!selectedDiff &&
      isHeavyGeneratedDiffPath(selectedDiff) &&
      !allowHeavyDiffLoad;
    const chatConfigOptions = chatConfigDisplay.visible;
    const chatConfigOverflowOptions = chatConfigDisplay.overflow;
    const selectedFileIsImage = isImageFile(
      selectedFile,
      fileInfo?.mimeType,
    );
    const selectedFileImageSrc = selectedFileIsImage
      ? buildImageDataUrl({
          content: fileContent,
          path: selectedFile,
          mimeType: fileInfo?.mimeType,
          isBinary: fileInfo?.isBinary,
        })
      : '';
    const activeChatSlashCommand = chatSlashMenuVisible
      ? chatSlashMenuOptions[Math.max(0, Math.min(chatSlashActiveIndex, chatSlashMenuOptions.length - 1))]
      : null;
    const renderChatConfigValueMenu = (option: RegistrySessionConfigOption) => {
      const optionValues = option.options ?? [];
      const currentValue = chatConfigCurrentValue(option);
      if (optionValues.length === 0) {
        return null;
      }
      return (
        <div className="chat-config-value-menu" role="menu">
          {optionValues.map(item => {
            const selected = item.value === currentValue;
            return (
              <button
                key={`${option.id}:${item.value}`}
                type="button"
                className={`chat-config-value-option${selected ? ' selected' : ''}`}
                role="menuitemradio"
                aria-checked={selected}
                onClick={() => {
                  setChatConfigMenuOptionId('');
                  setChatConfigOverflowOpen(false);
                  handleChatConfigOptionChange(option, item.value).catch(() => undefined);
                }}
              >
                <span className="chat-config-value-label">{item.name || item.value}</span>
                {selected ? (
                  <span className="codicon codicon-check" aria-hidden="true" />
                ) : null}
              </button>
            );
          })}
        </div>
      );
    };
    const renderChatConfigPill = (option: RegistrySessionConfigOption) => {
      const optionValues = option.options ?? [];
      const optionLabel = option.name || option.id;
      const currentLabel = chatConfigCurrentLabel(option);
      const updating =
        chatConfigUpdatingKey ===
        `${selectedChatSession?.sessionId ?? ''}:${option.id}`;
      const open = chatConfigMenuOptionId === option.id;
      return (
        <div key={option.id} className="chat-config-item">
          <button
            type="button"
            className="chat-config-pill"
            disabled={updating || optionValues.length === 0}
            title={optionLabel}
            aria-label={optionLabel}
            aria-haspopup="menu"
            aria-expanded={open}
            onClick={() => {
              setChatPromptMenuOpen(false);
              setChatConfigOverflowOpen(false);
              setChatConfigMenuOptionId(current => (current === option.id ? '' : option.id));
            }}
          >
            <span
              className={`codicon ${updating ? 'codicon-loading codicon-modifier-spin' : chatConfigIconClass(option)}`}
              aria-hidden="true"
            />
            <span className="chat-config-pill-value">{currentLabel}</span>
            <span className="codicon codicon-chevron-down" aria-hidden="true" />
          </button>
          {open ? renderChatConfigValueMenu(option) : null}
        </div>
      );
    };

    if (tab === 'chat') {
      return (
        <div className="content">
          <div className="block-title">
            {isWide ? (
              <>
                CHAT - {selectedChatSession?.title || 'New Session'}
              </>
            ) : (
              renderBreadcrumbTitle(chatBreadcrumbLabel)
            )}
          </div>
          <div
            className="chat-main"
            style={chatKeyboardInset > 0 ? { paddingBottom: `${chatKeyboardInset}px` } : undefined}
          >
            <div
              ref={chatScrollRef}
              className="scroll-panel chat-block"
              onScroll={event => {
                const nextScrollTop = event.currentTarget.scrollTop;
                const direction = nextScrollTop >= chatLastScrollTopRef.current ? 'down' : 'up';
                chatLastScrollTopRef.current = nextScrollTop;
                updateChatFollowModeFromScroll(event.currentTarget);
                updateSelectedChatWindowFromScroll(event.currentTarget, direction);
              }}
              onPointerDown={() => { chatPointerScrollingRef.current = true; }}
              onPointerUp={() => { chatPointerScrollingRef.current = false; updateChatFollowModeFromScroll(); }}
              onPointerCancel={() => { chatPointerScrollingRef.current = false; updateChatFollowModeFromScroll(); }}
              onTouchStart={() => { chatPointerScrollingRef.current = true; }}
              onTouchEnd={() => { chatPointerScrollingRef.current = false; updateChatFollowModeFromScroll(); }}
              onTouchCancel={() => { chatPointerScrollingRef.current = false; updateChatFollowModeFromScroll(); }}
            >
              {chatLoading ? (
                <div className="muted block">Loading chat...</div>
              ) : null}
              {!chatLoading && chatMessages.length === 0 && !selectedPendingPrompt ? (
                <div className="empty-card">
                  <div className="empty-title">Start chatting</div>
                  <div className="empty-subtitle">
                    Messages stream here for the selected session.
                  </div>
                </div>
              ) : null}
              {renderedChatTurns}
              {renderedPendingChatTurn}
            </div>
            {chatShowScrollToBottom ? (
            <button
              type="button"
              className="chat-scroll-bottom-button"
              onClick={forceChatScrollToBottom}
              title="Scroll to bottom"
              aria-label="Scroll to bottom"
            >
              <span className="chat-scroll-bottom-glyph" aria-hidden="true">
                <span className="codicon codicon-arrow-down" />
              </span>
            </button>
          ) : null}
          <div className="chat-composer">
            <input
              ref={chatFileInputRef}
              type="file"
              accept="image/*"
              multiple
              style={{ display: 'none' }}
              onChange={event => {
                handleChatFileChange(event).catch(err =>
                  setError(err instanceof Error ? err.message : String(err)),
                );
              }}
            />
            <div className="chat-composer-frame">
              {chatAttachments.length > 0 ? (
                <div className="chat-attachment-preview-list">
                  {chatAttachments.map(attachment => (
                    <div key={attachment.id} className="chat-attachment-preview">
                      <img
                        className="chat-attachment-thumb"
                        src={`data:${attachment.mimeType || 'image/png'};base64,${attachment.data}`}
                        alt={attachment.name || 'attachment preview'}
                      />
                      <div className="chat-attachment-meta">
                        <div className="chat-attachment-name">{attachment.name}</div>
                      </div>
                      <button
                        type="button"
                        className="chat-attachment-remove"
                        onClick={() => removeChatAttachment(attachment.id)}
                        title="Remove image"
                        aria-label="Remove image"
                      >
                        <span className="codicon codicon-close" />
                      </button>
                    </div>
                  ))}
                </div>
              ) : null}
              <div className="chat-composer-input-row">
                <button
                  ref={chatPromptButtonRef}
                  type="button"
                  className="chat-composer-prompt-trigger"
                  onClick={openChatPromptMenu}
                  title="Commands"
                  aria-label="Commands"
                  aria-haspopup="listbox"
                  aria-expanded={chatPromptMenuOpen}
                >
                  {'>'}
                </button>
                <div className="chat-composer-input-shell">
                  <textarea
                    ref={chatComposerTextareaRef}
                    rows={1}
                    className="chat-composer-input"
                    value={chatComposerText}
                    onChange={event => updateChatComposerText(event.target.value)}
                    onPaste={event => {
                      if (!supportsChatClipboardImages) {
                        return;
                      }
                      const attachmentDraftKey = currentChatDraftKeyRef.current;
                      const attachmentDraftGeneration = getChatDraftGeneration(attachmentDraftKey);
                      const items = Array.from(event.clipboardData?.items ?? []);
                      const imageItems = items.filter(item =>
                        item.type.toLowerCase().startsWith('image/'),
                      );
                      if (imageItems.length === 0) {
                        return;
                      }
                      event.preventDefault();
                      beginChatAttachmentRead(attachmentDraftKey);
                      let readError = '';
                      Promise.all(
                        imageItems.map(async (item, index) => {
                          const file = item.getAsFile();
                          if (!file) {
                            if (!readError) {
                              readError = 'Clipboard image is unavailable';
                            }
                            return null;
                          }
                          try {
                            return await readChatAttachmentFile(file, `pasted-image-${index + 1}.png`);
                          } catch (err) {
                            if (!readError) {
                              readError = err instanceof Error ? err.message : String(err);
                            }
                            return null;
                          }
                        }),
                      )
                        .then(nextAttachments => {
                          appendChatAttachments(
                            nextAttachments.filter(
                              (attachment): attachment is ChatAttachment => !!attachment,
                            ),
                            attachmentDraftKey,
                            attachmentDraftGeneration,
                          );
                          if (readError) {
                            setError(readError);
                          }
                        })
                        .catch(err =>
                          setError(err instanceof Error ? err.message : String(err)),
                        )
                        .finally(() => {
                          endChatAttachmentRead(attachmentDraftKey);
                        });
                    }}
                    onKeyDown={event => {
                      if (chatSlashMenuVisible) {
                        if (event.key === 'ArrowDown') {
                          event.preventDefault();
                          setChatSlashActiveIndex(prev => {
                            if (chatSlashMenuOptions.length === 0) {
                              return 0;
                            }
                            return (prev + 1) % chatSlashMenuOptions.length;
                          });
                          return;
                        }
                        if (event.key === 'ArrowUp') {
                          event.preventDefault();
                          setChatSlashActiveIndex(prev => {
                            if (chatSlashMenuOptions.length === 0) {
                              return 0;
                            }
                            return (prev - 1 + chatSlashMenuOptions.length) % chatSlashMenuOptions.length;
                          });
                          return;
                        }
                        if (event.key === 'Enter' && !event.altKey && !event.nativeEvent.isComposing) {
                          if (!activeChatSlashCommand) {
                            return;
                          }
                          event.preventDefault();
                          applyChatSlashCommand(activeChatSlashCommand);
                          return;
                        }
                      }
                      if (!isWindowsPlatform) {
                        return;
                      }
                      if (event.key !== 'Enter' || event.altKey || event.nativeEvent.isComposing) {
                        return;
                      }
                      event.preventDefault();
                      if (chatSending || chatAttachmentReadPending) {
                        return;
                      }
                      sendChatMessage().catch(() => undefined);
                    }}
                    placeholder="Send a message..."
                  />
                </div>
                <button
                  type="button"
                  className="chat-send-button"
                  onClick={() => sendChatMessage().catch(() => undefined)}
                  disabled={chatSending || chatAttachmentReadPending}
                  title="Send"
                  aria-label="Send message"
                >
                  <span className="codicon codicon-send" />
                </button>
              </div>
              {chatSlashMenuVisible ? (
                <div ref={chatSlashMenuRef} className="chat-slash-menu" role="listbox" aria-label="Available skills">
                  {chatSlashMenuOptions.map((option, index) => {
                    const selected = index === chatSlashActiveIndex;
                    return (
                      <button
                        key={option.name}
                        type="button"
                        className={`chat-slash-item${selected ? ' active' : ''}`}
                        role="option"
                        aria-selected={selected}
                        onMouseEnter={() => setChatSlashActiveIndex(index)}
                        onMouseDown={event => event.preventDefault()}
                        onClick={() => applyChatSlashCommand(option)}
                      >
                        <span className="chat-slash-name">{option.name}</span>
                        {option.description ? (
                          <span className="chat-slash-description">{option.description}</span>
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              ) : null}
              <div className="chat-composer-toolbar">
                <div className="chat-composer-tools">
                  <button
                    type="button"
                    className="chat-tool-button chat-photo-button"
                    onClick={() => {
                      setChatPromptMenuOpen(false);
                      setChatConfigMenuOptionId('');
                      setChatConfigOverflowOpen(false);
                      chatFileInputRef.current?.click();
                    }}
                    title="Image"
                    aria-label="Attach image"
                  >
                    <span className="codicon codicon-file-media" />
                  </button>
                  <button
                    type="button"
                    className="chat-tool-button chat-voice-button"
                    onClick={() => {
                      setChatPromptMenuOpen(false);
                      setChatConfigMenuOptionId('');
                      setChatConfigOverflowOpen(false);
                      setError('Voice input is not available yet.');
                    }}
                    title="Voice"
                    aria-label="Voice input"
                  >
                    <span className="codicon codicon-mic" />
                  </button>
                </div>
                {selectedChatConfigOptions.length > 0 ? (
                  <div className="chat-config-options-wrap">
                    <div className="chat-config-options-shell">
                      <div ref={chatConfigOptionsRef} className="chat-config-options">
                        {chatConfigOptions.map(option => renderChatConfigPill(option))}
                      </div>
                      {chatConfigOverflowOptions.length > 0 ? (
                        <div ref={chatConfigOverflowRef} className="chat-config-overflow-anchor">
                          <button
                            type="button"
                            className="chat-config-overflow-button"
                            aria-label={`Show ${chatConfigOverflowOptions.length} more config options`}
                            aria-expanded={chatConfigOverflowOpen}
                            title="More config options"
                            onClick={() => {
                              setChatPromptMenuOpen(false);
                              setChatConfigMenuOptionId('');
                              setChatConfigOverflowOpen(prev => !prev);
                            }}
                          >
                            <span className="codicon codicon-ellipsis" aria-hidden="true" />
                            <span className="codicon codicon-chevron-down" aria-hidden="true" />
                          </button>
                          {chatConfigOverflowOpen ? (
                            <div className="chat-config-overflow-menu" aria-label="More config options">
                              {chatConfigOverflowOptions.map(option => {
                                const optionValues = option.options ?? [];
                                const currentValue = chatConfigCurrentValue(option);
                                const updating =
                                  chatConfigUpdatingKey ===
                                  `${selectedChatSession?.sessionId ?? ''}:${option.id}`;
                                const optionLabel = option.name || option.id;
                                return (
                                  <div key={`overflow:${option.id}`} className="chat-config-overflow-group">
                                    <div className="chat-config-item-label" title={optionLabel}>
                                      {optionLabel}
                                    </div>
                                    <div className="chat-config-overflow-values">
                                      {optionValues.map(item => {
                                        const selected = item.value === currentValue;
                                        return (
                                          <button
                                            key={`overflow:${option.id}:${item.value}`}
                                            type="button"
                                            className={`chat-config-value-option${selected ? ' selected' : ''}`}
                                            disabled={updating}
                                            aria-pressed={selected}
                                            onClick={() => {
                                              setChatConfigOverflowOpen(false);
                                              handleChatConfigOptionChange(
                                                option,
                                                item.value,
                                              ).catch(() => undefined);
                                            }}
                                          >
                                            <span className="chat-config-value-label">{item.name || item.value}</span>
                                            {selected ? (
                                              <span className="codicon codicon-check" aria-hidden="true" />
                                            ) : null}
                                          </button>
                                        );
                                      })}
                                    </div>
                                  </div>
                                );
                              })}
                            </div>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                  </div>
                ) : null}
              </div>
            </div>
          </div>
          </div>
        </div>
      );
    }
    if (tab === 'file') {
      return (
        <div className="content">
          <div className="block-title with-tools file-title-bar">
            {isWide ? (
              <span className="title-text">
                {selectedFile || 'Select a file'}
              </span>
            ) : (
              renderBreadcrumbTitle(fileBreadcrumbLabel)
            )}
            <div className="view-tools">{renderViewTools()}</div>
          </div>
          <div className="file-pane">
            <div className="file-main-col">
              {hasPinnedFiles ? (
                <div className="pinned-strip">
                  <span className="pinned-label">Pinned</span>
                  {pinnedFiles.map(path => (
                    <div
                      key={path}
                      className={`pinned-entry ${
                        selectedFile === path ? 'active' : ''
                      }`}
                    >
                      <button
                        type="button"
                        className="pinned-open"
                        onClick={() => setSelectedFile(path)}
                        title={path}
                      >
                        {path.split('/').pop() || path}
                      </button>
                      <button
                        type="button"
                        className="pinned-close"
                        onClick={() =>
                          setPinnedFiles(prev =>
                            prev.filter(item => item !== path),
                          )
                        }
                        aria-label={`Unpin ${path}`}
                      >
                        x
                      </button>
                    </div>
                  ))}
                </div>
              ) : null}
              <div className="file-code-area">
                <div ref={fileSideActionsRef} className="file-side-actions">
                  <button
                    type="button"
                    className={`pinned-pin-toggle file-pin-floating ${
                      isSelectedFilePinned ? 'active' : ''
                    }`}
                    onClick={togglePinSelectedFile}
                    disabled={!selectedFile}
                    title={
                      isSelectedFilePinned
                        ? 'Unpin current file'
                        : 'Pin current file'
                    }
                    aria-label={
                      isSelectedFilePinned
                        ? 'Unpin current file'
                        : 'Pin current file'
                    }
                  >
                    <span className="codicon codicon-pinned view-tool-icon" />
                  </button>
                  <div className="file-action-group side-action-group">
                    <button
                      type="button"
                      className={`view-tool ${gotoToolsOpen ? 'active' : ''}`}
                      onClick={() => {
                        setGotoToolsOpen(value => {
                          const next = !value;
                          if (next) setSearchToolsOpen(false);
                          return next;
                        });
                      }}
                      title="Toggle go to line"
                      aria-label="Toggle go to line"
                    >
                      <span className="codicon codicon-symbol-number view-tool-icon" />
                    </button>
                    <div
                      className={`file-action-panel side-action-panel ${
                        gotoToolsOpen ? 'open' : ''
                      }`}
                    >
                      <input
                        className="goto-input"
                        value={gotoLineInput}
                        onChange={event => setGotoLineInput(event.target.value)}
                        onKeyDown={event => {
                          if (event.key === 'Enter') {
                            event.preventDefault();
                            triggerGoToLine();
                          }
                        }}
                        inputMode="numeric"
                        placeholder="Line"
                      />
                      <button
                        type="button"
                        className="view-tool goto-trigger"
                        title="Go to line"
                        onClick={triggerGoToLine}
                      >
                        <span className="codicon codicon-arrow-right view-tool-icon" />
                      </button>
                    </div>
                  </div>
                  <div className="file-action-group side-action-group">
                    <button
                      type="button"
                      className={`view-tool ${searchToolsOpen ? 'active' : ''}`}
                      onClick={() => {
                        setSearchToolsOpen(value => {
                          const next = !value;
                          if (next) setGotoToolsOpen(false);
                          return next;
                        });
                      }}
                      title="Toggle search"
                      aria-label="Toggle search"
                    >
                      <span className="codicon codicon-search view-tool-icon" />
                    </button>
                    <div
                      className={`file-action-panel side-action-panel ${
                        searchToolsOpen ? 'open' : ''
                      }`}
                    >
                      <input
                        className="search-input"
                        value={fileSearchQuery}
                        onChange={event =>
                          setFileSearchQuery(event.target.value)
                        }
                        onKeyDown={event => {
                          if (event.key === 'Enter') {
                            event.preventDefault();
                            navigateSearchMatch(1);
                          }
                        }}
                        placeholder="Find in file"
                      />
                      <button
                        type="button"
                        className="view-tool search-nav"
                        title="Previous match"
                        onClick={() => navigateSearchMatch(-1)}
                      >
                        <span className="codicon codicon-chevron-up view-tool-icon" />
                      </button>
                      <button
                        type="button"
                        className="view-tool search-nav"
                        title="Next match"
                        onClick={() => navigateSearchMatch(1)}
                      >
                        <span className="codicon codicon-chevron-down view-tool-icon" />
                      </button>
                      <span className="search-count">
                        {fileSearchMatches.length === 0
                          ? '0/0'
                          : `${currentMatchIndex + 1}/${
                              fileSearchMatches.length
                            }`}
                      </span>
                    </div>
                  </div>
                </div>
                <div
                  ref={fileScrollRef}
                  className="scroll-panel"
                  onScroll={event => {
                    const path = selectedFileRef.current;
                    if (!path) return;
                    fileScrollTopByPathRef.current[path] = event.currentTarget.scrollTop;
                  }}
                >
                  {fileLoading ? (
                    <div className="muted block">Loading file...</div>
                  ) : selectedFileIsImage ? (
                    selectedFileImageSrc ? (
                      <div className="file-image-preview-wrap">
                        <img
                          className="file-image-preview"
                          src={selectedFileImageSrc}
                          alt={selectedFile.split('/').pop() || 'image preview'}
                        />
                      </div>
                    ) : (
                      <div className="muted block">Image content is unavailable.</div>
                    )
                  ) : selectedFileIsMarkdown && markdownPreviewEnabled ? (
                    <MarkdownPreview
                      content={fileContent}
                      themeMode={themeMode}
                      codeTheme={codeTheme}
                      codeFont={codeFont}
                      codeFontSize={codeFontSize}
                      codeLineHeight={codeLineHeight}
                      codeTabSize={codeTabSize}
                      wrap={wrapLines}
                      lineNumbers={showLineNumbers}
                    />
                  ) : (
                    renderCodePane(
                      fileContent,
                      false,
                      detectCodeLanguage(selectedFile),
                    )
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      );
    }

    return (
      <div className="content">
        <div className="block-title with-tools">
          {isWide ? (
            <span className="title-text">
              {selectedDiff || 'Select a changed file'}
            </span>
          ) : (
            renderBreadcrumbTitle(gitBreadcrumbLabel)
          )}
          <div className="view-tools">{renderViewTools()}</div>
        </div>
        <div className="scroll-panel">
          {heavyDiffDeferred ? (
            <div className="muted block">
              Heavy generated file selected. Diff loading is paused to keep UI
              responsive.
              <div style={{ marginTop: 10 }}>
                <button
                  type="button"
                  className="button"
                  onClick={() => setAllowHeavyDiffLoad(true)}
                >
                  Load Diff
                </button>
              </div>
            </div>
          ) : diffLoading ? (
            <div className="muted block">Loading diff...</div>
          ) : (
            renderDiffPane(diffText)
          )}
        </div>
      </div>
    );
  };

  const hasCachedWorkspace = projects.length > 0 || !!projectId;
  const keepWorkspaceVisible =
    reconnecting && hasCachedWorkspace;

  if (!connected && !keepWorkspaceVisible) {
    return (
      <div className={`page theme-${themeMode}`}>
        <style>{setiFontCss}</style>
        <div className="connect">
          <h3>Connect to WheelMaker Registry</h3>
          <input
            className="input"
            value={address}
            onChange={e => setAddress(e.target.value)}
            placeholder="127.0.0.1:9630 or ws://127.0.0.1:9630/ws"
          />
          <input
            className="input"
            value={token}
            onChange={e => setToken(e.target.value)}
            placeholder="Token (optional)"
          />
          <button
            className="button"
            onClick={() => connect().catch(() => undefined)}
          >
            {autoConnecting ? 'Connecting...' : 'Connect'}
          </button>
          {error ? <div className="error">{error}</div> : null}
        </div>
      </div>
    );
  }

  const projectMenu = projectMenuOpen ? (
    <div className="project-menu">
      {projects.map(projectItem => (
        <div
          key={projectItem.projectId}
          className={`item project-menu-item ${
            projectItem.projectId === projectId ? 'selected' : ''
          }`}
          onClick={() =>
            switchProject(projectItem.projectId).catch(() => undefined)
          }
        >
          <div className="project-menu-main">
            <span className="project-menu-name">{projectItem.name}</span>
            <span
              className="project-menu-path"
              title={projectItem.path || ''}
            >
              {projectItem.path || '-'}
            </span>
          </div>
          <span className="project-menu-hub">
            {projectItem.hubId || 'local-hub'}
          </span>
        </div>
      ))}
    </div>
  ) : null;

  const refreshButtonContent = refreshingProject ? (
    '...'
  ) : reconnecting ? (
    <span className="codicon codicon-loading codicon-modifier-spin" />
  ) : (
    <span className="codicon codicon-refresh" />
  );

  const desktopActivityBar = isWide ? (
    <nav className="desktop-activity-bar" aria-label="Workspace navigation">
      <div className="desktop-activity-primary">
        <button
          type="button"
          className={`desktop-activity-button${tab === 'chat' && !sidebarSettingsOpen ? ' active' : ''}`}
          onClick={() => handleDesktopActivitySelect('chat')}
          title="Chat"
          aria-label="Chat"
        >
          <span className="codicon codicon-comment-discussion" />
        </button>
        <button
          type="button"
          className={`desktop-activity-button${tab === 'file' && !sidebarSettingsOpen ? ' active' : ''}`}
          onClick={() => handleDesktopActivitySelect('file')}
          title="File"
          aria-label="File"
        >
          <span className="codicon codicon-files" />
        </button>
        <button
          type="button"
          className={`desktop-activity-button${tab === 'git' && !sidebarSettingsOpen ? ' active' : ''}`}
          onClick={() => handleDesktopActivitySelect('git')}
          title="Git"
          aria-label="Git"
        >
          <span className="codicon codicon-source-control" />
        </button>
      </div>
      <div className="desktop-activity-secondary">
        <button
          type="button"
          className={`desktop-activity-button refresh-btn${hasPendingProjectUpdates && !refreshingProject && !reconnecting ? ' has-update-badge' : ''}`}
          onClick={() => refreshProject().catch(() => undefined)}
          title={reconnecting ? 'Reconnecting...' : 'Refresh project'}
          aria-label={reconnecting ? 'Reconnecting' : 'Refresh project'}
          disabled={refreshingProject || reconnecting}
        >
          {refreshingProject || reconnecting ? (
            <span className="codicon codicon-loading codicon-modifier-spin" />
          ) : (
            <span className="codicon codicon-refresh" />
          )}
        </button>
        <button
          type="button"
          className={`desktop-activity-button${sidebarSettingsOpen ? ' active' : ''}`}
          onClick={handleDesktopSettingsSelect}
          title="Settings"
          aria-label="Settings"
        >
          <span className="codicon codicon-settings-gear" />
        </button>
      </div>
    </nav>
  ) : null;

  const floatingControlStack = !isWide ? (
    <div className="floating-control-stack-layer">
      <div
        ref={floatingControlStackRef}
        className="floating-control-stack"
        data-drag-state={floatingDragVisualState}
        style={floatingControlStackStyle}
        onPointerDown={beginFloatingPress}
        onPointerMove={handleFloatingPointerMove}
        onPointerUp={event => {
          floatingIgnoreLostCaptureRef.current = true;
          finishFloatingDrag(event.pointerId);
        }}
        onPointerCancel={event => {
          floatingIgnoreLostCaptureRef.current = true;
          cancelFloatingDrag(event.pointerId);
        }}
        onLostPointerCapture={event => {
          if (floatingIgnoreLostCaptureRef.current) {
            floatingIgnoreLostCaptureRef.current = false;
            return;
          }
          cancelFloatingDrag(event.pointerId);
        }}
      >
        <div
          className="floating-nav-group"
          aria-label="Primary navigation"
          style={floatingNavIndicatorStyle}
        >
          <div className="floating-nav-indicator" />
          <button
            type="button"
            className="floating-nav-button"
            data-active={tab === 'chat'}
            onPointerDown={handleFloatingControlButtonPointerDown}
            onClick={() => handleFloatingNavSelect('chat')}
            title="Chat"
            aria-label="Chat"
          >
            <span className="codicon codicon-comment-discussion" />
          </button>
          <button
            type="button"
            className="floating-nav-button"
            data-active={tab === 'file'}
            onPointerDown={handleFloatingControlButtonPointerDown}
            onClick={() => handleFloatingNavSelect('file')}
            title="File"
            aria-label="File"
          >
            <span className="codicon codicon-files" />
          </button>
          <button
            type="button"
            className="floating-nav-button"
            data-active={tab === 'git'}
            onPointerDown={handleFloatingControlButtonPointerDown}
            onClick={() => handleFloatingNavSelect('git')}
            title="Git"
            aria-label="Git"
          >
            <span className="codicon codicon-source-control" />
          </button>
        </div>
        <button
          type="button"
          className="drawer-toggle-bubble"
          data-active={drawerOpen}
          onPointerDown={handleFloatingControlButtonPointerDown}
          onClick={handleFloatingDrawerToggle}
          title="Toggle drawer"
          aria-label="Toggle drawer"
          aria-expanded={drawerOpen}
        >
          <span className="codicon codicon-menu" />
        </button>
      </div>
    </div>
  ) : null;

  const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (
    <div
      className="mobile-settings-screen"
      role="dialog"
      aria-modal="true"
      aria-label="Settings"
    >
      <div className="mobile-settings-nav">
        <button
          type="button"
          className="mobile-settings-back"
          onClick={() => setSidebarSettingsOpen(false)}
          aria-label="Back to drawer"
          title="Back"
        >
          <span className="codicon codicon-arrow-left" />
        </button>
        <div className="mobile-settings-title">Settings</div>
        <div aria-hidden="true" />
      </div>
      <div className="mobile-settings-scroll">
        <div className="mobile-settings-group">
          {renderSettingsContent(false)}
        </div>
      </div>
    </div>
  ) : null;

  return (
    <ResponsiveShell
      mode={layoutMode}
      themeMode={themeMode}
      setiFontCss={setiFontCss}
      desktopActivityBar={desktopActivityBar}
      desktopSidebarWidth={effectiveDesktopSidebarWidth}
      floatingControlStack={floatingControlStack}
      mobileSettingsScreen={mobileSettingsScreen}
      sidebar={renderSidebar()}
      main={renderMain()}
      sidebarCollapsed={sidebarCollapsed}
      drawerOpen={drawerOpen}
      onCloseDrawer={() => setDrawerOpen(false)}
    />
  );
}

if ('serviceWorker' in navigator && window.isSecureContext) {
  window.addEventListener('load', () => {
    let reloading = false;
    // Reload when a new service worker takes control (after skipWaiting).
    navigator.serviceWorker.addEventListener('controllerchange', () => {
      if (reloading) return;
      reloading = true;
      window.location.reload();
    });

    navigator.serviceWorker
      .register('/service-worker.js')
      .then(registration => {
        // Periodic update check (every 5 minutes).
        const checkUpdate = () => {
          registration.update().catch(() => undefined);
        };
        window.setTimeout(checkUpdate, 1500);
        window.setInterval(checkUpdate, 5 * 60 * 1000);

        if (registration.waiting) {
          registration.waiting.postMessage('SKIP_WAITING');
        }

        registration.addEventListener('updatefound', () => {
          const installing = registration.installing;
          if (!installing) return;
          installing.addEventListener('statechange', () => {
            if (
              installing.state === 'installed' &&
              navigator.serviceWorker.controller
            ) {
              registration.waiting?.postMessage('SKIP_WAITING');
            }
          });
        });
      })
      .catch(() => undefined);
  });
}

workspaceStore.ready().then(() => {
  createRoot(document.getElementById('root')!).render(<App />);
}).catch(error => {
  const root = document.getElementById('root');
  if (!root) return;
  const message = error instanceof Error ? error.message : String(error);
  root.innerHTML = '';
  const box = document.createElement('div');
  box.style.cssText = 'padding:16px;color:#ff7b72;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;';
  box.textContent = `IndexedDB initialization failed: ${message}`;
  root.appendChild(box);
});




















