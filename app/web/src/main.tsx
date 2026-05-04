import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
import { RegistryWorkspaceService } from './services/registryWorkspaceService';
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
import type {
  RegistryChatContentBlock,
  RegistryChatMessage,
  RegistryChatMessageEventPayload,
  RegistryChatSession,
  RegistryResumableSession,
  RegistrySessionConfigOption,
  RegistryFsEntry,
  RegistryFsInfo,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitStatus,
  RegistryProject,
} from './types/registry';
import './styles.css';

type Tab = 'chat' | 'file' | 'git';
type ThemeMode = 'dark' | 'light';
type DirEntries = Record<string, RegistryFsEntry[]>;
type GitDiffSource = 'commit' | 'worktree';
type ChatAttachment = {
  name: string;
  mimeType: string;
  data: string;
};
type PendingNewChatDraft = {
  title: string;
  text: string;
  blocks: RegistryChatContentBlock[];
};
type WorkingTreeFileEntry = {
  path: string;
  status: string;
  scope: 'staged' | 'unstaged' | 'untracked';
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
const CHAT_SWIPE_DELETE_WIDTH = 78;
const CHAT_SWIPE_RELOAD_WIDTH = 72;
const CHAT_SWIPE_TOTAL_ACTIONS_WIDTH = CHAT_SWIPE_DELETE_WIDTH + CHAT_SWIPE_RELOAD_WIDTH;
const CHAT_SWIPE_REVEAL_THRESHOLD = 20;
const CHAT_SWIPE_OPEN_THRESHOLD = 56;
let mermaidRenderSequence = 0;

function nextMermaidRenderId(): string {
  mermaidRenderSequence += 1;
  return `wm-mermaid-${mermaidRenderSequence}`;
}


function sortChatSessions(items: RegistryChatSession[]): RegistryChatSession[] {
  return [...items].sort((a, b) =>
    (b.updatedAt || '').localeCompare(a.updatedAt || ''),
  );
}

function mergeChatSession(
  list: RegistryChatSession[],
  next: Partial<RegistryChatSession> & {sessionId: string},
): RegistryChatSession[] {
  const existing = list.find(item => item.sessionId === next.sessionId);
  const merged: RegistryChatSession = {
    sessionId: next.sessionId,
    title: next.title ?? existing?.title ?? next.sessionId,
    preview: next.preview ?? existing?.preview ?? '',
    updatedAt: next.updatedAt ?? existing?.updatedAt ?? '',
    messageCount: next.messageCount ?? existing?.messageCount ?? 0,
    unreadCount: next.unreadCount ?? existing?.unreadCount,
    agentType: next.agentType ?? existing?.agentType,
    configOptions:
      next.configOptions ??
      (existing?.configOptions
        ? [...existing.configOptions]
        : undefined),
  };
  const filtered = list.filter(item => item.sessionId !== next.sessionId);
  return sortChatSessions([merged, ...filtered]);
}


type ChatSessionGroup = {
  agentKey: string;
  label: string;
  sessions: RegistryChatSession[];
};

function sessionUpdatedAtSortKey(session?: RegistryChatSession): string {
  return session?.updatedAt || '';
}

function groupChatSessionsByAgent(
  sessions: RegistryChatSession[],
): ChatSessionGroup[] {
  const grouped = new Map<string, RegistryChatSession[]>();
  for (const session of sortChatSessions(sessions)) {
    const agentKey = (session.agentType || '').trim() || 'unknown';
    const bucket = grouped.get(agentKey) ?? [];
    bucket.push(session);
    grouped.set(agentKey, bucket);
  }

  const groups = Array.from(grouped.entries()).map(([agentKey, items]) => ({
    agentKey,
    label: agentKey === 'unknown' ? 'Unknown Agent' : agentKey,
    sessions: sortChatSessions(items),
  }));

  groups.sort((a, b) => {
    const aUpdated = sessionUpdatedAtSortKey(a.sessions[0]);
    const bUpdated = sessionUpdatedAtSortKey(b.sessions[0]);
    const updatedDelta = bUpdated.localeCompare(aUpdated);
    if (updatedDelta !== 0) return updatedDelta;
    return a.label.localeCompare(b.label);
  });

  return groups;
}

function upsertChatMessage(
  list: RegistryChatMessage[],
  next: RegistryChatMessage,
): RegistryChatMessage[] {
  const key = `${next.sessionId}:${next.promptIndex}:${next.turnIndex}`;
  const index = list.findIndex(
    item => `${item.sessionId}:${item.promptIndex}:${item.turnIndex}` === key,
  );
  if (index < 0) {
    return [...list, next].sort((a, b) => {
      const pd = (a.promptIndex ?? 0) - (b.promptIndex ?? 0);
      if (pd !== 0) return pd;
      return (a.turnIndex ?? 0) - (b.turnIndex ?? 0);
    });
  }
  const copy = [...list];
  copy[index] = next;
  return copy;
}

function replaceChatMessagesFromPrompt(
  list: RegistryChatMessage[],
  nextMessages: RegistryChatMessage[],
  promptIndex: number,
  checkpointTurnIndex?: number,
): RegistryChatMessage[] {
  const base = list.filter(item => {
    const si = item.promptIndex ?? 0;
    if (promptIndex <= 0) return false;
    if (si < promptIndex) return true;
    // Preserve turns at-or-before checkpoint within the boundary prompt
    // (server skips them, so they won't be in nextMessages)
    if (
      si === promptIndex &&
      checkpointTurnIndex != null &&
      checkpointTurnIndex > 0
    ) {
      return (item.turnIndex ?? 0) <= checkpointTurnIndex;
    }
    return false;
  });
  return nextMessages.reduce(
    (items, message) => upsertChatMessage(items, message),
    base,
  );
}

function getLatestChatSyncCursor(messages: RegistryChatMessage[]): {
  syncIndex: number;
  syncSubIndex: number;
} {
  return messages.reduce(
    (latest, message) => {
      const syncIndex = message.promptIndex ?? 0;
      const syncSubIndex = message.turnIndex ?? 0;
      if (
        syncIndex > latest.syncIndex ||
        (syncIndex === latest.syncIndex && syncSubIndex > latest.syncSubIndex)
      ) {
        return { syncIndex, syncSubIndex };
      }
      return latest;
    },
    { syncIndex: 0, syncSubIndex: 0 },
  );
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

function msgStatus(method: string, param: Record<string, unknown>): string {
  const streamingMethods = [
    'user_message_chunk',
    'agent_message_chunk',
    'agent_thought_chunk',
    'agent_plan',
  ];
  if (streamingMethods.includes(method)) {
    return 'streaming';
  }
  if (method === 'tool_call') {
    const s = typeof param.status === 'string' ? param.status.trim().toLowerCase() : '';
    if (s === 'streaming' || s === 'running' || s === 'in_progress') {
      return 'streaming';
    }
    if (s === 'need_action' || s === 'needs_action') {
      return 'needs_action';
    }
    return 'done';
  }
  return 'done';
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
  if (method === 'prompt_request') {
    return Array.isArray(param.contentBlocks) ? param.contentBlocks as RegistrySessionContentBlock[] : [];
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

function decodeSessionMessageFromEventPayload(
  payload: RegistryChatMessageEventPayload,
): RegistryChatMessage | null {
  const sessionId = typeof payload.sessionId === 'string' ? payload.sessionId.trim() : '';
  const content = typeof payload.content === 'string' ? payload.content.trim() : '';
  const promptIndex = Number(payload.promptIndex ?? 0);
  const turnIndex = Number(payload.turnIndex ?? 0);
  if (!sessionId || promptIndex <= 0 || turnIndex <= 0) {
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
    return { sessionId, promptIndex, turnIndex, method, param };
  } catch {
    // Unparseable content: store as system message
    return {
      sessionId,
      promptIndex,
      turnIndex,
      method: 'system',
      param: { text: content },
    };
  }
}

type ChatPromptEntryKind = 'tool' | 'thought' | 'plan' | 'message';

type ChatPromptEntry = {
  key: string;
  kind: ChatPromptEntryKind;
  text: string;
  turnIndex: number;
  order: number;
  planEntries?: { content: string; status?: string }[];
};

type ChatPromptGroup = {
  key: string;
  promptIndex: number;
  userMessages: RegistryChatMessage[];
  entries: ChatPromptEntry[];
  modelName: string;
  durationMs: number;
  finished: boolean;
};

type ChatPromptGroupViewProps = {
  group: ChatPromptGroup;
  markdownComponents: Components;
  markdownUrlTransform: (value: string) => string;
};

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
        <span className="chat-thought-label">Thought</span>
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

const ChatPromptGroupView = React.memo(function ChatPromptGroupView({
  group,
  markdownComponents,
  markdownUrlTransform,
}: ChatPromptGroupViewProps) {
  const userText = group.userMessages
    .map(m => msgText(m.method, m.param).trim())
    .filter(Boolean)
    .join('\n')
    .trim();
  const imageBlocks = groupImageBlocks(group.userMessages);

  return (
    <div className="chat-prompt-group">
      {userText ? (
        <div className="chat-prompt-user">{userText}</div>
      ) : null}
      {imageBlocks.length > 0 ? (
        <div className="chat-image-strip">
          {imageBlocks.map((block, index) => (
            <img
              key={`${group.key}:img:${index}`}
              className="chat-inline-image"
              src={`data:${block.mimeType || 'image/png'};base64,${block.data}`}
              alt="chat attachment"
            />
          ))}
        </div>
      ) : null}
      {group.entries.map(entry => {
        if (entry.kind === 'tool') {
          return (
            <div key={entry.key} className="chat-tool-line" title={entry.text}>
              <span className="codicon codicon-tools" />
              <span>{entry.text}</span>
            </div>
          );
        }
        if (entry.kind === 'thought') {
          return (
            <CollapsibleThought
              key={entry.key}
              text={entry.text}
              markdownComponents={markdownComponents}
              markdownUrlTransform={markdownUrlTransform}
            />
          );
        }
        if (entry.kind === 'plan') {
          const planEntries = entry.planEntries ?? [];
          if (planEntries.length === 0) {
            return null;
          }
          return (
            <div key={entry.key} className="chat-plan-block">
              <div className="chat-plan-title">
                <span className="codicon codicon-checklist" />
                <span>Plan</span>
              </div>
              <ul className="chat-plan-list">
                {planEntries.map((item, index) => {
                  const done = isPlanEntryCompleted(item.status);
                  return (
                    <li
                      key={`${entry.key}:plan:${index}`}
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
        return (
          <div key={entry.key} className="chat-main-message">
            <ReactMarkdown
              remarkPlugins={[remarkGfm, remarkMath]}
              urlTransform={markdownUrlTransform}
              rehypePlugins={[rehypeKatex]}
              components={markdownComponents}
            >
              {entry.text}
            </ReactMarkdown>
          </div>
        );
      })}
      {group.finished ? (
        <div className="chat-prompt-separator">
          <hr />
          <span className="chat-prompt-separator-label">
            By {group.modelName || 'unknown'}
            {group.durationMs > 0 ? ` · ${formatPromptDuration(group.durationMs)}` : ''}
          </span>
        </div>
      ) : null}
    </div>
  );
});

// -- Prompt separator helpers --

function formatPromptDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const seconds = ms / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  const secs = seconds % 60;
  return secs > 0 ? `${minutes}m ${secs.toFixed(0)}s` : `${minutes}m`;
}

function groupChatMessagesByPrompt(
  messages: RegistryChatMessage[],
  promptSnapshots?: RegistrySessionPromptSnapshot[],
): ChatPromptGroup[] {
  const groups = new Map<string, ChatPromptGroup>();
  const entryIndexByKey = new Map<string, number>();
  let entryOrder = 0;

  const ordered = [...messages].sort((a, b) => {
    const pd = (a.promptIndex ?? 0) - (b.promptIndex ?? 0);
    if (pd !== 0) return pd;
    return (a.turnIndex ?? 0) - (b.turnIndex ?? 0);
  });

  for (const message of ordered) {
    const promptIndex = message.promptIndex ?? 0;
    const groupKey =
      promptIndex > 0 ? `prompt:${promptIndex}` : `msg:${message.sessionId}:${message.turnIndex}`;
    const snapshot = Array.isArray(promptSnapshots)
      ? promptSnapshots.find(s => s.promptIndex === promptIndex)
      : undefined;
    const existing =
      groups.get(groupKey) ??
      ({
        key: groupKey,
        promptIndex,
        userMessages: [],
        entries: [],
        modelName: snapshot?.modelName ?? '',
        durationMs: snapshot?.durationMs ?? 0,
        finished: snapshot?.finished ?? false,
      } as ChatPromptGroup);

    const role = msgRole(message.method);

    if (role === 'user') {
      existing.userMessages.push(message);
    } else {
      const kindStr = msgKind(message.method);
      let kind: ChatPromptEntryKind | null = null;
      let text = '';
      let planEntries: { content: string; status?: string }[] = [];

      if (kindStr === 'tool') {
        kind = 'tool';
        text = msgText(message.method, message.param).replace(/\s+/g, ' ').trim();
      } else if (kindStr === 'thought') {
        kind = 'thought';
        text = msgText(message.method, message.param).trim();
      } else if (kindStr === 'plan') {
        kind = 'plan';
        planEntries = msgPlanEntries(message.method, message.param);
        if (planEntries.length === 0) {
          const rawText = msgText(message.method, message.param).trim();
          if (rawText) {
            planEntries = rawText
              .split('\n')
              .map(line => line.trim())
              .filter(Boolean)
              .map(content => ({ content }));
          }
        }
        text = planEntries.map(item => item.content).join('\n').trim();
      } else {
        kind = 'message';
        text = msgText(message.method, message.param).trim();
      }

      if (kind && text) {
        const turnIndex = message.turnIndex ?? 0;
        const dedupeKey =
          turnIndex > 0
            ? `${groupKey}:${kind}:turn:${turnIndex}`
            : `${groupKey}:${kind}:msg:${message.sessionId}:${message.promptIndex}:${message.turnIndex}`;
        const existingIndex = entryIndexByKey.get(dedupeKey);
        if (typeof existingIndex === 'number') {
          const previous = existing.entries[existingIndex];
          existing.entries[existingIndex] = {
            ...previous,
            text,
            turnIndex,
            planEntries: kind === 'plan' ? planEntries : previous.planEntries,
          };
        } else {
          existing.entries.push({
            key: dedupeKey,
            kind,
            text,
            turnIndex,
            order: entryOrder,
            planEntries: kind === 'plan' ? planEntries : undefined,
          });
          entryIndexByKey.set(dedupeKey, existing.entries.length - 1);
          entryOrder += 1;
        }
      }
    }

    groups.set(groupKey, existing);
  }

  for (const group of groups.values()) {
    group.entries.sort((a, b) => {
      if (a.turnIndex > 0 && b.turnIndex > 0 && a.turnIndex !== b.turnIndex) {
        return a.turnIndex - b.turnIndex;
      }
      return a.order - b.order;
    });
  }

  return [...groups.values()].sort((a, b) => a.promptIndex - b.promptIndex);
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
  const codeFontFamily = useMemo(
    () => resolveCodeFontFamily(codeFont),
    [codeFont],
  );
  const setiFontCss = useMemo(() => setiFontFaceCss(), []);
  const resolveFileIcon = (name: string) => resolveSetiIcon(name, themeMode);

  const [windowWidth, setWindowWidth] = useState<number>(window.innerWidth);
  const isWide = windowWidth >= 900;

  const [tab, setTab] = useState<Tab>(persistedGlobal.tab ?? 'file');
  const tabRef = useRef<Tab>(persistedGlobal.tab ?? 'file');
  const previousTabForChatListRef = useRef<Tab>(persistedGlobal.tab ?? 'file');
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [sidebarSettingsOpen, setSidebarSettingsOpen] = useState(false);

  const [projectMenuOpen, setProjectMenuOpen] = useState(false);

  const [projects, setProjects] = useState<RegistryProject[]>([]);
  const [projectId, setProjectId] = useState('');
  const projectIdRef = useRef('');
  const currentProjectRef = useRef<RegistryProject | null>(null);
  const knownProjectRevRef = useRef('');
  const knownGitRevRef = useRef('');
  const knownWorktreeRevRef = useRef('');
  const [loadingProject, setLoadingProject] = useState(false);
  const [refreshingProject, setRefreshingProject] = useState(false);

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
  const chatComposerTextareaRef = useRef<HTMLTextAreaElement | null>(null);
  const chatComposerActionMenuRef = useRef<HTMLDivElement | null>(null);
  const chatSelectedIdRef = useRef('');
  const chatSyncIndexRef = useRef<Record<string, number>>({});
  const chatSyncSubIndexRef = useRef<Record<string, number>>({});
  const chatMessageStoreRef = useRef<Record<string, RegistryChatMessage[]>>({});
  const chatPromptSnapshotsRef = useRef<Record<string, RegistrySessionPromptSnapshot[]>>({});
  const notifiedChatMessageIdsRef = useRef<Set<string>>(new Set());
  const newChatFlowGuardRef = useRef(false);
  const chatSwipeSessionIdRef = useRef('');
  const chatSwipeStartXRef = useRef(0);
  const chatSwipeSuppressClickRef = useRef(false);
  const [chatSessions, setChatSessions] = useState<RegistryChatSession[]>([]);
  const [selectedChatId, setSelectedChatId] = useState('');
  const [chatMessages, setChatMessages] = useState<RegistryChatMessage[]>([]);
  const [chatLoading, setChatLoading] = useState(false);
  const [chatSending, setChatSending] = useState(false);
  const [chatDeletingSessionId, setChatDeletingSessionId] = useState('');
  const [chatReloadingSessionId, setChatReloadingSessionId] = useState('');
  const [chatSwipeOpenSessionId, setChatSwipeOpenSessionId] = useState('');
  const [chatSwipeDraggingSessionId, setChatSwipeDraggingSessionId] = useState('');
  const [chatSwipeDraggingOffset, setChatSwipeDraggingOffset] = useState(0);
  const [chatConfigUpdatingKey, setChatConfigUpdatingKey] = useState('');
  const [chatConfigFeedback, setChatConfigFeedback] = useState('');
  const [chatComposerText, setChatComposerText] = useState('');
  const [chatAttachment, setChatAttachment] = useState<ChatAttachment | null>(
    null,
  );
  const [chatComposerActionMenuOpen, setChatComposerActionMenuOpen] = useState(false);
  const [newChatAgentPickerOpen, setNewChatAgentPickerOpen] = useState(false);
  const [pendingNewChatDraft, setPendingNewChatDraft] = useState<PendingNewChatDraft | null>(null);
  const [resumeAgentPickerOpen, setResumeAgentPickerOpen] = useState(false);
  const [resumeAgentType, setResumeAgentType] = useState('');
  const [resumeSessions, setResumeSessions] = useState<RegistryResumableSession[]>([]);
  const [resumeLoading, setResumeLoading] = useState(false);

  const [gitLoading, setGitLoading] = useState(false);
  const [gitError, setGitError] = useState('');
  const [gitCurrentBranch, setGitCurrentBranch] = useState('');
  const [gitBranches, setGitBranches] = useState<string[]>([]);
  const [gitSelectedBranches, setGitSelectedBranches] = useState<string[]>([]);
  const [gitBranchPickerOpen, setGitBranchPickerOpen] = useState(false);
  const [gitDirty, setGitDirty] = useState(false);
  const [gitStatusSummary, setGitStatusSummary] = useState({
    staged: 0,
    unstaged: 0,
    untracked: 0,
  });
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
    const previousTab = previousTabForChatListRef.current;
    const enteredChat = previousTab !== 'chat' && tab === 'chat';
    previousTabForChatListRef.current = tab;
    tabRef.current = tab;
    if (!enteredChat) {
      return;
    }
    if (!connected || !projectIdRef.current) {
      return;
    }
    loadChatSessions(projectIdRef.current).catch(() => undefined);
  }, [tab, connected]);


  useEffect(() => {
    if (!chatSwipeOpenSessionId) {
      return;
    }
    const stillExists = chatSessions.some(
      session => session.sessionId === chatSwipeOpenSessionId,
    );
    if (stillExists) {
      return;
    }
    setChatSwipeOpenSessionId('');
    setChatSwipeDraggingSessionId('');
    setChatSwipeDraggingOffset(0);
  }, [chatSessions, chatSwipeOpenSessionId]);


  useEffect(() => {
    const input = chatComposerTextareaRef.current;
    if (!input) {
      return;
    }
    input.style.height = '0px';
    const nextHeight = Math.max(36, Math.min(input.scrollHeight, 180));
    input.style.height = `${nextHeight}px`;
    input.style.overflowY = input.scrollHeight > 180 ? 'auto' : 'hidden';
  }, [chatComposerText]);

  useEffect(() => {
    if (tab !== 'chat') {
      return;
    }
    const container = chatScrollRef.current;
    if (!container) {
      return;
    }
    window.requestAnimationFrame(() => {
      container.scrollTop = container.scrollHeight;
    });
  }, [tab, selectedChatId, chatMessages, chatLoading]);

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
    const onResize = () => setWindowWidth(window.innerWidth);
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  useEffect(() => {
    if (isWide) {
      setDrawerOpen(false);
    }
  }, [isWide]);

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
    if (!chatComposerActionMenuOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && chatComposerActionMenuRef.current?.contains(target)) return;
      setChatComposerActionMenuOpen(false);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [chatComposerActionMenuOpen]);

  useEffect(() => {
    if (!chatComposerActionMenuOpen) {
      return;
    }
    if (chatComposerText.length > 0) {
      setChatComposerActionMenuOpen(false);
    }
  }, [chatComposerText, chatComposerActionMenuOpen]);

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
      tab,
      selectedProjectId: projectId,
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
    tab,
    projectId,
  ]);

  useEffect(() => {
    if (!projectId) return;
    workspaceStore.rememberProjectSnapshot(projectId, {
      dirEntries,
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
    dirEntries,
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
  const project = currentProject;
  const availableChatAgents = useMemo(() => {
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
    for (const item of project?.agents ?? []) {
      append(item);
    }
    append(project?.agent);
    for (const session of chatSessions) {
      append(session.agentType);
    }
    return agents;
  }, [project?.agents, project?.agent, chatSessions]);
  currentProjectRef.current = currentProject;
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
      const result = await service.listDirectory(
        path,
        dirHashRef.current[path],
      );
      if (!result.notModified) {
        const entries = sortEntries(result.entries);
        setDirEntries(prev => ({ ...prev, [path]: entries }));
        if (result.hash) {
          dirHashRef.current[path] = result.hash;
        }
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
      const isFirstLoad = !fileHashRef.current[path];
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
        knownHash: fileHashRef.current[path],
      });
      if (requestSeq !== fileReadSeqRef.current) return;
      if (result.notModified) {
        const cachedContent = fileCacheRef.current[path];
        setFileContent(typeof cachedContent === 'string' ? cachedContent : '');
        if (result.hash) {
          fileHashRef.current[path] = result.hash;
        }
        if (shouldRestoreScroll) {
          scheduleRestoreSelectedFileScroll(path);
        }
        return;
      }
      setFileContent(result.content);
      fileCacheRef.current[path] = result.content;
      if (result.hash) {
        fileHashRef.current[path] = result.hash;
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
      setGitDirty(statusData.dirty);
      setGitStatusSummary({
        staged: statusData.staged.length,
        unstaged: statusData.unstaged.length,
        untracked: statusData.untracked.length,
      });
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
      setGitDirty(statusData.dirty);
      setGitStatusSummary({
        staged: statusData.staged.length,
        unstaged: statusData.unstaged.length,
        untracked: statusData.untracked.length,
      });
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
    if (!activeProjectId || !sessionId) return;
    setChatLoading(true);
    try {
      const useIncremental = options?.forceFull
        ? false
        : (options?.incremental ?? true);
      const checkpointPromptIndex = useIncremental
        ? chatSyncIndexRef.current[sessionId] ?? 0
        : 0;
      const checkpointTurnIndex = useIncremental
        ? chatSyncSubIndexRef.current[sessionId] ?? 0
        : 0;
      // Snapshot existing messages BEFORE the await so the base is
      // consistent with the cursor. Live session.message events may
      // mutate chatMessageStoreRef during the network round-trip.
      const existingMessages = chatMessageStoreRef.current[sessionId] ?? [];
      const result = await service.readSession(
        sessionId,
        checkpointPromptIndex,
        checkpointTurnIndex,
      );
      if (
        options?.preserveUserSelection &&
        chatSelectedIdRef.current !== (options.selectionSnapshot ?? '') &&
        chatSelectedIdRef.current !== sessionId
      ) {
        return;
      }

      let nextMessages: RegistryChatMessage[];
      if (useIncremental) {
        if (result.messages.length > 0) {
          const firstReturnedPromptIndex = result.messages[0].promptIndex;
          nextMessages = replaceChatMessagesFromPrompt(
            existingMessages,
            result.messages,
            firstReturnedPromptIndex,
            checkpointTurnIndex,
          );
        } else {
          nextMessages = existingMessages;
        }
      } else {
        nextMessages = result.messages;
      }

      // Reconcile: live session.message events may have landed in the store
      // during the await. Fold them in so they aren't lost.
      const fresh = chatMessageStoreRef.current[result.session.sessionId] ?? [];
      for (const m of fresh) {
        if (!nextMessages.some(
          n => n.promptIndex === m.promptIndex && n.turnIndex === m.turnIndex,
        )) {
          nextMessages = upsertChatMessage(nextMessages, m);
        }
      }

      chatMessageStoreRef.current[result.session.sessionId] = nextMessages;
      chatPromptSnapshotsRef.current[result.session.sessionId] = result.prompts;
      const latestSyncCursor = getLatestChatSyncCursor(nextMessages);
      chatSyncIndexRef.current[result.session.sessionId] = latestSyncCursor.syncIndex;
      chatSyncSubIndexRef.current[result.session.sessionId] = latestSyncCursor.syncSubIndex;
      setChatSessions(prev => mergeChatSession(prev, result.session));
      setSelectedChatId(result.session.sessionId);
      setChatMessages(nextMessages);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setChatLoading(false);
    }
  };

  const loadChatSessions = async (
    activeProjectId = projectIdRef.current,
  ) => {
    if (!activeProjectId) return;
    try {
      const sessions = sortChatSessions(await service.listSessions());
      setChatSessions(prev =>
        sessions.reduce(
          (acc, session) => mergeChatSession(acc, session),
          prev.filter(item =>
            sessions.every(session => session.sessionId !== item.sessionId),
          ),
        ),
      );
      const currentSelection = chatSelectedIdRef.current;
      if (!currentSelection || !sessions.some(session => session.sessionId === currentSelection)) {
        setSelectedChatId('');
        setChatMessages([]);
        return;
      }
      setSelectedChatId(currentSelection);
      setChatMessages(chatMessageStoreRef.current[currentSelection] ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };
  const createChatSession = async (agentType: string, title = '') => {
    const normalizedAgentType = agentType.trim();
    if (!normalizedAgentType) {
      setError('No agent selected for new session');
      return '';
    }
    try {
      const result = await service.createSession(normalizedAgentType, title);
      if (!result.session.sessionId) {
        throw new Error('Session was created without a sessionId');
      }
      setChatSessions(prev => mergeChatSession(prev, result.session));
      setSelectedChatId(result.session.sessionId);
      chatSelectedIdRef.current = result.session.sessionId;
      chatMessageStoreRef.current[result.session.sessionId] = [];
      chatSyncIndexRef.current[result.session.sessionId] = 0;
      chatSyncSubIndexRef.current[result.session.sessionId] = 0;
      setChatMessages([]);
      return result.session.sessionId;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return '';
    }
  };

  const beginNewChatFlow = (draft: PendingNewChatDraft): boolean => {
    setPendingNewChatDraft(draft);
    setNewChatAgentPickerOpen(true);
    return true;
  };

  const resetChatComposer = () => {
    setChatComposerText('');
    setChatAttachment(null);
    if (chatFileInputRef.current) {
      chatFileInputRef.current.value = '';
    }
  };

  const completeNewChatFlow = async (agentType: string) => {
    if (newChatFlowGuardRef.current) {
      return;
    }
    const draft = pendingNewChatDraft;
    if (!draft) {
      return;
    }
    newChatFlowGuardRef.current = true;
    try {
      const sessionId = await createChatSession(agentType, draft.title);
      setNewChatAgentPickerOpen(false);
      setPendingNewChatDraft(null);
      if (!sessionId) {
        return;
      }
      if (draft.text.trim() || draft.blocks.length > 0) {
        await service.sendSessionMessage({
          sessionId,
          text: draft.text,
          blocks: draft.blocks,
        });
      }
      resetChatComposer();
    } finally {
      newChatFlowGuardRef.current = false;
    }
    if (!isWide) {
      setDrawerOpen(false);
    }
  };

  const handleResumePickAgent = async (agentType: string) => {
    setResumeAgentType(agentType);
    setResumeLoading(true);
    setResumeSessions([]);
    try {
      const sessions = await service.listResumableSessions(agentType);
      setResumeSessions(sessions);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setResumeAgentPickerOpen(false);
    } finally {
      setResumeLoading(false);
    }
  };

  const handleResumeImport = async (agentType: string, sessionId: string) => {
    setResumeLoading(true);
    try {
      const result = await service.importResumedSession(agentType, sessionId);
      if (result.ok && result.session.sessionId) {
        setChatSessions(prev => mergeChatSession(prev, result.session));
        setSelectedChatId(result.session.sessionId);
        chatSelectedIdRef.current = result.session.sessionId;
        chatMessageStoreRef.current[result.session.sessionId] = [];
        chatSyncIndexRef.current[result.session.sessionId] = 0;
        chatSyncSubIndexRef.current[result.session.sessionId] = 0;
        setChatMessages([]);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setResumeLoading(false);
      setResumeAgentPickerOpen(false);
      setResumeSessions([]);
    }
  };

  const handleDismissResume = () => {
    setResumeAgentPickerOpen(false);
    setResumeAgentType('');
    setResumeSessions([]);
  };

  const removeChatSessionFromState = (sessionId: string) => {
    if (!sessionId) return;
    setChatSessions(prev => prev.filter(item => item.sessionId !== sessionId));
    if (chatSelectedIdRef.current === sessionId) {
      setSelectedChatId('');
      chatSelectedIdRef.current = '';
      setChatMessages([]);
    }
    if (chatSwipeOpenSessionId === sessionId) {
      setChatSwipeOpenSessionId('');
      setChatSwipeDraggingSessionId('');
      setChatSwipeDraggingOffset(0);
    }
    const nextMessageStore = {...chatMessageStoreRef.current};
    const nextSyncIndex = {...chatSyncIndexRef.current};
    const nextSyncSubIndex = {...chatSyncSubIndexRef.current};
    const nextSnapshots = {...chatPromptSnapshotsRef.current};
    delete nextMessageStore[sessionId];
    delete nextSyncIndex[sessionId];
    delete nextSyncSubIndex[sessionId];
    delete nextSnapshots[sessionId];
    chatMessageStoreRef.current = nextMessageStore;
    chatSyncIndexRef.current = nextSyncIndex;
    chatSyncSubIndexRef.current = nextSyncSubIndex;
    chatPromptSnapshotsRef.current = nextSnapshots;
  };

  const handleDeleteChatSession = async (sessionId: string) => {
    const normalizedSessionId = sessionId.trim();
    if (!normalizedSessionId || chatDeletingSessionId) {
      return;
    }

    const confirmed = window.confirm(
      'Delete this session and all related prompts? This action cannot be undone.',
    );
    if (!confirmed) {
      setChatSwipeOpenSessionId('');
      return;
    }

    setChatDeletingSessionId(normalizedSessionId);
    try {
      const result = await service.deleteSession(normalizedSessionId);
      if (!result.ok) {
        throw new Error('session.delete returned ok=false');
      }
      removeChatSessionFromState(result.sessionId || normalizedSessionId);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setChatDeletingSessionId('');
    }
  };

  const handleReloadChatSession = async (sessionId: string) => {
    const normalizedSessionId = sessionId.trim();
    if (!normalizedSessionId || chatReloadingSessionId) {
      return;
    }
    setChatReloadingSessionId(normalizedSessionId);
    setChatSwipeOpenSessionId('');
    try {
      const result = await service.reloadSession(normalizedSessionId);
      if (!result.ok) {
        throw new Error('session.reload returned ok=false');
      }
      // Reset sync state so next load fetches from prompt 0
      chatSyncIndexRef.current[normalizedSessionId] = 0;
      chatSyncSubIndexRef.current[normalizedSessionId] = 0;
      chatMessageStoreRef.current[normalizedSessionId] = [];
      // Reload the session messages if currently selected
      if (chatSelectedIdRef.current === normalizedSessionId) {
        setChatMessages([]);
        await loadChatSession(normalizedSessionId, projectIdRef.current, { forceFull: true });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setChatReloadingSessionId('');
    }
  };

  const resolveChatSessionSwipeOffset = (sessionId: string): number => {
    if (chatSwipeDraggingSessionId === sessionId) {
      return chatSwipeDraggingOffset;
    }
    return chatSwipeOpenSessionId === sessionId ? -CHAT_SWIPE_TOTAL_ACTIONS_WIDTH : 0;
  };


  const isChatSessionSwipeOpen = (sessionId: string): boolean => {
    if (chatSwipeOpenSessionId === sessionId) {
      return true;
    }
    if (chatSwipeDraggingSessionId !== sessionId) {
      return false;
    }
    return chatSwipeDraggingOffset <= -CHAT_SWIPE_REVEAL_THRESHOLD;
  };
  const beginChatSessionSwipe = (event: React.TouchEvent<HTMLDivElement>, sessionId: string) => {
    if (event.touches.length !== 1) {
      return;
    }
    chatSwipeSessionIdRef.current = sessionId;
    chatSwipeStartXRef.current = event.touches[0].clientX;
    chatSwipeSuppressClickRef.current = false;
    setChatSwipeDraggingSessionId(sessionId);
    setChatSwipeDraggingOffset(
      chatSwipeOpenSessionId === sessionId ? -CHAT_SWIPE_TOTAL_ACTIONS_WIDTH : 0,
    );
  };

  const moveChatSessionSwipe = (event: React.TouchEvent<HTMLDivElement>, sessionId: string) => {
    if (chatSwipeSessionIdRef.current !== sessionId || event.touches.length !== 1) {
      return;
    }
    const startX = chatSwipeStartXRef.current;
    const currentX = event.touches[0].clientX;
    const deltaX = currentX - startX;
    if (Math.abs(deltaX) > 6) {
      chatSwipeSuppressClickRef.current = true;
    }
    const anchoredDelta =
      chatSwipeOpenSessionId === sessionId
        ? deltaX - CHAT_SWIPE_TOTAL_ACTIONS_WIDTH
        : deltaX;
    const nextOffset = Math.max(
      -CHAT_SWIPE_TOTAL_ACTIONS_WIDTH,
      Math.min(0, anchoredDelta),
    );
    setChatSwipeDraggingOffset(nextOffset);
  };

  const endChatSessionSwipe = (sessionId: string) => {
    if (chatSwipeSessionIdRef.current !== sessionId) {
      return;
    }
    const shouldOpen = chatSwipeDraggingOffset <= -CHAT_SWIPE_OPEN_THRESHOLD;
    setChatSwipeOpenSessionId(shouldOpen ? sessionId : '');
    setChatSwipeDraggingSessionId('');
    setChatSwipeDraggingOffset(0);
    chatSwipeSessionIdRef.current = '';
  };

  const selectChatSession = (sessionId: string) => {
    if (!sessionId) {
      return;
    }
    if (chatSwipeSuppressClickRef.current) {
      chatSwipeSuppressClickRef.current = false;
      return;
    }
    if (chatSwipeOpenSessionId) {
      if (chatSwipeOpenSessionId === sessionId) {
        setChatSwipeOpenSessionId('');
        return;
      }
      setChatSwipeOpenSessionId('');
    }
    chatSelectedIdRef.current = sessionId;
    setSelectedChatId(sessionId);
    setChatMessages(chatMessageStoreRef.current[sessionId] ?? []);
    loadChatSession(sessionId, projectIdRef.current, {
      incremental: true,
    }).catch(() => undefined);
    if (!isWide) setDrawerOpen(false);
  };


  const sendChatMessage = async () => {
    const trimmedText = chatComposerText.trim();
    if (!trimmedText && !chatAttachment) return;
    const blocks: RegistryChatContentBlock[] = [];
    if (trimmedText) {
      blocks.push({ type: 'text', text: trimmedText });
    }
    if (chatAttachment) {
      blocks.push({
        type: 'image',
        mimeType: chatAttachment.mimeType,
        data: chatAttachment.data,
      });
    }

    // Clear UI immediately after capturing text — before any async work
    resetChatComposer();
    setChatSending(true);
    try {
        if (!selectedChatId) {
          const started = beginNewChatFlow({
            title: trimmedText || chatAttachment?.name || '',
            text: trimmedText,
            blocks,
          });
          if (!started) {
            return;
          }
          setChatSending(false);
          return;
        }
        const sessionId = selectedChatId;
      if (!sessionId) {
        return;
      }
      const result = await service.sendSessionMessage({
        sessionId,
        text: trimmedText,
        blocks,
      });
      const nextSessionId = result.sessionId || sessionId;
      setSelectedChatId(nextSessionId);
      if (!chatSessions.find(item => item.sessionId === nextSessionId)) {
        setChatSessions(prev =>
          mergeChatSession(prev, {
            sessionId: nextSessionId,
            title: trimmedText || chatAttachment?.name || nextSessionId,
            preview: trimmedText || chatAttachment?.name || '',
            updatedAt: new Date().toISOString(),
            messageCount: 0,
          }),
        );
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setChatSending(false);
    }
  };

  const applyChatSessionConfigOptions = (
    sessionId: string,
    configOptions: RegistrySessionConfigOption[],
  ) => {
    if (!sessionId) return;
    setChatSessions(prev => {
      const existing = prev.find(item => item.sessionId === sessionId);
      if (!existing) return prev;
      return mergeChatSession(prev, {
        ...existing,
        configOptions,
      });
    });
  };

  const handleChatConfigOptionChange = async (
    option: RegistrySessionConfigOption,
    value: string,
  ) => {
    const sessionId = chatSelectedIdRef.current.trim();
    const configId = option.id.trim();
    const nextValue = value;
    if (!sessionId || !configId || !nextValue || nextValue === option.currentValue) {
      return;
    }
    const updatingKey = `${sessionId}:${configId}`;
    setChatConfigUpdatingKey(updatingKey);
    setChatConfigFeedback('Applying config...');

    try {
      const result = await service.setSessionConfig({
        sessionId,
        configId,
        value: nextValue,
      });
      if (!result.ok) {
        throw new Error('session.setConfig returned ok=false');
      }
      if (result.configOptions.length > 0) {
        applyChatSessionConfigOptions(result.sessionId || sessionId, result.configOptions);
      }
      setChatConfigFeedback('');
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setError(message);
      setChatConfigFeedback('Config update failed: ' + message);
    } finally {
      setChatConfigUpdatingKey(prev => (prev === updatingKey ? '' : prev));
    }
  };

  const handleChatFileChange = async (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const file = event.target.files?.[0];
    if (!file) {
      setChatAttachment(null);
      return;
    }
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
    setChatAttachment({
      name: file.name,
      mimeType: file.type || 'image/png',
      data: base64,
    });
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
    const previousSelectedChatId = chatSelectedIdRef.current;
    setError('');
    clearReconnectTimer();
    if (!silentReconnect) {
      reconnectStartedAtRef.current = null;
      setReconnecting(false);
    }
    try {
      const ws = toRegistryWsUrl(nextAddress);
      const result = await workspaceController.connect(ws, trimmedToken);
      setProjects(result.projects);
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
      setGitDirty(
        Boolean(
          result.projects.find(
            item => item.projectId === result.hydrated.projectId,
          )?.git?.dirty,
        ),
      );
      reconnectStartedAtRef.current = null;
      setReconnecting(false);
      setConnected(true);
      if (!silentReconnect) {
        setChatMessages([]);
        setChatSessions([]);
        setSelectedChatId('');
        chatSelectedIdRef.current = '';
        chatSyncIndexRef.current = {};
        chatSyncSubIndexRef.current = {};
        chatMessageStoreRef.current = {};
      }
      if (silentReconnect) {
        const shouldSyncSelectedSession =
          tabRef.current === 'chat' &&
          !!previousSelectedChatId;
        if (shouldSyncSelectedSession) {
          await loadChatSession(previousSelectedChatId, result.hydrated.projectId, {
            incremental: true,
            preserveUserSelection: true,
            selectionSnapshot: previousSelectedChatId,
          });
        }
      } else if (tabRef.current === 'chat') {
        await loadChatSessions(result.hydrated.projectId);
      }
      workspaceController
        .validateExpandedDirectories(
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
            RECONNECT_GRACE_PERIOD_MS / 1000,
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
      reason !== 'stop' && !!tokenRef.current.trim() && !!projectIdRef.current;
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
  ) => {
    const messageKey = `${message.sessionId}:${message.promptIndex}:${message.turnIndex}`;
    if (!message.sessionId || msgRole(message.method) === 'user') {
      return;
    }
    if (notifiedChatMessageIdsRef.current.has(messageKey)) {
      return;
    }
    const isVisible =
      typeof document !== 'undefined' && document.visibilityState === 'visible';
    if (isVisible && message.sessionId === chatSelectedIdRef.current) {
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
          !!tokenRef.current.trim() &&
          !!addressRef.current.trim() &&
          !!projectIdRef.current;
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

  const clearLocalCache = () => {
    const confirmed = window.confirm(
      'Clear all local cache data except token?',
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
      applyHydratedProjectState(result.hydrated);
      setChatMessages([]);
      setChatSessions([]);
      setSelectedChatId('');
      chatSelectedIdRef.current = '';
      chatSyncIndexRef.current = {};
      chatSyncSubIndexRef.current = {};
      chatMessageStoreRef.current = {};
      workspaceController
        .validateExpandedDirectories(
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
    } finally {
      refreshInFlightRef.current = false;
      if (!silent) {
        setRefreshingProject(false);
      }
    }
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
      }
      if (event.method === 'session.updated') {
        if (eventProjectId && eventProjectId !== projectIdRef.current) {
          return;
        }
        const payload = (event.payload ?? {}) as {
          session?: RegistryChatSession;
        };
        if (payload.session?.sessionId) {
          setChatSessions(prev => mergeChatSession(prev, payload.session!));
        }
        return;
      }
      if (event.method === 'session.message') {
        if (eventProjectId && eventProjectId !== projectIdRef.current) {
          return;
        }
        const payload = (event.payload ??
          {}) as RegistryChatMessageEventPayload;
        const message = decodeSessionMessageFromEventPayload(payload);
        if (!message) {
          return;
        }

        const sessionId = message.sessionId;
        const messageText = msgText(message.method, message.param);
        setChatSessions(prev => {
          const existing = prev.find(item => item.sessionId === sessionId);
          const fallbackTitle =
            msgRole(message.method) === 'user' && messageText
              ? messageText.slice(0, 120)
              : existing?.title || sessionId;
          return mergeChatSession(prev, {
            sessionId,
            title: fallbackTitle,
            preview: messageText || existing?.preview || '',
            updatedAt: existing?.updatedAt || '',
            messageCount: existing?.messageCount ?? 0,
            unreadCount: existing?.unreadCount,
            agentType: existing?.agentType,
          });
        });

        maybeNotifyChatMessage(message);

        const incomingPromptIndex = message.promptIndex ?? 0;
        const incomingTurnIndex = message.turnIndex ?? 0;
        const currentPromptIndex = chatSyncIndexRef.current[sessionId] ?? 0;
        const currentTurnIndex = chatSyncSubIndexRef.current[sessionId] ?? 0;
        if (
          incomingPromptIndex > currentPromptIndex ||
          (incomingPromptIndex === currentPromptIndex && incomingTurnIndex > currentTurnIndex)
        ) {
          chatSyncIndexRef.current[sessionId] = incomingPromptIndex;
          chatSyncSubIndexRef.current[sessionId] = incomingTurnIndex;
        }

        const merged = upsertChatMessage(
          chatMessageStoreRef.current[sessionId] ?? [],
          message,
        );
        chatMessageStoreRef.current[sessionId] = merged;

        if (sessionId === chatSelectedIdRef.current) {
          setChatMessages(merged);
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
        !!tokenRef.current.trim() && !!projectIdRef.current;
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
    }, 1000);
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

  const groupedChatSessions = useMemo(
    () => groupChatSessionsByAgent(chatSessions),
    [chatSessions],
  );

  const renderSidebarMain = () => {
    if (tab === 'chat') {
      return (
        <>
          <div className="section-title">CHAT SESSIONS</div>
          <div className="list">
            {chatSessions.length === 0 ? (
              <div className="muted block">No chat sessions yet</div>
            ) : null}
            <div className="button-row" style={{ marginBottom: 10 }}>
              <button
                type="button"
                className="button button-icon"
                style={{ marginTop: 0 }}
                onClick={() => {
                  setResumeAgentPickerOpen(false);
                  setResumeSessions([]);
                  beginNewChatFlow({ title: '', text: '', blocks: [] });
                }}
              >
                <span className="codicon codicon-add" />
                New
              </button>
              <button
                type="button"
                className="button secondary button-icon"
                style={{ marginTop: 0 }}
                onClick={() => {
                  setNewChatAgentPickerOpen(false);
                  setPendingNewChatDraft(null);
                  setResumeAgentPickerOpen(true);
                }}
              >
                <span className="codicon codicon-history" />
                Resume
              </button>
            </div>
            {resumeAgentPickerOpen ? (
              <div className="chat-agent-picker-card">
                <div className="chat-agent-picker-header">
                  <span className="codicon codicon-history" />
                  <span className="chat-agent-picker-title">Resume Session</span>
                </div>
                {resumeSessions.length === 0 && !resumeLoading ? (
                  <>
                    <div className="chat-agent-picker-subtitle">Select an agent</div>
                    <div className="chat-agent-picker-actions">
                      {availableChatAgents.map(agentType => (
                        <button
                          key={agentType}
                          type="button"
                          className="button secondary"
                          disabled={resumeLoading}
                          onClick={() => { handleResumePickAgent(agentType).catch(() => undefined); }}
                        >
                          <span className="codicon codicon-sparkle" />
                          {agentType}
                        </button>
                      ))}
                      {availableChatAgents.length === 0 ? (
                        <div className="muted block">No agent available.</div>
                      ) : null}
                    </div>
                  </>
                ) : null}
                {resumeLoading ? (
                  <div className="muted block">Loading sessions...</div>
                ) : null}
                {resumeSessions.length > 0 ? (
                  <div className="chat-resume-list">
                    {resumeSessions.map(s => (
                      <div
                        key={s.sessionId}
                        className="chat-resume-item"
                        onClick={() => { handleResumeImport(resumeAgentType, s.sessionId).catch(() => undefined); }}
                      >
                        <span className="chat-resume-item-title">
                          {s.title || s.sessionId}
                        </span>
                        <span className="chat-resume-item-meta">
                          <span>{formatCompactRelativeAge(s.updatedAt)}</span>
                          {s.messageCount > 0 ? <span>{s.messageCount} messages</span> : null}
                        </span>
                      </div>
                    ))}
                  </div>
                ) : null}
                <button
                  type="button"
                  className="button ghost"
                  style={{ alignSelf: 'flex-start', marginTop: 0, fontSize: 12 }}
                  onClick={handleDismissResume}
                >
                  <span className="codicon codicon-close" style={{ marginRight: 4 }} />
                  Cancel
                </button>
              </div>
            ) : null}
            {newChatAgentPickerOpen && pendingNewChatDraft ? (
              <div className="chat-agent-picker-card">
                <div className="chat-agent-picker-header">
                  <span className="codicon codicon-add" />
                  <span className="chat-agent-picker-title">New Session</span>
                </div>
                <div className="chat-agent-picker-subtitle">Choose an agent</div>
                <div className="chat-agent-picker-actions">
                  {availableChatAgents.map(agentType => (
                    <button
                      key={agentType}
                      type="button"
                      className="button secondary"
                      onClick={() => {
                        completeNewChatFlow(agentType).catch(() => undefined);
                      }}
                    >
                      <span className="codicon codicon-sparkle" />
                      {agentType}
                    </button>
                  ))}
                </div>
                {availableChatAgents.length === 0 ? (
                  <div className="muted block">No agents available.</div>
                ) : null}
                <button
                  type="button"
                  className="button ghost"
                  style={{ alignSelf: 'flex-start', marginTop: 0, fontSize: 12 }}
                  onClick={() => {
                    setNewChatAgentPickerOpen(false);
                    setPendingNewChatDraft(null);
                  }}
                >
                  <span className="codicon codicon-close" style={{ marginRight: 4 }} />
                  Cancel
                </button>
              </div>
            ) : null}
            {groupedChatSessions.map(group => (
              <div key={`chat-group:${group.agentKey}`} className="chat-session-group">
                <div className="chat-session-group-title">{group.label}</div>
                {group.sessions.map(session => (
                  <div key={session.sessionId} className={`chat-session-swipe-row ${isChatSessionSwipeOpen(session.sessionId) ? 'open' : ''}`}>
                    <button
                      type="button"
                      className="chat-session-reload-action"
                      disabled={chatReloadingSessionId === session.sessionId}
                      onClick={event => {
                        event.stopPropagation();
                        handleReloadChatSession(session.sessionId).catch(() => undefined);
                      }}
                    >
                      {chatReloadingSessionId === session.sessionId ? '...' : 'Reload'}
                    </button>
                    <button
                      type="button"
                      className="chat-session-delete-action"
                      disabled={chatDeletingSessionId === session.sessionId}
                      onClick={event => {
                        event.stopPropagation();
                        handleDeleteChatSession(session.sessionId).catch(() => undefined);
                      }}
                    >
                      {chatDeletingSessionId === session.sessionId ? '...' : 'Delete'}
                    </button>
                    <div
                      className={`item chat-session-item ${
                        selectedChatId === session.sessionId ? 'selected' : ''
                      } ${
                        chatSwipeDraggingSessionId === session.sessionId ? 'swiping' : ''
                      }`}
                      style={{
                        transform: `translateX(${resolveChatSessionSwipeOffset(session.sessionId)}px)`
                      }}
                      onClick={() => {
                        selectChatSession(session.sessionId);
                      }}
                      onTouchStart={event => {
                        beginChatSessionSwipe(event, session.sessionId);
                      }}
                      onTouchMove={event => {
                        moveChatSessionSwipe(event, session.sessionId);
                      }}
                      onTouchEnd={() => {
                        endChatSessionSwipe(session.sessionId);
                      }}
                      onTouchCancel={() => {
                        endChatSessionSwipe(session.sessionId);
                      }}
                    >
                      <span className="file-dot codicon codicon-comment-discussion" />
                      <span className="label chat-session-meta">
                        <span className="chat-session-title">
                          {session.title || session.sessionId}
                        </span>
                        <span className="chat-session-updated muted" title={session.updatedAt || ''}>
                          {formatCompactRelativeAge(session.updatedAt)}
                        </span>
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            ))}
          </div>
        </>
      );
    }
    if (tab === 'file') {
      return (
        <>
          <div className="section-title">EXPLORER</div>
          <div className="list">{renderFileTree('.', 0)}</div>
        </>
      );
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

  const renderSidebar = () => (
    <>
      <div className="sidebar-scroll">
        {sidebarSettingsOpen ? (
          <>
            <div className="section-title">SETTINGS</div>
            <div className="list">
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
                className="sidebar-clear-cache-btn"
                onClick={clearLocalCache}
              >
                Clear Local Cache (Keep Token)
              </button>
            </div>
          </>
        ) : (
          renderSidebarMain()
        )}
      </div>
      <div className="sidebar-footer">
        <button
          type="button"
          className="sidebar-settings-btn"
          onClick={() => setSidebarSettingsOpen(value => !value)}
          title={sidebarSettingsOpen ? 'Back to sidebar' : 'Open settings'}
        >
          <span
            className={`codicon ${
              sidebarSettingsOpen
                ? 'codicon-arrow-left'
                : 'codicon-settings-gear'
            }`}
          />
          <span>{sidebarSettingsOpen ? 'Back' : 'Settings'}</span>
        </button>
      </div>
    </>
  );

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

  const chatPromptGroups = useMemo(
    () => {
      const currentSessionId = chatSelectedIdRef.current;
      const snapshots = currentSessionId
        ? chatPromptSnapshotsRef.current[currentSessionId]
        : undefined;
      return groupChatMessagesByPrompt(chatMessages, snapshots);
    },
    [chatMessages],
  );

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

  const renderedChatPromptGroups = useMemo(
    () =>
      chatPromptGroups.map(group => (
        <ChatPromptGroupView
          key={group.key}
          group={group}
          markdownComponents={chatMarkdownComponents}
          markdownUrlTransform={chatMarkdownUrlTransform}
        />
      )),
    [chatPromptGroups, chatMarkdownComponents, chatMarkdownUrlTransform],
  );
  const renderMain = () => {
    const heavyDiffDeferred =
      !!selectedDiff &&
      isHeavyGeneratedDiffPath(selectedDiff) &&
      !allowHeavyDiffLoad;
    const selectedChatSession = chatSessions.find(
      item => item.sessionId === selectedChatId,
    );

    if (tab === 'chat') {
      return (
        <div className="content">
          <div className="block-title">
            CHAT - {selectedChatSession?.title || 'New Session'}
          </div>
          <div ref={chatScrollRef} className="scroll-panel chat-block">
            {chatLoading ? (
              <div className="muted block">Loading chat...</div>
            ) : null}
            {!chatLoading && chatPromptGroups.length === 0 ? (
              <div className="empty-card">
                <div className="empty-title">Start chatting</div>
                <div className="empty-subtitle">
                  Messages stream here for the current project.
                </div>
              </div>
            ) : null}
            {renderedChatPromptGroups}
          </div>
          <div className="chat-composer">
            <input
              ref={chatFileInputRef}
              type="file"
              accept="image/*"
              style={{ display: 'none' }}
              onChange={event => {
                handleChatFileChange(event).catch(err =>
                  setError(err instanceof Error ? err.message : String(err)),
                );
              }}
            />
            {chatAttachment ? (
              <div className="chat-attachment-pill">{chatAttachment.name}</div>
            ) : null}
            <div className="chat-composer-main">
              <div className="chat-composer-input-shell">
                <textarea
                  ref={chatComposerTextareaRef}
                  rows={1}
                  className={`chat-composer-input${chatComposerText.length === 0 ? ' has-inline-action' : ''}`}
                  value={chatComposerText}
                  onChange={event => setChatComposerText(event.target.value)}
                  placeholder="Send a message..."
                />
                {chatComposerText.length === 0 ? (
                  <div
                    ref={chatComposerActionMenuRef}
                    className={`chat-action-menu chat-action-menu-inline${chatComposerActionMenuOpen ? ' open' : ''}`}
                  >
                    <button
                      type="button"
                      className="chat-action-menu-inline-toggle"
                      onClick={() =>
                        setChatComposerActionMenuOpen(prevOpen => !prevOpen)
                      }
                      title="Media actions"
                      aria-label="Media actions"
                      aria-haspopup="menu"
                      aria-expanded={chatComposerActionMenuOpen}
                    >
                      <span className="codicon codicon-chevron-down" />
                    </button>
                    {chatComposerActionMenuOpen ? (
                      <div className="chat-action-menu-panel" role="menu">
                        <button
                          type="button"
                          className="chat-action-menu-item"
                          role="menuitem"
                          onClick={() => {
                            setChatComposerActionMenuOpen(false);
                            setError('Voice input is not available yet.');
                          }}
                        >
                          <span className="codicon codicon-mic" />
                          <span>Voice</span>
                        </button>
                        <button
                          type="button"
                          className="chat-action-menu-item"
                          role="menuitem"
                          onClick={() => {
                            setChatComposerActionMenuOpen(false);
                            chatFileInputRef.current?.click();
                          }}
                        >
                          <span className="codicon codicon-file-media" />
                          <span>Photo Library</span>
                        </button>
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </div>
              <button
                type="button"
                className="chat-send-button"
                onClick={() => sendChatMessage().catch(() => undefined)}
                title="Send"
                aria-label="Send message"
              >
                <span
                  className={`codicon ${chatSending ? 'codicon-loading codicon-modifier-spin' : 'codicon-send'}`}
                />
              </button>
            </div>
            {(selectedChatSession?.configOptions?.length ?? 0) > 0 ? (
              <div className="chat-config-options">
                {(selectedChatSession?.configOptions ?? [])
                  .map(option => {
                    const optionValues = option.options ?? [];
                    const currentValue =
                      option.currentValue || optionValues[0]?.value || '';
                    const hasChoices = optionValues.length > 0;
                    const updating =
                      chatConfigUpdatingKey ===
                      `${selectedChatSession?.sessionId ?? ''}:${option.id}`;
                    return (
                      <div key={option.id} className="chat-config-item">
                        <select
                          className="chat-config-select"
                          value={currentValue}
                          disabled={updating || !hasChoices}
                          title={option.name || option.id}
                          aria-label={option.name || option.id}
                          onChange={event => {
                            handleChatConfigOptionChange(
                              option,
                              event.target.value,
                            ).catch(() => undefined);
                          }}
                        >
                          {!optionValues.some(item => item.value === currentValue) &&
                          currentValue ? (
                            <option value={currentValue}>{currentValue}</option>
                          ) : null}
                          {optionValues.map(item => (
                            <option
                              key={`${option.id}:${item.value}`}
                              value={item.value}
                            >
                              {item.name || item.value}
                            </option>
                          ))}
                        </select>
                      </div>
                    );
                  })}
              </div>
            ) : null}
            {chatConfigFeedback ? (
              <div className="chat-config-feedback muted">{chatConfigFeedback}</div>
            ) : null}
          </div>
        </div>
      );
    }
    if (tab === 'file') {
      return (
        <div className="content">
          <div className="block-title with-tools file-title-bar">
            <span className="title-text">
              {selectedFile || 'Select a file'}
            </span>
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
          <span className="title-text">
            {selectedDiff || 'Select a changed file'}
          </span>
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
    reconnecting && !!token.trim() && hasCachedWorkspace;

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

  return (
    <div className={`workspace theme-${themeMode}`}>
      <style>{setiFontCss}</style>
      <header className="header">
        <button
          className="header-btn"
          onClick={() => {
            if (isWide) {
              setSidebarCollapsed(value => !value);
            } else {
              setDrawerOpen(value => !value);
            }
          }}
        >
          <span
            className={`codicon ${
              isWide
                ? sidebarCollapsed
                  ? 'codicon-layout-sidebar-left-off'
                  : 'codicon-layout-sidebar-left'
                : 'codicon-menu'
            }`}
          />
        </button>

        <div
          className="project-wrap"
          onPointerDown={event => event.stopPropagation()}
        >
          <button
            className="project-btn"
            onClick={() => setProjectMenuOpen(value => !value)}
          >
            <span className="project-arrow codicon codicon-chevron-down" />
            <span className="project-name" title={currentProjectName}>
              {currentProjectName}
            </span>
            <span
              className={`project-presence ${
                currentProject?.online ? 'online' : 'offline'
              }`}
            />
            {gitDirty ? <span className="project-dirty">dirty</span> : null}
            {loadingProject || refreshingProject || reconnecting ? (
              <span className="muted">...</span>
            ) : null}
          </button>
          {projectMenuOpen ? (
            <div className="project-menu">
              {projects.map(project => (
                <div
                  key={project.projectId}
                  className={`item project-menu-item ${
                    project.projectId === projectId ? 'selected' : ''
                  }`}
                  onClick={() =>
                    switchProject(project.projectId).catch(() => undefined)
                  }
                >
                  <div className="project-menu-main">
                    <span className="project-menu-name">{project.name}</span>
                    <span
                      className="project-menu-path"
                      title={project.path || ''}
                    >
                      {project.path || '-'}
                    </span>
                  </div>
                  <span
                    className={`project-menu-state ${
                      project.online ? 'online' : 'offline'
                    }`}
                  >
                    {project.online ? 'online' : 'offline'}
                  </span>
                  <span className="project-menu-hub">
                    {project.hubId || 'local-hub'}
                  </span>
                </div>
              ))}
            </div>
          ) : null}
        </div>

        <button
          className="header-btn refresh-btn"
          onClick={() => refreshProject().catch(() => undefined)}
          title={reconnecting ? 'Reconnecting...' : 'Refresh project'}
          disabled={refreshingProject || reconnecting}
        >
          {refreshingProject ? (
            '...'
          ) : reconnecting ? (
            <span className="codicon codicon-loading codicon-modifier-spin" />
          ) : (
            <span className="codicon codicon-refresh" />
          )}
        </button>

        <div className="header-spacer" />

        <div className="tabs">
          <button
            className={`tab ${tab === 'chat' ? 'active' : ''}`}
            onClick={() => setTab('chat')}
          >
            <span className="codicon codicon-comment-discussion tab-icon" />
            <span className="tab-label">CHAT</span>
          </button>
          <button
            className={`tab ${tab === 'file' ? 'active' : ''}`}
            onClick={() => setTab('file')}
          >
            <span className="codicon codicon-files tab-icon" />
            <span className="tab-label">FILE</span>
          </button>
          <button
            className={`tab ${tab === 'git' ? 'active' : ''}`}
            onClick={() => setTab('git')}
          >
            <span className="codicon codicon-source-control tab-icon" />
            <span className="tab-label">GIT</span>
          </button>
        </div>
      </header>

      <div className="body">
        {isWide && !sidebarCollapsed ? (
          <aside className="workspace-left">{renderSidebar()}</aside>
        ) : null}
        <main className="workspace-right">{renderMain()}</main>
      </div>

      {tab === 'file' ? (
        <div className="status-bar">
          {selectedFile ? (
            <span className="statusbar-item">
              <span className="codicon codicon-file" />
              <span className="statusbar-text">
                {selectedFile.split('/').pop()}
              </span>
            </span>
          ) : null}
          {selectedFile && fileContent.length > 0 ? (
            <span className="statusbar-muted">
              {fileInfo?.isBinary
                ? `${fileInfo.size ?? 0} bytes`
                : `${
                    fileInfo?.totalLines ?? fileContent.split('\n').length
                  } lines`}
            </span>
          ) : null}
          <span className="statusbar-spacer" />
          <span className="statusbar-muted">
            {gitDirty
              ? `dirty ${gitStatusSummary.staged}/${gitStatusSummary.unstaged}/${gitStatusSummary.untracked}`
              : 'clean'}
          </span>
          {gitCurrentBranch ? (
            <span className="statusbar-item">
              <span className="codicon codicon-git-branch" />
              <span className="statusbar-text">{gitCurrentBranch}</span>
            </span>
          ) : null}
        </div>
      ) : null}

      {!isWide ? (
        <div
          className={`drawer-overlay ${drawerOpen ? 'show' : ''}`}
          onClick={() => setDrawerOpen(false)}
        >
          <aside
            className={`drawer ${drawerOpen ? 'show' : ''}`}
            onClick={event => event.stopPropagation()}
          >
            {renderSidebar()}
          </aside>
        </div>
      ) : null}
    </div>
  );
}

if ('serviceWorker' in navigator && window.isSecureContext) {
  window.addEventListener('load', () => {
    navigator.serviceWorker
      .register('/service-worker.js')
      .then(registration => {
        window.setTimeout(() => {
          registration.update().catch(() => undefined);
        }, 1500);

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

createRoot(document.getElementById('root')!).render(<App />);

