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
import remarkGfm from 'remark-gfm';

declare const require: (id: string) => any;

import { getDefaultRegistryAddress, toRegistryWsUrl } from './runtime';
import { appendPortRelayAutoAuthCode, appendPortRelayOpenPath, parsePortRelayLocalHttpUrl, resolvePortRelayOpenUrl } from './portRelayUrl';
import type { PortRelayLocalHttpUrl } from './portRelayUrl';
import {
  normalizePortRelayListenPort,
  normalizePortRelayTarget,
  normalizePortRelayTargets,
  orderPortRelayTargetsForMenu,
  portRelayTargetKey,
  removePortRelayTarget,
  reconcilePortRelayTargetSelection,
  samePortRelayTarget,
  samePortRelayTargets,
  upsertPortRelayTarget,
  type PortRelayTarget,
} from './portRelayTargets';
import { initializePWAFoundation } from './pwa';
import { DesktopTitleBar } from './shell/DesktopTitleBar';
import { submitDesktopRemoteWebCandidate } from './shell/desktop/webSource';
import { ResponsiveShell } from './shell/ResponsiveShell';
import {
  getLatestSessionReadCursor,
  isFinishedChatMessage,
  needsPromptTurnRefresh,
} from './chatSync';
import { compareUpdatedAtDesc, formatPromptDurationMs } from './sessionTime';
import {
  resolveChatSessionVisualState as resolveChatSessionVisualStateValue,
  type ChatSessionVisualState,
} from './chatSessionState';
import {
  chatSessionKeyFromParts,
  decodeChatSessionKey,
  encodeChatSessionKey,
  type ChatSessionKey,
} from './chat/chatSessionKey';
import {buildMobileChatQuickSwitchSections} from './chat/mobileChatQuickSwitch';
import { resolveChatSessionTitle } from './chat/chatSessionTitle';
import {decodeSessionTurnToMessage, normalizeSessionMessagePayload} from './chat/chatWire';
import {
  applySessionReadResult,
  buildMergedRawTurns,
  createEmptyChatTurnStore,
  hydrateFinishedStore,
  isStaleSessionReadResult,
  mergeRealtimeTurn,
  shouldReadRepairForIncomingTurn,
  type ChatTurnStoreState,
} from './chat/chatTurnStores';
import {createChatDurablePersistQueue} from './chat/chatDurablePersist';
import {createChatReadRepairQueue} from './chat/chatReadRepair';
import {buildChatDisplayIndex} from './chat/chatDisplayIndex';
import {
  buildSessionSearchSections,
  mergeSessionSearchResultsByProject,
  resolveSessionSearchPollDelay,
  splitSessionSearchTitleHighlight,
  type SessionSearchResultsByProjectId,
  type SessionSearchSectionRow,
} from './chat/sessionSearchState';
import {useChatLayoutMetrics} from './chat/chatLayoutMetrics';
import {resolveWideProjectActionPopoverPlacement, type WideProjectActionPopoverPlacement} from './chat/wideProjectActionPopover';
import {ChatVirtuosoTurnList, type ChatVirtuosoTurnListHandle} from './chat/ChatVirtuosoTurnList';
import {
  CHAT_FONT_OPTIONS,
  DEFAULT_CHAT_FONT,
  isChatFontId,
  resolveChatFontFamily,
  type ChatFontId,
} from './chat/chatTypography';
import { buildPromptDoneCopyRange } from './chat/chatCopyRange';
import {
  chatPromptAttachmentLabel,
  chatPromptAttachmentMeta,
  isPromptAttachmentContentBlock,
} from './chat/chatPromptAttachments';
import {
  buildPromptMarkdownImageFileName,
  downloadBlobAsFile,
  renderMarkdownElementToPngBlob,
} from './chatMarkdownImageExport';
import {RegistryDebugPanel} from './debug/RegistryDebugPanel';
import {createRegistryDebugStore} from './debug/registryDebug';
import type {RegistryDebugRecord} from './debug/registryDebug';
import {
  extractChatConfirmationReply,
  extractChatOptionReplies,
  splitChatConfirmationReplyText,
  splitChatOptionReplyText,
  type ChatConfirmationReply,
  type ChatOptionReply,
} from './chat/chatOptionReplies';
import { insertChatSlashCommandText } from './chat/chatSlashInsertion';
import {
  isChatUserScrollLocked,
  nextChatUserScrollLockUntil,
  resolveChatKeyboardInsetScrollAction,
  resolveChatSessionReadWindowUpdate,
  resolveChatScrollToBottomVisibility,
  shouldAutoScrollChatToBottom,
} from './chat/chatScrollIntent';
import { resolvePromptDoneStatus, resolvePromptTurnStatus, type ChatPromptStatus } from './chat/chatPromptStatus';
import { mergeChatSessionList, shouldUpdateCurrentProjectSessions } from './chat/chatIndexState';
import {
  resolveChatListSelection,
  resolveSelectedChatVisibilityRecovery,
  shouldApplyLoadedChatSelection,
  shouldApplyPreservedChatLoad,
  shouldApplySentChatSelection,
} from './chat/chatSelectionGuard';
import { RegistryWorkspaceService } from './services/registryWorkspaceService';
import { sortProjectsByPin, togglePinnedProjectId } from './services/projectNavigation';
import {
  GESTURE_LONG_PRESS_MS,
  GESTURE_MOVE_LONG_PRESS_MS,
  resolveGestureDirectionCandidate,
  resolveGesturePressIntent,
  shouldStartGestureMove,
  type GestureNavigationTab,
} from './services/gestureNavigation';
import {
  createMobileSettingsHistoryState,
  isMobileSettingsHistoryState,
  mobileSettingsHistoryKey,
  resolveMobileSettingsPopAction,
  type MobileSettingsHistoryDetail,
} from './services/mobileSettingsHistory';
import { installMobileViewportZoomGuard } from './services/mobileViewportZoomGuard';
import { resolveLayoutMode } from './services/responsiveLayout';
import {
  buildTokenStatCards,
  type TokenProviderSectionView,
  type TokenStatCardView,
} from './tokenStatsView';
import {
  AGENT_PACKAGE_SCAN_TIMEOUT_MS,
  deriveNpmPackageUpdateTargets,
  deriveRegistryHubIds,
  npmPackageUpdateSummary,
  packageStatusLabel,
  shouldShowWheelMakerUpdateAction,
  wheelMakerUpdateStatusLabel,
  withAgentPackageTimeout,
  type NpmPackageUpdateTarget,
} from './agentPackageUpdateView';
import {
  deriveSkillHubIds,
  groupSkillsByCategory,
  isSkillActionPendingForHub,
  parseSkillSourceInput,
  skillOperationStatusLabel,
  skillScopeLabel,
  sortSkillProjects,
} from './skillManagementView';
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
      RegistrySessionSummary,
      RegistrySessionTurn,
  RegistryFsEntry,
  RegistryFsInfo,
  RegistryGitCommit,
  RegistryGitCommitFile,
  RegistryGitStatus,
  RegistryNpmHubSnapshot,
  RegistryNpmOperation,
  RegistryNpmPackage,
  RegistryHub,
  RegistryProject,
  RegistryPortRelaySnapshot,
  RegistrySkillCommandResponse,
  RegistrySkillScope,
  RegistrySkillSnapshot,
  RegistrySkillSourceCandidate,
  RegistryTokenScanResult,
  RegistryWheelMakerUpdateResponse,
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
  size: number;
  status: 'queued' | 'uploading' | 'failed' | 'completed';
  progress: number;
  file?: File;
  objectUrl?: string;
  block?: RegistryChatContentBlock;
  uploadId?: string;
  attachmentId?: string;
  error?: string;
};
type WideProjectActionMenuState = {
  projectId: string;
  kind: 'new' | 'resume';
  phase: 'agents' | 'sessions';
  agentType: string;
  popover?: WideProjectActionPopoverPlacement | null;
};
type MobileProjectActionMenuState = WideProjectActionMenuState;
type ProjectSessionActionMenuState = {
  projectId: string;
  sessionId: string;
  popover?: WideProjectActionPopoverPlacement | null;
};
type RenameSessionTarget = {
  projectId: string;
  sessionId: string;
  title: string;
};
type ConfirmTarget =
  | {
      kind: 'archive';
      projectId: string;
      sessionId: string;
      title: string;
    }
  | {
      kind: 'delete';
      projectId: string;
      sessionId: string;
      title: string;
    }
  | {kind: 'clearCache'}
  | {
      kind: 'npmPackage';
      action: 'install' | 'update' | 'uninstall';
      hubId: string;
      packageName: string;
      displayName: string;
      installedVersion: string;
      latestVersion: string;
    }
  | {
      kind: 'npmPackageHubUpdate';
      hubId: string;
      packages: NpmPackageUpdateTarget[];
    }
  | {
      kind: 'wheelMakerUpdate';
      hubId: string;
      currentSha: string;
      latestSha: string;
      behindCount: number;
    }
  | {
      kind: 'wheelMakerUpdateAll';
      hubIds: string[];
    }
  | {
      kind: 'skillInstall';
      hubId: string;
      scope: RegistrySkillScope;
      projectName?: string;
      source: string;
      skills: string[];
    }
  | {
      kind: 'skillUninstall';
      hubId: string;
      scope: RegistrySkillScope;
      projectName?: string;
      skillName: string;
    }
  | {
      kind: 'skillUpdate';
      hubId: string;
      scope: RegistrySkillScope;
      projectName?: string;
      includeProjects?: boolean;
    };
type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | 'portRelay' | null;
type ActiveSettingsDetailView = Exclude<SettingsDetailView, null>;
type SettingsDetailShellOptions = {
  hideDetailHeader?: boolean;
};
const SKILLS_MARKETPLACE_URL = 'https://www.skills.sh/';
type WheelMakerUpdateHubView = {
  hubId: string;
  loading: boolean;
  error: string;
  data: RegistryWheelMakerUpdateResponse | null;
};
type AgentPackageHubView = {
  hubId: string;
  loading: boolean;
  error: string;
  updatedAt: string;
  hub: RegistryNpmHubSnapshot | null;
  operation: RegistryNpmOperation | null;
};
type SkillHubView = {
  hubId: string;
  loading: boolean;
  error: string;
  data: RegistrySkillCommandResponse | null;
};
type SkillInstallTarget = {
  hubId: string;
  scope: RegistrySkillScope;
  projectName?: string;
};
type ChatComposerDraft = {
  text: string;
  attachments: ChatAttachment[];
};
type PendingChatPrompt = {
  sessionId: string;
  blocks: RegistryChatContentBlock[];
  createdAt: string;
  turnIndex: number;
  status: 'confirming' | 'undelivered';
  errorMessage?: string;
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
type FloatingDragState = {
  active: boolean;
  pressing: boolean;
  pointerId: number;
  originY: number;
  startTop: number;
  currentTop: number;
  cooldownUntil: number;
};
type PortRelayTargetMenuPressState = {
  pointerId: number;
  originX: number;
  originY: number;
  longPressed: boolean;
};
type ChatQuickSwitchPressState = {
  pointerId: number;
  originX: number;
  originY: number;
  longPressed: boolean;
};
type GestureNavigationState = {
  phase: 'pressing' | 'neutral' | 'expanded';
  pointerId: number;
  originX: number;
  originY: number;
  currentX: number;
  currentY: number;
  startedAt: number;
  candidate: GestureNavigationTab | null;
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
const registryDebugStore = createRegistryDebugStore();
const service = new RegistryWorkspaceService(registryDebugStore.recordCaptureEvent);
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
const CHAT_KEYBOARD_INSET_SETTLE_DELAY_MS = 120;
const CHAT_PENDING_CONFIRM_TIMEOUT_MS = 5000;
const CHAT_ATTACHMENT_CHUNK_SIZE = 1024 * 1024;
const CHAT_CONFIG_PRIORITY_IDS = ['mode', 'model', 'effort'] as const;
const CHAT_CONFIG_PRIORITY_MATCHERS = ['mode', 'model', 'effort', 'thought'] as const;
const CHAT_CONFIG_INLINE_LIMIT = 3;
const fileMemoryCacheKey = (activeProjectId: string, path: string) => `${activeProjectId}\n${path}`;
const WIDE_PROJECT_SESSION_LIMIT = 5;
const PROJECT_PIN_LONG_PRESS_MS = 450;
const PROJECT_SESSION_LONG_PRESS_MS = 450;
const DESKTOP_SIDEBAR_VIEWPORT_MAX_RATIO = 0.45;
const FLOATING_CONTROL_SLOT_ORDER = ['upper', 'upper-middle', 'center', 'lower-middle'] as const;
const PORT_RELAY_FLOATING_SLOT_STORAGE_KEY = 'wheelmaker:portRelayFloatingSlot';
const EMPTY_CHAT_COMPOSER_DRAFT: ChatComposerDraft = { text: '', attachments: [] };
const DEFAULT_PORT_RELAY_SNAPSHOT: RegistryPortRelaySnapshot = {ok: true, enabled: false, status: 'Disabled'};
let mermaidRenderSequence = 0;
let mermaidModulePromise: Promise<typeof import('mermaid').default> | null = null;

function isRegistryChatContentBlock(block: RegistryChatContentBlock | undefined): block is RegistryChatContentBlock {
  return !!block && typeof block.type === 'string' && block.type.length > 0;
}

function isChatAttachmentUploadPending(attachment: ChatAttachment): boolean {
  return attachment.status === 'uploading';
}

function chatAttachmentPreviewSrc(attachment: ChatAttachment): string {
  if (attachment.objectUrl) {
    return attachment.objectUrl;
  }
  const data = attachment.block?.data;
  if (attachment.block?.type === 'image' && data) {
    return `data:${attachment.mimeType || 'image/png'};base64,${data}`;
  }
  return '';
}

function attachmentIdFromBlock(block: RegistryChatContentBlock | undefined): string {
  const uri = block?.uri || '';
  if (!uri) {
    return '';
  }
  const last = uri.split(/[\\/]/).pop() || '';
  const withoutQuery = last.split(/[?#]/)[0] || '';
  const withoutExtension = withoutQuery.replace(/\.[^.]+$/, '');
  return withoutExtension.startsWith('sha256-') ? withoutExtension : '';
}

function revokeChatAttachmentObjectUrl(attachment: ChatAttachment): void {
  if (attachment.objectUrl) {
    URL.revokeObjectURL(attachment.objectUrl);
  }
}

function chatFilesFromDataTransferItems(items: DataTransferItemList | DataTransferItem[] | undefined | null): File[] {
  return Array.from(items ?? [])
    .filter(item => item.kind === 'file')
    .map(item => item.getAsFile())
    .filter((file): file is File => !!file);
}

function chatFilesFromFileList(files: FileList | File[] | undefined | null): File[] {
  return Array.from(files ?? []).filter((file): file is File => !!file);
}

function chatFallbackAttachmentName(index: number): string {
  return `attachment-${index + 1}`;
}

function formatChatAttachmentSize(size: number): string {
  if (!Number.isFinite(size) || size <= 0) {
    return '0 B';
  }
  if (size < 1024) {
    return `${Math.round(size)} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

async function blobToBase64(blob: Blob): Promise<string> {
  const dataUrl = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
    reader.onerror = () => reject(reader.error ?? new Error('Failed to read attachment chunk'));
    reader.readAsDataURL(blob);
  });
  return dataUrl.includes(',') ? dataUrl.slice(dataUrl.indexOf(',') + 1) : dataUrl;
}

async function sha256Hex(file: File): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', await file.arrayBuffer());
  return Array.from(new Uint8Array(digest))
    .map(value => value.toString(16).padStart(2, '0'))
    .join('');
}

type MarkdownRemarkPlugins = NonNullable<React.ComponentProps<typeof ReactMarkdown>['remarkPlugins']>;
type MarkdownRehypePlugins = NonNullable<React.ComponentProps<typeof ReactMarkdown>['rehypePlugins']>;
type MarkdownMathPipeline = {
  remarkPlugins: MarkdownRemarkPlugins;
  rehypePlugins: MarkdownRehypePlugins;
};
type MarkdownCapabilityPlugins = MarkdownMathPipeline & {
  pending: boolean;
};

let markdownMathPipeline: MarkdownMathPipeline | null = null;
let markdownMathPipelinePromise: Promise<MarkdownMathPipeline> | null = null;
const markdownMathPattern = /\\\(|\\\[|\$\$|(?:^|[^\\])\$(?:[^\s$\\]|[^\s$][^$\n]*[^\s$])\$/m;

function nextMermaidRenderId(): string {
  mermaidRenderSequence += 1;
  return `wm-mermaid-${mermaidRenderSequence}`;
}

function loadMermaid(): Promise<typeof import('mermaid').default> {
  if (!mermaidModulePromise) {
    mermaidModulePromise = import('mermaid')
      .then(module => module.default)
      .catch(error => {
        mermaidModulePromise = null;
        throw error;
      });
  }
  return mermaidModulePromise;
}

function markdownNeedsMath(content: string): boolean {
  return markdownMathPattern.test(content);
}

function loadMarkdownMathPipeline(): Promise<MarkdownMathPipeline> {
  if (markdownMathPipeline) {
    return Promise.resolve(markdownMathPipeline);
  }
  if (!markdownMathPipelinePromise) {
    markdownMathPipelinePromise = Promise.all([
      import('remark-math'),
      import('rehype-katex'),
      import('katex/dist/katex.min.css'),
    ])
      .then(([remarkMathModule, rehypeKatexModule]) => {
        markdownMathPipeline = {
          remarkPlugins: [remarkMathModule.default],
          rehypePlugins: [rehypeKatexModule.default],
        };
        return markdownMathPipeline;
      })
      .catch(error => {
        markdownMathPipelinePromise = null;
        throw error;
      });
  }
  return markdownMathPipelinePromise;
}

function useMarkdownCapabilityPlugins(content: string): MarkdownCapabilityPlugins {
  const needsMath = useMemo(() => markdownNeedsMath(content), [content]);
  const [loadedMathPipeline, setLoadedMathPipeline] = useState<MarkdownMathPipeline | null>(() =>
    needsMath ? markdownMathPipeline : null,
  );

  useEffect(() => {
    let cancelled = false;
    if (!needsMath) {
      setLoadedMathPipeline(null);
      return () => {
        cancelled = true;
      };
    }
    if (markdownMathPipeline) {
      setLoadedMathPipeline(markdownMathPipeline);
      return () => {
        cancelled = true;
      };
    }
    loadMarkdownMathPipeline()
      .then(pipeline => {
        if (!cancelled) {
          setLoadedMathPipeline(pipeline);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setLoadedMathPipeline(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [needsMath]);

  const activeMathPipeline = needsMath
    ? loadedMathPipeline ?? markdownMathPipeline
    : null;

  return useMemo(
    () => ({
      remarkPlugins: activeMathPipeline
        ? [remarkGfm, ...activeMathPipeline.remarkPlugins]
        : [remarkGfm],
      rehypePlugins: activeMathPipeline ? activeMathPipeline.rehypePlugins : [],
      pending: needsMath && !activeMathPipeline,
    }),
    [activeMathPipeline, needsMath],
  );
}

function buildChatDraftKey(activeProjectId: string, sessionId: string): string {
  const projectKey = activeProjectId.trim() || CHAT_DRAFT_KEY_PROJECT_FALLBACK;
  const sessionKey = sessionId.trim() || CHAT_NEW_DRAFT_SESSION_KEY;
  return `${projectKey}:${sessionKey}`;
}

function generatePortRelayAccessCode(): string {
  const crypto = globalThis.crypto;
  if (crypto?.getRandomValues) {
    const bytes = new Uint8Array(4);
    crypto.getRandomValues(bytes);
    const value = ((bytes[0] << 24) >>> 0) + (bytes[1] << 16) + (bytes[2] << 8) + bytes[3];
    return String(value % 1_000_000).padStart(6, '0');
  }
  return String(Math.floor(Math.random() * 1_000_000)).padStart(6, '0');
}

function agentPackageActionForPackage(pkg: RegistryNpmPackage): 'install' | 'update' | 'uninstall' | null {
  if (pkg.canInstall) return 'install';
  if (pkg.canUpdate) return 'update';
  if (pkg.canUninstall) return 'uninstall';
  return null;
}

function agentPackageActionLabel(action: 'install' | 'update' | 'uninstall'): string {
  switch (action) {
    case 'update':
      return 'Update';
    case 'uninstall':
      return 'Uninstall';
    default:
      return 'Install';
  }
}

function skillActionPendingKey(input: {hubId: string; scope: RegistrySkillScope; projectName?: string; skillName?: string; action: string}): string {
  return [
    input.hubId,
    input.scope,
    input.projectName || '',
    input.skillName || '',
    input.action,
  ].join(':');
}

function sameSkillInstallTarget(left: SkillInstallTarget | null, right: SkillInstallTarget): boolean {
  return !!left &&
    left.hubId === right.hubId &&
    left.scope === right.scope &&
    (left.projectName || '') === (right.projectName || '');
}

function skillCommandErrorMessage(result: RegistrySkillCommandResponse): string {
  return result.errorSummary || result.message || 'Skill operation failed.';
}

function shortGitSha(value: string): string {
  const trimmed = value.trim();
  return trimmed.length > 7 ? trimmed.slice(0, 7) : trimmed || '-';
}

function wheelMakerBehindCopy(data: RegistryWheelMakerUpdateResponse | null): string {
  if (!data?.release) return 'Unknown';
  const behind = data.git?.behindCount ?? 0;
  if (behind <= 0) return 'Up to date';
  return `${behind} ${behind === 1 ? 'commit' : 'commits'} behind`;
}

function wheelMakerReleaseRef(data: RegistryWheelMakerUpdateResponse | null): string {
  const remote = data?.release?.remote || data?.git?.remote || 'origin';
  const branch = data?.release?.branch || data?.git?.branch || '';
  return branch ? `${remote}/${branch}` : remote;
}

function formatWheelMakerDateTime(value: string): string {
  if (!value) return '-';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString([], {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
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

function isFloatingControlSlot(value: unknown): value is PersistedFloatingControlSlot {
  return value === 'upper' ||
    value === 'upper-middle' ||
    value === 'center' ||
    value === 'lower-middle';
}

function readPortRelayFloatingSlot(): PersistedFloatingControlSlot | null {
  try {
    const value = window.localStorage.getItem(PORT_RELAY_FLOATING_SLOT_STORAGE_KEY);
    return isFloatingControlSlot(value) ? value : null;
  } catch {
    return null;
  }
}

function tabIconClass(tab: GestureNavigationTab): string {
  switch (tab) {
    case 'chat':
      return 'codicon-comment-discussion';
    case 'git':
      return 'codicon-source-control';
    default:
      return 'codicon-files';
  }
}

function gestureTabFromElement(element: Element | null): GestureNavigationTab | null {
  const target = element?.closest<HTMLElement>('[data-gesture-nav-tab]');
  const tab = target?.dataset.gestureNavTab;
  return tab === 'chat' || tab === 'file' || tab === 'git' ? tab : null;
}

function settingsDetailTitle(detail: ActiveSettingsDetailView): string {
  switch (detail) {
    case 'update':
      return 'Update';
    case 'skills':
      return 'Skills';
    case 'tokenStats':
      return 'Token Stats';
    case 'ccSwitch':
      return 'CC Switch';
    case 'database':
      return 'Database';
    case 'portRelay':
      return 'Port Relay';
  }
}

const AGENT_TAG_VARIANT_INDEX: Record<string, number> = {
  codex: 0,
  copilot: 1,
  claude: 2,
  opencode: 3,
  codebuddy: 4,
};

function normalizeAgentTypeName(value?: string | null): string {
  return (value || '').trim();
}

function tagVariantClass(prefix: string, value: string): string {
  const normalized = normalizeAgentTypeName(value).toLowerCase();
  if (prefix === 'wide-session-agent' || prefix === 'token-stats-pill-agent') {
    const explicitIndex = AGENT_TAG_VARIANT_INDEX[normalized];
    if (typeof explicitIndex === 'number') {
      return `${prefix}-${explicitIndex}`;
    }
  }

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

function mergeKnownChatSessions(
  left: RegistryChatSession[],
  right: RegistryChatSession[],
): RegistryChatSession[] {
  let merged = left;
  for (const session of right) {
    merged = mergeChatSession(merged, session);
  }
  return merged;
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
      pendingStatus: prompt.status,
      message: prompt.errorMessage,
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
  const normalized = normalizeSessionMessagePayload(payload);
  if (!normalized) return null;
  return decodeSessionTurnToMessage(normalized.sessionId, normalized.turn);
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
  const markdownCapabilities = useMarkdownCapabilityPlugins(text);
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
        <div
          className="chat-thought-content"
          data-markdown-export-pending={markdownCapabilities.pending ? 'true' : undefined}
        >
          <ReactMarkdown
            remarkPlugins={markdownCapabilities.remarkPlugins}
            urlTransform={markdownUrlTransform}
            rehypePlugins={markdownCapabilities.rehypePlugins}
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

function groupPromptAttachmentBlocks(msgs: RegistryChatMessage[]): RegistrySessionContentBlock[] {
  const blocks: RegistrySessionContentBlock[] = [];
  for (const m of msgs) {
    for (const b of msgBlocks(m.method, m.param)) {
      if (isPromptAttachmentContentBlock(b)) {
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
  onExportPromptDoneImage?: () => void;
  optionReplies?: ChatOptionReply[];
  optionRepliesDisabled?: boolean;
  onSelectOptionReply?: (label: string) => void;
  confirmationReply?: ChatConfirmationReply | null;
  onSelectConfirmationReply?: (replyText: string) => void;
  onRetryPendingPrompt?: () => void;
  onEditPendingPrompt?: () => void;
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
  onExportPromptDoneImage,
  optionReplies = [],
  optionRepliesDisabled = false,
  onSelectOptionReply,
  confirmationReply = null,
  onSelectConfirmationReply,
  onRetryPendingPrompt,
  onEditPendingPrompt,
}: ChatTurnViewProps) {
  const text = msgText(message.method, message.param).trim();
  const kind = msgKind(message.method);
  const markdownCapabilities = useMarkdownCapabilityPlugins(text);

  if (message.method === 'prompt_request' || message.method === 'user_message_chunk') {
    const imageBlocks = groupImageBlocks([message]);
    const attachmentBlocks = groupPromptAttachmentBlocks([message]);
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
            {promptStatus === 'confirming' ? (
              <span className="chat-prompt-status chat-prompt-status-confirming" title="Sending">
                <span className="codicon codicon-sync" aria-hidden="true" />
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
        {attachmentBlocks.length > 0 ? (
          <div className="chat-prompt-attachment-strip">
            {attachmentBlocks.map((block, index) => {
              const label = chatPromptAttachmentLabel(block, index);
              const meta = chatPromptAttachmentMeta(block);
              return (
                <div
                  key={`${message.sessionId}:${message.turnIndex}:attachment:${index}`}
                  className={`chat-prompt-attachment-chip ${block.type === 'image' ? 'image' : 'file'}`}
                  title={meta ? `${label} | ${meta}` : label}
                >
                  <span
                    className={`codicon ${block.type === 'image' ? 'codicon-file-media' : 'codicon-file'} chat-prompt-attachment-icon`}
                    aria-hidden="true"
                  />
                  <span className="chat-prompt-attachment-body">
                    <span className="chat-prompt-attachment-name">{label}</span>
                    {meta ? (
                      <span className="chat-prompt-attachment-meta">{meta}</span>
                    ) : null}
                  </span>
                </div>
              );
            })}
          </div>
        ) : null}
        {promptStatus === 'undelivered' ? (
          <div className="chat-prompt-delivery-line">
            <span>Not delivered</span>
            <button type="button" onClick={() => onRetryPendingPrompt?.()}>
              Retry
            </button>
            <button type="button" onClick={() => onEditPendingPrompt?.()}>
              Edit
            </button>
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
    const doneStatus = resolvePromptDoneStatus(message.param);
    return (
      <div className="chat-prompt-separator">
        <hr />
        <span className="chat-prompt-separator-label">
          By {modelName || 'unknown'}
          {durationMs > 0 ? ` · ${formatPromptDurationMs(durationMs)}` : ''}
          {doneStatus ? (
            <span className={`chat-prompt-stop-reason ${doneStatus.kind}`}>
              {doneStatus.label}
            </span>
          ) : null}
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
          <button
            type="button"
            className="chat-prompt-action-button"
            onClick={() => onExportPromptDoneImage?.()}
            disabled={copyDisabled}
            title="Export response image"
            aria-label="Export response markdown image"
          >
            <span className="codicon codicon-device-camera" />
          </button>
        </div>
        {doneStatus ? (
          <div className={`chat-prompt-result-line ${doneStatus.kind}`}>
            {doneStatus.message || `Response ${doneStatus.label.toLowerCase()}.`}
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
  const optionReplyParts = splitChatOptionReplyText(text);
  const confirmationReplyParts = splitChatConfirmationReplyText(text);
  const hasOptionReplyParts = optionReplyParts.some(part => part.type === 'option');
  const selectableOptionReplies = optionReplies.length > 0;
  const selectableConfirmationReply = optionReplies.length === 0 ? confirmationReply : null;
  const hasConfirmationReplyParts =
    !!selectableConfirmationReply &&
    !hasOptionReplyParts &&
    confirmationReplyParts.some(part => part.type === 'confirmation');
  return (
    <div
      className="chat-main-message"
      data-markdown-export-pending={markdownCapabilities.pending ? 'true' : undefined}
    >
      {hasOptionReplyParts ? (
        optionReplyParts.map((part, index) => {
          if (part.type === 'markdown') {
            return part.text ? (
              <ReactMarkdown
                key={`markdown:${index}`}
                remarkPlugins={markdownCapabilities.remarkPlugins}
                urlTransform={markdownUrlTransform}
                rehypePlugins={markdownCapabilities.rehypePlugins}
                components={markdownComponents}
              >
                {part.text}
              </ReactMarkdown>
            ) : null;
          }
          const optionContent = (
            <>
              <span className="chat-option-reply-label">{part.reply.label}.</span>
              <span className="chat-option-reply-text">{part.reply.text}</span>
            </>
          );
          return (
            <div key={`option:${part.reply.label}:${index}`} className="chat-option-reply-line">
              {selectableOptionReplies ? (
                <button
                  type="button"
                  className="chat-option-reply-inline-button"
                  onClick={() => onSelectOptionReply?.(part.reply.label)}
                  disabled={optionRepliesDisabled}
                  title={part.reply.text}
                  aria-label={`Reply ${part.reply.label}: ${part.reply.text}`}
                >
                  {optionContent}
                </button>
              ) : (
                <div className="chat-option-reply-static" title={part.reply.text}>
                  {optionContent}
                </div>
              )}
            </div>
          );
        })
      ) : hasConfirmationReplyParts ? (
        confirmationReplyParts.map((part, index) => {
          if (part.type === 'markdown') {
            return part.text ? (
              <ReactMarkdown
                key={`markdown:${index}`}
                remarkPlugins={markdownCapabilities.remarkPlugins}
                urlTransform={markdownUrlTransform}
                rehypePlugins={markdownCapabilities.rehypePlugins}
                components={markdownComponents}
              >
                {part.text}
              </ReactMarkdown>
            ) : null;
          }
          return (
            <div key={`confirmation:${index}`} className="chat-confirmation-reply-line">
              <button
                type="button"
                className="chat-confirmation-reply-action"
                onClick={() => onSelectConfirmationReply?.(part.reply.replyText)}
                disabled={optionRepliesDisabled}
                title={part.reply.replyText}
                aria-label={`Reply ${part.reply.replyText}: ${part.reply.sentence}`}
              >
                <span className="chat-confirmation-reply-check" aria-hidden="true">
                  <span className="codicon codicon-check" />
                </span>
                <span className="chat-confirmation-reply-text">{part.reply.sentence}</span>
              </button>
            </div>
          );
        })
      ) : (
        <ReactMarkdown
          remarkPlugins={markdownCapabilities.remarkPlugins}
          urlTransform={markdownUrlTransform}
          rehypePlugins={markdownCapabilities.rehypePlugins}
          components={markdownComponents}
        >
          {text}
        </ReactMarkdown>
      )}
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
    return !!text ||
      !!promptStatus ||
      groupImageBlocks([message]).length > 0 ||
      groupPromptAttachmentBlocks([message]).length > 0;
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

function isHtmlPath(path: string): boolean {
  const ext = getFileExtension(path);
  return ext === 'html' || ext === 'htm';
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
      data-markdown-export-pending={html ? undefined : 'true'}
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
        const mermaid = await loadMermaid();
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
    return (
      <div className="muted block" data-markdown-export-pending="true">
        Rendering mermaid diagram...
      </div>
    );
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

type HtmlPreviewProps = {
  content: string;
  scriptsEnabled: boolean;
};

type MarkdownImageExportRequest = {
  id: number;
  content: string;
  fileName: string;
};

type MarkdownImageExportSurfaceProps = {
  request: MarkdownImageExportRequest;
  markdownComponents: Components;
  markdownUrlTransform: (value: string) => string;
  onComplete: () => void;
  onError: (message: string) => void;
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
  const markdownCapabilities = useMarkdownCapabilityPlugins(content);

  return (
    <div
      className="markdown-preview"
      data-markdown-export-pending={markdownCapabilities.pending ? 'true' : undefined}
    >
      <ReactMarkdown
        remarkPlugins={markdownCapabilities.remarkPlugins}
        rehypePlugins={markdownCapabilities.rehypePlugins}
        components={markdownComponents}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}, markdownPreviewPropsEqual);

const HtmlPreview = React.memo(function HtmlPreview({
  content,
  scriptsEnabled,
}: HtmlPreviewProps) {
  return (
    <div className="html-preview">
      <iframe
        className="html-preview-frame"
        title="HTML preview"
        sandbox={scriptsEnabled ? 'allow-scripts' : ''}
        srcDoc={content}
      />
    </div>
  );
});

const MarkdownImageExportSurface = React.memo(function MarkdownImageExportSurface({
  request,
  markdownComponents,
  markdownUrlTransform,
  onComplete,
  onError,
}: MarkdownImageExportSurfaceProps) {
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const markdownCapabilities = useMarkdownCapabilityPlugins(request.content);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const surface = surfaceRef.current;
      if (!surface) {
        return;
      }
      try {
        const backgroundColor = getComputedStyle(surface).backgroundColor || '#ffffff';
        const blob = await renderMarkdownElementToPngBlob(surface, {
          backgroundColor,
        });
        if (cancelled) {
          return;
        }
        downloadBlobAsFile(blob, request.fileName);
        onComplete();
      } catch (err) {
        if (!cancelled) {
          onError(err instanceof Error ? err.message : String(err));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [
    request.id,
    request.fileName,
    markdownComponents,
    markdownUrlTransform,
    onComplete,
    onError,
  ]);

  return (
    <div className="markdown-image-export-host" aria-hidden="true">
      <div
        ref={surfaceRef}
        className="markdown-image-export-surface markdown-preview"
        data-markdown-export-pending={markdownCapabilities.pending ? 'true' : undefined}
      >
        <ReactMarkdown
          remarkPlugins={markdownCapabilities.remarkPlugins}
          urlTransform={markdownUrlTransform}
          rehypePlugins={markdownCapabilities.rehypePlugins}
          components={markdownComponents}
        >
          {request.content}
        </ReactMarkdown>
      </div>
    </div>
  );
});
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
  const [chatFont, setChatFont] = useState<ChatFontId>(
    typeof persistedGlobal.chatFont === 'string' &&
      isChatFontId(persistedGlobal.chatFont)
      ? persistedGlobal.chatFont
      : DEFAULT_CHAT_FONT,
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
      : true,
  );
  const [registryDebug, setRegistryDebug] = useState(
    typeof persistedGlobal.registryDebug === 'boolean'
      ? persistedGlobal.registryDebug
      : false,
  );
  const [localHubReadEnabled, setLocalHubReadEnabled] = useState(
    typeof persistedGlobal.localHubReadEnabled === 'boolean'
      ? persistedGlobal.localHubReadEnabled
      : true,
  );
  const [registryDebugPanelOpen, setRegistryDebugPanelOpen] = useState(
    typeof persistedGlobal.registryDebug === 'boolean'
      ? persistedGlobal.registryDebug
      : false,
  );
  const [registryDebugRecords, setRegistryDebugRecords] = useState(registryDebugStore.getRecords());
  const [selectedRegistryDebugRecordId, setSelectedRegistryDebugRecordId] = useState<number | null>(null);
  const [selectedRegistryDebugScope, setSelectedRegistryDebugScope] = useState('All');
  const [selectedRegistryDebugSessionId, setSelectedRegistryDebugSessionId] = useState('All');
  const [registryDebugIncludeMultiSessionRecords, setRegistryDebugIncludeMultiSessionRecords] = useState(false);
  const [gestureNavigation, setGestureNavigation] = useState(
    typeof persistedGlobal.gestureNavigation === 'boolean'
      ? persistedGlobal.gestureNavigation
      : false,
  );
  const codeFontFamily = useMemo(
    () => resolveCodeFontFamily(codeFont),
    [codeFont],
  );
  const chatFontFamily = useMemo(
    () => resolveChatFontFamily(chatFont),
    [chatFont],
  );
  const setiFontCss = useMemo(() => setiFontFaceCss(), []);
  const resolveFileIcon = (name: string) => resolveSetiIcon(name, themeMode);

  const [windowWidth, setWindowWidth] = useState<number>(window.innerWidth);
  const [windowHeight, setWindowHeight] = useState<number>(window.innerHeight);
  const [safeAreaTopInset, setSafeAreaTopInset] = useState<number>(() => readSafeAreaTopInset());
  const layoutMode = resolveLayoutMode(windowWidth);
  const isWide = layoutMode === 'desktop';
  const supportsChatClipboardFiles = useMemo(() => {
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
        floatingControlSlot: globalState.floatingControlSlot ?? readPortRelayFloatingSlot() ?? 'upper-middle',
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
  const chatKeyboardInsetRef = useRef(chatKeyboardInset);
  const chatKeyboardInsetSettleTimerRef = useRef<number | null>(null);
  const tabRef = useRef<Tab>(tab);
  const floatingDragStateRef = useRef<FloatingDragState | null>(null);
  const [gestureNavState, setGestureNavState] = useState<GestureNavigationState | null>(null);
  const gestureNavStateRef = useRef<GestureNavigationState | null>(null);
  const gestureLongPressTimerRef = useRef<number | null>(null);
  const gestureMoveLongPressTimerRef = useRef<number | null>(null);
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
  const mobileSettingsHistoryKeyRef = useRef<string | null>(null);
  const sidebarSettingsOpenRef = useRef(sidebarSettingsOpen);
  const settingsDetailViewRef = useRef<SettingsDetailView>(settingsDetailView);
  const [desktopSidebarResizing, setDesktopSidebarResizing] = useState(false);
  const [desktopSidebarDraftWidth, setDesktopSidebarDraftWidth] = useState<number | null>(null);
  const [tokenStatsLoading, setTokenStatsLoading] = useState(false);
  const [tokenStatsError, setTokenStatsError] = useState('');
  const [tokenStatsUpdatedAt, setTokenStatsUpdatedAt] = useState('');
  const [tokenStatsProviders, setTokenStatsProviders] = useState<TokenProviderSectionView[]>([]);
  const [wheelMakerUpdateHubs, setWheelMakerUpdateHubs] = useState<Record<string, WheelMakerUpdateHubView>>({});
  const [wheelMakerUpdatesLoading, setWheelMakerUpdatesLoading] = useState(false);
  const [wheelMakerUpdatesError, setWheelMakerUpdatesError] = useState('');
  const [wheelMakerUpdatePendingHubId, setWheelMakerUpdatePendingHubId] = useState('');
  const [wheelMakerUpdateAllPending, setWheelMakerUpdateAllPending] = useState(false);
  const [agentPackageHubs, setAgentPackageHubs] = useState<Record<string, AgentPackageHubView>>({});
  const [agentPackagesLoading, setAgentPackagesLoading] = useState(false);
  const [agentPackagesError, setAgentPackagesError] = useState('');
  const [agentPackageActionPendingKey, setAgentPackageActionPendingKey] = useState('');
  const [agentPackageHubUpdatePendingId, setAgentPackageHubUpdatePendingId] = useState('');
  const [expandedNpmUpdateHubIds, setExpandedNpmUpdateHubIds] = useState<Record<string, boolean>>({});
  const agentPackageScanPollTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);
  const [skillHubs, setSkillHubs] = useState<Record<string, SkillHubView>>({});
  const [skillsLoading, setSkillsLoading] = useState(false);
  const [skillsError, setSkillsError] = useState('');
  const [skillsPendingKey, setSkillsPendingKey] = useState('');
  const skillOperationPollTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);
  const skillOperationPollHubIdsRef = useRef<Set<string>>(new Set());
  const refreshSkillManagementHubRef = useRef<((hubId: string) => Promise<void>) | null>(null);
  const [skillInstallTarget, setSkillInstallTarget] = useState<SkillInstallTarget | null>(null);
  const [skillSourceInput, setSkillSourceInput] = useState('');
  const [skillSourceCandidates, setSkillSourceCandidates] = useState<RegistrySkillSourceCandidate[]>([]);
  const [skillSourceSelectedNames, setSkillSourceSelectedNames] = useState<string[]>([]);
  const [skillSourceLoading, setSkillSourceLoading] = useState(false);
  const [skillSourceError, setSkillSourceError] = useState('');
  const [portRelaySnapshot, setPortRelaySnapshot] = useState<RegistryPortRelaySnapshot>(DEFAULT_PORT_RELAY_SNAPSHOT);
  const [portRelayLoading, setPortRelayLoading] = useState(false);
  const [portRelayError, setPortRelayError] = useState('');
  const [portRelayListenPort, setPortRelayListenPort] = useState(String(persistedGlobal.portRelayListenPort || 28810));
  const [portRelayTargets, setPortRelayTargets] = useState<PortRelayTarget[]>(
    normalizePortRelayTargets(persistedGlobal.portRelayTargets),
  );
  const [selectedPortRelayTarget, setSelectedPortRelayTarget] = useState<PortRelayTarget | null>(
    normalizePortRelayTarget(persistedGlobal.selectedPortRelayTarget),
  );
  const [portRelayDraftHubId, setPortRelayDraftHubId] = useState('');
  const [portRelayDraftPort, setPortRelayDraftPort] = useState('80');
  const [portRelayAccessCode, setPortRelayAccessCode] = useState('');
  const [portRelayKnownAccessCodeGeneration, setPortRelayKnownAccessCodeGeneration] = useState<number | null>(null);
  const [portRelayCodeCopied, setPortRelayCodeCopied] = useState(false);
  const [portRelayFrameOpen, setPortRelayFrameOpen] = useState(false);
  const [portRelayFramePath, setPortRelayFramePath] = useState('');
  const [portRelayFrameAutoOpenPending, setPortRelayFrameAutoOpenPending] = useState(false);
  const [portRelayTargetMenuOpen, setPortRelayTargetMenuOpen] = useState(false);
  const [portRelayMenuSwitchingTarget, setPortRelayMenuSwitchingTarget] = useState<PortRelayTarget | null>(null);
  const portRelayCodeCopyTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);
  const portRelayTargetMenuTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);
  const portRelayTargetMenuPressRef = useRef<PortRelayTargetMenuPressState | null>(null);
  const portRelayTargetMenuRef = useRef<HTMLDivElement | null>(null);
  const portRelayReady = portRelaySnapshot.enabled && portRelaySnapshot.status === 'Up';
  const portRelayAccessCodeUnknown = portRelaySnapshot.enabled && (
    !portRelayAccessCode ||
    (
      typeof portRelaySnapshot.accessCodeGeneration === 'number' &&
      portRelayKnownAccessCodeGeneration !== portRelaySnapshot.accessCodeGeneration
    )
  );
  const portRelayFrameAccessCode = portRelayAccessCodeUnknown ? '' : portRelayAccessCode;
  const portRelayFrameUrl = useMemo(() => {
    if (!portRelayReady) {
      return '';
    }
    const baseUrl = resolvePortRelayOpenUrl({
      relayUrl: portRelaySnapshot.relayUrl,
      registryAddress: address,
      listenPort: portRelaySnapshot.listenPort || portRelayListenPort,
    });
    return appendPortRelayAutoAuthCode(
      appendPortRelayOpenPath(baseUrl, portRelayFramePath),
      portRelayFrameAccessCode,
    );
  }, [address, portRelayFrameAccessCode, portRelayFramePath, portRelayListenPort, portRelayReady, portRelaySnapshot.listenPort, portRelaySnapshot.relayUrl]);
  const snapshotPortRelayTarget = useMemo(() => normalizePortRelayTarget({
    hubId: portRelaySnapshot.hubId,
    targetPort: portRelaySnapshot.targetPort,
  }), [portRelaySnapshot.hubId, portRelaySnapshot.targetPort]);
  const activePortRelayTarget = portRelaySnapshot.enabled
    ? snapshotPortRelayTarget ?? selectedPortRelayTarget
    : selectedPortRelayTarget ?? snapshotPortRelayTarget;
  const portRelayTargetMenuTargets = useMemo(
    () => orderPortRelayTargetsForMenu(portRelayTargets, activePortRelayTarget),
    [activePortRelayTarget, portRelayTargets],
  );
  const mobilePortRelayFrameOpen = !isWide && portRelayFrameOpen && !!portRelayFrameUrl;

  useEffect(() => {
    if (!portRelayReady || !portRelayFrameUrl) {
      setPortRelayFrameOpen(false);
    }
  }, [portRelayFrameUrl, portRelayReady]);

  useEffect(() => {
    if (!portRelayFrameAutoOpenPending) {
      return;
    }
    if (portRelayReady && portRelayFrameUrl) {
      setPortRelayFrameOpen(true);
      setPortRelayFrameAutoOpenPending(false);
      return;
    }
    if (!portRelaySnapshot.enabled || portRelaySnapshot.status === 'Error') {
      setPortRelayFrameAutoOpenPending(false);
    }
  }, [
    portRelayFrameAutoOpenPending,
    portRelayFrameUrl,
    portRelayReady,
    portRelaySnapshot.enabled,
    portRelaySnapshot.status,
  ]);

  useEffect(() => {
    if (!mobilePortRelayFrameOpen) {
      return;
    }
    setDrawerOpen(false);
    setSidebarSettingsOpen(false);
  }, [mobilePortRelayFrameOpen, setDrawerOpen, setSidebarSettingsOpen]);

  const clearPortRelayTargetMenuTimer = useCallback(() => {
    if (portRelayTargetMenuTimerRef.current) {
      window.clearTimeout(portRelayTargetMenuTimerRef.current);
      portRelayTargetMenuTimerRef.current = null;
    }
  }, []);

  useEffect(() => () => {
    clearPortRelayTargetMenuTimer();
  }, [clearPortRelayTargetMenuTimer]);

  useEffect(() => {
    if (!mobilePortRelayFrameOpen) {
      setPortRelayTargetMenuOpen(false);
      setPortRelayMenuSwitchingTarget(null);
    }
  }, [mobilePortRelayFrameOpen]);

  useEffect(() => {
    if (!portRelayTargetMenuOpen) {
      return;
    }
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target instanceof Node ? event.target : null;
      if (target && portRelayTargetMenuRef.current?.contains(target)) {
        return;
      }
      setPortRelayTargetMenuOpen(false);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setPortRelayTargetMenuOpen(false);
      }
    };
    window.addEventListener('pointerdown', handlePointerDown);
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('pointerdown', handlePointerDown);
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [portRelayTargetMenuOpen]);

  useEffect(() => () => {
    if (portRelayCodeCopyTimerRef.current) {
      window.clearTimeout(portRelayCodeCopyTimerRef.current);
    }
  }, []);

  const [projectMenuOpen, setProjectMenuOpen] = useState(false);
  const [workspaceProjectMenuOpen, setWorkspaceProjectMenuOpen] = useState(false);

  const [projects, setProjects] = useState<RegistryProject[]>([]);
  const [registryHubs, setRegistryHubs] = useState<RegistryHub[]>([]);
  const [localHubReadStatuses, setLocalHubReadStatuses] = useState<Record<string, 'Local' | 'Remote'>>({});
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
  const [htmlPreviewEnabled, setHtmlPreviewEnabled] = useState(false);
  const [htmlPreviewScriptsEnabled, setHtmlPreviewScriptsEnabled] = useState(false);
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
  const chatVirtuosoListRef = useRef<ChatVirtuosoTurnListHandle | null>(null);
  const chatLayoutMetrics = useChatLayoutMetrics(chatScrollRef);
  const chatAutoScrollFollowRef = useRef(true);
  const chatPointerScrollingRef = useRef(false);
  const chatUserScrollLockUntilRef = useRef(0);
  const chatComposerTextareaRef = useRef<HTMLTextAreaElement | null>(null);
  const chatPromptButtonRef = useRef<HTMLButtonElement | null>(null);
  const chatFileMentionButtonRef = useRef<HTMLButtonElement | null>(null);
  const chatFileMentionMenuRef = useRef<HTMLDivElement | null>(null);
  const chatSlashMenuRef = useRef<HTMLDivElement | null>(null);
  const chatConfigOptionsRef = useRef<HTMLDivElement | null>(null);
  const chatConfigOverflowRef = useRef<HTMLDivElement | null>(null);
  const wideProjectActionMenuRef = useRef<HTMLDivElement | null>(null);
  const projectSessionSentinelRefs = useRef<Record<string, HTMLDivElement | null>>({});
  const chatSelectedIdRef = useRef('');
  const selectedChatKeyRef = useRef<ChatSessionKey | null>(null);
  const chatVisibleRuntimeKeyRef = useRef('');
  const chatSelectedLoadAttemptRuntimeKeyRef = useRef('');
  const chatFinishedCursorRef = useRef<Record<string, number>>({});
  const chatMessageStoreRef = useRef<Record<string, RegistryChatMessage[]>>({});
  const chatTurnStoreRef = useRef<Record<string, ChatTurnStoreState>>({});
  const chatReadRepairQueueRef = useRef(createChatReadRepairQueue());
  const chatDurablePersistQueueRef = useRef(createChatDurablePersistQueue(runtimeKey => {
    const key = decodeChatSessionKey(runtimeKey);
    if (!key) return;
    const state = chatTurnStoreRef.current[runtimeKey];
    workspaceStore.rememberChatSessionTurns(key.projectId, key.sessionId, state?.finished ?? []);
  }, 5000));
  const chatMessagesRef = useRef<RegistryChatMessage[]>([]);
  const notifiedChatMessageIdsRef = useRef<Set<string>>(new Set());
  const chatIndexFullRefreshInFlightRef = useRef(false);
  const chatIndexFullRefreshDirtyRef = useRef(false);
  const chatProjectRefreshInFlightRef = useRef<Record<string, boolean>>({});
  const chatProjectRefreshDirtyRef = useRef<Record<string, boolean>>({});
  const [chatSessions, setChatSessions] = useState<RegistryChatSession[]>([]);
  const chatSessionsRef = useRef<RegistryChatSession[]>([]);
  const [projectSessionsByProjectId, setProjectSessionsByProjectId] = useState<Record<string, RegistryChatSession[]>>({});
  const projectSessionsByProjectIdRef = useRef<Record<string, RegistryChatSession[]>>({});
  const [sessionSearchOpen, setSessionSearchOpen] = useState(false);
  const [sessionSearchInput, setSessionSearchInput] = useState('');
  const sessionSearchInputRef = useRef<HTMLInputElement | null>(null);
  const [activeSessionSearchId, setActiveSessionSearchId] = useState('');
  const activeSessionSearchIdRef = useRef('');
  const [sessionSearchQuery, setSessionSearchQuery] = useState('');
  const [searchResultsByProjectId, setSearchResultsByProjectId] = useState<SessionSearchResultsByProjectId>({});
  const [sessionSearchDoneByProjectId, setSessionSearchDoneByProjectId] = useState<Record<string, boolean>>({});
  const sessionSearchDoneByProjectIdRef = useRef<Record<string, boolean>>({});
  const [sessionSearchErrorsByProjectId, setSessionSearchErrorsByProjectId] = useState<Record<string, string>>({});
  const sessionSearchUnchangedPollsRef = useRef(0);
  const sessionSearchPollTimerRef = useRef<number | null>(null);
  const sessionSearchIdCounterRef = useRef(0);
  const [sessionSearchTargetTurn, setSessionSearchTargetTurn] = useState<{
    runtimeKey: string;
    turnIndex: number;
    generation: number;
  } | null>(null);
  const sessionSearchHighlightTimerRef = useRef<number | null>(null);
  const registryDebugSessionLabels = useMemo(() => {
    const labels: Record<string, string> = {};
    for (const projectItem of projects) {
      const projectName = projectItem.name || projectItem.projectId;
      const projectSessions = projectSessionsByProjectId[projectItem.projectId] ?? [];
      for (const session of projectSessions) {
        const sessionTitle = resolveChatSessionTitle(session.title ?? '') || session.sessionId;
        labels[session.sessionId] = `${projectName} / ${sessionTitle}`;
      }
    }
    return labels;
  }, [projectSessionsByProjectId, projects]);
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
  const [chatReloadingSessionId, setChatReloadingSessionId] = useState('');
  const [chatArchivingSessionId, setChatArchivingSessionId] = useState('');
  const [chatDeletingSessionId, setChatDeletingSessionId] = useState('');
  const [chatRenamingSessionId, setChatRenamingSessionId] = useState('');
  const [renameTarget, setRenameTarget] = useState<RenameSessionTarget | null>(null);
  const [renameTitleDraft, setRenameTitleDraft] = useState('');
  const [renameError, setRenameError] = useState('');
  const [confirmTarget, setConfirmTarget] = useState<ConfirmTarget | null>(null);
  const [confirmError, setConfirmError] = useState('');
  const [chatConfigUpdatingKey, setChatConfigUpdatingKey] = useState('');
  const [chatComposerText, setChatComposerText] = useState('');
  const [chatAttachments, setChatAttachments] = useState<ChatAttachment[]>([]);
  const chatAttachmentUploadPending = chatAttachments.some(isChatAttachmentUploadPending);
  const [chatComposerDragActive, setChatComposerDragActive] = useState(false);
  const [chatComposerDrafts, setChatComposerDrafts] = useState<Record<string, ChatComposerDraft>>({});
  const [chatPendingPromptsByKey, setChatPendingPromptsByKey] = useState<Record<string, PendingChatPrompt>>({});
  const [chatCancellingRuntimeKey, setChatCancellingRuntimeKey] = useState('');
  const [markdownImageExportRequest, setMarkdownImageExportRequest] = useState<MarkdownImageExportRequest | null>(null);
  const markdownImageExportIdRef = useRef(0);
  const chatComposerTextRef = useRef('');
  const chatAttachmentsRef = useRef<ChatAttachment[]>([]);
  const chatComposerDraftsRef = useRef<Record<string, ChatComposerDraft>>({});
  const chatPendingPromptsByKeyRef = useRef<Record<string, PendingChatPrompt>>({});
  const chatPendingPromptTimersRef = useRef<Record<string, number>>({});
  const chatDraftGenerationRef = useRef<Record<string, number>>({});
  const currentChatDraftKeyRef = useRef('');
  const chatAttachmentIdRef = useRef(0);
  const chatAttachmentCancelIdsRef = useRef<Set<string>>(new Set());
  const [chatPromptMenuOpen, setChatPromptMenuOpen] = useState(false);
  const [chatFileMentionMenuOpen, setChatFileMentionMenuOpen] = useState(false);
  const [chatConfigMenuOptionId, setChatConfigMenuOptionId] = useState('');
  const [chatHubMenuOpen, setChatHubMenuOpen] = useState(false);
  const chatHubMenuRef = useRef<HTMLDivElement | null>(null);
  const [chatQuickSwitchMenuOpen, setChatQuickSwitchMenuOpen] = useState(false);
  const chatQuickSwitchTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);
  const chatQuickSwitchPressRef = useRef<ChatQuickSwitchPressState | null>(null);
  const chatQuickSwitchMenuRef = useRef<HTMLDivElement | null>(null);
  const [chatSlashActiveIndex, setChatSlashActiveIndex] = useState(0);
  const [resumeSessions, setResumeSessions] = useState<RegistryResumableSession[]>([]);
  const [resumeLoading, setResumeLoading] = useState(false);

  function knownChatSessionsForProject(targetProjectId: string): RegistryChatSession[] {
    const projectSessions = projectSessionsByProjectIdRef.current[targetProjectId] ?? [];
    if (!shouldUpdateCurrentProjectSessions(targetProjectId, projectIdRef.current)) {
      return projectSessions;
    }
    return mergeKnownChatSessions(projectSessions, chatSessionsRef.current);
  }

  function mergeKnownChatSessionForProject(
    targetProjectId: string,
    session: RegistryChatSession,
  ): RegistryChatSession {
    return mergeChatSession(knownChatSessionsForProject(targetProjectId), session)
      .find(item => item.sessionId === session.sessionId) ?? session;
  }

  const selectedChatEncodedKey = useMemo(
    () => encodeChatSessionKey(selectedChatKey),
    [selectedChatKey],
  );

  const selectedChatSession = useMemo(
    () => {
      if (!selectedChatKey) {
        return undefined;
      }
      const projectSession = projectSessionsByProjectId[selectedChatKey.projectId]
        ?.find(item => item.sessionId === selectedChatKey.sessionId);
      const currentProjectSession =
        selectedChatKey.projectId === projectId
          ? chatSessions.find(item => item.sessionId === selectedChatKey.sessionId)
          : undefined;
      if (projectSession && currentProjectSession) {
        return mergeChatSession([projectSession], currentProjectSession)[0];
      }
      return projectSession ?? currentProjectSession;
    },
    [chatSessions, projectId, projectSessionsByProjectId, selectedChatKey],
  );

  const selectedChatConfigOptions = useMemo(() => {
    return selectedChatSession?.configOptions ?? [];
  }, [selectedChatSession]);

  const selectedFullChatMessages =
    selectedChatEncodedKey
      ? chatMessageStoreRef.current[selectedChatEncodedKey] ?? []
      : [];

  const selectedPendingPrompt = selectedChatEncodedKey
    ? chatPendingPromptsByKey[selectedChatEncodedKey]
    : undefined;

  const chatDisplayIndex = useMemo(() => buildChatDisplayIndex(chatMessages, {
    hideToolCalls,
    layoutMetrics: chatLayoutMetrics,
    promptStatus: message => isPromptStartMessage(message)
      ? resolvePromptTurnStatus(selectedFullChatMessages, message)
      : null,
    shouldRender: (message, promptStatus) => {
      const resolvedPromptStatus = isPromptStartMessage(message)
        ? promptStatus
        : null;
      return shouldRenderChatTurn(message, hideToolCalls, resolvedPromptStatus);
    },
    pendingKey: selectedPendingPrompt
      ? `${selectedChatEncodedKey}:pending:${selectedPendingPrompt.createdAt}`
      : undefined,
    pendingEstimatedHeight: 120,
  }), [
    chatMessages,
    chatLayoutMetrics,
    hideToolCalls,
    selectedChatEncodedKey,
    selectedFullChatMessages,
    selectedPendingPrompt,
  ]);

  useEffect(() => {
    if (
      !sessionSearchTargetTurn ||
      sessionSearchTargetTurn.runtimeKey !== selectedChatEncodedKey ||
      chatDisplayIndex.items.length === 0
    ) {
      return;
    }
    const frameId = window.requestAnimationFrame(() => {
      chatVirtuosoListRef.current?.scrollToTurnIndex(sessionSearchTargetTurn.turnIndex, 'smooth');
    });
    return () => window.cancelAnimationFrame(frameId);
  }, [chatDisplayIndex, selectedChatEncodedKey, sessionSearchTargetTurn]);

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

  const chatSlashMenuVisible = !chatFileMentionMenuOpen && chatSlashMenuOptions.length > 0;


  const currentChatDraftKey = useMemo(
    () => buildChatDraftKey(selectedChatKey?.projectId ?? projectId, selectedChatId),
    [projectId, selectedChatId, selectedChatKey?.projectId],
  );

  const buildChatRuntimeKey = (
    activeProjectId: string,
    sessionId: string,
  ): string => encodeChatSessionKey(chatSessionKeyFromParts(activeProjectId, sessionId));

  const runtimeKeysFromChatStores = (): string[] =>
    Array.from(new Set([
      ...Object.keys(chatTurnStoreRef.current),
      ...Object.keys(chatMessageStoreRef.current),
      ...Object.keys(chatFinishedCursorRef.current),
    ]));

  const ensureChatTurnStore = (runtimeKey: string): ChatTurnStoreState => {
    const existing = chatTurnStoreRef.current[runtimeKey];
    if (existing) return existing;
    const created = createEmptyChatTurnStore();
    chatTurnStoreRef.current[runtimeKey] = created;
    return created;
  };

  const decodeRawTurnsForSession = (
    sessionId: string,
    turns: RegistrySessionTurn[],
  ): RegistryChatMessage[] => turns
    .map(turn => decodeSessionTurnToMessage(sessionId, turn))
    .filter((item): item is RegistryChatMessage => !!item);

  const messagesFromTurnStore = (
    runtimeKey: string,
    sessionId: string,
  ): RegistryChatMessage[] => {
    const state = chatTurnStoreRef.current[runtimeKey];
    if (!state) return [];
    return decodeRawTurnsForSession(sessionId, buildMergedRawTurns(state));
  };

  const markChatSessionTurnsDirty = (runtimeKey: string): void => {
    chatDurablePersistQueueRef.current.markDirty(runtimeKey);
  };

  const setVisibleChatMessagesForRuntimeKey = useCallback((
    runtimeKey: string,
    fullMessages: RegistryChatMessage[],
    options?: { resetToLatest?: boolean; followLatest?: boolean },
  ) => {
    if (!runtimeKey) {
      chatVisibleRuntimeKeyRef.current = '';
      chatMessagesRef.current = [];
      setChatMessages([]);
      return;
    }
    const resettingToLatest = options?.resetToLatest === true;
    if (resettingToLatest && encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey) {
      chatAutoScrollFollowRef.current = true;
      chatUserScrollLockUntilRef.current = 0;
      chatPointerScrollingRef.current = false;
      setChatShowScrollToBottom(false);
    }
    if (encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey) {
      chatVisibleRuntimeKeyRef.current = runtimeKey;
      chatMessagesRef.current = fullMessages;
      setChatMessages(fullMessages);
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

  const markChatUserScrollIntent = useCallback(() => {
    chatUserScrollLockUntilRef.current = nextChatUserScrollLockUntil();
  }, []);

  const shouldAutoscrollChat = useCallback((force = false) => (
    shouldAutoScrollChatToBottom({
      force,
      followsLatest: chatAutoScrollFollowRef.current,
      pointerScrolling: chatPointerScrollingRef.current,
      userScrollLocked: isChatUserScrollLocked(chatUserScrollLockUntilRef.current),
    })
  ), []);

  const handleChatAtBottomChange = useCallback((atBottom: boolean) => {
    chatAutoScrollFollowRef.current = atBottom;
    setChatShowScrollToBottom(!atBottom);
  }, []);

  const handleChatScroll = useCallback((event: React.UIEvent<HTMLDivElement>) => {
    const visibility = resolveChatScrollToBottomVisibility({
      scrollTop: event.currentTarget.scrollTop,
      scrollHeight: event.currentTarget.scrollHeight,
      clientHeight: event.currentTarget.clientHeight,
      threshold: CHAT_AUTO_SCROLL_BOTTOM_THRESHOLD,
    });
    chatAutoScrollFollowRef.current = visibility.atBottom;
    setChatShowScrollToBottom(visibility.showScrollToBottom);
  }, []);

  const scrollChatToBottom = useCallback((force = false) => {
    if (!shouldAutoscrollChat(force)) {
      return;
    }
    window.requestAnimationFrame(() => {
      if (!shouldAutoscrollChat(force)) {
        return;
      }
      chatVirtuosoListRef.current?.scrollToBottom('auto');
      chatAutoScrollFollowRef.current = true;
      setChatShowScrollToBottom(false);
    });
  }, [shouldAutoscrollChat]);

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

  const chatMainStyle = useMemo(
    () => ({
      '--chat-message-font-family': chatFontFamily,
      ...(chatKeyboardInset > 0 ? { paddingBottom: `${chatKeyboardInset}px` } : {}),
    }) as React.CSSProperties,
    [chatFontFamily, chatKeyboardInset],
  );

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
      const input = chatComposerTextareaRef.current;
      const inserted = insertChatSlashCommandText(
        chatComposerText,
        command.name,
        input?.selectionStart ?? chatComposerText.length,
        input?.selectionEnd ?? input?.selectionStart ?? chatComposerText.length,
      );
      setChatPromptMenuOpen(false);
      setChatFileMentionMenuOpen(false);
      setChatConfigMenuOptionId('');
      setChatConfigOverflowOpen(false);
      updateChatComposerText(inserted.text);
      window.requestAnimationFrame(() => {
        const input = chatComposerTextareaRef.current;
        if (!input) {
          return;
        }
        input.focus();
        input.setSelectionRange(inserted.selectionStart, inserted.selectionEnd);
      });
    },
    [chatComposerText, setChatConfigOverflowOpen, updateChatComposerText],
  );

  const openChatPromptMenu = useCallback(() => {
    setChatFileMentionMenuOpen(false);
    setChatConfigMenuOptionId('');
    setChatConfigOverflowOpen(false);
    setChatPromptMenuOpen(value => !value);
    window.requestAnimationFrame(() => {
      chatComposerTextareaRef.current?.focus();
    });
  }, [setChatConfigOverflowOpen]);

  const openChatFileMentionMenu = useCallback(() => {
    setChatPromptMenuOpen(false);
    setChatConfigMenuOptionId('');
    setChatConfigOverflowOpen(false);
    setChatFileMentionMenuOpen(value => !value);
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

  const updateChatAttachment = useCallback(
    (
      attachmentId: string,
      updater: (attachment: ChatAttachment) => ChatAttachment,
      draftKey = currentChatDraftKeyRef.current,
      expectedGeneration = getChatDraftGeneration(draftKey),
    ) => {
      applyChatAttachments(
        current => current.map(attachment => (
          attachment.id === attachmentId ? updater(attachment) : attachment
        )),
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
      const attachment = chatAttachmentsRef.current.find(item => item.id === attachmentId);
      if (!attachment) {
        return;
      }
      revokeChatAttachmentObjectUrl(attachment);
      const selectedKey = selectedChatKeyRef.current;
      if (selectedKey?.sessionId) {
        const selectedProjectId = selectedKey.projectId;
        if (attachment.uploadId && isChatAttachmentUploadPending(attachment)) {
          chatAttachmentCancelIdsRef.current.add(attachment.id);
          service.cancelProjectSessionAttachment(selectedProjectId, {
            sessionId: selectedKey.sessionId,
            uploadId: attachment.uploadId,
          }).catch(() => undefined);
        } else if (attachment.attachmentId && attachment.status === 'completed') {
          service.deleteProjectSessionAttachment(selectedProjectId, {
            sessionId: selectedKey.sessionId,
            attachmentId: attachment.attachmentId,
          }).catch(err => setError(err instanceof Error ? err.message : String(err)));
        }
      }
      applyChatAttachments(current => {
        const filtered = current.filter(attachment => attachment.id !== attachmentId);
        return filtered.length === current.length ? current : filtered;
      });
    },
    [applyChatAttachments],
  );

  const uploadChatAttachmentFile = useCallback(
    async (
      file: File,
      fallbackName: string,
      selectedProjectId: string,
      sessionId: string,
      attachmentId: string,
      draftKey: string,
      expectedGeneration: number,
    ): Promise<ChatAttachment> => {
      const attachmentName = file.name || fallbackName;
      let uploadId = '';
      try {
        updateChatAttachment(
          attachmentId,
          attachment => ({...attachment, status: 'uploading', progress: 1, error: '', uploadId: undefined}),
          draftKey,
          expectedGeneration,
        );
        const start = await service.startProjectSessionAttachment(selectedProjectId, {
          sessionId,
          name: attachmentName,
          mimeType: file.type || '',
          size: file.size,
        });
        if (!start.ok || !start.uploadId) {
          throw new Error('session.attachment.start returned ok=false');
        }
        uploadId = start.uploadId;
        updateChatAttachment(
          attachmentId,
          attachment => ({...attachment, uploadId, progress: Math.max(2, attachment.progress)}),
          draftKey,
          expectedGeneration,
        );
        const chunkSize = Math.max(1, start.chunkSize || CHAT_ATTACHMENT_CHUNK_SIZE);
        let offset = 0;
        while (offset < file.size) {
          if (chatAttachmentCancelIdsRef.current.has(attachmentId)) {
            throw new Error('Attachment upload cancelled');
          }
          const nextOffset = Math.min(file.size, offset + chunkSize);
          const data = await blobToBase64(file.slice(offset, nextOffset));
          const chunk = await service.uploadProjectSessionAttachmentChunk(selectedProjectId, {
            sessionId,
            uploadId,
            offset,
            data,
          });
          if (!chunk.ok) {
            throw new Error('session.attachment.chunk returned ok=false');
          }
          offset = nextOffset;
          const progress = file.size > 0 ? Math.max(3, Math.min(95, Math.round((offset / file.size) * 95))) : 95;
          updateChatAttachment(
            attachmentId,
            attachment => ({...attachment, progress}),
            draftKey,
            expectedGeneration,
          );
        }
        const sha256 = await sha256Hex(file);
        const finished = await service.finishProjectSessionAttachment(selectedProjectId, {
          sessionId,
          uploadId,
          sha256,
        });
        if (!finished.ok || !finished.block) {
          throw new Error('session.attachment.finish returned ok=false');
        }
        const completedPatch = {
          status: 'completed' as const,
          progress: 100,
          block: finished.block,
          attachmentId: finished.attachment?.id || attachmentIdFromBlock(finished.block),
          error: '',
        };
        let uploadedAttachment: ChatAttachment = {
          id: attachmentId,
          name: attachmentName,
          mimeType: file.type || '',
          size: file.size,
          status: completedPatch.status,
          progress: completedPatch.progress,
          file,
          block: completedPatch.block,
          attachmentId: completedPatch.attachmentId,
          error: completedPatch.error,
        };
        updateChatAttachment(
          attachmentId,
          attachment => {
            uploadedAttachment = {...attachment, ...completedPatch};
            return uploadedAttachment;
          },
          draftKey,
          expectedGeneration,
        );
        return uploadedAttachment;
      } catch (err) {
        if (uploadId && !chatAttachmentCancelIdsRef.current.has(attachmentId)) {
          service.cancelProjectSessionAttachment(selectedProjectId, {sessionId, uploadId}).catch(() => undefined);
        }
        if (!chatAttachmentCancelIdsRef.current.has(attachmentId)) {
          const message = err instanceof Error ? err.message : String(err);
          updateChatAttachment(
            attachmentId,
            attachment => ({...attachment, status: 'failed', error: message}),
            draftKey,
            expectedGeneration,
          );
          setError(message);
        }
        throw err;
      } finally {
        chatAttachmentCancelIdsRef.current.delete(attachmentId);
      }
    },
    [updateChatAttachment],
  );

  const uploadChatAttachmentsForSend = useCallback(
    async (
      attachments: ChatAttachment[],
      selectedProjectId: string,
      sessionId: string,
      draftKey: string,
      expectedGeneration: number,
    ): Promise<ChatAttachment[]> => {
      const uploadedAttachments: ChatAttachment[] = [];
      for (const attachment of attachments) {
        if (attachment.status === 'completed' && attachment.block) {
          uploadedAttachments.push(attachment);
          continue;
        }
        if (!attachment.file) {
          throw new Error('Attachment can only be uploaded before the page is refreshed.');
        }
        uploadedAttachments.push(await uploadChatAttachmentFile(
          attachment.file,
          attachment.name,
          selectedProjectId,
          sessionId,
          attachment.id,
          draftKey,
          expectedGeneration,
        ));
      }
      return uploadedAttachments;
    },
    [uploadChatAttachmentFile],
  );

  const enqueueChatAttachmentFiles = useCallback(
    (
      files: File[],
      draftKey = currentChatDraftKeyRef.current,
      expectedGeneration = getChatDraftGeneration(draftKey),
    ) => {
      if (files.length === 0) {
        return;
      }
      const attachments = files.map((file, index): ChatAttachment => {
        chatAttachmentIdRef.current += 1;
        const name = file.name || chatFallbackAttachmentName(index);
        return {
          id: `chat-attachment-${chatAttachmentIdRef.current}`,
          name,
          mimeType: file.type || '',
          size: file.size,
          status: 'queued',
          progress: 0,
          file,
          objectUrl: (file.type || '').toLowerCase().startsWith('image/') ? URL.createObjectURL(file) : undefined,
        };
      });
      appendChatAttachments(attachments, draftKey, expectedGeneration);
    },
    [appendChatAttachments, getChatDraftGeneration],
  );

  const retryChatAttachment = useCallback(
    (attachmentId: string) => {
      const attachment = chatAttachmentsRef.current.find(item => item.id === attachmentId);
      if (!attachment?.file) {
        setError('Attachment can only be retried before the page is refreshed.');
        return;
      }
      updateChatAttachment(
        attachmentId,
        current => ({...current, status: 'queued', progress: 0, error: '', uploadId: undefined, block: undefined, attachmentId: undefined}),
      );
    },
    [updateChatAttachment],
  );

  useEffect(() => {
    currentChatDraftKeyRef.current = currentChatDraftKey;
  }, [currentChatDraftKey]);

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
    return () => {
      for (const timerId of Object.values(chatPendingPromptTimersRef.current)) {
        window.clearTimeout(timerId);
      }
      chatPendingPromptTimersRef.current = {};
    };
  }, []);

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
  const sessionSearchSections = useMemo(
    () => buildSessionSearchSections({
      projects: sortedProjectItems,
      sessionsByProjectId: projectSessionsByProjectId,
      resultsByProjectId: searchResultsByProjectId,
    }),
    [projectSessionsByProjectId, searchResultsByProjectId, sortedProjectItems],
  );
  const sessionSearchResultCount = useMemo(
    () => sessionSearchSections.reduce((sum, section) => sum + section.rows.length, 0),
    [sessionSearchSections],
  );
  const sessionSearchActive = !!activeSessionSearchId;
  const sessionSearchHeaderExpanded = sessionSearchOpen || sessionSearchActive;
  const sessionSearchProjectDoneCount = useMemo(() => {
    if (!activeSessionSearchId) {
      return 0;
    }
    return sortedProjectItems.reduce(
      (sum, item) => sum + (sessionSearchDoneByProjectId[item.projectId] === true ? 1 : 0),
      0,
    );
  }, [activeSessionSearchId, sessionSearchDoneByProjectId, sortedProjectItems]);
  const sessionSearchErrorCount = useMemo(
    () => Object.values(sessionSearchErrorsByProjectId).filter(message => message.trim()).length,
    [sessionSearchErrorsByProjectId],
  );
  const sessionSearchAllDone = useMemo(() => {
    if (!activeSessionSearchId || sortedProjectItems.length === 0) {
      return false;
    }
    return sortedProjectItems.every(item => sessionSearchDoneByProjectId[item.projectId] === true);
  }, [activeSessionSearchId, sessionSearchDoneByProjectId, sortedProjectItems]);
  const sessionSearchStatusParts = useMemo(() => {
    if (!sessionSearchActive) {
      return [];
    }
    const resultLabel = `${sessionSearchResultCount} result${sessionSearchResultCount === 1 ? '' : 's'}`;
    const parts = sessionSearchAllDone
      ? [resultLabel]
      : [
          `Searching ${sessionSearchProjectDoneCount}/${sortedProjectItems.length} projects`,
          resultLabel,
        ];
    if (sessionSearchErrorCount > 0) {
      parts.push(`${sessionSearchErrorCount} error${sessionSearchErrorCount === 1 ? '' : 's'}`);
    }
    return parts;
  }, [
    sessionSearchActive,
    sessionSearchAllDone,
    sessionSearchErrorCount,
    sessionSearchProjectDoneCount,
    sessionSearchResultCount,
    sortedProjectItems.length,
  ]);
  useEffect(() => {
    if (!sessionSearchHeaderExpanded) {
      return;
    }
    const input = sessionSearchInputRef.current;
    if (!input) {
      return;
    }
    input.focus();
    const cursor = input.value.length;
    input.setSelectionRange(cursor, cursor);
  }, [sessionSearchHeaderExpanded]);
  const mobileChatQuickSwitchSections = useMemo(
    () => buildMobileChatQuickSwitchSections({
      projects: sortedProjectItems,
      sessionsByProjectId: projectSessionsByProjectId,
      limit: 8,
    }),
    [projectSessionsByProjectId, sortedProjectItems],
  );
  const chatQuickSwitchMenuStyle = useMemo<React.CSSProperties>(() => ({
    top: portRelayReady && portRelayFrameUrl ? 56 : 0,
  }), [portRelayFrameUrl, portRelayReady]);
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
    chatSessionsRef.current = chatSessions;
  }, [chatSessions]);
  useEffect(() => {
    projectSessionsByProjectIdRef.current = projectSessionsByProjectId;
  }, [projectSessionsByProjectId]);
  useEffect(() => {
    activeSessionSearchIdRef.current = activeSessionSearchId;
  }, [activeSessionSearchId]);
  useEffect(() => {
    sessionSearchDoneByProjectIdRef.current = sessionSearchDoneByProjectId;
  }, [sessionSearchDoneByProjectId]);
  useEffect(() => {
    return () => {
      if (sessionSearchPollTimerRef.current !== null) {
        window.clearTimeout(sessionSearchPollTimerRef.current);
        sessionSearchPollTimerRef.current = null;
      }
      if (sessionSearchHighlightTimerRef.current !== null) {
        window.clearTimeout(sessionSearchHighlightTimerRef.current);
        sessionSearchHighlightTimerRef.current = null;
      }
    };
  }, []);
  const cancelSessionSearch = useCallback(async (
    searchId = activeSessionSearchIdRef.current,
    projectItems = sortedProjectItems,
  ) => {
    const normalizedSearchId = searchId.trim();
    if (!normalizedSearchId) {
      return;
    }
    await Promise.allSettled(
      projectItems.map(projectItem =>
        service.cancelProjectSessionSearch(projectItem.projectId, normalizedSearchId),
      ),
    );
  }, [sortedProjectItems]);

  const querySessionSearch = useCallback(async (
    searchId = activeSessionSearchIdRef.current,
  ): Promise<boolean> => {
    const normalizedSearchId = searchId.trim();
    if (!normalizedSearchId) {
      return false;
    }
    const doneSnapshot = sessionSearchDoneByProjectIdRef.current;
    const pendingProjects = sortedProjectItems.filter(projectItem =>
      doneSnapshot[projectItem.projectId] !== true,
    );
    if (pendingProjects.length === 0) {
      return false;
    }
    let anyChanged = false;
    await Promise.all(
      pendingProjects.map(async projectItem => {
        try {
          const response = await service.queryProjectSessionSearch(projectItem.projectId, normalizedSearchId);
          if (activeSessionSearchIdRef.current !== normalizedSearchId) {
            return;
          }
          setSearchResultsByProjectId(prev => {
            const merged = mergeSessionSearchResultsByProject(prev, projectItem.projectId, response.results);
            anyChanged = anyChanged || merged.changed;
            return merged.resultsByProjectId;
          });
          setSessionSearchDoneByProjectId(prev => ({
            ...prev,
            [projectItem.projectId]: response.done,
          }));
          setSessionSearchErrorsByProjectId(prev => ({
            ...prev,
            [projectItem.projectId]: response.errors.map(item => item.message).join('\n'),
          }));
        } catch (err) {
          if (activeSessionSearchIdRef.current !== normalizedSearchId) {
            return;
          }
          anyChanged = true;
          setSessionSearchDoneByProjectId(prev => ({
            ...prev,
            [projectItem.projectId]: true,
          }));
          setSessionSearchErrorsByProjectId(prev => ({
            ...prev,
            [projectItem.projectId]: err instanceof Error ? err.message : String(err),
          }));
        }
      }),
    );
    sessionSearchUnchangedPollsRef.current = anyChanged
      ? 0
      : sessionSearchUnchangedPollsRef.current + 1;
    return anyChanged;
  }, [sortedProjectItems]);

  const clearSessionSearchState = useCallback(() => {
    activeSessionSearchIdRef.current = '';
    setActiveSessionSearchId('');
    setSessionSearchQuery('');
    setSearchResultsByProjectId({});
    setSessionSearchDoneByProjectId({});
    setSessionSearchErrorsByProjectId({});
    sessionSearchUnchangedPollsRef.current = 0;
    if (sessionSearchPollTimerRef.current !== null) {
      window.clearTimeout(sessionSearchPollTimerRef.current);
      sessionSearchPollTimerRef.current = null;
    }
  }, []);

  const exitSessionSearch = useCallback(async () => {
    const searchId = activeSessionSearchIdRef.current;
    const projectItems = sortedProjectItems;
    clearSessionSearchState();
    setSessionSearchOpen(false);
    await cancelSessionSearch(searchId, projectItems);
  }, [cancelSessionSearch, clearSessionSearchState, sortedProjectItems]);

  const startSessionSearch = useCallback(async () => {
    const query = sessionSearchInput.trim();
    if (!query) {
      await exitSessionSearch();
      return;
    }
    const previousSearchId = activeSessionSearchIdRef.current;
    const projectItems = sortedProjectItems;
    if (previousSearchId) {
      await cancelSessionSearch(previousSearchId, projectItems);
    }
    sessionSearchIdCounterRef.current += 1;
    const searchId = `session-search-${Date.now()}-${sessionSearchIdCounterRef.current}`;
    activeSessionSearchIdRef.current = searchId;
    setActiveSessionSearchId(searchId);
    setSessionSearchQuery(query);
    setSearchResultsByProjectId({});
    setSessionSearchErrorsByProjectId({});
    setSessionSearchDoneByProjectId(
      Object.fromEntries(projectItems.map(projectItem => [projectItem.projectId, false])),
    );
    sessionSearchUnchangedPollsRef.current = 0;
    setSessionSearchOpen(true);

    await Promise.all(
      projectItems.map(async projectItem => {
        try {
          await service.startProjectSessionSearch(projectItem.projectId, searchId, query);
        } catch (err) {
          if (activeSessionSearchIdRef.current !== searchId) {
            return;
          }
          setSessionSearchDoneByProjectId(prev => ({
            ...prev,
            [projectItem.projectId]: true,
          }));
          setSessionSearchErrorsByProjectId(prev => ({
            ...prev,
            [projectItem.projectId]: err instanceof Error ? err.message : String(err),
          }));
        }
      }),
    );
    if (activeSessionSearchIdRef.current === searchId) {
      querySessionSearch(searchId).catch(() => undefined);
    }
  }, [cancelSessionSearch, exitSessionSearch, querySessionSearch, sessionSearchInput, sortedProjectItems]);

  useEffect(() => {
    if (!activeSessionSearchId || tab !== 'chat') {
      if (sessionSearchPollTimerRef.current !== null) {
        window.clearTimeout(sessionSearchPollTimerRef.current);
        sessionSearchPollTimerRef.current = null;
      }
      return;
    }
    let cancelled = false;
    const poll = async () => {
      const searchId = activeSessionSearchIdRef.current;
      if (!searchId) {
        return;
      }
      const changed = await querySessionSearch(searchId);
      if (cancelled || activeSessionSearchIdRef.current !== searchId) {
        return;
      }
      const doneSnapshot = sessionSearchDoneByProjectIdRef.current;
      const allDone = sortedProjectItems.length > 0 &&
        sortedProjectItems.every(projectItem => doneSnapshot[projectItem.projectId] === true);
      if (allDone) {
        return;
      }
      const delay = resolveSessionSearchPollDelay({
        changed,
        unchangedPolls: sessionSearchUnchangedPollsRef.current,
      });
      sessionSearchPollTimerRef.current = window.setTimeout(() => {
        poll().catch(() => undefined);
      }, delay);
    };
    poll().catch(() => undefined);
    return () => {
      cancelled = true;
      if (sessionSearchPollTimerRef.current !== null) {
        window.clearTimeout(sessionSearchPollTimerRef.current);
        sessionSearchPollTimerRef.current = null;
      }
    };
  }, [activeSessionSearchId, querySessionSearch, sortedProjectItems, tab]);
  useEffect(() => {
    floatingDragStateRef.current = floatingDragState;
  }, [floatingDragState]);
  useEffect(() => {
    gestureNavStateRef.current = gestureNavState;
  }, [gestureNavState]);
  useEffect(() => {
    sidebarSettingsOpenRef.current = sidebarSettingsOpen;
  }, [sidebarSettingsOpen]);
  useEffect(() => {
    settingsDetailViewRef.current = settingsDetailView;
  }, [settingsDetailView]);
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
    const shouldHydrateProjectSessionIndex =
      isWide || (!isWide && tab === 'chat' && drawerOpen);
    if (!shouldHydrateProjectSessionIndex || projects.length === 0) return;
    setProjectSessionsByProjectId(prev => {
      const next = {...prev};
      for (const projectItem of projects) {
        const cachedSessions = workspaceStore
          .hydrateChatSessions(projectItem.projectId)
          .map(entry => entry.session);
        const sortedCachedSessions = sortChatSessions(cachedSessions);
        if (sortedCachedSessions.length > 0) {
          next[projectItem.projectId] = mergeChatSessionList(
            next[projectItem.projectId] ?? [],
            sortedCachedSessions,
          );
        } else if (!next[projectItem.projectId]) {
          next[projectItem.projectId] = [];
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
          const knownSessions = knownChatSessionsForProject(projectItem.projectId);
          const mergedSessions = mergeChatSessionList(knownSessions, sortedSessions);
          setProjectSessionsByProjectId(prev => ({
            ...prev,
            [projectItem.projectId]: mergeChatSessionList(
              prev[projectItem.projectId] ?? knownSessions,
              sortedSessions,
            ),
          }));
          const cached = workspaceStore.hydrateChatSessions(projectItem.projectId);
          const cursorBySessionId: Record<string, {turnIndex: number}> = {};
          for (const entry of cached) {
            cursorBySessionId[entry.session.sessionId] = entry.cursor;
          }
          workspaceStore.replaceChatSessions(
            projectItem.projectId,
            mergedSessions,
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
  }, [tab, selectedChatId, chatMessages, chatPendingPromptsByKey, chatLoading, resizeChatComposerTextarea]);

  useEffect(() => {
    if (tab !== 'chat') {
      chatKeyboardInsetRef.current = chatKeyboardInset;
      if (chatKeyboardInsetSettleTimerRef.current !== null) {
        window.clearTimeout(chatKeyboardInsetSettleTimerRef.current);
        chatKeyboardInsetSettleTimerRef.current = null;
      }
      return;
    }
    const keyboardInsetScrollAction = resolveChatKeyboardInsetScrollAction({
      previousInset: chatKeyboardInsetRef.current,
      nextInset: chatKeyboardInset,
    });
    chatKeyboardInsetRef.current = chatKeyboardInset;
    if (chatKeyboardInsetSettleTimerRef.current !== null) {
      window.clearTimeout(chatKeyboardInsetSettleTimerRef.current);
      chatKeyboardInsetSettleTimerRef.current = null;
    }
    if (keyboardInsetScrollAction === 'immediate') {
      scrollChatToBottom();
      return;
    }
    if (keyboardInsetScrollAction === 'deferred') {
      chatKeyboardInsetSettleTimerRef.current = window.setTimeout(() => {
        chatKeyboardInsetSettleTimerRef.current = null;
        scrollChatToBottom();
      }, CHAT_KEYBOARD_INSET_SETTLE_DELAY_MS);
    }
    return () => {
      if (chatKeyboardInsetSettleTimerRef.current !== null) {
        window.clearTimeout(chatKeyboardInsetSettleTimerRef.current);
        chatKeyboardInsetSettleTimerRef.current = null;
      }
    };
  }, [tab, chatKeyboardInset, scrollChatToBottom]);

  useEffect(() => {
    gitSelectedBranchesRef.current = gitSelectedBranches;
  }, [gitSelectedBranches]);

  useEffect(() => {
    setMarkdownPreviewEnabled(isMarkdownPath(selectedFile));
    setHtmlPreviewEnabled(isHtmlPath(selectedFile));
    setHtmlPreviewScriptsEnabled(false);
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
    gestureNavigation,
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
    if (gestureLongPressTimerRef.current !== null) {
      window.clearTimeout(gestureLongPressTimerRef.current);
      gestureLongPressTimerRef.current = null;
    }
    if (gestureMoveLongPressTimerRef.current !== null) {
      window.clearTimeout(gestureMoveLongPressTimerRef.current);
      gestureMoveLongPressTimerRef.current = null;
    }
    if (floatingCooldownTimerRef.current !== null) {
      window.clearTimeout(floatingCooldownTimerRef.current);
      floatingCooldownTimerRef.current = null;
    }
    floatingIgnoreLostCaptureRef.current = false;
    setFloatingDragState(null);
    gestureNavStateRef.current = null;
    setGestureNavState(null);
    setFloatingKeyboardOffset(0);
  }, [isWide]);

  useEffect(
    () => () => {
      if (floatingLongPressTimerRef.current !== null) {
        window.clearTimeout(floatingLongPressTimerRef.current);
        floatingLongPressTimerRef.current = null;
      }
      if (gestureLongPressTimerRef.current !== null) {
        window.clearTimeout(gestureLongPressTimerRef.current);
        gestureLongPressTimerRef.current = null;
      }
      if (gestureMoveLongPressTimerRef.current !== null) {
        window.clearTimeout(gestureMoveLongPressTimerRef.current);
        gestureMoveLongPressTimerRef.current = null;
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
    if (!chatFileMentionMenuOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (
        target &&
        (chatFileMentionMenuRef.current?.contains(target) ||
          chatFileMentionButtonRef.current?.contains(target))
      ) {
        return;
      }
      setChatFileMentionMenuOpen(false);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [chatFileMentionMenuOpen]);

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
    if (!chatHubMenuOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && chatHubMenuRef.current?.contains(target)) return;
      setChatHubMenuOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setChatHubMenuOpen(false);
      }
    };
    window.addEventListener('pointerdown', onPointerDown);
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.removeEventListener('pointerdown', onPointerDown);
      window.removeEventListener('keydown', onKeyDown);
    };
  }, [chatHubMenuOpen]);

  useEffect(() => {
    if (tab !== 'chat' || sidebarSettingsOpen) {
      setChatHubMenuOpen(false);
    }
  }, [sidebarSettingsOpen, tab]);

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
    return registryDebugStore.subscribe((records: RegistryDebugRecord[]) => {
      setRegistryDebugRecords(records);
      setSelectedRegistryDebugRecordId(current =>
        current !== null && records.some(record => record.id === current)
          ? current
          : records[records.length - 1]?.id ?? null,
      );
    });
  }, []);

  useEffect(() => {
    registryDebugStore.setEnabled(registryDebug);
    if (registryDebug) {
      setRegistryDebugPanelOpen(true);
    } else {
      setRegistryDebugPanelOpen(false);
      setSelectedRegistryDebugRecordId(null);
      setSelectedRegistryDebugScope('All');
      setSelectedRegistryDebugSessionId('All');
      setRegistryDebugIncludeMultiSessionRecords(false);
    }
  }, [registryDebug]);

  useEffect(() => {
    service.setLocalHubReadEnabled(localHubReadEnabled);
    setLocalHubReadStatuses(service.getLocalHubReadStatuses(registryHubs));
    workspaceStore.rememberGlobalState({ localHubReadEnabled });
  }, [localHubReadEnabled, registryHubs]);

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
      chatFont,
      wrapLines,
      showLineNumbers,
      hideToolCalls,
      registryDebug,
      localHubReadEnabled,
      gestureNavigation,
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
    chatFont,
    wrapLines,
    showLineNumbers,
    hideToolCalls,
    registryDebug,
    localHubReadEnabled,
    gestureNavigation,
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
  const chatBreadcrumbProjectName = useMemo(
    () => {
      const selectedProjectId = selectedChatKey?.projectId;
      if (!selectedProjectId || selectedProjectId === projectId) {
        return breadcrumbProjectName;
      }
      const selectedProjectName = projects.find(item => item.projectId === selectedProjectId)?.name;
      return selectedProjectName || 'Project';
    },
    [breadcrumbProjectName, projectId, projects, selectedChatKey?.projectId],
  );
  const fileBreadcrumbLabel = useMemo(
    () => splitPathForDisplay(selectedFile).fileName || 'No Selected File',
    [selectedFile],
  );
  const resolveSessionDisplayTitle = useCallback(
    (session?: Pick<RegistrySessionSummary, 'sessionId' | 'title'> | null) =>
      resolveChatSessionTitle(session?.title ?? '') ||
      session?.sessionId ||
      '',
    [],
  );
  const selectedChatDisplayTitle = useMemo(
    () =>
      resolveChatSessionTitle(selectedChatSession?.title ?? '') ||
      selectedChatSession?.sessionId ||
      '',
    [selectedChatSession],
  );
  const chatBreadcrumbLabel = useMemo(
    () => selectedChatDisplayTitle || 'No Selected Session',
    [selectedChatDisplayTitle],
  );
  const gitBreadcrumbLabel = useMemo(
    () => splitPathForDisplay(selectedDiff).fileName || 'No Selected Diff',
    [selectedDiff],
  );
  const renderBreadcrumbTitle = useCallback(
    (projectName: string, label: string) => (
      <div className="breadcrumb-title">
        <span className="breadcrumb-project-name">{projectName}</span>
        <span className="breadcrumb-separator" aria-hidden="true">
          &gt;
        </span>
        <span className="title-text breadcrumb-current" title={label}>
          {label}
        </span>
      </div>
    ),
    [],
  );
  const renderChatHubSummary = useCallback(() => {
    const hubCount = registryHubs.length;
    const chatHubSummaryLabel = `${hubCount} ${hubCount === 1 ? 'Hub' : 'Hubs'}`;
    return (
      <div ref={chatHubMenuRef} className="chat-hub-summary">
        <button
          type="button"
          className="chat-hub-summary-button"
          aria-label="Show connected hubs"
          aria-haspopup="menu"
          aria-expanded={chatHubMenuOpen}
          onClick={() => {
            setChatPromptMenuOpen(false);
            setChatFileMentionMenuOpen(false);
            setChatConfigMenuOptionId('');
            setChatConfigOverflowOpen(false);
            setChatHubMenuOpen(open => !open);
          }}
        >
          <span className="chat-hub-summary-label">{chatHubSummaryLabel}</span>
          <span className="codicon codicon-chevron-down" aria-hidden="true" />
        </button>
        {chatHubMenuOpen ? (
          <div className="chat-hub-popover" role="menu">
            {registryHubs.length > 0 ? (
              registryHubs.map(hub => {
                const readStatus = localHubReadStatuses[hub.hubId] ?? 'Remote';
                return (
                  <div key={hub.hubId} className="chat-hub-row" role="menuitem">
                    <span className="chat-hub-row-name">{hub.hubId}</span>
                    <span className={`chat-hub-read-tag ${readStatus.toLowerCase()}`}>
                      {readStatus}
                    </span>
                  </div>
                );
              })
            ) : (
              <div className="chat-hub-empty">No hubs</div>
            )}
          </div>
        ) : null}
      </div>
    );
  }, [chatHubMenuOpen, localHubReadStatuses, registryHubs, setChatConfigOverflowOpen]);
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
  const gestureNavigationExpanded = gestureNavState?.phase === 'expanded';
  const effectiveFloatingControlTop = useMemo(() => {
    if (!gestureNavigation || !gestureNavigationExpanded) {
      return floatingControlTop;
    }
    const expandedInset = 56;
    const minTop = floatingBounds.minTop + expandedInset;
    const maxTop = Math.max(minTop, floatingBounds.maxTop - expandedInset);
    return clampFloatingTop(floatingControlTop, minTop, maxTop);
  }, [
    floatingBounds.maxTop,
    floatingBounds.minTop,
    floatingControlTop,
    gestureNavigation,
    gestureNavigationExpanded,
  ]);
  const effectiveFloatingControlStackStyle = useMemo(
    () =>
      !isWide
        ? ({
            top: `${effectiveFloatingControlTop}px`,
          } as const)
        : undefined,
    [effectiveFloatingControlTop, isWide],
  );
  const floatingDragVisualState =
    floatingDragState?.active
      ? 'dragging'
      : gestureNavigationExpanded
        ? 'gesture-open'
        : floatingDragState?.pressing || gestureNavState?.phase === 'pressing'
          ? 'drag-ready'
          : 'idle';
  const clearFloatingLongPressTimer = useCallback(() => {
    if (floatingLongPressTimerRef.current !== null) {
      window.clearTimeout(floatingLongPressTimerRef.current);
      floatingLongPressTimerRef.current = null;
    }
  }, []);
  const clearGestureLongPressTimer = useCallback(() => {
    if (gestureLongPressTimerRef.current !== null) {
      window.clearTimeout(gestureLongPressTimerRef.current);
      gestureLongPressTimerRef.current = null;
    }
  }, []);
  const clearGestureMoveLongPressTimer = useCallback(() => {
    if (gestureMoveLongPressTimerRef.current !== null) {
      window.clearTimeout(gestureMoveLongPressTimerRef.current);
      gestureMoveLongPressTimerRef.current = null;
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
  const clearChatQuickSwitchTimer = useCallback(() => {
    if (chatQuickSwitchTimerRef.current) {
      window.clearTimeout(chatQuickSwitchTimerRef.current);
      chatQuickSwitchTimerRef.current = null;
    }
  }, []);
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
  const handleChatQuickSwitchPointerDown = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      event.stopPropagation();
      if (isWide || event.button !== 0) {
        return;
      }
      if (floatingClickCooldownUntilRef.current > Date.now()) {
        return;
      }
      clearChatQuickSwitchTimer();
      event.currentTarget.setPointerCapture(event.pointerId);
      chatQuickSwitchPressRef.current = {
        pointerId: event.pointerId,
        originX: event.clientX,
        originY: event.clientY,
        longPressed: false,
      };
      chatQuickSwitchTimerRef.current = window.setTimeout(() => {
        const current = chatQuickSwitchPressRef.current;
        if (!current || current.pointerId !== event.pointerId) {
          return;
        }
        chatQuickSwitchPressRef.current = {
          ...current,
          longPressed: true,
        };
        chatQuickSwitchTimerRef.current = null;
        floatingClickCooldownUntilRef.current = Date.now() + 180;
        setPortRelayTargetMenuOpen(false);
        setChatQuickSwitchMenuOpen(true);
      }, GESTURE_LONG_PRESS_MS);
    },
    [clearChatQuickSwitchTimer, isWide],
  );
  const handleChatQuickSwitchPointerMove = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      event.stopPropagation();
      const current = chatQuickSwitchPressRef.current;
      if (!current || current.pointerId !== event.pointerId || current.longPressed) {
        return;
      }
      const distancePx = Math.hypot(event.clientX - current.originX, event.clientY - current.originY);
      if (distancePx < 10) {
        return;
      }
      clearChatQuickSwitchTimer();
      chatQuickSwitchPressRef.current = null;
    },
    [clearChatQuickSwitchTimer],
  );
  const finishChatQuickSwitchPress = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      event.stopPropagation();
      const current = chatQuickSwitchPressRef.current;
      if (!current || current.pointerId !== event.pointerId) {
        return;
      }
      clearChatQuickSwitchTimer();
      chatQuickSwitchPressRef.current = null;
      try {
        event.currentTarget.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture can already be released by the browser on some mobile WebViews.
      }
      if (!current.longPressed) {
        return;
      }
      event.preventDefault();
      floatingClickCooldownUntilRef.current = Date.now() + 180;
    },
    [clearChatQuickSwitchTimer],
  );
  useEffect(() => () => {
    clearChatQuickSwitchTimer();
  }, [clearChatQuickSwitchTimer]);
  useEffect(() => {
    if (mobilePortRelayFrameOpen) {
      setChatQuickSwitchMenuOpen(false);
    }
  }, [mobilePortRelayFrameOpen]);
  useEffect(() => {
    if (!chatQuickSwitchMenuOpen) {
      return;
    }
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target instanceof Node ? event.target : null;
      if (target && chatQuickSwitchMenuRef.current?.contains(target)) {
        return;
      }
      setChatQuickSwitchMenuOpen(false);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setChatQuickSwitchMenuOpen(false);
      }
    };
    window.addEventListener('pointerdown', handlePointerDown);
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('pointerdown', handlePointerDown);
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [chatQuickSwitchMenuOpen]);
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
      try {
        window.localStorage.setItem(PORT_RELAY_FLOATING_SLOT_STORAGE_KEY, nextSlot);
      } catch {
        // Ignore local storage failures in private or restricted contexts.
      }
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
    setChatQuickSwitchMenuOpen(false);
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
  const beginGestureNavigationPress = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      if (isWide || event.button !== 0) {
        return;
      }
      if (floatingClickCooldownUntilRef.current > Date.now()) {
        return;
      }
      clearGestureLongPressTimer();
      clearGestureMoveLongPressTimer();
      floatingIgnoreLostCaptureRef.current = false;
      event.currentTarget.setPointerCapture(event.pointerId);
      const startedAt = Date.now();
      const nextState: GestureNavigationState = {
        phase: 'pressing',
        pointerId: event.pointerId,
        originX: event.clientX,
        originY: event.clientY,
        currentX: event.clientX,
        currentY: event.clientY,
        startedAt,
        candidate: null,
      };
      gestureNavStateRef.current = nextState;
      setGestureNavState(nextState);
      gestureLongPressTimerRef.current = window.setTimeout(() => {
        setGestureNavState(current => {
          const next =
            current && current.pointerId === event.pointerId && current.phase === 'pressing'
              ? {...current, phase: 'expanded' as const}
              : current;
          gestureNavStateRef.current = next;
          return next;
        });
        gestureLongPressTimerRef.current = null;
      }, GESTURE_LONG_PRESS_MS);
      gestureMoveLongPressTimerRef.current = window.setTimeout(() => {
        const current = gestureNavStateRef.current;
        gestureMoveLongPressTimerRef.current = null;
        if (!current || current.pointerId !== event.pointerId) {
          return;
        }
        if (!shouldStartGestureMove({
          elapsedMs: Date.now() - current.startedAt,
          candidate: current.candidate,
        })) {
          return;
        }
        clearGestureLongPressTimer();
        gestureNavStateRef.current = null;
        setGestureNavState(null);
        setFloatingDragState({
          active: true,
          pressing: false,
          pointerId: current.pointerId,
          originY: current.currentY,
          startTop: floatingControlTop,
          currentTop: floatingControlTop,
          cooldownUntil: 0,
        });
      }, GESTURE_MOVE_LONG_PRESS_MS);
    },
    [
      clearGestureLongPressTimer,
      clearGestureMoveLongPressTimer,
      floatingControlTop,
      isWide,
      setFloatingDragState,
    ],
  );
  const handleGestureNavigationButtonPointerDown = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      beginGestureNavigationPress(event);
      event.stopPropagation();
    },
    [beginGestureNavigationPress],
  );
  const handleGestureNavigationPointerMove = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      const dragState = floatingDragStateRef.current;
      if (dragState?.pointerId === event.pointerId) {
        handleFloatingPointerMove(event);
        return;
      }
      const current = gestureNavStateRef.current;
      if (!current || current.pointerId !== event.pointerId) {
        return;
      }
      const deltaX = event.clientX - current.originX;
      const deltaY = event.clientY - current.originY;
      const distancePx = Math.hypot(deltaX, deltaY);
      const nextCurrent = {
        ...current,
        currentX: event.clientX,
        currentY: event.clientY,
      };
      if (current.phase !== 'expanded') {
        const intent = resolveGesturePressIntent({
          distancePx,
          elapsedMs: Date.now() - current.startedAt,
        });
        if (intent === 'neutral' && current.phase !== 'neutral') {
          clearGestureLongPressTimer();
          const nextState = {...nextCurrent, phase: 'neutral' as const};
          gestureNavStateRef.current = nextState;
          setGestureNavState(nextState);
          return;
        }
        if (intent === 'expand') {
          const nextState = {...nextCurrent, phase: 'expanded' as const};
          gestureNavStateRef.current = nextState;
          setGestureNavState(nextState);
          return;
        }
        if (current.currentX !== event.clientX || current.currentY !== event.clientY) {
          gestureNavStateRef.current = nextCurrent;
          setGestureNavState(nextCurrent);
        }
        return;
      }
      event.preventDefault();
      const directCandidate = gestureTabFromElement(
        document.elementFromPoint(event.clientX, event.clientY),
      );
      const candidate =
        directCandidate ?? resolveGestureDirectionCandidate({deltaX, deltaY});
      if (
        candidate !== current.candidate ||
        current.currentX !== event.clientX ||
        current.currentY !== event.clientY
      ) {
        const nextState = {...nextCurrent, candidate};
        gestureNavStateRef.current = nextState;
        setGestureNavState(nextState);
      }
    },
    [
      clearGestureLongPressTimer,
      handleFloatingPointerMove,
    ],
  );
  const finishGestureNavigation = useCallback(
    (pointerId: number) => {
      const current = gestureNavStateRef.current;
      if (!current || current.pointerId !== pointerId) {
        return;
      }
      clearGestureLongPressTimer();
      clearGestureMoveLongPressTimer();
      gestureNavStateRef.current = null;
      setGestureNavState(null);
      if (current.phase === 'pressing') {
        return;
      }
      if (current.phase === 'expanded' && current.candidate) {
        handleFloatingNavSelect(current.candidate);
      }
      const cooldownUntil = Date.now() + 120;
      floatingClickCooldownUntilRef.current = cooldownUntil;
      clearFloatingCooldownState(cooldownUntil);
    },
    [
      clearFloatingCooldownState,
      clearGestureLongPressTimer,
      clearGestureMoveLongPressTimer,
      handleFloatingNavSelect,
    ],
  );
  const cancelGestureNavigation = useCallback(
    (pointerId?: number) => {
      const current = gestureNavStateRef.current;
      if (typeof pointerId === 'number' && current && current.pointerId !== pointerId) {
        return;
      }
      clearGestureLongPressTimer();
      clearGestureMoveLongPressTimer();
      gestureNavStateRef.current = null;
      setGestureNavState(null);
      const cooldownUntil = Date.now() + 120;
      floatingClickCooldownUntilRef.current = cooldownUntil;
      clearFloatingCooldownState(cooldownUntil);
    },
    [clearFloatingCooldownState, clearGestureLongPressTimer, clearGestureMoveLongPressTimer],
  );
  const handleGestureNavigationOptionClick = useCallback(
    (nextTab: GestureNavigationTab, event: React.MouseEvent<HTMLButtonElement>) => {
      event.preventDefault();
      event.stopPropagation();
      clearGestureLongPressTimer();
      clearGestureMoveLongPressTimer();
      gestureNavStateRef.current = null;
      setGestureNavState(null);
      handleFloatingNavSelect(nextTab);
    },
    [clearGestureLongPressTimer, clearGestureMoveLongPressTimer, handleFloatingNavSelect],
  );
  useEffect(() => {
    if (!gestureNavigationExpanded) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        cancelGestureNavigation();
      }
    };
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && floatingControlStackRef.current?.contains(target)) {
        return;
      }
      cancelGestureNavigation();
    };
    window.addEventListener('keydown', onKeyDown);
    window.addEventListener('pointerdown', onPointerDown);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
      window.removeEventListener('pointerdown', onPointerDown);
    };
  }, [cancelGestureNavigation, gestureNavigationExpanded]);
  const handleDesktopActivitySelect = useCallback((nextTab: Tab) => {
    setPortRelayFrameOpen(false);
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
    setPortRelayFrameOpen(false);
    if (sidebarSettingsOpen && (
      settingsDetailView === 'update' ||
      settingsDetailView === 'skills' ||
      settingsDetailView === 'tokenStats' ||
      settingsDetailView === 'portRelay'
    )) {
      setSettingsDetailView(null);
      setSidebarSettingsOpen(true);
      setSidebarCollapsed(false);
      return;
    }
    const nextOpen = !sidebarSettingsOpen;
    setSidebarSettingsOpen(nextOpen);
    if (nextOpen) {
      setSettingsDetailView(null);
      setSidebarCollapsed(false);
    }
  }, [sidebarSettingsOpen, settingsDetailView, setSidebarSettingsOpen, setSidebarCollapsed]);
  const openSettingsDetail = useCallback((detail: Exclude<SettingsDetailView, null>) => {
    if (detail !== 'portRelay') {
      setPortRelayFrameOpen(false);
    }
    setSidebarSettingsOpen(true);
    setSidebarCollapsed(false);
    if (detail === 'tokenStats') {
      setTokenStatsError('');
    }
    if (detail === 'database') {
      openDatabasePanel();
    }
    if (detail === 'portRelay') {
      setPortRelayError('');
    }
    setSettingsDetailView(detail);
  }, [setSidebarSettingsOpen, setSidebarCollapsed]);
  const handleDesktopPortRelaySelect = useCallback(() => {
    setSidebarSettingsOpen(true);
    setSidebarCollapsed(false);
    setSettingsDetailView('portRelay');
    setPortRelayError('');
    if (!portRelayReady || !portRelayFrameUrl) {
      setPortRelayFrameOpen(false);
      return;
    }
    setPortRelayFrameOpen(open => !open);
  }, [portRelayFrameUrl, portRelayReady, setSidebarCollapsed, setSidebarSettingsOpen]);
  const handlePortRelayFloatingToggle = useCallback(() => {
    if (floatingClickCooldownUntilRef.current > Date.now()) {
      return;
    }
    setPortRelayTargetMenuOpen(false);
    if (!portRelayReady || !portRelayFrameUrl) {
      setPortRelayFrameOpen(false);
      return;
    }
    setDrawerOpen(false);
    setSidebarSettingsOpen(false);
    setPortRelayFrameOpen(open => !open);
  }, [portRelayFrameUrl, portRelayReady, setDrawerOpen, setSidebarSettingsOpen]);
  const handlePortRelayFloatingPointerDown = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      event.stopPropagation();
      if (isWide || event.button !== 0) {
        return;
      }
      if (floatingClickCooldownUntilRef.current > Date.now()) {
        return;
      }
      if (!mobilePortRelayFrameOpen) {
        return;
      }
      clearPortRelayTargetMenuTimer();
      event.currentTarget.setPointerCapture(event.pointerId);
      portRelayTargetMenuPressRef.current = {
        pointerId: event.pointerId,
        originX: event.clientX,
        originY: event.clientY,
        longPressed: false,
      };
      portRelayTargetMenuTimerRef.current = window.setTimeout(() => {
        const current = portRelayTargetMenuPressRef.current;
        if (!current || current.pointerId !== event.pointerId) {
          return;
        }
        portRelayTargetMenuPressRef.current = {
          ...current,
          longPressed: true,
        };
        portRelayTargetMenuTimerRef.current = null;
        floatingClickCooldownUntilRef.current = Date.now() + 180;
        setPortRelayTargetMenuOpen(true);
      }, GESTURE_LONG_PRESS_MS);
    },
    [clearPortRelayTargetMenuTimer, isWide, mobilePortRelayFrameOpen],
  );
  const handlePortRelayFloatingPointerMove = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      event.stopPropagation();
      const current = portRelayTargetMenuPressRef.current;
      if (!current || current.pointerId !== event.pointerId || current.longPressed) {
        return;
      }
      const distancePx = Math.hypot(event.clientX - current.originX, event.clientY - current.originY);
      if (distancePx < 10) {
        return;
      }
      clearPortRelayTargetMenuTimer();
      portRelayTargetMenuPressRef.current = null;
    },
    [clearPortRelayTargetMenuTimer],
  );
  const finishPortRelayFloatingPress = useCallback(
    (event: React.PointerEvent<HTMLButtonElement>) => {
      event.stopPropagation();
      const current = portRelayTargetMenuPressRef.current;
      if (!current || current.pointerId !== event.pointerId) {
        return;
      }
      clearPortRelayTargetMenuTimer();
      portRelayTargetMenuPressRef.current = null;
      try {
        event.currentTarget.releasePointerCapture(event.pointerId);
      } catch {
        // Pointer capture can already be released by the browser on some mobile WebViews.
      }
      if (!current.longPressed) {
        return;
      }
      event.preventDefault();
      floatingClickCooldownUntilRef.current = Date.now() + 180;
    },
    [clearPortRelayTargetMenuTimer],
  );
  useEffect(() => {
    if (isWide || !sidebarSettingsOpen) {
      mobileSettingsHistoryKeyRef.current = null;
      return;
    }
    const nextKey = mobileSettingsHistoryKey(settingsDetailView as MobileSettingsHistoryDetail | null);
    if (mobileSettingsHistoryKeyRef.current === nextKey) {
      return;
    }
    window.history.pushState(createMobileSettingsHistoryState(settingsDetailView as MobileSettingsHistoryDetail | null), '', window.location.href);
    mobileSettingsHistoryKeyRef.current = nextKey;
  }, [isWide, sidebarSettingsOpen, settingsDetailView]);
  useEffect(() => {
    const handleMobileSettingsPopState = (event: PopStateEvent) => {
      const nextState = event.state;
      const nextIsMobileSettingsHistory = isMobileSettingsHistoryState(nextState);
      const hadMobileSettingsHistory = mobileSettingsHistoryKeyRef.current !== null;
      if (!nextIsMobileSettingsHistory && !hadMobileSettingsHistory) {
        return;
      }
      mobileSettingsHistoryKeyRef.current = nextIsMobileSettingsHistory
        ? mobileSettingsHistoryKey(nextState.detail)
        : null;
      const action = resolveMobileSettingsPopAction({
        nextState,
        settingsOpen: sidebarSettingsOpenRef.current,
        settingsDetailView: settingsDetailViewRef.current as MobileSettingsHistoryDetail | null,
      });
      if (action === 'back-to-list') {
        setSettingsDetailView(null);
        return;
      }
      if (action === 'close-settings') {
        setSettingsDetailView(null);
        setSidebarSettingsOpen(false);
      }
    };
    window.addEventListener('popstate', handleMobileSettingsPopState);
    return () => window.removeEventListener('popstate', handleMobileSettingsPopState);
  }, [setSidebarSettingsOpen]);
  const handleSettingsDetailBack = useCallback(() => {
    if (!isWide && sidebarSettingsOpen && settingsDetailView !== null && mobileSettingsHistoryKeyRef.current !== null) {
      window.history.back();
      return;
    }
    setSettingsDetailView(null);
  }, [isWide, sidebarSettingsOpen, settingsDetailView]);
  const handleMobileSettingsBackButton = useCallback(() => {
    if (!isWide && sidebarSettingsOpen && mobileSettingsHistoryKeyRef.current !== null) {
      window.history.back();
      return;
    }
    if (settingsDetailView !== null) {
      setSettingsDetailView(null);
      return;
    }
    setSidebarSettingsOpen(false);
  }, [isWide, sidebarSettingsOpen, settingsDetailView, setSidebarSettingsOpen]);
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
        const normalized = normalizeAgentTypeName(value);
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
        try { navigator.vibrate?.(12); } catch { /* ignore */ }
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
      const pressX = event.clientX;
      const pressY = event.clientY;
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
        try { navigator.vibrate?.(12); } catch { /* ignore */ }
        setProjectSessionActionMenu({
          projectId: targetProjectId,
          sessionId,
          popover: resolveWideProjectActionPopoverPlacement({
            anchorRect: {
              left: pressX,
              top: pressY,
              right: pressX,
              bottom: pressY,
            },
            viewportWidth: window.innerWidth,
            viewportHeight: window.innerHeight,
            preferredWidth: 156,
            preferredMaxHeight: 190,
            align: 'start',
          }),
        });
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
      if (target?.closest('.project-session-action-menu')) {
        return;
      }
      setProjectSessionActionMenu(null);
    };
    window.addEventListener('pointerdown', onPointerDown);
    return () => window.removeEventListener('pointerdown', onPointerDown);
  }, [projectSessionActionMenu]);
  const openProjectSessionContextMenu = (
    targetProjectId: string,
    sessionId: string,
    event: React.MouseEvent<HTMLButtonElement>,
  ) => {
    event.preventDefault();
    event.stopPropagation();
    clearProjectSessionLongPress();
    const normalizedSessionId = sessionId.trim();
    if (!targetProjectId || !normalizedSessionId) {
      return;
    }
    projectSessionLongPressTargetRef.current = projectSessionActionKey(targetProjectId, normalizedSessionId);
    setProjectSessionActionMenu({
      projectId: targetProjectId,
      sessionId: normalizedSessionId,
      popover: resolveWideProjectActionPopoverPlacement({
        anchorRect: {
          left: event.clientX,
          top: event.clientY,
          bottom: event.clientY,
          right: event.clientX,
        },
        viewportWidth: window.innerWidth,
        viewportHeight: window.innerHeight,
        preferredWidth: 156,
        preferredMaxHeight: 190,
        align: 'start',
      }),
    });
  };
  currentProjectRef.current = currentProject;
  projectsRef.current = projects;
  expandedDirsRef.current = expandedDirs;
  selectedFileRef.current = selectedFile;

  const worktreeActive = selectedDiffSource === 'worktree';

  const isExpanded = (path: string) => expandedDirs.includes(path);
  const selectedFileIsMarkdown = isMarkdownPath(selectedFile);
  const selectedFileIsHtml = isHtmlPath(selectedFile);
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
      dirHashRef.current = {};
      fileHashRef.current = {};
      fileCacheRef.current = {};
    }
    const previousProjectId = projectIdRef.current;
    if (hydrated.projectId !== previousProjectId) {
      knownProjectRevRef.current = '';
      knownGitRevRef.current = '';
      knownWorktreeRevRef.current = '';
    }
    expandedDirsRef.current = hydrated.expandedDirs;
    selectedFileRef.current = hydrated.selectedFile;
    projectIdRef.current = hydrated.projectId;
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
    setWorkspaceProjectMenuOpen(false);
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

  const loadDirectory = async (path: string, options?: {projectId?: string}) => {
    if (loadingDirs[path]) return;
    const targetProjectId = options?.projectId || projectIdRef.current || projectId;
    setLoadingDirs(prev => ({ ...prev, [path]: true }));
    try {
      const persistedCache = targetProjectId
        ? workspaceStore.getCachedDirectory(targetProjectId, path)
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
          if (targetProjectId && Array.isArray(cachedEntries)) {
            workspaceStore.cacheDirectory(targetProjectId, path, result.hash, cachedEntries);
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
      if (targetProjectId) {
        workspaceStore.cacheDirectory(targetProjectId, path, nextHash, entries);
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
    const targetProjectId = projectIdRef.current || projectId;
    if (!targetProjectId) return;
    const requestSeq = fileReadSeqRef.current + 1;
    fileReadSeqRef.current = requestSeq;
    const silentRead = options?.silent === true;
    if (!silentRead) {
      setFileLoading(true);
    }
    const shouldRestoreScroll = options?.restoreScroll === true;
    try {
      const info = await service.getProjectFileInfo(targetProjectId, path);
      if (requestSeq !== fileReadSeqRef.current || projectIdRef.current !== targetProjectId) return;
      setFileInfo(info);
      const cacheKey = fileMemoryCacheKey(targetProjectId, path);
      const persistedFile = workspaceStore.getCachedFile(targetProjectId, path);
      if (
        typeof persistedFile?.content === 'string' &&
        fileCacheRef.current[cacheKey] === undefined
      ) {
        fileCacheRef.current[cacheKey] = persistedFile.content;
      }
      if (persistedFile?.hash && !fileHashRef.current[cacheKey]) {
        fileHashRef.current[cacheKey] = persistedFile.hash;
      }
      const cachedContent = fileCacheRef.current[cacheKey] ?? persistedFile?.content;
      const knownHash = typeof cachedContent === 'string'
        ? fileHashRef.current[cacheKey] || persistedFile?.hash || ''
        : '';
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
      const result = await service.readProjectFile(path, targetProjectId, {
        knownHash: knownHash || undefined,
      });
      if (requestSeq !== fileReadSeqRef.current || projectIdRef.current !== targetProjectId) return;
      if (result.notModified) {
        if (typeof cachedContent !== 'string') {
          const freshResult = await service.readProjectFile(path, targetProjectId);
          if (requestSeq !== fileReadSeqRef.current || projectIdRef.current !== targetProjectId) return;
          setFileContent(freshResult.content);
          fileCacheRef.current[cacheKey] = freshResult.content;
          const freshHash = freshResult.hash || knownHash;
          if (freshHash) {
            fileHashRef.current[cacheKey] = freshHash;
          }
          workspaceStore.cacheFile(targetProjectId, path, freshHash, freshResult.content);
          if (shouldRestoreScroll) {
            scheduleRestoreSelectedFileScroll(path);
          }
          return;
        }
        setFileContent(cachedContent);
        const nextHash = result.hash || knownHash;
        if (nextHash) {
          fileHashRef.current[cacheKey] = nextHash;
          workspaceStore.cacheFile(targetProjectId, path, nextHash, cachedContent);
        }
        if (shouldRestoreScroll) {
          scheduleRestoreSelectedFileScroll(path);
        }
        return;
      }
      setFileContent(result.content);
      fileCacheRef.current[cacheKey] = result.content;
      const nextHash = result.hash || knownHash;
      if (nextHash) {
        fileHashRef.current[cacheKey] = nextHash;
      }
      workspaceStore.cacheFile(targetProjectId, path, nextHash, result.content);
      if (shouldRestoreScroll) {
        scheduleRestoreSelectedFileScroll(path);
      }
    } catch (err) {
      if (requestSeq !== fileReadSeqRef.current || projectIdRef.current !== targetProjectId) return;
      if (!silentRead) {
        setFileInfo(null);
        setFileContent('');
      }
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (
        requestSeq === fileReadSeqRef.current &&
        projectIdRef.current === targetProjectId &&
        !silentRead
      ) {
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

  const loadGit = async (preferredRefs?: string[]): Promise<boolean> => {
    const targetProjectId = projectIdRef.current || projectId;
    if (!targetProjectId) return false;
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
      const loadedProject = currentProjectRef.current;
      if (loadedProject?.projectId === targetProjectId && loadedProject.git?.gitRev) {
        knownGitRevRef.current = loadedProject.git.gitRev;
      }
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
      return true;
    } catch (err) {
      setGitError(err instanceof Error ? err.message : String(err));
      return false;
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
    applySelectedChatKey(
      preferredSelection
        ? chatSessionKeyFromParts(projectIdRef.current, preferredSelection)
        : null,
    );
    chatVisibleRuntimeKeyRef.current = '';
    chatSelectedLoadAttemptRuntimeKeyRef.current = '';
    chatFinishedCursorRef.current = {};
    chatMessageStoreRef.current = {};
    chatTurnStoreRef.current = {};
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
        chatFinishedCursorRef.current[runtimeKey] = 0;
      } else {
        const cursor = getLatestSessionReadCursor(inMemoryMessages);
        chatFinishedCursorRef.current[runtimeKey] = cursor.turnIndex;
      }
      return inMemoryMessages;
    }

    const cachedMessages = [...cached.messages];
    chatTurnStoreRef.current[runtimeKey] = hydrateFinishedStore(cached.turns);
    chatMessageStoreRef.current[runtimeKey] = cachedMessages;

    const cursor = chatTurnStoreRef.current[runtimeKey].cursor;
    chatFinishedCursorRef.current[runtimeKey] = cursor.turnIndex;
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
    const sortedSessionRows = mergeChatSessionList(
      knownChatSessionsForProject(activeProjectId),
      sortChatSessions(sessionRows),
    );
    if (shouldUpdateCurrentProjectSessions(activeProjectId, projectIdRef.current)) {
      setChatSessions(prev => mergeChatSessionList(prev, sortedSessionRows));
    }
    setProjectSessionsByProjectId(prev => ({
      ...prev,
      [activeProjectId]: mergeChatSessionList(
        prev[activeProjectId] ?? knownChatSessionsForProject(activeProjectId),
        sortedSessionRows,
      ),
    }));

    for (const cached of cachedSessions) {
      const sessionId = cached.session.sessionId;
      if (!sessionId) continue;
      const content = workspaceStore.getCachedChatSessionContent(activeProjectId, sessionId);
      const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
      if (content) {
        chatTurnStoreRef.current[runtimeKey] = hydrateFinishedStore(content.turns);
      }
      const cursor = chatTurnStoreRef.current[runtimeKey]?.cursor ?? cached.cursor;
      chatFinishedCursorRef.current[runtimeKey] = cursor.turnIndex;
    }

    const persistedSelection = workspaceStore.migrateSelectedChatSessionKey(activeProjectId);
    const selectionResolution = resolveChatListSelection({
      activeProjectId,
      allowMissingSelection: true,
      availableSessionIds: sessionRows.map(session => session.sessionId),
      currentKey: selectedChatKeyRef.current,
      legacySelectionId: workspaceStore.getSelectedChatSessionId(activeProjectId),
      persistedKey: persistedSelection,
      preferredSelection,
    });
    if (!selectionResolution.canMutateSelection) {
      return;
    }
    const currentSelection = selectionResolution.sessionId;
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

  const persistChatSessionContent = (
    sessionId: string,
    activeProjectId = projectIdRef.current,
    session?: RegistryChatSession,
  ) => {
    if (!activeProjectId || !sessionId) return;
    const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
    const messages = chatMessageStoreRef.current[runtimeKey] ?? [];
    const turnState = chatTurnStoreRef.current[runtimeKey];
    const cursor = {
      turnIndex: turnState?.cursor.turnIndex ?? chatFinishedCursorRef.current[runtimeKey] ?? 0,
    };
    if (turnState) {
      workspaceStore.rememberChatSessionTurns(activeProjectId, sessionId, turnState.finished);
    } else {
      const cacheableMessages = messages.filter(isFinishedChatMessage);
      workspaceStore.rememberChatSessionContent(activeProjectId, sessionId, cacheableMessages);
    }
    const knownSession =
      projectSessionsByProjectId[activeProjectId]?.find(item => item.sessionId === sessionId) ??
      (shouldUpdateCurrentProjectSessions(activeProjectId, projectIdRef.current)
        ? chatSessions.find(item => item.sessionId === sessionId)
        : undefined);
    const targetSession = session
      ? mergeKnownChatSessionForProject(activeProjectId, session)
      : knownSession;
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
      if (result.session) {
        workspaceStore.rememberChatSession(
          activeProjectId,
          mergeKnownChatSessionForProject(activeProjectId, result.session),
          { turnIndex: chatFinishedCursorRef.current[runtimeKey] ?? 0 },
        );
      }
    } catch {
      // The next session.list/session.updated response will reconcile read state.
    }
  };

  const resolveSessionVisualState = (session: RegistryChatSession, activeProjectId = projectIdRef.current): ChatSessionVisualState => {
    void activeProjectId;
    return resolveChatSessionVisualStateValue(session);
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
    chatFinishedCursorRef.current[runtimeKey] = 0;
    chatMessageStoreRef.current[runtimeKey] = [];
    chatTurnStoreRef.current[runtimeKey] = createEmptyChatTurnStore();
    workspaceStore.rememberChatSessionTurns(targetProjectId, sessionId, []);
    const targetSession =
      projectSessionsByProjectId[targetProjectId]?.find(item => item.sessionId === sessionId) ??
      (targetProjectId === projectIdRef.current
        ? chatSessions.find(item => item.sessionId === sessionId)
        : undefined);
    if (targetSession) {
      workspaceStore.rememberChatSession(targetProjectId, targetSession, {turnIndex: 0});
    }
  };

  const readProjectSessionWithStaleCacheRepair = async (
    activeProjectId: string,
    sessionId: string,
    afterTurnIndex: number,
  ): Promise<{result: Awaited<ReturnType<typeof service.readProjectSession>>; appliedAfterTurnIndex: number}> => {
    const checkpoint = Math.max(0, Math.trunc(afterTurnIndex));
    const result = await service.readProjectSession(activeProjectId, sessionId, checkpoint);
    if (!isStaleSessionReadResult(checkpoint, result.latestTurnIndex)) {
      return {result, appliedAfterTurnIndex: checkpoint};
    }
    clearProjectSessionCache(activeProjectId, sessionId);
    const repairedResult = await service.readProjectSession(activeProjectId, sessionId, 0);
    return {result: repairedResult, appliedAfterTurnIndex: 0};
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
      const turnState = ensureChatTurnStore(runtimeKey);
      const existingStoreMessages = messagesFromTurnStore(runtimeKey, sessionId);
      const existingMessages = existingStoreMessages.length > 0
        ? existingStoreMessages
        : chatMessageStoreRef.current[runtimeKey] ?? [];
      const checkpointTurnIndex = requestedIncremental
        ? turnState.cursor.turnIndex
        : 0;
      const fallbackToFullRead =
        requestedIncremental &&
        existingMessages.length === 0 &&
        checkpointTurnIndex > 0;
      const useIncremental = requestedIncremental && !fallbackToFullRead;
      const readResult = await readProjectSessionWithStaleCacheRepair(
        activeProjectId,
        sessionId,
        useIncremental ? checkpointTurnIndex : 0,
      );
      const {result, appliedAfterTurnIndex} = readResult;
      const selectionSnapshot = options?.selectionSnapshot ?? '';
      if (
        options?.preserveUserSelection &&
        !shouldApplyPreservedChatLoad(selectedChatKeyRef.current, selectionSnapshot)
      ) {
        return false;
      }
      const resultSessionId = result.sessionId || result.session?.sessionId || sessionId;
      const resultRuntimeKey = buildChatRuntimeKey(activeProjectId, resultSessionId);
      const resultTurnState = ensureChatTurnStore(resultRuntimeKey);
      applySessionReadResult(
        resultTurnState,
        appliedAfterTurnIndex,
        result.turns,
        result.latestTurnIndex,
      );
      const nextMessages = messagesFromTurnStore(resultRuntimeKey, resultSessionId);
      forgetPendingPromptIfResolved(resultRuntimeKey, nextMessages);

      chatMessageStoreRef.current[resultRuntimeKey] = nextMessages;
      const latestSyncCursor = resultTurnState.cursor;
      chatFinishedCursorRef.current[resultRuntimeKey] = latestSyncCursor.turnIndex;
      const resultSession = result.session;
      if (resultSession) {
        setProjectSessionsByProjectId(prev => mergeProjectSessionMap(prev, activeProjectId, resultSession));
        if (activeProjectId === projectIdRef.current) {
          setChatSessions(prev => mergeChatSession(prev, resultSession));
        }
      }
      const nextSelectedKey = chatSessionKeyFromParts(activeProjectId, resultSessionId);
      const canApplyLoadedSelection = shouldApplyLoadedChatSelection(
        selectedChatKeyRef.current,
        nextSelectedKey,
      );
      if (canApplyLoadedSelection) {
        applySelectedChatKey(nextSelectedKey);
        workspaceStore.rememberSelectedChatSessionKey(nextSelectedKey);
        setVisibleChatMessagesForRuntimeKey(
          resultRuntimeKey,
          nextMessages,
          resolveChatSessionReadWindowUpdate({
            useIncremental: appliedAfterTurnIndex > 0,
            followsLatest: chatAutoScrollFollowRef.current,
          }),
        );
      }
      persistChatSessionContent(resultSessionId, activeProjectId, result.session);
      const knownSession =
        resultSession ??
        projectSessionsByProjectId[activeProjectId]?.find(item => item.sessionId === resultSessionId) ??
        (shouldUpdateCurrentProjectSessions(activeProjectId, projectIdRef.current)
          ? chatSessions.find(item => item.sessionId === resultSessionId)
          : undefined);
      if (
        canApplyLoadedSelection &&
        (knownSession?.lastDoneTurnIndex ?? 0) > (knownSession?.lastReadTurnIndex ?? 0)
      ) {
        markChatSessionRead(
          activeProjectId,
          resultSessionId,
          knownSession?.lastDoneTurnIndex ?? 0,
        ).catch(() => undefined);
      }
      return canApplyLoadedSelection;
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
    selectionSnapshot = '',
  ) => {
    if (!activeProjectId || !sessionId) return false;
    const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
    try {
      const turnState = ensureChatTurnStore(runtimeKey);
      const checkpointTurnIndex = turnState.cursor.turnIndex;
      const {result, appliedAfterTurnIndex} = await readProjectSessionWithStaleCacheRepair(
        activeProjectId,
        sessionId,
        checkpointTurnIndex,
      );
      if (!shouldApplyPreservedChatLoad(selectedChatKeyRef.current, selectionSnapshot)) {
        return false;
      }
      const resultSessionId = result.sessionId || result.session?.sessionId || sessionId;
      const resultRuntimeKey = buildChatRuntimeKey(activeProjectId, resultSessionId);
      const resultTurnState = ensureChatTurnStore(resultRuntimeKey);
      applySessionReadResult(
        resultTurnState,
        appliedAfterTurnIndex,
        result.turns,
        result.latestTurnIndex,
      );
      const nextMessages = messagesFromTurnStore(resultRuntimeKey, resultSessionId);
      forgetPendingPromptIfResolved(resultRuntimeKey, nextMessages);

      chatMessageStoreRef.current[resultRuntimeKey] = nextMessages;
      const latestSyncCursor = resultTurnState.cursor;
      chatFinishedCursorRef.current[resultRuntimeKey] = latestSyncCursor.turnIndex;
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
      if (
        encodeChatSessionKey(selectedChatKeyRef.current) === resultRuntimeKey &&
        resultSession &&
        (resultSession.lastDoneTurnIndex ?? 0) > (resultSession.lastReadTurnIndex ?? 0)
      ) {
        markChatSessionRead(
          activeProjectId,
          resultSessionId,
          resultSession.lastDoneTurnIndex ?? 0,
        ).catch(() => undefined);
      }
      return true;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return false;
    }
  };

  const syncChatSessionsAfterReconnect = async (
    preferredSelectedChatKey: ChatSessionKey | null | undefined,
  ) => {
    const selectedRuntimeKey =
      encodeChatSessionKey(preferredSelectedChatKey) ||
      encodeChatSessionKey(selectedChatKeyRef.current);
    const runtimeKeys = new Set(runtimeKeysFromChatStores());
    if (selectedRuntimeKey) {
      runtimeKeys.add(selectedRuntimeKey);
    }

    await Promise.all(Array.from(runtimeKeys).map(runtimeKey => {
      const key = decodeChatSessionKey(runtimeKey);
      if (!key) {
        return Promise.resolve();
      }
      const cursor = ensureChatTurnStore(runtimeKey).cursor.turnIndex;
      const selectionSnapshot = runtimeKey === selectedRuntimeKey ? runtimeKey : '';
      return chatReadRepairQueueRef.current.request(runtimeKey, cursor, async () => {
        await refreshSessionTurns(key.sessionId, key.projectId, selectionSnapshot);
      });
    }));
  };

  const loadChatSessions = async (
    activeProjectId = projectIdRef.current,
    preferredSelection = '',
  ) => {
    if (!activeProjectId) return;
    try {
      const listedSessions = sortChatSessions(await service.listProjectSessions(activeProjectId));
      const knownSessions = knownChatSessionsForProject(activeProjectId);
      const nextSessions = mergeChatSessionList(knownSessions, listedSessions);
      setProjectSessionsByProjectId(prev => ({
        ...prev,
        [activeProjectId]: mergeChatSessionList(
          prev[activeProjectId] ?? knownSessions,
          listedSessions,
        ),
      }));
      if (shouldUpdateCurrentProjectSessions(activeProjectId, projectIdRef.current)) {
        setChatSessions(prev => mergeChatSessionList(prev, listedSessions));
      }

      const cursorBySessionId: Record<string, {turnIndex: number}> = {};
      for (const session of nextSessions) {
        const sessionId = session.sessionId;
        if (!sessionId) continue;
        const runtimeKey = buildChatRuntimeKey(activeProjectId, sessionId);
        cursorBySessionId[sessionId] = {
          turnIndex: chatFinishedCursorRef.current[runtimeKey] ?? 0,
        };
      }
      workspaceStore.replaceChatSessions(activeProjectId, nextSessions, cursorBySessionId);

      const persistedSelection = workspaceStore.migrateSelectedChatSessionKey(activeProjectId);
      const selectionResolution = resolveChatListSelection({
        activeProjectId,
        availableSessionIds: nextSessions.map(session => session.sessionId),
        currentKey: selectedChatKeyRef.current,
        legacySelectionId: workspaceStore.getSelectedChatSessionId(activeProjectId),
        persistedKey: persistedSelection,
        preferredSelection,
      });
      if (!selectionResolution.canMutateSelection) {
        return;
      }
      const currentSelection = selectionResolution.sessionId;
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

  useEffect(() => {
    const selectedKey = selectedChatKeyRef.current;
    const runtimeKey = encodeChatSessionKey(selectedKey);
    if (!selectedKey || !runtimeKey) {
      return;
    }
    if (tab !== 'chat' || !connected || chatLoading) {
      return;
    }
    const shouldInspectCache =
      chatVisibleRuntimeKeyRef.current !== runtimeKey ||
      chatMessagesRef.current.length === 0;
    const cachedMessages = shouldInspectCache
      ? hydrateChatSessionContentFromCache(selectedKey.sessionId, selectedKey.projectId)
      : [];
    const selectedVisibilityRecovery = resolveSelectedChatVisibilityRecovery({
      tab,
      connected,
      chatLoading,
      selectedRuntimeKey: runtimeKey,
      visibleRuntimeKey: chatVisibleRuntimeKeyRef.current,
      visibleMessageCount: chatMessagesRef.current.length,
      cachedMessageCount: cachedMessages.length,
      attemptedRuntimeKey: chatSelectedLoadAttemptRuntimeKeyRef.current,
    });

    if (selectedVisibilityRecovery === 'restore-cache') {
      setVisibleChatMessagesForRuntimeKey(runtimeKey, cachedMessages, {resetToLatest: true});
      return;
    }

    if (selectedVisibilityRecovery === 'read-session') {
      chatSelectedLoadAttemptRuntimeKeyRef.current = runtimeKey;
      if (chatVisibleRuntimeKeyRef.current !== runtimeKey) {
        setVisibleChatMessagesForRuntimeKey(runtimeKey, [], {resetToLatest: true});
      }
      loadChatSession(selectedKey.sessionId, selectedKey.projectId, {
        incremental: true,
        preserveUserSelection: true,
        selectionSnapshot: runtimeKey,
      }).then(loaded => {
        if (!loaded && chatSelectedLoadAttemptRuntimeKeyRef.current === runtimeKey) {
          chatSelectedLoadAttemptRuntimeKeyRef.current = '';
        }
      }).catch(() => {
        if (chatSelectedLoadAttemptRuntimeKeyRef.current === runtimeKey) {
          chatSelectedLoadAttemptRuntimeKeyRef.current = '';
        }
      });
    }
  }, [tab, connected, selectedChatEncodedKey, chatMessages.length, chatLoading, setVisibleChatMessagesForRuntimeKey]);
  const resetChatComposer = () => {
    chatAttachmentsRef.current.forEach(revokeChatAttachmentObjectUrl);
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

  const resetChatComposerDraft = (draftKey: string) => {
    const normalizedDraftKey = draftKey.trim();
    if (!normalizedDraftKey) {
      return;
    }
    if (normalizedDraftKey === currentChatDraftKeyRef.current) {
      resetChatComposer();
      return;
    }
    const draft = chatComposerDraftsRef.current[normalizedDraftKey];
    draft?.attachments.forEach(revokeChatAttachmentObjectUrl);
    bumpChatDraftGeneration(normalizedDraftKey);
    saveChatComposerDraft(normalizedDraftKey, '', []);
  };

  const clearPendingChatPromptTimer = (runtimeKey: string) => {
    const timerId = chatPendingPromptTimersRef.current[runtimeKey];
    if (timerId !== undefined) {
      window.clearTimeout(timerId);
      const nextTimers = {...chatPendingPromptTimersRef.current};
      delete nextTimers[runtimeKey];
      chatPendingPromptTimersRef.current = nextTimers;
    }
  };

  const markPendingChatPromptUndelivered = (runtimeKey: string, errorMessage = 'Server did not confirm receipt.') => {
    const pending = chatPendingPromptsByKeyRef.current[runtimeKey];
    if (!pending || pending.status === 'undelivered') return;
    clearPendingChatPromptTimer(runtimeKey);
    const next = {
      ...chatPendingPromptsByKeyRef.current,
      [runtimeKey]: {
        ...pending,
        status: 'undelivered' as const,
        errorMessage,
      },
    };
    chatPendingPromptsByKeyRef.current = next;
    setChatPendingPromptsByKey(next);
    setChatSending(false);
  };

  const rememberPendingChatPrompt = (runtimeKey: string, prompt: PendingChatPrompt) => {
    if (!runtimeKey) return;
    clearPendingChatPromptTimer(runtimeKey);
    const next = {
      ...chatPendingPromptsByKeyRef.current,
      [runtimeKey]: prompt,
    };
    chatPendingPromptsByKeyRef.current = next;
    setChatPendingPromptsByKey(next);
    if (prompt.status === 'confirming') {
      const timerId = window.setTimeout(() => {
        markPendingChatPromptUndelivered(runtimeKey);
      }, CHAT_PENDING_CONFIRM_TIMEOUT_MS);
      chatPendingPromptTimersRef.current = {
        ...chatPendingPromptTimersRef.current,
        [runtimeKey]: timerId,
      };
    }
  };

  const forgetPendingChatPrompt = (runtimeKey: string) => {
    if (!runtimeKey || !chatPendingPromptsByKeyRef.current[runtimeKey]) return;
    clearPendingChatPromptTimer(runtimeKey);
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
    clearPendingChatPromptTimer(fromRuntimeKey);
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
    if (prompt.status === 'confirming') {
      const timerId = window.setTimeout(() => {
        markPendingChatPromptUndelivered(toRuntimeKey);
      }, CHAT_PENDING_CONFIRM_TIMEOUT_MS);
      chatPendingPromptTimersRef.current = {
        ...chatPendingPromptTimersRef.current,
        [toRuntimeKey]: timerId,
      };
    }
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
    anchor: HTMLElement | null,
  ) => {
    resetProjectResumeState();
    setMobileProjectActionMenu(null);
    setWideProjectActionMenu({
      projectId: targetProjectId,
      kind,
      phase: 'agents',
      agentType: '',
      popover: resolveWideProjectActionPopoverPlacement({
        anchorRect: anchor?.getBoundingClientRect() ?? null,
        viewportWidth: window.innerWidth,
        viewportHeight: window.innerHeight,
      }),
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
            popover: null,
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
    if (encodeChatSessionKey(selectedChatKeyRef.current) === runtimeKey) {
        applySelectedChatKey(null);
        setChatMessages([]);
        workspaceStore.rememberSelectedChatSessionKey(null);
    }
      const nextMessageStore = {...chatMessageStoreRef.current};
      const nextFinishedCursor = {...chatFinishedCursorRef.current};
      delete nextMessageStore[runtimeKey];
      delete nextFinishedCursor[runtimeKey];
      chatMessageStoreRef.current = nextMessageStore;
      chatFinishedCursorRef.current = nextFinishedCursor;
    workspaceStore.deleteChatSession(targetProjectId, sessionId);
  };

  const handleArchiveProjectSession = async (targetProjectId: string, sessionId: string) => {
    const normalizedSessionId = sessionId.trim();
    if (!targetProjectId || !normalizedSessionId || chatArchivingSessionId) {
      return;
    }
    setConfirmError('');
    setChatArchivingSessionId(normalizedSessionId);
    try {
      const result = await service.archiveProjectSession(targetProjectId, normalizedSessionId);
      if (!result.ok) {
        throw new Error('session.archive returned ok=false');
      }
      removeProjectChatSessionFromState(
        targetProjectId,
        result.sessionId || normalizedSessionId,
      );
      setProjectSessionActionMenu(null);
      setConfirmTarget(null);
      setConfirmError('');
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setConfirmError(message);
      setError(message);
    } finally {
      setChatArchivingSessionId('');
    }
  };

  const handleDeleteProjectSession = async (targetProjectId: string, sessionId: string) => {
    const normalizedSessionId = sessionId.trim();
    if (!targetProjectId || !normalizedSessionId || chatDeletingSessionId) {
      return;
    }
    setConfirmError('');
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
      setConfirmTarget(null);
      setConfirmError('');
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setConfirmError(message);
      setError(message);
    } finally {
      setChatDeletingSessionId('');
    }
  };

  const handleRenameProjectSession = async (targetProjectId: string, sessionId: string, title: string) => {
    const normalizedSessionId = sessionId.trim();
    const normalizedTitle = Array.from(title.replace(/\r\n|\r|\n/g, ' ').trim()).slice(0, 200).join('');
    if (!targetProjectId || !normalizedSessionId || chatRenamingSessionId) {
      return;
    }
    setRenameError('');
    setChatRenamingSessionId(normalizedSessionId);
    try {
      const result = await service.renameProjectSession(targetProjectId, normalizedSessionId, normalizedTitle);
      if (!result.ok) {
        throw new Error('session.rename returned ok=false');
      }
      const session = result.session ?? {sessionId: result.sessionId || normalizedSessionId, title: normalizedTitle};
      rememberChatSessionSummary(targetProjectId, session);
      const runtimeKey = buildChatRuntimeKey(targetProjectId, session.sessionId);
      workspaceStore.rememberChatSession(
        targetProjectId,
        mergeKnownChatSessionForProject(targetProjectId, session),
        {turnIndex: chatFinishedCursorRef.current[runtimeKey] ?? 0},
      );
      setProjectSessionActionMenu(null);
      setRenameTarget(null);
      setRenameTitleDraft('');
      setRenameError('');
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setRenameError(message);
      setError(message);
    } finally {
      setChatRenamingSessionId('');
    }
  };

  const requestRenameProjectSession = (targetProjectId: string, session: RegistrySessionSummary) => {
    const normalizedSessionId = session.sessionId.trim();
    if (!targetProjectId || !normalizedSessionId || chatRenamingSessionId) {
      return;
    }
    const title = resolveSessionDisplayTitle(session);
    setProjectSessionActionMenu(null);
    setRenameError('');
    setRenameTitleDraft(title);
    setRenameTarget({
      projectId: targetProjectId,
      sessionId: normalizedSessionId,
      title,
    });
  };

  useEffect(() => {
    if (!isWide || tab !== 'chat' || sidebarSettingsOpen || !selectedChatKey || !selectedChatSession || renameTarget || confirmTarget) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (
        event.defaultPrevented ||
        event.key !== 'F2' ||
        event.repeat ||
        event.isComposing ||
        event.ctrlKey ||
        event.metaKey ||
        event.altKey ||
        event.shiftKey
      ) {
        return;
      }
      event.preventDefault();
      requestRenameProjectSession(selectedChatKey.projectId, selectedChatSession);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [
    confirmTarget,
    isWide,
    renameTarget,
    requestRenameProjectSession,
    sidebarSettingsOpen,
    selectedChatKey,
    selectedChatSession,
    tab,
  ]);

  const requestArchiveProjectSession = (targetProjectId: string, session: RegistrySessionSummary) => {
    const normalizedSessionId = session.sessionId.trim();
    if (!targetProjectId || !normalizedSessionId || session.running || chatArchivingSessionId) {
      return;
    }
    setProjectSessionActionMenu(null);
    setConfirmError('');
    setConfirmTarget({
      kind: 'archive',
      projectId: targetProjectId,
      sessionId: normalizedSessionId,
      title: resolveSessionDisplayTitle(session),
    });
  };

  const requestDeleteProjectSession = (targetProjectId: string, session: RegistrySessionSummary) => {
    const normalizedSessionId = session.sessionId.trim();
    if (!targetProjectId || !normalizedSessionId || session.running || chatDeletingSessionId) {
      return;
    }
    setProjectSessionActionMenu(null);
    setConfirmError('');
    setConfirmTarget({
      kind: 'delete',
      projectId: targetProjectId,
      sessionId: normalizedSessionId,
      title: resolveSessionDisplayTitle(session),
    });
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

  const sendChatMessage = async (options: {
    textOverride?: string;
    attachmentsOverride?: ChatAttachment[];
    blocksOverride?: RegistryChatContentBlock[];
    preserveComposer?: boolean;
  } = {}) => {
    if (chatSending) {
      return;
    }
    const sourceAttachments = options.attachmentsOverride ?? chatAttachments;
    const trimmedText = (options.textOverride ?? chatComposerText).trim();
    if (trimmedText === '/cancel' && sourceAttachments.length === 0 && !options.blocksOverride) {
      setError('Use the stop button to cancel in app.');
      return;
    }
    if (!options.blocksOverride && !trimmedText && sourceAttachments.length === 0) {
      return;
    }
    if (options.blocksOverride && options.blocksOverride.length === 0) {
      return;
    }
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
    const draftKey = currentChatDraftKeyRef.current;
    const draftGeneration = getChatDraftGeneration(draftKey);
    let pendingRemembered = false;
    setChatSending(true);
    try {
      const uploadedAttachments = options.blocksOverride ? sourceAttachments : await uploadChatAttachmentsForSend(
        sourceAttachments,
        selectedProjectId,
        sessionId,
        draftKey,
        draftGeneration,
      );
      const blocks: RegistryChatContentBlock[] = [];
      if (options.blocksOverride) {
        blocks.push(...options.blocksOverride.map(block => ({...block})));
      } else {
        if (trimmedText) {
          blocks.push({ type: 'text', text: trimmedText });
        }
        blocks.push(...uploadedAttachments.map(attachment => attachment.block).filter(isRegistryChatContentBlock));
      }
      if (blocks.length === 0) return;
      const firstAttachmentName = uploadedAttachments[0]?.name || '';
      const previewText = trimmedText || firstAttachmentName || msgText('prompt_request', {contentBlocks: blocks}).trim();
      const createdAt = new Date().toISOString();
      rememberPendingChatPrompt(runtimeKey, {
        sessionId,
        blocks: blocks.map(block => ({...block})),
        createdAt,
        turnIndex: nextPromptTurnIndex(chatMessageStoreRef.current[runtimeKey] ?? []),
        status: 'confirming',
      });
      pendingRemembered = true;

      if (!options.preserveComposer) {
        resetChatComposerDraft(draftKey);
      }
      forceChatScrollToBottom();
      const result = await service.sendProjectSessionMessage(selectedProjectId, {
        sessionId,
        text: trimmedText || previewText,
        blocks,
      });
      if (!result.ok) {
        throw new Error('session.send returned ok=false');
      }
      const nextSessionId = result.sessionId || sessionId;
      if (nextSessionId !== sessionId) {
        movePendingChatPrompt(runtimeKey, buildChatRuntimeKey(selectedProjectId, nextSessionId), nextSessionId);
      }
      const nextSelectedKey = chatSessionKeyFromParts(selectedProjectId, nextSessionId);
      if (shouldApplySentChatSelection(selectedChatKeyRef.current, selectedKey)) {
        applySelectedChatKey(nextSelectedKey);
        workspaceStore.rememberSelectedChatSessionKey(nextSelectedKey);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      if (pendingRemembered) {
        markPendingChatPromptUndelivered(runtimeKey, message);
      }
      setError(message);
    } finally {
      setChatSending(false);
    }
  };

  const sendDirectChatText = async (text: string) => {
    const normalizedText = text.trim();
    if (!normalizedText) {
      return;
    }
    setChatFileMentionMenuOpen(false);
    setChatPromptMenuOpen(false);
    setChatConfigMenuOptionId('');
    setChatConfigOverflowOpen(false);
    await sendChatMessage({
      textOverride: normalizedText,
      attachmentsOverride: [],
      preserveComposer: true,
    });
  };

  const buildChatAttachmentsFromBlocks = (blocks: RegistryChatContentBlock[]): ChatAttachment[] => {
    const attachments: ChatAttachment[] = [];
    for (const [index, block] of blocks.entries()) {
      const blockType = typeof block.type === 'string' ? block.type.trim().toLowerCase() : '';
      if (blockType !== 'image' && blockType !== 'resource_link') {
        continue;
      }
      if (!block.uri && !block.data) {
        continue;
      }
      chatAttachmentIdRef.current += 1;
      attachments.push({
        id: `chat-attachment-${chatAttachmentIdRef.current}`,
        name: block.name || `undelivered-attachment-${index + 1}`,
        mimeType: typeof block.mimeType === 'string' ? block.mimeType : '',
        size: typeof block.size === 'number' ? block.size : 0,
        status: 'completed',
        progress: 100,
        block: {...block},
        attachmentId: attachmentIdFromBlock(block),
      });
    }
    return attachments;
  };

  const retryPendingChatPrompt = (runtimeKey: string) => {
    const pending = chatPendingPromptsByKeyRef.current[runtimeKey];
    if (!pending) return;
    sendChatMessage({
      textOverride: '',
      attachmentsOverride: [],
      blocksOverride: pending.blocks,
      preserveComposer: true,
    }).catch(() => undefined);
  };

  const editPendingChatPrompt = (runtimeKey: string) => {
    const pending = chatPendingPromptsByKeyRef.current[runtimeKey];
    if (!pending) return;
    if (
      (chatComposerTextRef.current.trim() || chatAttachmentsRef.current.length > 0) &&
      !window.confirm('Replace the current draft with this undelivered message?')
    ) {
      return;
    }
    const text = extractTextFromACPContent(pending.blocks);
    const attachments = buildChatAttachmentsFromBlocks(pending.blocks);
    chatComposerTextRef.current = text;
    chatAttachmentsRef.current = attachments;
    bumpChatDraftGeneration(currentChatDraftKeyRef.current);
    setChatComposerText(text);
    setChatAttachments(attachments);
    saveChatComposerDraft(currentChatDraftKeyRef.current, text, attachments);
    forgetPendingChatPrompt(runtimeKey);
    window.setTimeout(resizeChatComposerTextarea, 0);
  };

  const cancelSelectedChatPrompt = async () => {
    const selectedKey = selectedChatKeyRef.current;
    if (!selectedKey?.sessionId) {
      return;
    }
    const runtimeKey = encodeChatSessionKey(selectedKey);
    if (!runtimeKey || chatCancellingRuntimeKey === runtimeKey) {
      return;
    }
    setChatCancellingRuntimeKey(runtimeKey);
    setError('');
    try {
      const result = await service.cancelProjectSession(selectedKey.projectId, selectedKey.sessionId);
      if (!result.ok) {
        throw new Error('session.cancel returned ok=false');
      }
    } catch (err) {
      setChatCancellingRuntimeKey(current => (current === runtimeKey ? '' : current));
      setError(err instanceof Error ? err.message : String(err));
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

  const handleChatFileChange = (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    if (chatSending) {
      event.target.value = '';
      return;
    }
    const files = chatFilesFromFileList(event.target.files);
    if (files.length === 0) {
      return;
    }
    const attachmentDraftKey = currentChatDraftKeyRef.current;
    const attachmentDraftGeneration = getChatDraftGeneration(attachmentDraftKey);
    enqueueChatAttachmentFiles(files, attachmentDraftKey, attachmentDraftGeneration);
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
      submitDesktopRemoteWebCandidate(ws);
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
      setRegistryHubs(result.hubs);
      setLocalHubReadStatuses(service.getLocalHubReadStatuses(result.hubs));
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
        syncChatSessionsAfterReconnect(preferredSelectedChatKey).catch(() => undefined);
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

  const handleRegistryDebugLogout = () => {
    supervisorManagedCloseRef.current = true;
    clearReconnectTimer();
    reconnectStartedAtRef.current = null;
    workspaceStore.clearLocalToken();
    tokenRef.current = '';
    setToken('');
    setError('');
    setAutoConnecting(false);
    setReconnecting(false);
    setRegistryDebugPanelOpen(false);
    setConnected(false);
    clearChatRuntimeState();
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

    const sessionDisplayTitle = resolveSessionDisplayTitle(session);
    const title = sessionDisplayTitle
      ? `Chat: ${sessionDisplayTitle}`
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

  const mergeTokenProviders = useCallback(
    (entries: Array<{hubId: string; projectId?: string; result: RegistryTokenScanResult}>): TokenProviderSectionView[] => {
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
              projectId: entry.projectId ?? '',
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
      const snapshot = await service.listProjectSnapshot();
      if (snapshot.projects.length > 0) {
        setProjects(snapshot.projects);
      }
      setRegistryHubs(snapshot.hubs);
      setLocalHubReadStatuses(service.getLocalHubReadStatuses(snapshot.hubs));
      const hubIds = deriveRegistryHubIds(snapshot.hubs);
      if (hubIds.length === 0) {
        setTokenStatsProviders([]);
        setTokenStatsUpdatedAt('');
        setTokenStatsError('No hubs available.');
        return;
      }
      const requests = hubIds.map(async hubId => {
        const result = await service.scanTokenStats(hubId);
        return {hubId, result};
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

  const agentPackageActionKey = useCallback((hubId: string, packageName: string): string => {
    return `${hubId}:${packageName}`;
  }, []);

  const refreshProjectHubSnapshot = useCallback(async (): Promise<string[]> => {
    const snapshot = await service.listProjectSnapshot();
    if (snapshot.projects.length > 0) {
      setProjects(snapshot.projects);
    }
    setRegistryHubs(snapshot.hubs);
    setLocalHubReadStatuses(service.getLocalHubReadStatuses(snapshot.hubs));
    return deriveRegistryHubIds(snapshot.hubs);
  }, []);

  const persistPortRelaySettings = useCallback((patch: {
    targets?: PortRelayTarget[];
    selectedTarget?: PortRelayTarget | null;
    listenPort?: string | number;
  }) => {
    const nextListenPort = normalizePortRelayListenPort(
      patch.listenPort ?? portRelayListenPort,
      normalizePortRelayListenPort(portRelayListenPort),
    );
    workspaceStore.rememberGlobalState({
      portRelayTargets: patch.targets ?? portRelayTargets,
      selectedPortRelayTarget: patch.selectedTarget !== undefined ? patch.selectedTarget : selectedPortRelayTarget,
      portRelayListenPort: nextListenPort,
    });
  }, [portRelayListenPort, portRelayTargets, selectedPortRelayTarget]);

  const applyPortRelaySnapshot = useCallback((snapshot: RegistryPortRelaySnapshot) => {
    setPortRelaySnapshot(snapshot);
    if (typeof snapshot.listenPort === 'number') {
      setPortRelayListenPort(String(snapshot.listenPort));
      persistPortRelaySettings({listenPort: snapshot.listenPort});
    }
    if (!snapshot.enabled) {
      return;
    }
    const reconciled = reconcilePortRelayTargetSelection({
      targets: portRelayTargets,
      selectedTarget: selectedPortRelayTarget,
      snapshot,
    });
    if (!samePortRelayTargets(portRelayTargets, reconciled.targets)) {
      setPortRelayTargets(reconciled.targets);
    }
    if (!samePortRelayTarget(selectedPortRelayTarget, reconciled.selectedTarget)) {
      setSelectedPortRelayTarget(reconciled.selectedTarget);
    }
    persistPortRelaySettings({
      targets: reconciled.targets,
      selectedTarget: reconciled.selectedTarget,
      listenPort: snapshot.listenPort,
    });
  }, [persistPortRelaySettings, portRelayTargets, selectedPortRelayTarget]);

  const refreshPortRelayStatus = useCallback(async (options?: {silent?: boolean}) => {
    const silent = options?.silent === true;
    if (!silent) {
      setPortRelayLoading(true);
      setPortRelayError('');
    }
    try {
      const snapshot = await service.getPortRelayStatus();
      applyPortRelaySnapshot(snapshot);
    } catch (err) {
      if (!silent) {
        setPortRelayError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (!silent) {
        setPortRelayLoading(false);
      }
    }
  }, [applyPortRelaySnapshot]);

  useEffect(() => {
    if (!connected || !portRelaySnapshot.enabled || portRelaySnapshot.status !== 'Opening') {
      return;
    }
    const timer = window.setInterval(() => {
      refreshPortRelayStatus({silent: true}).catch(() => undefined);
    }, 1000);
    return () => {
      window.clearInterval(timer);
    };
  }, [connected, portRelaySnapshot.enabled, portRelaySnapshot.status, refreshPortRelayStatus]);

  useEffect(() => {
    if (selectedPortRelayTarget || portRelayTargets.length === 0) {
      return;
    }
    const nextTarget = portRelayTargets[0];
    setSelectedPortRelayTarget(nextTarget);
    persistPortRelaySettings({selectedTarget: nextTarget});
  }, [persistPortRelaySettings, portRelayTargets, selectedPortRelayTarget]);

  useEffect(() => {
    if (settingsDetailView !== 'portRelay') {
      return;
    }
    refreshPortRelayStatus().catch(() => undefined);
  }, [settingsDetailView]);

  useEffect(() => {
    if (settingsDetailView !== 'portRelay' || portRelayAccessCode || portRelaySnapshot.enabled) {
      return;
    }
    setPortRelayAccessCode(generatePortRelayAccessCode());
  }, [portRelayAccessCode, portRelaySnapshot.enabled, settingsDetailView]);

  const commitPortRelayDraftTarget = useCallback((): PortRelayTarget | null => {
    const target = normalizePortRelayTarget({
      hubId: portRelayDraftHubId,
      targetPort: portRelayDraftPort,
    });
    if (!target) {
      return null;
    }
    const nextTargets = upsertPortRelayTarget(portRelayTargets, target);
    setPortRelayTargets(nextTargets);
    setSelectedPortRelayTarget(target);
    setPortRelayDraftHubId('');
    setPortRelayDraftPort('80');
    persistPortRelaySettings({
      targets: nextTargets,
      selectedTarget: target,
    });
    return target;
  }, [persistPortRelaySettings, portRelayDraftHubId, portRelayDraftPort, portRelayTargets]);

  const enablePortRelayForTarget = useCallback(async (
    target: PortRelayTarget | null,
    listenPortValue = portRelayListenPort,
    options: {framePath?: string; openFrame?: boolean} = {},
  ) => {
    const normalizedTarget = normalizePortRelayTarget(target);
    const listenPort = Number(listenPortValue);
    if (portRelayAccessCodeUnknown) {
      setPortRelayError('Access code is unknown on this device. Generate a new code before switching target.');
      return;
    }
    if (options.framePath !== undefined) {
      setPortRelayFramePath(options.framePath);
    }
    const accessCode = portRelayAccessCode || generatePortRelayAccessCode();
    setPortRelayAccessCode(accessCode);
    if (!Number.isInteger(listenPort) || listenPort < 1 || listenPort > 65535) {
      setPortRelayError('Listen port must be in 1..65535.');
      return;
    }
    if (!normalizedTarget) {
      setPortRelayError('Target is required.');
      return;
    }
    const nextTargets = upsertPortRelayTarget(portRelayTargets, normalizedTarget);
    setPortRelayTargets(nextTargets);
    setSelectedPortRelayTarget(normalizedTarget);
    persistPortRelaySettings({
      targets: nextTargets,
      selectedTarget: normalizedTarget,
      listenPort,
    });
    setPortRelayLoading(true);
    setPortRelayError('');
    try {
      const snapshot = await service.enablePortRelay({
        listenPort,
        hubId: normalizedTarget.hubId,
        targetHost: '127.0.0.1',
        targetPort: normalizedTarget.targetPort,
        accessCode,
      });
      setPortRelayKnownAccessCodeGeneration(typeof snapshot.accessCodeGeneration === 'number' ? snapshot.accessCodeGeneration : null);
      setPortRelayFrameAutoOpenPending((options.openFrame ?? isWide) && snapshot.enabled);
      applyPortRelaySnapshot(snapshot);
    } catch (err) {
      setPortRelayError(err instanceof Error ? err.message : String(err));
    } finally {
      setPortRelayLoading(false);
    }
  }, [applyPortRelaySnapshot, isWide, persistPortRelaySettings, portRelayAccessCode, portRelayAccessCodeUnknown, portRelayListenPort, portRelayTargets]);

  const enablePortRelay = useCallback(async () => {
    const target = selectedPortRelayTarget ?? commitPortRelayDraftTarget();
    await enablePortRelayForTarget(target, portRelayListenPort, {framePath: ''});
  }, [commitPortRelayDraftTarget, enablePortRelayForTarget, portRelayListenPort, selectedPortRelayTarget]);

  const disablePortRelay = useCallback(async () => {
    setPortRelayLoading(true);
    setPortRelayError('');
    try {
      const snapshot = await service.disablePortRelay();
      setPortRelayFramePath('');
      applyPortRelaySnapshot(snapshot);
    } catch (err) {
      setPortRelayError(err instanceof Error ? err.message : String(err));
    } finally {
      setPortRelayLoading(false);
    }
  }, [applyPortRelaySnapshot]);

  const regeneratePortRelayAccessCode = useCallback(async () => {
    const accessCode = generatePortRelayAccessCode();
    setPortRelayAccessCode(accessCode);
    setPortRelayCodeCopied(false);
    if (!portRelaySnapshot.enabled) {
      return;
    }
    setPortRelayLoading(true);
    setPortRelayError('');
    try {
      const snapshot = await service.regeneratePortRelayAccessCode(accessCode);
      setPortRelayKnownAccessCodeGeneration(typeof snapshot.accessCodeGeneration === 'number' ? snapshot.accessCodeGeneration : null);
      applyPortRelaySnapshot(snapshot);
    } catch (err) {
      setPortRelayError(err instanceof Error ? err.message : String(err));
    } finally {
      setPortRelayLoading(false);
    }
  }, [applyPortRelaySnapshot, portRelaySnapshot.enabled]);

  const copyPortRelayAccessCode = useCallback(async () => {
    setPortRelayError('');
    try {
      if (portRelayAccessCodeUnknown) {
        setPortRelayCodeCopied(false);
        setPortRelayError('Access code is unknown on this device. Generate a new code before copying.');
        return;
      }
      if (!portRelayAccessCode) {
        const accessCode = generatePortRelayAccessCode();
        setPortRelayAccessCode(accessCode);
        await writeTextToClipboard(accessCode);
      } else {
        await writeTextToClipboard(portRelayAccessCode);
      }
      setPortRelayCodeCopied(true);
      if (portRelayCodeCopyTimerRef.current) {
        window.clearTimeout(portRelayCodeCopyTimerRef.current);
      }
      portRelayCodeCopyTimerRef.current = window.setTimeout(() => {
        setPortRelayCodeCopied(false);
        portRelayCodeCopyTimerRef.current = null;
      }, 1400);
    } catch (err) {
      setPortRelayCodeCopied(false);
      setPortRelayError(err instanceof Error ? err.message : String(err));
    }
  }, [portRelayAccessCode, portRelayAccessCodeUnknown]);

  const selectPortRelayTarget = useCallback(async (target: PortRelayTarget) => {
    setSelectedPortRelayTarget(target);
    persistPortRelaySettings({selectedTarget: target});
    if (!portRelaySnapshot.enabled || samePortRelayTarget(selectedPortRelayTarget, target)) {
      return;
    }
    await enablePortRelayForTarget(target, String(portRelaySnapshot.listenPort || portRelayListenPort), {framePath: ''});
  }, [
    enablePortRelayForTarget,
    persistPortRelaySettings,
    portRelayListenPort,
    portRelaySnapshot.enabled,
    portRelaySnapshot.listenPort,
    selectedPortRelayTarget,
  ]);

  const handleMobilePortRelayTargetMenuSelect = useCallback(async (target: PortRelayTarget) => {
    setPortRelayTargetMenuOpen(false);
    if (samePortRelayTarget(activePortRelayTarget, target)) {
      return;
    }
    if (portRelayAccessCodeUnknown) {
      setPortRelayFrameOpen(false);
      openSettingsDetail('portRelay');
      setPortRelayError('Access code is unknown on this device. Generate a new code before switching target.');
      return;
    }
    setPortRelayMenuSwitchingTarget(target);
    try {
      await enablePortRelayForTarget(target, String(portRelaySnapshot.listenPort || portRelayListenPort), {framePath: '', openFrame: true});
      setPortRelayFrameOpen(true);
    } finally {
      setPortRelayMenuSwitchingTarget(null);
    }
  }, [
    activePortRelayTarget,
    enablePortRelayForTarget,
    openSettingsDetail,
    portRelayAccessCodeUnknown,
    portRelayListenPort,
    portRelaySnapshot.listenPort,
  ]);

  const deletePortRelayTarget = useCallback(async (target: PortRelayTarget) => {
    const nextTargets = removePortRelayTarget(portRelayTargets, target);
    const deletingSelected = samePortRelayTarget(selectedPortRelayTarget, target);
    const nextSelectedTarget = deletingSelected ? nextTargets[0] ?? null : selectedPortRelayTarget;
    setPortRelayTargets(nextTargets);
    setSelectedPortRelayTarget(nextSelectedTarget);
    persistPortRelaySettings({
      targets: nextTargets,
      selectedTarget: nextSelectedTarget,
    });
    if (!deletingSelected || !portRelaySnapshot.enabled) {
      return;
    }
    setPortRelayLoading(true);
    setPortRelayError('');
    try {
      const snapshot = await service.disablePortRelay();
      setPortRelayFramePath('');
      setPortRelayFrameAutoOpenPending(false);
      applyPortRelaySnapshot(snapshot);
    } catch (err) {
      setPortRelayError(err instanceof Error ? err.message : String(err));
    } finally {
      setPortRelayLoading(false);
    }
  }, [applyPortRelaySnapshot, persistPortRelaySettings, portRelaySnapshot.enabled, portRelayTargets, selectedPortRelayTarget]);

  const openChatPortRelayLink = useCallback(async (localUrl: PortRelayLocalHttpUrl) => {
    const hubId = currentProject?.hubId || '';
    if (!hubId) {
      setError('Current project has no hub for Port Relay.');
      return;
    }
    if (portRelayAccessCodeUnknown) {
      setPortRelayError('Access code is unknown on this device. Generate a new code before opening local relay links.');
      setSidebarSettingsOpen(true);
      setSidebarCollapsed(false);
      setSettingsDetailView('portRelay');
      setPortRelayFrameOpen(false);
      return;
    }
    const target: PortRelayTarget = {
      hubId,
      targetPort: localUrl.targetPort,
    };
    const listenPort = Number(portRelayListenPort);
    if (!Number.isInteger(listenPort) || listenPort < 1 || listenPort > 65535) {
      setPortRelayError('Listen port must be in 1..65535.');
      setSidebarSettingsOpen(true);
      setSidebarCollapsed(false);
      setSettingsDetailView('portRelay');
      return;
    }
    const nextTargets = upsertPortRelayTarget(portRelayTargets, target);
    setPortRelayTargets(nextTargets);
    setSelectedPortRelayTarget(target);
    persistPortRelaySettings({
      targets: nextTargets,
      selectedTarget: target,
      listenPort,
    });
    setPortRelayFramePath(localUrl.path);
    const activeTarget = normalizePortRelayTarget({
      hubId: portRelaySnapshot.hubId,
      targetPort: portRelaySnapshot.targetPort,
    });
    const activeRelayMatches =
      portRelaySnapshot.enabled &&
      portRelaySnapshot.status !== 'Error' &&
      portRelaySnapshot.listenPort === listenPort &&
      samePortRelayTarget(activeTarget, target);
    if (activeRelayMatches) {
      setPortRelayFrameAutoOpenPending(true);
      if (portRelayReady) {
        setPortRelayFrameOpen(true);
      }
      return;
    }
    await enablePortRelayForTarget(target, portRelayListenPort, {
      framePath: localUrl.path,
      openFrame: true,
    });
  }, [
    currentProject?.hubId,
    enablePortRelayForTarget,
    persistPortRelaySettings,
    portRelayAccessCodeUnknown,
    portRelayListenPort,
    portRelayReady,
    portRelaySnapshot.enabled,
    portRelaySnapshot.hubId,
    portRelaySnapshot.listenPort,
    portRelaySnapshot.status,
    portRelaySnapshot.targetPort,
    portRelayTargets,
    setSidebarCollapsed,
    setSidebarSettingsOpen,
  ]);

  const updateHubCards = useMemo(() => {
    const hubIds = new Set<string>([
      ...deriveRegistryHubIds(registryHubs),
      ...Object.keys(wheelMakerUpdateHubs),
      ...Object.keys(agentPackageHubs),
    ]);
    return Array.from(hubIds).sort((left, right) => {
      if (left < right) return -1;
      if (left > right) return 1;
      return 0;
    }).map(hubId => ({
      hubId,
      wheelMaker: wheelMakerUpdateHubs[hubId] ?? null,
      agentPackage: agentPackageHubs[hubId] ?? null,
    }));
  }, [agentPackageHubs, registryHubs, wheelMakerUpdateHubs]);

  const agentPackageHubCards = useMemo(() => {
    return Object.values(agentPackageHubs).sort((left, right) => {
      if (left.hubId < right.hubId) return -1;
      if (left.hubId > right.hubId) return 1;
      return 0;
    });
  }, [agentPackageHubs]);

  const refreshWheelMakerUpdateHub = useCallback(async (hubId: string) => {
    setWheelMakerUpdateHubs(prev => ({
      ...prev,
      [hubId]: {
        ...(prev[hubId] ?? {hubId, loading: false, error: '', data: null}),
        loading: true,
        error: '',
      },
    }));
    try {
      const result = await service.queryWheelMakerUpdate(hubId);
      setWheelMakerUpdateHubs(prev => ({
        ...prev,
        [hubId]: {
          hubId,
          loading: false,
          error: result.ok ? '' : result.error || 'Update check failed.',
          data: result,
        },
      }));
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setWheelMakerUpdateHubs(prev => ({
        ...prev,
        [hubId]: {
          ...(prev[hubId] ?? {hubId, loading: false, error: '', data: null}),
          loading: false,
          error: message,
        },
      }));
    }
  }, []);

  const refreshWheelMakerUpdates = useCallback(async () => {
    setWheelMakerUpdatesLoading(true);
    setWheelMakerUpdatesError('');
    try {
      const hubIds = await refreshProjectHubSnapshot();
      if (hubIds.length === 0) {
        setWheelMakerUpdateHubs({});
        setWheelMakerUpdatesError('No hubs available.');
        return;
      }
      setWheelMakerUpdateHubs(prev => {
        const next: Record<string, WheelMakerUpdateHubView> = {};
        hubIds.forEach(hubId => {
          next[hubId] = {
            hubId,
            loading: true,
            error: '',
            data: prev[hubId]?.data ?? null,
          };
        });
        return next;
      });
      const responses = await Promise.all(hubIds.map(async hubId => {
        try {
          const result = await service.queryWheelMakerUpdate(hubId);
          return {hubId, result};
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          return {hubId, error: message};
        }
      }));
      setWheelMakerUpdateHubs(prev => {
        const next: Record<string, WheelMakerUpdateHubView> = {};
        responses.forEach(entry => {
          if ('error' in entry) {
            next[entry.hubId] = {
              hubId: entry.hubId,
              loading: false,
              error: entry.error || '',
              data: prev[entry.hubId]?.data ?? null,
            };
            return;
          }
          next[entry.hubId] = {
            hubId: entry.hubId,
            loading: false,
            error: entry.result.ok ? '' : entry.result.error || 'Update check failed.',
            data: entry.result,
          };
        });
        return next;
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setWheelMakerUpdatesError(message);
    } finally {
      setWheelMakerUpdatesLoading(false);
    }
  }, [refreshProjectHubSnapshot]);

  const refreshAgentPackages = useCallback(async () => {
    if (agentPackageScanPollTimerRef.current) {
      window.clearTimeout(agentPackageScanPollTimerRef.current);
      agentPackageScanPollTimerRef.current = null;
    }
    setAgentPackagesLoading(true);
    setAgentPackagesError('');
    try {
      const hubIds = await refreshProjectHubSnapshot();
      if (hubIds.length === 0) {
        setAgentPackageHubs({});
        setAgentPackagesError('No hubs available.');
        return;
      }
      setAgentPackageHubs(prev => {
        const next: Record<string, AgentPackageHubView> = {};
        hubIds.forEach(hubId => {
          next[hubId] = {
            hubId,
            loading: true,
            error: '',
            updatedAt: prev[hubId]?.updatedAt || '',
            hub: prev[hubId]?.hub ?? null,
            operation: prev[hubId]?.operation ?? null,
          };
        });
        return next;
      });
      const responses = await Promise.all(hubIds.map(async hubId => {
        try {
          const result = await withAgentPackageTimeout(
            service.scanNpmPackages(hubId),
            AGENT_PACKAGE_SCAN_TIMEOUT_MS,
            `${hubId} npm package scan timed out`,
          );
          return {hubId, result};
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          return {hubId, error: message};
        }
      }));
      setAgentPackageHubs(prev => {
        const next: Record<string, AgentPackageHubView> = {};
        responses.forEach(entry => {
          if ('error' in entry) {
            next[entry.hubId] = {
              hubId: entry.hubId,
              loading: false,
              error: entry.error || '',
              updatedAt: prev[entry.hubId]?.updatedAt || '',
              hub: prev[entry.hubId]?.hub ?? null,
              operation: prev[entry.hubId]?.operation ?? null,
            };
            return;
          }
          const hub = entry.result.hub ?? {
            hubId: entry.hubId,
            nodeVersion: '',
            npmVersion: '',
            npmPrefix: '',
            warning: '',
            error: '',
            packages: [],
          };
          next[entry.hubId] = {
            hubId: entry.hubId,
            loading: false,
            error: entry.result.ok ? '' : hub.error || 'Scan failed.',
            updatedAt: entry.result.updatedAt || '',
            hub,
            operation: entry.result.operation ?? prev[entry.hubId]?.operation ?? null,
          };
        });
        return next;
      });
      if (responses.some(entry => !('error' in entry) && entry.result.operation?.running)) {
        agentPackageScanPollTimerRef.current = window.setTimeout(() => {
          agentPackageScanPollTimerRef.current = null;
          refreshAgentPackages().catch(() => undefined);
        }, 1000);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setAgentPackagesError(message);
    } finally {
      setAgentPackagesLoading(false);
    }
  }, [refreshProjectHubSnapshot]);

  useEffect(() => {
    if (settingsDetailView !== 'update') {
      return;
    }
    refreshWheelMakerUpdates().catch(() => undefined);
    refreshAgentPackages().catch(() => undefined);
  }, [settingsDetailView, refreshAgentPackages, refreshWheelMakerUpdates]);

  const clearSkillOperationPollTimer = useCallback(() => {
    if (skillOperationPollTimerRef.current) {
      window.clearTimeout(skillOperationPollTimerRef.current);
      skillOperationPollTimerRef.current = null;
    }
    skillOperationPollHubIdsRef.current.clear();
  }, []);

  const scheduleSkillOperationPoll = useCallback((hubIds: string | string[]) => {
    const ids = Array.isArray(hubIds) ? hubIds : [hubIds];
    ids
      .map(hubId => hubId.trim())
      .filter(Boolean)
      .forEach(hubId => skillOperationPollHubIdsRef.current.add(hubId));
    if (skillOperationPollTimerRef.current) {
      return;
    }
    skillOperationPollTimerRef.current = window.setTimeout(() => {
      skillOperationPollTimerRef.current = null;
      const pendingHubIds = Array.from(skillOperationPollHubIdsRef.current);
      skillOperationPollHubIdsRef.current.clear();
      Promise.all(pendingHubIds.map(hubId => refreshSkillManagementHubRef.current?.(hubId))).catch(() => undefined);
    }, 1000);
  }, []);

  const refreshSkillManagementHub = useCallback(async (hubId: string) => {
    setSkillHubs(prev => ({
      ...prev,
      [hubId]: {
        ...(prev[hubId] ?? {hubId, loading: false, error: '', data: null}),
        loading: true,
        error: '',
      },
    }));
    try {
      const result = await service.scanSkills(hubId);
      setSkillHubs(prev => ({
        ...prev,
        [hubId]: {
          hubId,
          loading: false,
          data: result,
          error: result.ok ? '' : skillCommandErrorMessage(result),
        },
      }));
      if (result.operation?.running) {
        scheduleSkillOperationPoll(hubId);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setSkillHubs(prev => ({
        ...prev,
        [hubId]: {
          ...(prev[hubId] ?? {hubId, loading: false, error: '', data: null}),
          loading: false,
          error: message,
        },
      }));
    }
  }, [scheduleSkillOperationPoll]);

  const refreshSkillManagement = useCallback(async () => {
    clearSkillOperationPollTimer();
    setSkillsLoading(true);
    setSkillsError('');
    try {
      const snapshot = await service.listProjectSnapshot();
      if (snapshot.projects.length > 0) {
        setProjects(snapshot.projects);
      }
      setRegistryHubs(snapshot.hubs);
      setLocalHubReadStatuses(service.getLocalHubReadStatuses(snapshot.hubs));
      const hubIds = deriveSkillHubIds(snapshot.hubs);
      if (hubIds.length === 0) {
        setSkillHubs({});
        setSkillsError('No hubs available.');
        return;
      }
      setSkillHubs(prev => {
        const next: Record<string, SkillHubView> = {};
        hubIds.forEach(hubId => {
          next[hubId] = {
            hubId,
            loading: true,
            error: '',
            data: prev[hubId]?.data ?? null,
          };
        });
        return next;
      });
      const responses = await Promise.all(hubIds.map(async hubId => {
        try {
          const result = await service.scanSkills(hubId);
          return {hubId, result};
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          return {hubId, error: message};
        }
      }));
      setSkillHubs(prev => {
        const next: Record<string, SkillHubView> = {};
        responses.forEach(entry => {
          if ('error' in entry) {
            next[entry.hubId] = {
              hubId: entry.hubId,
              loading: false,
              error: entry.error || 'Skills scan failed.',
              data: prev[entry.hubId]?.data ?? null,
            };
            return;
          }
          next[entry.hubId] = {
            hubId: entry.hubId,
            loading: false,
            error: entry.result.ok ? '' : skillCommandErrorMessage(entry.result),
            data: entry.result,
          };
        });
        return next;
      });
      const runningHubIds = responses
        .filter((entry): entry is {hubId: string; result: RegistrySkillCommandResponse} => !('error' in entry) && entry.result.operation?.running === true)
        .map(entry => entry.hubId);
      if (runningHubIds.length > 0) {
        scheduleSkillOperationPoll(runningHubIds);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setSkillsError(message);
    } finally {
      setSkillsLoading(false);
    }
  }, [clearSkillOperationPollTimer, scheduleSkillOperationPoll]);

  refreshSkillManagementHubRef.current = refreshSkillManagementHub;

  useEffect(() => () => {
    clearSkillOperationPollTimer();
  }, [clearSkillOperationPollTimer]);

  useEffect(() => {
    if (settingsDetailView !== 'skills') {
      return;
    }
    refreshSkillManagement().catch(() => undefined);
  }, [settingsDetailView, refreshSkillManagement]);

  const requestSkillInstall = useCallback((target: SkillInstallTarget) => {
    const sameTarget = sameSkillInstallTarget(skillInstallTarget, target);
    setSkillInstallTarget(target);
    if (!sameTarget) {
      setSkillSourceError('');
      setSkillSourceCandidates([]);
      setSkillSourceSelectedNames([]);
    }
  }, [skillInstallTarget]);

  const toggleSkillSourceCandidate = useCallback((name: string) => {
    setSkillSourceSelectedNames(prev => (
      prev.includes(name)
        ? prev.filter(item => item !== name)
        : [...prev, name]
    ));
  }, []);

  const toggleAllSkillSourceCandidates = useCallback(() => {
    const candidateNames = Array.from(new Set(skillSourceCandidates
      .map(candidate => candidate.name)
      .filter(Boolean)));
    setSkillSourceSelectedNames(prev => {
      const selected = new Set(prev);
      const allSelected = candidateNames.length > 0 && candidateNames.every(name => selected.has(name));
      return allSelected ? [] : candidateNames;
    });
  }, [skillSourceCandidates]);

  const listSkillSource = useCallback(async () => {
    const target = skillInstallTarget;
    const sourceInput = parseSkillSourceInput(skillSourceInput);
    const source = sourceInput.source;
    if (!target || !source) {
      setSkillSourceError('Source is required.');
      return;
    }
    setSkillSourceLoading(true);
    setSkillSourceError('');
    try {
      const result = await service.listSkillsSource(target.hubId, source);
      if (!result.ok) {
        throw new Error(skillCommandErrorMessage(result));
      }
      const candidates = result.candidates ?? [];
      if (sourceInput.skillNames.length === 0) {
        setSkillSourceCandidates(candidates);
        setSkillSourceSelectedNames([]);
        return;
      }
      const candidateByName = new Map(candidates.map(candidate => [candidate.name, candidate]));
      const filteredCandidates = sourceInput.skillNames
        .map(name => candidateByName.get(name))
        .filter((candidate): candidate is RegistrySkillSourceCandidate => !!candidate);
      const foundNames = new Set(filteredCandidates.map(candidate => candidate.name));
      const missingNames = sourceInput.skillNames.filter(name => !foundNames.has(name));
      setSkillSourceCandidates(filteredCandidates);
      setSkillSourceSelectedNames(filteredCandidates.map(candidate => candidate.name));
      if (missingNames.length > 0) {
        setSkillSourceError(`Skill not found in source: ${missingNames.join(', ')}`);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setSkillSourceError(message);
    } finally {
      setSkillSourceLoading(false);
    }
  }, [skillInstallTarget, skillSourceInput]);

  const requestSkillInstallConfirm = useCallback(() => {
    const target = skillInstallTarget;
    const source = parseSkillSourceInput(skillSourceInput).source;
    const skills = skillSourceSelectedNames;
    if (!target || !source) {
      setSkillSourceError('Source is required.');
      return;
    }
    if (skills.length === 0) {
      setSkillSourceError('Select at least one skill.');
      return;
    }
    setConfirmError('');
    setConfirmTarget({
      kind: 'skillInstall',
      hubId: target.hubId,
      scope: target.scope,
      projectName: target.projectName,
      source,
      skills,
    });
  }, [skillInstallTarget, skillSourceInput, skillSourceSelectedNames]);

  const requestSkillUninstall = useCallback((target: {hubId: string; scope: RegistrySkillScope; projectName?: string; skillName: string}) => {
    setConfirmError('');
    setConfirmTarget({kind: 'skillUninstall', ...target});
  }, []);

  const requestSkillUpdate = useCallback((target: {hubId: string; scope: RegistrySkillScope; projectName?: string; includeProjects?: boolean}) => {
    setConfirmError('');
    setConfirmTarget({kind: 'skillUpdate', ...target});
  }, []);

  const handleSkillConfirmedAction = useCallback(async (
    target: Extract<ConfirmTarget, {kind: 'skillInstall' | 'skillUninstall' | 'skillUpdate'}>,
  ) => {
    const pendingKey = skillActionPendingKey({
      hubId: target.hubId,
      scope: target.scope,
      projectName: target.projectName,
      skillName: target.kind === 'skillUninstall' ? target.skillName : undefined,
      action: target.kind,
    });
    setConfirmError('');
    setSkillsPendingKey(pendingKey);
    try {
      let results: RegistrySkillCommandResponse[] = [];
      if (target.kind === 'skillInstall') {
        results = [await service.installSkills({
          hubId: target.hubId,
          scope: target.scope,
          projectName: target.projectName,
          source: target.source,
          skills: target.skills,
        })];
      } else if (target.kind === 'skillUninstall') {
        results = [await service.uninstallSkills({
          hubId: target.hubId,
          scope: target.scope,
          projectName: target.projectName,
          skills: [target.skillName],
        })];
      } else {
        results = [await service.updateSkills({
          hubId: target.hubId,
          scope: target.scope,
          projectName: target.projectName,
          includeProjects: target.includeProjects,
        })];
      }
      const failed = results.find(result => !result.ok);
      if (failed) {
        throw new Error(skillCommandErrorMessage(failed));
      }
      setConfirmTarget(null);
      setConfirmError('');
      if (target.kind === 'skillInstall') {
        setSkillInstallTarget(null);
        setSkillSourceCandidates([]);
        setSkillSourceSelectedNames([]);
      }
      await refreshSkillManagementHub(target.hubId);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setConfirmError(message);
      setSkillsError(message);
      setError(message);
    } finally {
      setSkillsPendingKey('');
    }
  }, [refreshSkillManagementHub]);

  const requestAgentPackageAction = useCallback((
    action: 'install' | 'update' | 'uninstall',
    hubId: string,
    pkg: RegistryNpmPackage,
  ) => {
    setConfirmError('');
    setConfirmTarget({
      kind: 'npmPackage',
      action,
      hubId,
      packageName: pkg.packageName,
      displayName: pkg.displayName,
      installedVersion: pkg.installedVersion,
      latestVersion: pkg.latestVersion,
    });
  }, []);

  const requestAgentPackageHubUpdate = useCallback((hubId: string, packages: NpmPackageUpdateTarget[]) => {
    if (packages.length === 0) {
      return;
    }
    setConfirmError('');
    setConfirmTarget({
      kind: 'npmPackageHubUpdate',
      hubId,
      packages,
    });
  }, []);

  const requestWheelMakerUpdatePublish = useCallback((hubId: string, data: RegistryWheelMakerUpdateResponse | null) => {
    setConfirmError('');
    setConfirmTarget({
      kind: 'wheelMakerUpdate',
      hubId,
      currentSha: data?.release?.sha || data?.git?.currentSha || '',
      latestSha: data?.git?.latestSha || '',
      behindCount: data?.git?.behindCount ?? 0,
    });
  }, []);

  const requestWheelMakerUpdateAll = useCallback((hubIds: string[]) => {
    const uniqueHubIds = Array.from(new Set(hubIds.filter(Boolean))).sort();
    if (uniqueHubIds.length === 0) {
      return;
    }
    setConfirmError('');
    setConfirmTarget({
      kind: 'wheelMakerUpdateAll',
      hubIds: uniqueHubIds,
    });
  }, []);

  const handleWheelMakerUpdateConfirmedAction = useCallback(async (target: Extract<ConfirmTarget, {kind: 'wheelMakerUpdate'}>) => {
    setConfirmError('');
    setWheelMakerUpdatePendingHubId(target.hubId);
    try {
      const result = await service.requestWheelMakerUpdatePublish(target.hubId);
      setWheelMakerUpdateHubs(prev => ({
        ...prev,
        [target.hubId]: {
          hubId: target.hubId,
          loading: false,
          error: result.ok ? '' : result.error || 'Update request failed.',
          data: result,
        },
      }));
      setConfirmTarget(null);
      setConfirmError('');
      await refreshWheelMakerUpdateHub(target.hubId);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setConfirmError(message);
      setWheelMakerUpdatesError(message);
      setError(message);
    } finally {
      setWheelMakerUpdatePendingHubId('');
    }
  }, [refreshWheelMakerUpdateHub]);

  const handleWheelMakerUpdateAllConfirmedAction = useCallback(async (target: Extract<ConfirmTarget, {kind: 'wheelMakerUpdateAll'}>) => {
    if (target.hubIds.length === 0) {
      setConfirmTarget(null);
      return;
    }
    setConfirmError('');
    setWheelMakerUpdatesError('');
    setWheelMakerUpdateAllPending(true);
    setWheelMakerUpdatePendingHubId('');
    try {
      const responses = await Promise.all(target.hubIds.map(async hubId => {
        try {
          const result = await service.requestWheelMakerUpdatePublish(hubId);
          return {
            hubId,
            result,
            error: result.ok ? '' : result.error || 'Update request failed.',
          };
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          return {hubId, error: message};
        }
      }));
      setWheelMakerUpdateHubs(prev => {
        const next = {...prev};
        responses.forEach(entry => {
          next[entry.hubId] = {
            ...(prev[entry.hubId] ?? {hubId: entry.hubId, loading: false, error: '', data: null}),
            hubId: entry.hubId,
            loading: false,
            error: entry.error || '',
            data: 'result' in entry && entry.result ? entry.result : prev[entry.hubId]?.data ?? null,
          };
        });
        return next;
      });
      setConfirmTarget(null);
      setConfirmError('');
      await refreshWheelMakerUpdates();
      const failedUpdates = responses.filter(entry => entry.error);
      if (failedUpdates.length > 0) {
        const message = `Failed to update ${failedUpdates.length} of ${target.hubIds.length} hubs: ${failedUpdates.map(entry => entry.hubId).join(', ')}`;
        setWheelMakerUpdatesError(message);
        setWheelMakerUpdateHubs(prev => {
          const next = {...prev};
          failedUpdates.forEach(entry => {
            next[entry.hubId] = {
              ...(prev[entry.hubId] ?? {hubId: entry.hubId, loading: false, error: '', data: null}),
              hubId: entry.hubId,
              loading: false,
              error: entry.error || 'Update request failed.',
            };
          });
          return next;
        });
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setConfirmTarget(null);
      setConfirmError('');
      setWheelMakerUpdatesError(message);
      setError(message);
    } finally {
      setWheelMakerUpdateAllPending(false);
    }
  }, [refreshWheelMakerUpdates]);

  const handleAgentPackageConfirmedAction = useCallback(async (target: Extract<ConfirmTarget, {kind: 'npmPackage'}>) => {
    const pendingKey = agentPackageActionKey(target.hubId, target.packageName);
    setConfirmError('');
    setAgentPackageActionPendingKey(pendingKey);
    try {
      const result = target.action === 'uninstall'
        ? await service.uninstallNpmPackage(target.hubId, target.packageName)
        : await service.installNpmPackage(target.hubId, target.packageName, 'latest');
      setAgentPackageHubs(prev => ({
        ...prev,
        [target.hubId]: {
          ...(prev[target.hubId] ?? {hubId: target.hubId, loading: false, error: '', updatedAt: '', hub: null, operation: null}),
          operation: result.operation ?? null,
        },
      }));
      setConfirmTarget(null);
      setConfirmError('');
      await refreshAgentPackages();
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setConfirmError(message);
      setAgentPackagesError(message);
      setError(message);
    } finally {
      setAgentPackageActionPendingKey('');
    }
  }, [agentPackageActionKey, refreshAgentPackages]);

  const handleAgentPackageHubUpdateConfirmedAction = useCallback(async (target: Extract<ConfirmTarget, {kind: 'npmPackageHubUpdate'}>) => {
    if (target.packages.length === 0) {
      setConfirmTarget(null);
      return;
    }
    setConfirmError('');
    setAgentPackagesError('');
    setAgentPackageHubUpdatePendingId(target.hubId);
    try {
      const result = await service.installNpmPackages(target.hubId, target.packages.map(pkg => pkg.packageName), 'latest');
      setAgentPackageHubs(prev => ({
        ...prev,
        [target.hubId]: {
          ...(prev[target.hubId] ?? {hubId: target.hubId, loading: false, error: '', updatedAt: '', hub: null, operation: null}),
          operation: result.operation ?? null,
        },
      }));
      setConfirmTarget(null);
      setConfirmError('');
      await refreshAgentPackages();
      if (!result.ok) {
        const message = result.operation?.errorSummary || result.operation?.message || 'Update request failed.';
        setAgentPackagesError(message);
        setAgentPackageHubs(prev => ({
          ...prev,
          [target.hubId]: {
            ...(prev[target.hubId] ?? {hubId: target.hubId, loading: false, error: '', updatedAt: '', hub: null, operation: null}),
            error: message,
          },
        }));
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setConfirmTarget(null);
      setConfirmError('');
      setAgentPackagesError(message);
      setError(message);
    } finally {
      setAgentPackageHubUpdatePendingId('');
    }
  }, [refreshAgentPackages]);

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

  const requestClearLocalCache = () => {
    setConfirmError('');
    setConfirmTarget({kind: 'clearCache'});
  };

  const clearLocalCache = () => {
    workspaceStore.clearLocalCachePreservingToken();
    window.location.reload();
  };

  const syncWorkspaceProject = async (
    nextProjectId: string,
    options?: {reason?: 'chat' | 'manual'},
  ) => {
    if (!nextProjectId || nextProjectId === projectIdRef.current) {
      setWorkspaceProjectMenuOpen(false);
      return;
    }
    if (!projectsRef.current.some(item => item.projectId === nextProjectId)) {
      if (options?.reason !== 'chat') {
        setError('Project is no longer available');
      }
      setWorkspaceProjectMenuOpen(false);
      return;
    }

    captureSelectedFileScrollPosition();
    const previousProjectId = projectIdRef.current;
    if (previousProjectId) {
      workspaceStore.rememberProjectSnapshot(previousProjectId, {
        expandedDirs: expandedDirsRef.current,
        selectedFile: selectedFileRef.current,
        pinnedFiles,
        gitCurrentBranch,
        commits,
        selectedCommit,
        commitFilesBySha,
        selectedDiff,
      });
    }

    try {
      const result = await workspaceController.switchProjectLightweight(nextProjectId);
      projectsRef.current = result.projects;
      setProjects(result.projects);
      setRegistryHubs(result.hubs);
      setLocalHubReadStatuses(service.getLocalHubReadStatuses(result.hubs));
      setHasPendingProjectUpdates(false);
      workspaceStore.rememberGlobalState({
        selectedProjectId: nextProjectId,
      });
      skipNextSelectedFileAutoReadRef.current =
        tabRef.current !== 'file' && !!result.hydrated.selectedFile;
      applyHydratedProjectState(result.hydrated);
      setWorkspaceProjectMenuOpen(false);
      setError('');

      if (tabRef.current === 'file') {
        loadDirectory('.', {projectId: nextProjectId}).catch(err =>
          setError(err instanceof Error ? err.message : String(err)),
        );
      } else if (tabRef.current === 'git') {
        loadGit().catch(err =>
          setGitError(err instanceof Error ? err.message : String(err)),
        );
      }
    } catch (err) {
      if (options?.reason !== 'chat') {
        setError(err instanceof Error ? err.message : String(err));
      }
      setWorkspaceProjectMenuOpen(false);
    }
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
    syncWorkspaceProject(targetProjectId, {reason: 'chat'}).catch(() => undefined);
    setWideProjectActionMenu(null);
    setMobileProjectActionMenu(null);
    if (options?.closeMobileDrawer) {
      setDrawerOpen(false);
    }
    setTab('chat');
    applySelectedChatKey(nextSelectedKey);
    const runtimeKey = encodeChatSessionKey(nextSelectedKey);
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

  const handleSessionSearchResultClick = async (
    targetProjectId: string,
    row: SessionSearchSectionRow,
    options?: {closeMobileDrawer?: boolean},
  ) => {
    await selectProjectChatSession(targetProjectId, row.session.sessionId, options);
    if (row.result.source !== 'prompt' || !row.result.turnIndex) {
      return;
    }
    const runtimeKey = buildChatRuntimeKey(targetProjectId, row.session.sessionId);
    const generation = Date.now();
    setSessionSearchTargetTurn({
      runtimeKey,
      turnIndex: row.result.turnIndex,
      generation,
    });
    if (sessionSearchHighlightTimerRef.current !== null) {
      window.clearTimeout(sessionSearchHighlightTimerRef.current);
    }
    sessionSearchHighlightTimerRef.current = window.setTimeout(() => {
      setSessionSearchTargetTurn(current =>
        current?.generation === generation ? null : current,
      );
    }, 2000);
  };

  const renderSessionSearchHighlightedTitle = (
    title: string,
    row: SessionSearchSectionRow,
  ) => {
    const segments = splitSessionSearchTitleHighlight(title, sessionSearchQuery);
    if (segments.length === 0) {
      return title;
    }
    return segments.map((segment, index) => (
      <span
        key={`${row.session.sessionId}:title-highlight:${index}`}
        className={segment.match ? 'session-search-title-highlight' : undefined}
      >
        {segment.text}
      </span>
    ));
  };

  const renderSessionSearchStatusLine = () => {
    if (!sessionSearchActive || sessionSearchStatusParts.length === 0) {
      return null;
    }
    return (
      <div className="chat-header-search-status">
        {sessionSearchStatusParts.join(' · ')}
      </div>
    );
  };

  const renderChatHeaderSearchControls = (mobile: boolean) => {
    const hasActiveSearch = !!activeSessionSearchId;
    if (!sessionSearchOpen && !hasActiveSearch) {
      return (
        <div className={`chat-header-search-control compact${mobile ? ' mobile' : ''}`}>
          <button
            type="button"
            className="session-search-icon-btn"
            onClick={() => setSessionSearchOpen(true)}
            title="Search sessions"
            aria-label="Search sessions"
          >
            <span className="codicon codicon-search" />
          </button>
        </div>
      );
    }
    return (
      <div className={`chat-header-search-wrap${mobile ? ' mobile' : ''}`}>
        <form
          className={`chat-header-search-control open${hasActiveSearch ? ' active' : ''}${mobile ? ' mobile' : ''}`}
          onSubmit={event => {
            event.preventDefault();
            startSessionSearch().catch(() => undefined);
          }}
        >
          <span className="codicon codicon-search session-search-leading-icon" aria-hidden="true" />
          <input
            ref={sessionSearchInputRef}
            className="session-search-input"
            value={sessionSearchInput}
            onChange={event => setSessionSearchInput(event.target.value)}
            placeholder="Search sessions"
            aria-label="Search sessions"
          />
          <button
            type="submit"
            className="session-search-icon-btn"
            title="Start search"
            aria-label="Start search"
          >
            <span className="codicon codicon-check" />
          </button>
          <button
            type="button"
            className="session-search-icon-btn"
            title="Close search"
            aria-label="Close search"
            onClick={() => {
              if (hasActiveSearch) {
                exitSessionSearch().catch(() => undefined);
              } else {
                setSessionSearchOpen(false);
                setSessionSearchInput('');
              }
            }}
          >
            <span className="codicon codicon-close" />
          </button>
        </form>
        {renderSessionSearchStatusLine()}
      </div>
    );
  };

  const renderSessionSearchRow = (
    targetProjectId: string,
    row: SessionSearchSectionRow,
    mobile: boolean,
  ) => {
    const sessionAgent = (row.session.agentType || '').trim();
    const displaySessionAgent = normalizeAgentTypeName(sessionAgent);
    const title = resolveSessionDisplayTitle(row.session) || row.session.sessionId;
    const selected =
      selectedChatEncodedKey === buildChatRuntimeKey(targetProjectId, row.session.sessionId);
    return (
      <div
        key={`${targetProjectId}:search:${row.session.sessionId}`}
        className="project-session-row-wrap session-search-row-wrap"
      >
        <button
          type="button"
          className={`wide-session-row session-search-row${mobile ? ' mobile-session-row' : ''}${selected ? ' selected' : ''}`}
          title={title}
          onClick={() => {
            handleSessionSearchResultClick(targetProjectId, row, {
              closeMobileDrawer: mobile,
            }).catch(() => undefined);
          }}
        >
          {renderSessionStateMarker(row.session, targetProjectId)}
          <span className="wide-session-title session-search-title">
            {renderSessionSearchHighlightedTitle(title, row)}
          </span>
          {displaySessionAgent ? (
            <span className={`wide-session-agent-tag ${tagVariantClass('wide-session-agent', sessionAgent)}`}>
              {displaySessionAgent}
            </span>
          ) : null}
          <span className="wide-session-time" title={row.session.updatedAt || ''}>
            {formatCompactRelativeAge(row.session.updatedAt)}
          </span>
        </button>
      </div>
    );
  };

  const renderSessionSearchResults = (mobile: boolean) => {
    const errorMessages = Object.entries(sessionSearchErrorsByProjectId)
      .map(([entryProjectId, message]) => {
        const text = message.trim();
        if (!text) {
          return null;
        }
        const projectName = projects.find(item => item.projectId === entryProjectId)?.name || entryProjectId;
        return `${projectName}: ${text}`;
      })
      .filter((item): item is string => item !== null);
    const body = (
      <>
        {sessionSearchSections.map(section => {
          const projectHub = section.project.hubId || 'local';
          const projectHubVariant = tagVariantClass('wide-project-hub', section.project.hubId || 'local');
          return (
            <div
              key={`session-search-project:${section.project.projectId}`}
              className={`wide-project-section session-search-project-section${section.project.projectId === projectId ? ' active' : ''}`}
            >
              <div className={`wide-project-row session-search-project-row${mobile ? ' mobile-project-row' : ''}`}>
                <div className="wide-project-toggle session-search-project-label">
                  <span className="wide-project-folder-wrap">
                    <span className={`codicon codicon-search wide-project-folder-icon ${projectHubVariant}`} />
                  </span>
                  <span className="wide-project-title-group">
                    <span className="wide-project-name" title={section.project.name}>
                      {section.project.name}
                    </span>
                    <span className={`wide-project-hub-tag ${projectHubVariant}`}>
                      <span className="wide-project-hub-dot" aria-hidden="true" />
                      <span className="wide-project-hub-label">{projectHub}</span>
                    </span>
                  </span>
                </div>
              </div>
              <div className={`wide-project-session-list session-search-result-list${mobile ? ' mobile-project-session-list' : ''}`}>
                {section.rows.map(row => renderSessionSearchRow(section.project.projectId, row, mobile))}
              </div>
            </div>
          );
        })}
        {sessionSearchSections.length === 0 ? (
          <div className="wide-project-empty session-search-empty">
            {sessionSearchAllDone ? 'No matching sessions' : 'Searching... 0 results'}
          </div>
        ) : null}
        {errorMessages.length > 0 ? (
          <div className="session-search-error-list">
            {errorMessages.map(message => (
              <div key={message} className="session-search-error">
                {message}
              </div>
            ))}
          </div>
        ) : null}
      </>
    );
    if (mobile) {
      return (
        <div className="mobile-project-session-nav session-search-nav">
          {body}
        </div>
      );
    }
    return body;
  };

  const handleMobileChatQuickSwitchSelect = useCallback(async (targetProjectId: string, session: RegistryChatSession) => {
    setChatQuickSwitchMenuOpen(false);
    setPortRelayTargetMenuOpen(false);
    setPortRelayFrameOpen(false);
    setSidebarSettingsOpen(false);
    setDrawerOpen(false);
    setTab('chat');
    const currentKey = selectedChatKeyRef.current;
    if (currentKey?.projectId === targetProjectId && currentKey.sessionId === session.sessionId) {
      return;
    }
    await selectProjectChatSession(targetProjectId, session.sessionId, {closeMobileDrawer: true});
  }, [selectProjectChatSession, setDrawerOpen, setSidebarSettingsOpen, setTab]);

  const handleProjectCreateSession = async (
    targetProjectId: string,
    agentType: string,
    options?: {closeMobileDrawer?: boolean},
  ) => {
    agentType = normalizeAgentTypeName(agentType);
    if (!targetProjectId || !agentType) {
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
      chatTurnStoreRef.current[runtimeKey] = createEmptyChatTurnStore();
      chatFinishedCursorRef.current[runtimeKey] = 0;
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
    agentType = normalizeAgentTypeName(agentType);
    if (!targetProjectId || !agentType) {
      setError('No agent selected for resume');
      return;
    }
    setWideProjectActionMenu(current => ({
      projectId: targetProjectId,
      kind: 'resume',
      phase: 'sessions',
      agentType,
      popover: current?.projectId === targetProjectId && current.kind === 'resume'
        ? current.popover
        : null,
    }));
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
    agentType = normalizeAgentTypeName(agentType);
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
      chatTurnStoreRef.current[runtimeKey] = createEmptyChatTurnStore();
      chatFinishedCursorRef.current[runtimeKey] = 0;
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
    agentType = normalizeAgentTypeName(agentType);
    if (!targetProjectId || !agentType) {
      setError('No agent selected for resume');
      return;
    }
    setMobileProjectActionMenu({
      projectId: targetProjectId,
      kind: 'resume',
      phase: 'sessions',
      agentType,
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
    agentType = normalizeAgentTypeName(agentType);
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
      chatTurnStoreRef.current[runtimeKey] = createEmptyChatTurnStore();
      chatFinishedCursorRef.current[runtimeKey] = 0;
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

  const renderProjectSessionActionMenu = (targetProjectId: string, session: RegistrySessionSummary) => {
    const sessionId = session.sessionId;
    const sessionActionDisabled = !!session.running ||
      chatReloadingSessionId === sessionId ||
      chatArchivingSessionId === sessionId ||
      chatDeletingSessionId === sessionId;
    const renameActionDisabled = chatRenamingSessionId === sessionId;
    const actionsOpen = projectSessionActionMenu?.projectId === targetProjectId &&
      projectSessionActionMenu.sessionId === sessionId;
    return (
      <>
        {actionsOpen ? (
          <div
            className="project-session-action-menu"
            role="menu"
            style={projectSessionActionMenu.popover
              ? {
                  top: `${projectSessionActionMenu.popover.top}px`,
                  left: `${projectSessionActionMenu.popover.left}px`,
                  width: `${projectSessionActionMenu.popover.width}px`,
                  maxHeight: `${projectSessionActionMenu.popover.maxHeight}px`,
                  transform: projectSessionActionMenu.popover.placement === 'above'
                    ? 'translateY(-100%)'
                    : undefined,
                }
              : undefined}
          >
            <button
              type="button"
              className="project-session-menu-btn rename"
              role="menuitem"
              disabled={renameActionDisabled}
              onClick={event => {
                event.stopPropagation();
                requestRenameProjectSession(targetProjectId, session);
              }}
            >
              <span
                className={`codicon ${
                  chatRenamingSessionId === sessionId
                    ? 'codicon-loading codicon-modifier-spin'
                    : 'codicon-edit'
                }`}
              />
              <span className="project-session-menu-label">Rename</span>
            </button>
            <button
              type="button"
              className="project-session-menu-btn archive"
              role="menuitem"
              disabled={sessionActionDisabled}
              onClick={event => {
                event.stopPropagation();
                requestArchiveProjectSession(targetProjectId, session);
              }}
            >
              <span
                className={`codicon ${
                  chatArchivingSessionId === sessionId
                    ? 'codicon-loading codicon-modifier-spin'
                    : 'codicon-archive'
                }`}
              />
              <span className="project-session-menu-label">Archive</span>
            </button>
            <div className="project-session-menu-separator" aria-hidden="true" />
            <button
              type="button"
              className="project-session-menu-btn reload"
              role="menuitem"
              disabled={sessionActionDisabled}
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
              <span className="project-session-menu-label">Reload</span>
            </button>
            <button
              type="button"
              className="project-session-menu-btn delete"
              role="menuitem"
              disabled={sessionActionDisabled}
              onClick={event => {
                event.stopPropagation();
                requestDeleteProjectSession(targetProjectId, session);
              }}
            >
              <span
                className={`codicon ${
                  chatDeletingSessionId === sessionId
                    ? 'codicon-loading codicon-modifier-spin'
                    : 'codicon-trash'
                }`}
              />
              <span className="project-session-menu-label">Delete</span>
            </button>
          </div>
        ) : null}
      </>
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
        knownProjectRev: knownProjectRevRef.current,
        knownGitRev: knownGitRevRef.current,
        knownWorktreeRev: knownWorktreeRevRef.current,
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
      const needsGitRefresh = sync.staleDomains.some(
        domain =>
          domain === 'git' || domain === 'worktree' || domain === 'project',
      );
      if (needsGitRefresh) {
        const gitLoaded = await loadGit();
        if (gitLoaded) {
          knownGitRevRef.current = sync.gitRev ?? '';
        }
      }
      knownProjectRevRef.current = sync.projectRev ?? '';
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
    const listedSessions = sortChatSessions(sessions);
    const knownSessions = knownChatSessionsForProject(targetProjectId);
    const nextSessions = mergeChatSessionList(knownSessions, listedSessions);
    setProjectSessionsByProjectId(prev => ({
      ...prev,
      [targetProjectId]: mergeChatSessionList(
        prev[targetProjectId] ?? knownSessions,
        listedSessions,
      ),
    }));
    if (shouldUpdateCurrentProjectSessions(targetProjectId, projectIdRef.current)) {
      setChatSessions(prev => mergeChatSessionList(prev, listedSessions));
    }
    const cached = workspaceStore.hydrateChatSessions(targetProjectId);
    const cursorBySessionId: Record<string, {turnIndex: number}> = {};
    for (const entry of cached) {
      const sessionId = entry.session.sessionId;
      if (!sessionId) continue;
      const runtimeKey = buildChatRuntimeKey(targetProjectId, sessionId);
      cursorBySessionId[sessionId] = {
        turnIndex: chatFinishedCursorRef.current[runtimeKey] ?? entry.cursor.turnIndex,
      };
    }
    for (const session of nextSessions) {
      const sessionId = session.sessionId;
      if (!sessionId || cursorBySessionId[sessionId]) continue;
      const runtimeKey = buildChatRuntimeKey(targetProjectId, sessionId);
      cursorBySessionId[sessionId] = {
        turnIndex: chatFinishedCursorRef.current[runtimeKey] ?? 0,
      };
    }
    workspaceStore.replaceChatSessions(targetProjectId, nextSessions, cursorBySessionId);
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
          if (payload.session.running === false) {
            setChatCancellingRuntimeKey(current => (current === runtimeKey ? '' : current));
          }
          const mergedSession = mergeKnownChatSessionForProject(eventProjectId, payload.session);
          rememberChatSessionSummary(eventProjectId, mergedSession);
          workspaceStore.rememberChatSession(eventProjectId, mergedSession, {
            turnIndex: chatFinishedCursorRef.current[runtimeKey] ?? 0,
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
        const payload = (event.payload ?? {}) as RegistryChatMessageEventPayload;
        const normalizedPayload = normalizeSessionMessagePayload(payload);
        const message = normalizedPayload
          ? decodeSessionTurnToMessage(normalizedPayload.sessionId, normalizedPayload.turn)
          : decodeSessionMessageFromEventPayload(payload);
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
        }
        const existingSession = knownProjectSessions.find(item => item.sessionId === sessionId);
        maybeNotifyChatMessage(message, existingSession, eventProjectId);

        const turnState = ensureChatTurnStore(runtimeKey);
        const incomingTurn = normalizedPayload?.turn ?? {
          turnIndex: message.turnIndex ?? 0,
          content: JSON.stringify({method: message.method, param: message.param}),
          finished: message.finished === true,
        };
        const gapReadCursor = shouldReadRepairForIncomingTurn(turnState, incomingTurn);
        mergeRealtimeTurn(turnState, incomingTurn);
        const merged = messagesFromTurnStore(runtimeKey, sessionId);
        const latestSyncCursor = turnState.cursor;
        chatFinishedCursorRef.current[runtimeKey] = latestSyncCursor.turnIndex;
        chatMessageStoreRef.current[runtimeKey] = merged;
        if (incomingTurn.finished) {
          markChatSessionTurnsDirty(runtimeKey);
        }

        if (isSelectedSession) {
          setVisibleChatMessagesForRuntimeKey(runtimeKey, merged, {
            followLatest: chatAutoScrollFollowRef.current,
          });
        }
        if (gapReadCursor) {
          chatReadRepairQueueRef.current.request(runtimeKey, gapReadCursor.turnIndex, async cursor => {
            chatFinishedCursorRef.current[runtimeKey] = cursor;
            await refreshSessionTurns(sessionId, eventProjectId);
          }).catch(() => undefined);
        }
        if (message.method === 'prompt_request') {
          forgetPendingChatPrompt(runtimeKey);
        }
        if (message.method === 'prompt_done' && isSelectedSession) {
          setChatCancellingRuntimeKey(current => (current === runtimeKey ? '' : current));
          markChatSessionRead(
            eventProjectId,
            sessionId,
            message.turnIndex ?? 0,
          ).catch(() => undefined);
        }
        if (
          message.method === 'prompt_done' &&
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

  const renderWorkspaceProjectSelector = () => {
    const currentWorkspaceProject = projects.find(item => item.projectId === projectId);
    return (
      <div className="workspace-project-selector">
        <div className="workspace-project-label">WORKSPACE</div>
        <div className="workspace-project-control">
          <button
            type="button"
            className="workspace-project-button"
            onClick={() => setWorkspaceProjectMenuOpen(prev => !prev)}
            title={currentWorkspaceProject?.path || currentProjectName}
          >
            <span className="workspace-project-name">
              {currentWorkspaceProject?.name || currentProjectName}
            </span>
            <span className="codicon codicon-chevron-down" />
          </button>
          {workspaceProjectMenuOpen ? (
            <div className="workspace-project-menu">
              {sortedProjectItems.map(projectItem => (
                <button
                  key={`workspace:${projectItem.projectId}`}
                  type="button"
                  className={`workspace-project-menu-item ${
                    projectItem.projectId === projectId ? 'selected' : ''
                  }`}
                  onClick={() =>
                    syncWorkspaceProject(projectItem.projectId, {reason: 'manual'}).catch(() => undefined)
                  }
                  title={projectItem.path || projectItem.projectId}
                >
                  <span className="workspace-project-menu-name">{projectItem.name}</span>
                  <span className="workspace-project-menu-path">
                    {projectItem.path || projectItem.hubId || projectItem.projectId}
                  </span>
                </button>
              ))}
            </div>
          ) : null}
        </div>
      </div>
    );
  };

  const renderSidebarMain = (showSectionTitle = true) => {
    if (tab === 'file') {
      return (
        <>
          {isWide ? renderWorkspaceProjectSelector() : null}
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
        {isWide ? renderWorkspaceProjectSelector() : null}
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

  const renderSettingsSection = (title: string, rows: React.ReactNode, icon?: string) => (
    <section className="settings-section" aria-label={title}>
      <div className="settings-section-title">
        {icon ? <span className={`codicon codicon-${icon}`} aria-hidden="true" /> : null}
        <span>{title}</span>
      </div>
      <div className="settings-section-rows">{rows}</div>
    </section>
  );

  const renderSettingsDetailActions = (detail: ActiveSettingsDetailView): React.ReactNode => {
    if (detail === 'skills') {
      return (
        <button
          type="button"
          className="token-stats-refresh-btn token-stats-refresh-inline"
          onClick={() => refreshSkillManagement().catch(() => undefined)}
          disabled={skillsLoading}
        >
          {skillsLoading ? 'Refreshing...' : 'Refresh'}
        </button>
      );
    }
    if (detail === 'update') {
      return (
        <button
          type="button"
          className="token-stats-refresh-btn token-stats-refresh-inline"
          onClick={() => {
            refreshWheelMakerUpdates().catch(() => undefined);
            refreshAgentPackages().catch(() => undefined);
          }}
          disabled={wheelMakerUpdatesLoading || agentPackagesLoading}
        >
          {wheelMakerUpdatesLoading || agentPackagesLoading ? 'Refreshing...' : 'Refresh'}
        </button>
      );
    }
    if (detail === 'tokenStats') {
      return (
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
      );
    }
    if (detail === 'database') {
      return (
        <button
          type="button"
          className="git-section-btn"
          onClick={exportDatabaseDump}
          disabled={databaseLoading || !!databaseError || !databaseDumpText}
          title="Export current database dump"
        >
          Export
        </button>
      );
    }
    if (detail === 'portRelay') {
      return (
        <button
          type="button"
          className="token-stats-refresh-btn token-stats-refresh-inline"
          onClick={() => refreshPortRelayStatus().catch(() => undefined)}
          disabled={portRelayLoading}
        >
          {portRelayLoading ? 'Refreshing...' : 'Refresh'}
        </button>
      );
    }
    return null;
  };

  const renderSettingsDetailShell = (
    title: string,
    content: React.ReactNode,
    actions?: React.ReactNode,
    options: SettingsDetailShellOptions = {},
  ) => (
    <div className={`settings-detail-page${options.hideDetailHeader ? ' settings-detail-page-body-only' : ''}`}>
      {options.hideDetailHeader ? null : (
        <div className="settings-detail-header">
          <button
            type="button"
            className="mobile-settings-back settings-detail-back"
            onClick={handleSettingsDetailBack}
            aria-label="Back to settings"
            title="Back"
          >
            <span className="codicon codicon-arrow-left" />
          </button>
          <div className="settings-detail-title">{title}</div>
          {actions ?? <span className="settings-detail-header-spacer" aria-hidden="true" />}
        </div>
      )}
      <div className="settings-detail-body">{content}</div>
    </div>
  );

  const renderSkillIconButton = (options: {
    label: string;
    icon: string;
    onClick: () => void;
    disabled?: boolean;
    danger?: boolean;
    pending?: boolean;
  }) => (
    <button
      type="button"
      className={`settings-skill-icon-btn${options.danger ? ' danger' : ''}`}
      disabled={options.disabled}
      onClick={options.onClick}
      title={options.label}
      aria-label={options.label}
    >
      <span className={`codicon ${options.pending ? 'codicon-loading codicon-modifier-spin' : options.icon}`} />
    </button>
  );

  const renderSkillInstallPanel = (target: SkillInstallTarget) => {
    const activeInstallTarget = skillInstallTarget;
    if (!activeInstallTarget || !sameSkillInstallTarget(activeInstallTarget, target)) {
      return null;
    }
    const selected = new Set(skillSourceSelectedNames);
    const candidateNames = Array.from(new Set(skillSourceCandidates
      .map(candidate => candidate.name)
      .filter(Boolean)));
    const allCandidatesSelected = candidateNames.length > 0 && candidateNames.every(name => selected.has(name));
    return (
      <section className="settings-skills-install-panel">
        <div className="settings-skills-scope-header">
          <div className="settings-skills-scope-title">{skillScopeLabel(activeInstallTarget)}</div>
          <button
            type="button"
            className="settings-detail-action-btn"
            onClick={() => {
              setSkillInstallTarget(null);
              setSkillSourceError('');
              setSkillSourceCandidates([]);
              setSkillSourceSelectedNames([]);
            }}
          >
            Close
          </button>
        </div>
        <div className="settings-skills-source-row">
          <input
            className="settings-skills-source-input"
            value={skillSourceInput}
            onChange={event => setSkillSourceInput(event.target.value)}
            onKeyDown={event => {
              if (event.key === 'Enter') {
                listSkillSource().catch(() => undefined);
              }
            }}
            placeholder="owner/repo or npx skills add ... --skill name"
          />
          <button
            type="button"
            className="settings-detail-action-btn"
            disabled={skillSourceLoading}
            onClick={() => listSkillSource().catch(() => undefined)}
          >
            {skillSourceLoading ? 'Listing...' : 'List'}
          </button>
        </div>
        {skillSourceError ? (
          <div className="settings-metadata-error">{skillSourceError}</div>
        ) : null}
        {candidateNames.length > 0 ? (
          <div className="settings-skills-candidates">
            <label className="settings-skill-row settings-skill-candidate-row settings-skill-select-all-row">
              <input
                type="checkbox"
                checked={allCandidatesSelected}
                onChange={toggleAllSkillSourceCandidates}
              />
              <span className="settings-skill-row-main">
                <span className="settings-skill-name">Select all</span>
              </span>
            </label>
            {candidateNames.map(skillName => (
              <label key={`candidate:${skillName}`} className="settings-skill-row settings-skill-candidate-row">
                <input
                  type="checkbox"
                  checked={selected.has(skillName)}
                  onChange={() => toggleSkillSourceCandidate(skillName)}
                />
                <span className="settings-skill-row-main">
                  <span className="settings-skill-name">{skillName}</span>
                </span>
              </label>
            ))}
          </div>
        ) : null}
        <div className="settings-skills-install-actions">
          <span className="settings-skill-meta">Selected: {skillSourceSelectedNames.length}</span>
          <button
            type="button"
            className="settings-detail-action-btn"
            disabled={skillSourceSelectedNames.length === 0}
            onClick={requestSkillInstallConfirm}
          >
            Install
          </button>
        </div>
      </section>
    );
  };

  const renderSkillScopeRows = (
    hubId: string,
    title: string,
    skills: RegistrySkillSnapshot[],
    options: {
      scope: RegistrySkillScope;
      projectName?: string;
      disabled?: boolean;
      error?: string;
      allowUpdate?: boolean;
      operationRunning?: boolean;
      actionsDisabled?: boolean;
      loading?: boolean;
      summary?: string;
      updateIncludeProjects?: boolean;
      updateLabel?: string;
    },
  ) => {
    const groups = groupSkillsByCategory(skills);
    const disabled = options.disabled === true;
    const hubActionPending = isSkillActionPendingForHub(skillsPendingKey, hubId);
    const actionDisabled = disabled || options.actionsDisabled === true || options.operationRunning === true || hubActionPending;
    const skillCount = skills.length;
    const scopeKind = options.scope === 'hub' ? 'Hub' : 'Project';
    return (
      <section className={`settings-skills-scope settings-skills-scope-${options.scope}`}>
        <div className="settings-skills-scope-header">
          <div className="settings-skills-scope-heading">
            <div className="settings-skills-scope-title-wrap">
              <span className="settings-skills-scope-kind">{scopeKind}</span>
              <span className="settings-skills-scope-title" title={title}>{title}</span>
              {options.summary ? null : <span className="settings-skills-count">{skillCount}</span>}
              {disabled ? <span className="agent-package-status status-not_published">Offline</span> : null}
              {options.loading ? <span className="wide-session-agent-tag">Scanning</span> : null}
            </div>
            {options.summary ? <span className="settings-skills-scope-summary">{options.summary}</span> : null}
          </div>
          <div className="settings-skills-scope-actions">
            {options.allowUpdate ? renderSkillIconButton({
              label: options.updateLabel || 'Update skills',
              icon: 'codicon-sync',
              pending: options.updateIncludeProjects ? options.operationRunning : false,
              disabled: actionDisabled,
              onClick: () => requestSkillUpdate(options.updateIncludeProjects
                ? {hubId, scope: options.scope, projectName: options.projectName, includeProjects: true}
                : {hubId, scope: options.scope, projectName: options.projectName}),
            }) : null}
            {renderSkillIconButton({
              label: 'Add skills',
              icon: 'codicon-add',
              disabled: actionDisabled,
              onClick: () => requestSkillInstall({hubId, scope: options.scope, projectName: options.projectName}),
            })}
          </div>
        </div>
        {options.error ? (
          <div className="settings-metadata-error">{options.error}</div>
        ) : null}
        {renderSkillInstallPanel({hubId, scope: options.scope, projectName: options.projectName})}
        <div className="settings-skills-scope-body">
          {groups.length === 0 && !options.error ? (
            <div className="settings-skills-empty">No skills installed.</div>
          ) : null}
          {groups.map(group => (
            <div key={`${hubId}:${title}:${group.categoryKey}`} className="settings-skill-category-block">
              <div className="settings-skill-category">
                <span>{group.category}</span>
                <span>{group.skills.length}</span>
              </div>
              {group.skills.map(skill => {
                const managed = skill.managed !== false;
                const pendingKey = skillActionPendingKey({
                  hubId,
                  scope: options.scope,
                  projectName: options.projectName,
                  skillName: skill.name,
                  action: 'skillUninstall',
                });
                const pending = skillsPendingKey === pendingKey;
                return (
                  <div key={`${hubId}:${title}:${skill.name}`} className="settings-skill-row">
                    <div className="settings-skill-row-main">
                      <span className="settings-skill-name" title={skill.path || skill.name}>{skill.name}</span>
                      {managed ? null : <span className="settings-skill-readonly-tag">External</span>}
                    </div>
                    {managed ? renderSkillIconButton({
                      label: pending ? 'Removing skill' : 'Uninstall skill',
                      icon: 'codicon-trash',
                      danger: true,
                      pending,
                      disabled: actionDisabled,
                      onClick: () => requestSkillUninstall({
                        hubId,
                        scope: options.scope,
                        projectName: options.projectName,
                        skillName: skill.name,
                      }),
                    }) : null}
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </section>
    );
  };

  const renderSkillsSettingsDetail = (options?: SettingsDetailShellOptions) =>
    renderSettingsDetailShell(
      'Skills',
      <>
        <a
          className="settings-skills-marketplace-link"
          href={SKILLS_MARKETPLACE_URL}
          target="_blank"
          rel="noreferrer"
        >
          <span className="settings-skills-marketplace-main">
            <span className="settings-skills-marketplace-label">Marketplace</span>
            <span className="settings-skills-marketplace-url">{SKILLS_MARKETPLACE_URL}</span>
          </span>
          <span className="codicon codicon-link-external" aria-hidden="true" />
        </a>
        {skillsLoading && Object.keys(skillHubs).length === 0 ? (
          <div className="muted block">Loading skills...</div>
        ) : null}
        {skillsError ? (
          <div className="muted block settings-metadata-error">{skillsError}</div>
        ) : null}
        {!skillsLoading && Object.keys(skillHubs).length === 0 && !skillsError ? (
          <div className="muted block">No hubs available.</div>
        ) : null}
        <div className="settings-skills-list">
          {Object.values(skillHubs).sort((left, right) => left.hubId.localeCompare(right.hubId)).map(hub => {
            const data = hub.data;
            const operation = data?.operation ?? null;
            const operationRunning = operation?.running === true;
            const projects = sortSkillProjects(data?.projects ?? []);
            const hubSkillCount = data?.hubSkills?.skills.length ?? 0;
            const projectSkillCount = projects.reduce((total, project) => total + project.skills.length, 0);
            return (
              <section className="settings-skills-hub" key={`skills-hub:${hub.hubId}`}>
                {hub.error ? (
                  <div className="settings-metadata-error">{hub.error}</div>
                ) : null}
                {operation ? (
                  <div className={`agent-package-task ${operation.status === 'failed' ? 'failed' : ''}`}>
                    <span>{skillOperationStatusLabel(operation.status)}</span>
                    <span>{operation.action}</span>
                    {operation.includeProjects ? <span>Hub + projects</span> : null}
                    {operation.message ? <span>{operation.message}</span> : null}
                    {operation.errorSummary ? <span>{operation.errorSummary}</span> : null}
                  </div>
                ) : null}
                <div className="settings-skills-scope-grid">
                  {renderSkillScopeRows(hub.hubId, hub.hubId, data?.hubSkills?.skills ?? [], {
                    scope: 'hub',
                    operationRunning,
                    actionsDisabled: hub.loading,
                    loading: hub.loading,
                    allowUpdate: true,
                    updateIncludeProjects: true,
                    updateLabel: 'Update hub and project skills',
                    summary: `${hubSkillCount} hub skills / ${projects.length} projects / ${projectSkillCount} project skills`,
                  })}
                  {projects.map(project => (
                    <div className="settings-skills-project" key={`${hub.hubId}:${project.projectName}`}>
                      {renderSkillScopeRows(hub.hubId, project.projectName, project.skills, {
                        scope: 'project',
                        projectName: project.projectName,
                        disabled: !project.online,
                        error: project.error,
                        allowUpdate: true,
                        operationRunning,
                      })}
                    </div>
                  ))}
                </div>
              </section>
            );
          })}
        </div>
      </>,
      renderSettingsDetailActions('skills'),
      options,
    );

  const renderUpdateSettingsDetail = (options?: SettingsDetailShellOptions) =>
    renderSettingsDetailShell(
      'Update',
      <>
        <button
          type="button"
          className="wheelmaker-update-all-btn"
          disabled={updateHubCards.length === 0 || wheelMakerUpdateAllPending}
          onClick={() => requestWheelMakerUpdateAll(updateHubCards.map(card => card.hubId))}
        >
          <span
            className={`codicon ${
              wheelMakerUpdateAllPending
                ? 'codicon-loading codicon-modifier-spin'
                : 'codicon-cloud-download'
            }`}
          />
          <span>{wheelMakerUpdateAllPending ? 'Updating All Hubs...' : 'Update All Hubs'}</span>
        </button>
        {(wheelMakerUpdatesLoading || agentPackagesLoading) && updateHubCards.length === 0 ? (
          <div className="muted block">Scanning hubs...</div>
        ) : null}
        {wheelMakerUpdatesError || agentPackagesError ? (
          <div className="muted block settings-metadata-error">{wheelMakerUpdatesError || agentPackagesError}</div>
        ) : null}
        {!wheelMakerUpdatesLoading && !agentPackagesLoading && updateHubCards.length === 0 && !wheelMakerUpdatesError && !agentPackagesError ? (
          <div className="muted block">No hubs available.</div>
        ) : null}
        <div className="settings-metadata-list agent-package-hub-list">
          {updateHubCards.map(card => {
            const wheelMaker = card.wheelMaker;
            const wheelMakerData = wheelMaker?.data ?? null;
            const wheelMakerStatus = wheelMakerData?.status || (wheelMaker?.loading ? 'checking' : 'unknown');
            const agentCard = card.agentPackage;
            const hub = agentCard?.hub;
            const operation = agentCard?.operation;
            const npmUpdateTargets = deriveNpmPackageUpdateTargets(hub?.packages ?? []);
            const npmExpanded = expandedNpmUpdateHubIds[card.hubId] === true;
            const npmHubUpdatePending = agentPackageHubUpdatePendingId === card.hubId;
            const npmActionDisabled = npmHubUpdatePending || operation?.running === true || agentCard?.loading === true;
            const wheelMakerPending = wheelMakerUpdatePendingHubId === card.hubId;
            const showWheelMakerUpdateAction = shouldShowWheelMakerUpdateAction({
              data: wheelMakerData,
              loading: wheelMaker?.loading === true,
              pending: wheelMakerPending || wheelMakerUpdateAllPending,
            });
            const wheelMakerCurrentSha = wheelMakerData?.release?.sha || wheelMakerData?.git?.currentSha || '';
            const wheelMakerLatestSha = wheelMakerData?.git?.latestSha || '';
            const wheelMakerCurrentTime = formatWheelMakerDateTime(
              wheelMakerData?.release?.publishedAt || wheelMakerData?.git?.currentCommittedAt || '',
            );
            const wheelMakerLatestTime = formatWheelMakerDateTime(wheelMakerData?.git?.latestCommittedAt || '');
            return (
              <div key={`update-hub:${card.hubId}`} className="settings-metadata-card agent-package-hub-card">
                <div className="settings-metadata-line settings-metadata-line-tags update-hub-header">
                  <span className={`wide-project-hub-tag ${tagVariantClass('wide-project-hub', card.hubId)}`}>
                    <span className="wide-project-hub-dot" aria-hidden="true" />
                    <span className="wide-project-hub-label">{card.hubId}</span>
                  </span>
                  {wheelMaker?.loading || agentCard?.loading ? (
                    <span className="wide-session-agent-tag">Scanning</span>
                  ) : null}
                </div>
                <div className="wheelmaker-update-panel">
                  <div className="wheelmaker-update-title-line">
                    <span className="wheelmaker-update-scope">Release</span>
                    <span className={`agent-package-status status-${wheelMakerStatus}`}>
                      {wheelMaker?.loading ? 'Checking' : wheelMakerUpdateStatusLabel(wheelMakerStatus)}
                    </span>
                  </div>
                  <div className="wheelmaker-update-version-line">
                    <span className="wheelmaker-update-ref-tag">{wheelMakerReleaseRef(wheelMakerData)}</span>
                    <span className="wheelmaker-update-behind">{wheelMakerBehindCopy(wheelMakerData)}</span>
                  </div>
                  <div className="wheelmaker-update-sha-lines">
                    <div className="wheelmaker-update-sha-line" title={`Current ${wheelMakerCurrentSha || '-'} ${wheelMakerCurrentTime}`}>
                      <span className="wheelmaker-update-sha-label">Current</span>
                      <span className="wheelmaker-update-sha-value">{shortGitSha(wheelMakerCurrentSha)}</span>
                      <span className="wheelmaker-update-sha-time">{wheelMakerCurrentTime}</span>
                    </div>
                    <div className="wheelmaker-update-sha-line" title={`Latest ${wheelMakerLatestSha || '-'} ${wheelMakerLatestTime}`}>
                      <span className="wheelmaker-update-sha-label">Latest</span>
                      <span className="wheelmaker-update-sha-value">{shortGitSha(wheelMakerLatestSha)}</span>
                      <span className="wheelmaker-update-sha-time">{wheelMakerLatestTime}</span>
                    </div>
                  </div>
                  {wheelMaker?.error || wheelMakerData?.error ? (
                    <div className="settings-metadata-error">{wheelMaker?.error || wheelMakerData?.error}</div>
                  ) : null}
                  {showWheelMakerUpdateAction ? (
                    <button
                      type="button"
                      className="wheelmaker-update-action-btn"
                      disabled={wheelMakerUpdateAllPending || wheelMakerPending || wheelMakerData?.pendingSignal === true}
                      onClick={() => requestWheelMakerUpdatePublish(card.hubId, wheelMakerData)}
                    >
                      {wheelMakerPending ? 'Updating...' : wheelMakerData?.pendingSignal ? 'Requested' : 'Update'}
                    </button>
                  ) : null}
                </div>
                <section className="npm-update-section">
                  <div className="npm-update-disclosure">
                    <button
                      type="button"
                      className="npm-update-disclosure-btn"
                      aria-expanded={npmExpanded}
                      onClick={() => setExpandedNpmUpdateHubIds(prev => ({
                        ...prev,
                        [card.hubId]: !prev[card.hubId],
                      }))}
                    >
                      <span className={`codicon ${npmExpanded ? 'codicon-chevron-down' : 'codicon-chevron-right'}`} aria-hidden="true" />
                      <span className="npm-update-count">{npmPackageUpdateSummary(npmUpdateTargets.length)}</span>
                      <span className="npm-update-total">{hub?.packages.length ?? 0} packages</span>
                    </button>
                    <button
                      type="button"
                      className="npm-update-action-btn"
                      disabled={npmUpdateTargets.length === 0 || npmActionDisabled}
                      onClick={() => requestAgentPackageHubUpdate(card.hubId, npmUpdateTargets)}
                    >
                      {npmHubUpdatePending ? 'Updating...' : 'Update All'}
                    </button>
                  </div>
                  {npmExpanded ? (
                    <div className="npm-update-body">
                      {hub?.warning ? (
                        <div className="settings-metadata-line settings-metadata-error">{hub.warning}</div>
                      ) : null}
                      {agentCard?.error || hub?.error ? (
                        <div className="settings-metadata-line settings-metadata-error">{agentCard?.error || hub?.error}</div>
                      ) : null}
                      {operation ? (
                        <div className={`agent-package-task ${operation.status === 'failed' ? 'failed' : ''}`}>
                          <span>{packageStatusLabel(operation.status)}</span>
                          {operation.packageName ? <span>{operation.packageName}</span> : null}
                          {operation.message ? <span>{operation.message}</span> : null}
                          {operation.errorSummary ? <span>{operation.errorSummary}</span> : null}
                        </div>
                      ) : null}
                      <div className="agent-package-row-list">
                        {(hub?.packages ?? []).map(pkg => {
                          const action = agentPackageActionForPackage(pkg);
                          const pendingKey = agentPackageActionKey(card.hubId, pkg.packageName);
                          const pending = agentPackageActionPendingKey === pendingKey || operation?.running === true || npmHubUpdatePending;
                          return (
                            <div key={`${card.hubId}:${pkg.packageName}`} className="agent-package-row">
                              <div className="agent-package-title-line">
                                <span className="settings-metadata-title" title={pkg.displayName}>{pkg.displayName}</span>
                                {pkg.agentTypes.length > 0 ? (
                                  <span className="agent-package-agent-tags">
                                    {pkg.agentTypes.map(agent => (
                                      <span key={`${pkg.packageName}:${agent}`} className={`wide-session-agent-tag ${tagVariantClass('wide-session-agent', agent)}`}>
                                        {agent}
                                      </span>
                                    ))}
                                  </span>
                                ) : null}
                              </div>
                              <div className="agent-package-name-line">
                                <span className="agent-package-name" title={pkg.packageName}>{pkg.packageName}</span>
                              </div>
                              {action ? (
                                <button
                                  type="button"
                                  className={`agent-package-action-btn ${action === 'uninstall' ? 'danger' : ''}`}
                                  disabled={pending}
                                  onClick={() => requestAgentPackageAction(action, card.hubId, pkg)}
                                >
                                  {pending ? 'Running...' : agentPackageActionLabel(action)}
                                </button>
                              ) : null}
                              <div className="agent-package-version-line">
                                <span>Installed: {pkg.installedVersion || '-'}</span>
                                <span>Latest: {pkg.latestVersion || '-'}</span>
                                <span className={`agent-package-status agent-package-version-status status-${pkg.status}`}>{packageStatusLabel(pkg.status)}</span>
                              </div>
                              {pkg.error ? (
                                <div className="settings-metadata-error">{pkg.error}</div>
                              ) : null}
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  ) : null}
                </section>
              </div>
            );
          })}
        </div>
      </>,
      renderSettingsDetailActions('update'),
      options,
    );

  const renderTokenStatsSettingsDetail = (options?: SettingsDetailShellOptions) =>
    renderSettingsDetailShell(
      'Token Stats',
      <>
        {tokenStatsUpdatedAt ? (
          <div className="muted block">Updated: {tokenStatsUpdatedAt}</div>
        ) : null}
        {tokenStatsLoading ? (
          <div className="muted block">Scanning online hubs...</div>
        ) : null}
        {tokenStatsError ? (
          <div className="muted block settings-metadata-error">{tokenStatsError}</div>
        ) : null}
        {!tokenStatsLoading && tokenStatCards.length === 0 && !tokenStatsError ? (
          <div className="muted block">No token accounts discovered.</div>
        ) : null}
        <div className="settings-metadata-list token-stats-account-list-flat">
          {tokenStatCards.map(card => (
            <div key={card.id} className="settings-metadata-card">
              <div className="settings-metadata-line settings-metadata-line-tags">
                <span className={`token-stats-pill ${tokenTagVariantClass('agent', card.agentTag)}`}>
                  {card.agentTag}
                </span>
                {card.hubTags.map(hubTag => (
                  <span key={hubTag} className={`token-stats-pill ${tokenTagVariantClass('hub', hubTag)}`}>
                    {hubTag}
                  </span>
                ))}
              </div>
              <div className="settings-metadata-line settings-metadata-line-primary">
                <span className="settings-metadata-title">{card.accountName}</span>
              </div>
              {card.message ? (
                <div className="settings-metadata-error">{card.message}</div>
              ) : null}
              {card.secondaryLine ? (
                <div className="settings-metadata-line">{card.secondaryLine}</div>
              ) : null}
              {card.tertiaryLine ? (
                <div className="settings-metadata-line">{card.tertiaryLine}</div>
              ) : null}
            </div>
          ))}
        </div>
      </>,
      renderSettingsDetailActions('tokenStats'),
      options,
    );

  const renderCCSwitchSettingsDetail = (options?: SettingsDetailShellOptions) => {
    const activeHub = (currentProject?.hubId || 'unknown').trim() || 'unknown';
    const activeAgent = (selectedChatSession?.agentType || '').trim() || '-';
    const profileCards = projects
      .map(projectItem => {
        const projectHub = (projectItem.hubId || 'local').trim() || 'local';
        const projectSessions = projectSessionsByProjectId[projectItem.projectId] ?? [];
        const projectAgents = getWideProjectAgents(projectItem, projectSessions);
        const profiles = (projectItem.agentProfiles ?? [])
          .map(profile => {
            const profileName = (profile.name || '').trim();
            if (!profileName) {
              return null;
            }
            const skills = (profile.skills ?? [])
              .map(skill => (skill || '').trim())
              .filter(Boolean);
            return { profileName, skills };
          })
          .filter((item): item is { profileName: string; skills: string[] } => item !== null);
        return {
          projectId: projectItem.projectId,
          projectName: projectItem.name,
          projectHub,
          projectAgents,
          profiles,
        };
      })
      .filter(item => item.profiles.length > 0);

    return renderSettingsDetailShell(
      'CC Switch',
      <>
        <div className="settings-metadata-list">
          <div className="settings-metadata-card">
            <div className="settings-metadata-line settings-metadata-line-tags">
              <span className={`wide-project-hub-tag ${tagVariantClass('wide-project-hub', activeHub)}`}>
                <span className="wide-project-hub-dot" aria-hidden="true" />
                <span className="wide-project-hub-label">Hub: {activeHub}</span>
              </span>
              <span className={`wide-session-agent-tag ${tagVariantClass('wide-session-agent', activeAgent)}`}>
                Agent: {activeAgent}
              </span>
            </div>
          </div>
          {profileCards.map(card => (
            <div key={`cc-switch:${card.projectId}`} className="settings-metadata-card">
              <div className="settings-metadata-line settings-metadata-line-tags">
                <span className={`wide-project-hub-tag ${tagVariantClass('wide-project-hub', card.projectHub)}`}>
                  <span className="wide-project-hub-dot" aria-hidden="true" />
                  <span className="wide-project-hub-label">{card.projectHub}</span>
                </span>
                {card.projectAgents.map(agent => (
                  <span
                    key={`cc-switch:${card.projectId}:agent:${agent}`}
                    className={`wide-session-agent-tag ${tagVariantClass('wide-session-agent', agent)}`}
                  >
                    {agent}
                  </span>
                ))}
              </div>
              <div className="settings-metadata-line settings-metadata-line-primary">
                <span className="settings-metadata-title" title={card.projectName}>{card.projectName}</span>
              </div>
              {card.profiles.map(profile => (
                <div key={`cc-switch:${card.projectId}:profile:${profile.profileName}`} className="settings-metadata-line">
                  <span className={`wide-session-agent-tag ${tagVariantClass('wide-session-agent', profile.profileName)}`}>
                    {profile.profileName}
                  </span>
                  {profile.skills.length > 0 ? ` · ${profile.skills.join(', ')}` : ' · No skills'}
                </div>
              ))}
            </div>
          ))}
        </div>
        {profileCards.length === 0 ? (
          <div className="muted block">No CC Switch profile metadata found.</div>
        ) : null}
      </>,
      undefined,
      options,
    );
  };

  const renderDatabaseSettingsDetail = (options?: SettingsDetailShellOptions) =>
    renderSettingsDetailShell(
      'Database',
      <>
        {databaseLoading ? (
          <div className="muted block">Loading database...</div>
        ) : null}
        {databaseError ? (
          <div className="error">Database error: {databaseError}</div>
        ) : null}
        {!databaseLoading && !databaseError ? (
          <pre className="settings-database-dump">{databaseDumpText}</pre>
        ) : null}
      </>,
      renderSettingsDetailActions('database'),
      options,
    );

  const renderPortRelaySettingsDetail = (options?: SettingsDetailShellOptions) => {
    const hubIds = deriveRegistryHubIds(registryHubs);
    const selectedTarget = selectedPortRelayTarget ?? portRelayTargets[0] ?? null;
    const statusClass = String(portRelaySnapshot.status || 'Disabled').toLowerCase();
    const portRelayTargetDisplay = selectedTarget ? `${selectedTarget.hubId} -> 127.0.0.1:${selectedTarget.targetPort}` : 'No target';
    const listenPortNumber = Number(portRelayListenPort);
    const hasPendingListenPortChange =
      portRelaySnapshot.enabled &&
      typeof portRelaySnapshot.listenPort === 'number' &&
      Number.isInteger(listenPortNumber) &&
      listenPortNumber !== portRelaySnapshot.listenPort;
    return renderSettingsDetailShell(
      'Port Relay',
      <div className="port-relay-panel port-relay-panel-shell">
        <div className="port-relay-section port-relay-status-section">
          <div className="port-relay-header">
            <span className="port-relay-section-title">
              <span className="codicon codicon-radio-tower" aria-hidden="true" />
              Relay
            </span>
            <span className={`port-relay-status-pill ${statusClass}`}>{portRelaySnapshot.status}</span>
            <code className="port-relay-target-inline" title={portRelayTargetDisplay}>
              {portRelayTargetDisplay}
            </code>
          </div>
          {hasPendingListenPortChange ? (
            <div className="port-relay-pending-note">Listen port change applies on Enable.</div>
          ) : null}
        </div>
        {portRelayError || portRelaySnapshot.error ? (
          <div className="settings-metadata-error">{portRelayError || portRelaySnapshot.error}</div>
        ) : null}
        <div className="port-relay-section port-relay-control-section">
          <div className="port-relay-section-title">
            <span className="codicon codicon-key" aria-hidden="true" />
            Access
          </div>
          <div className="port-relay-form-grid">
            <label>
              <span>Listen Port</span>
              <input
                value={portRelayListenPort}
                inputMode="numeric"
                onChange={event => {
                  const nextValue = event.target.value.replace(/[^\d]/g, '').slice(0, 5);
                  const nextPort = Number(nextValue);
                  setPortRelayListenPort(nextValue);
                  if (Number.isInteger(nextPort) && nextPort >= 1 && nextPort <= 65535) {
                    persistPortRelaySettings({listenPort: nextPort});
                  }
                }}
              />
            </label>
          </div>
          <div className="port-relay-code-row">
            <input
              value={portRelayAccessCodeUnknown ? '' : portRelayAccessCode}
              placeholder={portRelayAccessCodeUnknown ? 'Unknown' : ''}
              readOnly
              aria-label="Port relay access code"
            />
            <button
              type="button"
              className="settings-detail-action-btn"
              onClick={() => regeneratePortRelayAccessCode().catch(() => undefined)}
              disabled={portRelayLoading}
            >
              {portRelayAccessCodeUnknown ? 'Reset Code' : 'Generate'}
            </button>
            <button
              type="button"
              className="settings-detail-action-btn port-relay-copy-btn"
              onClick={() => copyPortRelayAccessCode().catch(() => undefined)}
              disabled={portRelayAccessCodeUnknown || !portRelayAccessCode}
              aria-label="Copy port relay access code"
            >
              <span className={`codicon ${portRelayCodeCopied ? 'codicon-check' : 'codicon-copy'}`} aria-hidden="true" />
              {portRelayCodeCopied ? 'Copied' : 'Copy'}
            </button>
          </div>
        </div>
        <div className="port-relay-section port-relay-targets-section">
          <div className="port-relay-targets-header">
            <span className="port-relay-section-title">
              <span className="codicon codicon-server-process" aria-hidden="true" />
              Targets
            </span>
            <span>{'Hub -> 127.0.0.1:Port'}</span>
          </div>
          <div className="port-relay-target-list">
            {portRelayTargets.map(target => {
              const selected = samePortRelayTarget(selectedTarget, target);
              return (
                <div
                  key={`${target.hubId}:${target.targetPort}`}
                  className={`port-relay-target-list-row${selected ? ' selected' : ''}`}
                >
                  <input
                    type="checkbox"
                    checked={selected}
                    onChange={event => {
                      if (!event.target.checked) {
                        return;
                      }
                      selectPortRelayTarget(target).catch(() => undefined);
                    }}
                    aria-label={`Use ${target.hubId}:${target.targetPort}`}
                  />
                  <span className="port-relay-target-hub">{target.hubId}</span>
                  <code className="port-relay-target-port">{target.targetPort}</code>
                  <button
                    type="button"
                    className="port-relay-target-delete"
                    onClick={() => deletePortRelayTarget(target).catch(() => undefined)}
                    disabled={portRelayLoading}
                    title="Delete target"
                    aria-label={`Delete ${target.hubId}:${target.targetPort}`}
                  >
                    <span className="codicon codicon-close" />
                  </button>
                </div>
              );
            })}
            <div className="port-relay-target-list-row draft">
              <span className="port-relay-target-check-spacer" aria-hidden="true" />
              <select
                value={portRelayDraftHubId}
                onChange={event => setPortRelayDraftHubId(event.target.value)}
                disabled={hubIds.length === 0}
                aria-label="New relay target hub"
              >
                <option value="">{hubIds.length === 0 ? 'No hub' : 'Hub'}</option>
                {hubIds.map(hubId => (
                  <option key={hubId} value={hubId}>{hubId}</option>
                ))}
              </select>
              <input
                value={portRelayDraftPort}
                inputMode="numeric"
                onChange={event => setPortRelayDraftPort(event.target.value.replace(/[^\d]/g, '').slice(0, 5))}
                onBlur={() => {
                  commitPortRelayDraftTarget();
                }}
                onKeyDown={event => {
                  if (event.key !== 'Enter') {
                    return;
                  }
                  event.preventDefault();
                  commitPortRelayDraftTarget();
                }}
                aria-label="New relay target port"
              />
              <span className="port-relay-target-delete-spacer" aria-hidden="true" />
            </div>
          </div>
        </div>
        <div className="port-relay-actions">
          <button
            type="button"
            className="settings-detail-action-btn"
            onClick={() => enablePortRelay().catch(() => undefined)}
            disabled={portRelayLoading || !selectedTarget}
          >
            {portRelayLoading ? 'Working...' : 'Enable'}
          </button>
          <button
            type="button"
            className="settings-detail-action-btn danger"
            onClick={() => disablePortRelay().catch(() => undefined)}
            disabled={portRelayLoading || !portRelaySnapshot.enabled}
          >
            Disable
          </button>
        </div>
      </div>,
      renderSettingsDetailActions('portRelay'),
      options,
    );
  };

  const renderSettingsContent = (
    showSectionTitle: boolean,
    options: SettingsDetailShellOptions = {},
  ) => {
    if (settingsDetailView === 'update') {
      return renderUpdateSettingsDetail(options);
    }
    if (settingsDetailView === 'skills') {
      return renderSkillsSettingsDetail(options);
    }
    if (settingsDetailView === 'ccSwitch') {
      return renderCCSwitchSettingsDetail(options);
    }
    if (settingsDetailView === 'tokenStats') {
      return renderTokenStatsSettingsDetail(options);
    }
    if (settingsDetailView === 'database') {
      return renderDatabaseSettingsDetail(options);
    }
    if (settingsDetailView === 'portRelay') {
      return renderPortRelaySettingsDetail(options);
    }
    return (
    <>
      {showSectionTitle ? <div className="section-title">SETTINGS</div> : null}
      <div className="settings-list">
        {renderSettingsSection('Appearance', (
        <>
          <label className="settings-row sidebar-setting-row">
            <span>
              <span className="codicon codicon-color-mode settings-row-icon" aria-hidden="true" />
              Dark Mode
            </span>
            <input
              type="checkbox"
              checked={themeMode === 'dark'}
              onChange={e =>
                setThemeMode(e.target.checked ? 'dark' : 'light')
              }
            />
          </label>
          <label className="settings-row sidebar-setting-row">
            <span>
              <span className="codicon codicon-move settings-row-icon" aria-hidden="true" />
              Gesture Navigation
            </span>
            <input
              type="checkbox"
              checked={gestureNavigation}
              onChange={e => setGestureNavigation(e.target.checked)}
            />
          </label>
        </>
        ), 'paintcan')}
        {renderSettingsSection('Chat', (
        <>
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-tools settings-row-icon" aria-hidden="true" />
            Hide Tool Calls
          </span>
          <input
            type="checkbox"
            checked={hideToolCalls}
            onChange={e => setHideToolCalls(e.target.checked)}
          />
        </label>
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-cloud-download settings-row-icon" aria-hidden="true" />
            Local Hub Read
          </span>
          <input
            type="checkbox"
            checked={localHubReadEnabled}
            onChange={event => setLocalHubReadEnabled(event.target.checked)}
          />
        </label>
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-text-size settings-row-icon" aria-hidden="true" />
            Chat Font
          </span>
          <select
            className="sidebar-setting-select"
            value={chatFont}
            onChange={event => {
              const next = event.target.value;
              if (isChatFontId(next)) setChatFont(next);
            }}
          >
            {CHAT_FONT_OPTIONS.map(item => (
              <option key={item.id} value={item.id}>
                {item.label}
              </option>
            ))}
          </select>
        </label>
        </>
        ), 'comment-discussion')}
        {renderSettingsSection('Code Display', (
        <>
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-symbol-color settings-row-icon" aria-hidden="true" />
            Code Theme
          </span>
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
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-symbol-string settings-row-icon" aria-hidden="true" />
            Code Font
          </span>
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
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-text-size settings-row-icon" aria-hidden="true" />
            Font Size
          </span>
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
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-list-flat settings-row-icon" aria-hidden="true" />
            Line Height
          </span>
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
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-indent settings-row-icon" aria-hidden="true" />
            Tab Size
          </span>
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
        </>
        ), 'code')}
        {renderSettingsSection('More', (
        <>
        <button
          type="button"
          className="settings-row settings-detail-row"
          onClick={() => {
            setSettingsDetailView('update');
          }}
        >
          <span>
            <span className="codicon codicon-cloud-download settings-row-icon" aria-hidden="true" />
            Update
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        <button
          type="button"
          className="settings-row settings-detail-row"
          onClick={() => {
            setSkillsError('');
            setSettingsDetailView('skills');
          }}
        >
          <span>
            <span className="codicon codicon-extensions settings-row-icon" aria-hidden="true" />
            Skills
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        <button
          type="button"
          className="settings-row settings-detail-row"
          onClick={() => {
            setTokenStatsError('');
            setSettingsDetailView('tokenStats');
          }}
        >
          <span>
            <span className="codicon codicon-graph-line settings-row-icon" aria-hidden="true" />
            Token Stats
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        <button
          type="button"
          className="settings-row settings-detail-row"
          onClick={() => {
            setSettingsDetailView('ccSwitch');
          }}
        >
          <span>
            <span className="codicon codicon-arrow-swap settings-row-icon" aria-hidden="true" />
            CC Switch
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        <button
          type="button"
          className="settings-row settings-detail-row"
          onClick={() => {
            setSettingsDetailView('database');
            openDatabasePanel();
          }}
        >
          <span>
            <span className="codicon codicon-database settings-row-icon" aria-hidden="true" />
            Database
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        <button
          type="button"
          className="settings-row settings-detail-row"
          onClick={() => {
            setPortRelayError('');
            setSettingsDetailView('portRelay');
          }}
        >
          <span>
            <span className="codicon codicon-radio-tower settings-row-icon" aria-hidden="true" />
            Port Relay
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        <button
          type="button"
          className="settings-row settings-danger-row"
          onClick={requestClearLocalCache}
        >
          <span>
            <span className="codicon codicon-trash settings-row-icon" aria-hidden="true" />
            Clear Local Cache
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        </>
        ), 'list-unordered')}
        {renderSettingsSection('Debug', (
        <>
        <label className="settings-row sidebar-setting-row">
          <span>
            <span className="codicon codicon-bug settings-row-icon" aria-hidden="true" />
            Debug
          </span>
          <input
            type="checkbox"
            checked={registryDebug}
            onChange={event => setRegistryDebug(event.target.checked)}
          />
        </label>
        <button
          type="button"
          className="settings-row settings-detail-row"
          disabled={!registryDebug}
          onClick={() => {
            setRegistryDebugPanelOpen(true);
          }}
        >
          <span>
            <span className="codicon codicon-debug-alt settings-row-icon" aria-hidden="true" />
            Open Debug Panel
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        <button
          type="button"
          className="settings-row settings-danger-row"
          onClick={handleRegistryDebugLogout}
        >
          <span>
            <span className="codicon codicon-sign-out settings-row-icon" aria-hidden="true" />
            Logout
          </span>
          <span className="codicon codicon-chevron-right" />
        </button>
        </>
        ), 'bug')}
      </div>
    </>
    );
  };

  const renderMobileChatSessionSheet = () => {
    return (
      <>
        <div className={`mobile-chat-drawer-header${sessionSearchHeaderExpanded ? ' search-open' : ''}`}>
          {sessionSearchHeaderExpanded ? renderChatHeaderSearchControls(true) : (
            <>
              <div className="mobile-chat-toolbar" aria-label="Chat tools">
                <button
                  type="button"
                  className={`mobile-chat-toolbar-icon drawer-settings-icon-btn${sidebarSettingsOpen && !settingsDetailView ? ' active' : ''}`}
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
                <button
                  type="button"
                  className={`mobile-chat-toolbar-icon drawer-settings-icon-btn${sidebarSettingsOpen && settingsDetailView === 'update' ? ' active' : ''}`}
                  onClick={() => {
                    setProjectMenuOpen(false);
                    openSettingsDetail('update');
                  }}
                  title="Update"
                  aria-label="Update"
                >
                  <span className="codicon codicon-cloud-download" />
                </button>
                <button
                  type="button"
                  className={`mobile-chat-toolbar-icon drawer-settings-icon-btn${sidebarSettingsOpen && settingsDetailView === 'portRelay' ? ' active' : ''}`}
                  onClick={() => {
                    setProjectMenuOpen(false);
                    openSettingsDetail('portRelay');
                  }}
                  title="Port Relay"
                  aria-label="Port Relay"
                >
                  <span className="codicon codicon-radio-tower" />
                </button>
                <button
                  type="button"
                  className={`mobile-chat-toolbar-icon header-btn refresh-btn drawer-project-refresh${hasPendingProjectUpdates && !mobileProjectSessionsRefreshing && !reconnecting ? ' has-update-badge' : ''}`}
                  onClick={() => refreshMobileChatProjectSessions().catch(() => undefined)}
                  title={reconnecting ? 'Reconnecting...' : 'Refresh chats'}
                  disabled={mobileProjectSessionsRefreshing || reconnecting}
                >
                  {mobileProjectSessionsRefreshing ? '...' : refreshButtonContent}
                </button>
              </div>
              {renderChatHeaderSearchControls(true)}
              {renderChatHubSummary()}
            </>
          )}
        </div>
        {sessionSearchActive ? renderSessionSearchResults(true) : (
        <div className="mobile-project-session-nav">
          {projects.length === 0 ? (
            <div className="chat-empty-hint chat-empty-state">
              <span className="codicon codicon-inbox" aria-hidden="true" />
              <span>No projects available.</span>
            </div>
          ) : null}
          {sortedProjectItems.map(projectItem => {
            const targetProjectId = projectItem.projectId;
            const projectSessions = projectSessionsByProjectId[targetProjectId] ?? [];
            const visibleCount =
              wideProjectVisibleCounts[targetProjectId] ?? WIDE_PROJECT_SESSION_LIMIT;
            const visibleSessions = projectSessions.slice(0, visibleCount);
            const collapsed = collapsedProjectIds.includes(targetProjectId);
            const pinnedProject = pinnedProjectIds.includes(targetProjectId);
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
                      const displaySessionAgent = normalizeAgentTypeName(sessionAgent);
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
                            onContextMenu={event => openProjectSessionContextMenu(targetProjectId, session.sessionId, event)}
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
                              {resolveSessionDisplayTitle(session) || session.sessionId}
                            </span>
                            {displaySessionAgent ? (
                              <span className={`wide-session-agent-tag ${tagVariantClass('wide-session-agent', sessionAgent)}`}>
                                {displaySessionAgent}
                              </span>
                            ) : null}
                            <span className="wide-session-time" title={session.updatedAt || ''}>
                              {formatCompactRelativeAge(session.updatedAt)}
                            </span>
                          </button>
                          {renderProjectSessionActionMenu(targetProjectId, session)}
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
        )}
        {(() => {
          if (!mobileProjectActionMenu) return null;
          const sheetMenu = mobileProjectActionMenu;
          const sheetProject = sortedProjectItems.find(p => p.projectId === sheetMenu.projectId);
          if (!sheetProject) return null;
          const sheetProjectSessions = projectSessionsByProjectId[sheetMenu.projectId] ?? [];
          const sheetAgents = getWideProjectAgents(sheetProject, sheetProjectSessions);
          return (
            <>
              <div
                className="mobile-project-sheet-overlay"
                onClick={() => setMobileProjectActionMenu(null)}
                aria-hidden="true"
              />
              <div
                className="mobile-project-sheet"
                role="dialog"
                aria-modal="true"
                aria-label={sheetMenu.kind === 'new' ? 'New session' : 'Resume session'}
              >
                <div className="mobile-project-sheet-grip" aria-hidden="true" />
                <div className="mobile-project-sheet-header">
                  <span
                    className={`codicon ${sheetMenu.kind === 'new' ? 'codicon-add' : 'codicon-history'} mobile-project-sheet-icon`}
                    aria-hidden="true"
                  />
                  <span className="mobile-project-sheet-title-copy">
                    <span className="mobile-project-sheet-title">
                      {sheetMenu.kind === 'new' ? 'New Session' : 'Resume Session'}
                    </span>
                    <span className="mobile-project-sheet-subtitle">{sheetProject.name}</span>
                  </span>
                  <button
                    type="button"
                    className="mobile-project-sheet-close"
                    onClick={() => setMobileProjectActionMenu(null)}
                    aria-label="Close"
                    title="Close"
                  >
                    <span className="codicon codicon-close" />
                  </button>
                </div>
                <div className="mobile-project-sheet-body">
                  {sheetMenu.phase === 'agents' ? (
                    <>
                      {sheetAgents.map(agentType => (
                        <button
                          key={`${sheetMenu.projectId}:sheet:${sheetMenu.kind}:${agentType}`}
                          type="button"
                          className="wide-project-action-menu-item mobile-project-sheet-item"
                          onClick={() => {
                            if (sheetMenu.kind === 'new') {
                              handleMobileProjectCreateSession(
                                sheetMenu.projectId,
                                agentType,
                              ).catch(() => undefined);
                            } else {
                              handleMobileProjectResumeAgent(
                                sheetMenu.projectId,
                                agentType,
                              ).catch(() => undefined);
                            }
                          }}
                        >
                          <span className="codicon codicon-sparkle" />
                          <span>{agentType}</span>
                        </button>
                      ))}
                      {sheetAgents.length === 0 ? (
                        <div className="wide-project-action-empty">
                          <span className="codicon codicon-circle-slash" aria-hidden="true" />
                          <span>No agents available.</span>
                        </div>
                      ) : null}
                    </>
                  ) : (
                    <>
                      <button
                        type="button"
                        className="wide-project-action-back mobile-project-sheet-back"
                        onClick={() => {
                          setResumeSessions([]);
                          setResumeLoading(false);
                          setMobileProjectActionMenu({
                            ...sheetMenu,
                            phase: 'agents',
                            agentType: '',
                          });
                        }}
                      >
                        <span className="codicon codicon-arrow-left" />
                        <span>{sheetMenu.agentType}</span>
                      </button>
                      {resumeLoading ? (
                        <div className="wide-project-action-empty">
                          <span className="codicon codicon-loading codicon-modifier-spin" aria-hidden="true" />
                          <span>Loading sessions...</span>
                        </div>
                      ) : null}
                      {!resumeLoading
                        ? resumeSessions.map(session => (
                            <button
                              key={`${sheetMenu.projectId}:sheet-resume:${session.sessionId}`}
                              type="button"
                              className="wide-project-action-menu-item mobile-project-sheet-item"
                              onClick={() => {
                                handleMobileProjectResumeImport(
                                  sheetMenu.projectId,
                                  sheetMenu.agentType,
                                  session.sessionId,
                                ).catch(() => undefined);
                              }}
                            >
                              <span className="codicon codicon-history" />
                              <span>{resolveSessionDisplayTitle(session) || session.sessionId}</span>
                            </button>
                          ))
                        : null}
                      {!resumeLoading && resumeSessions.length === 0 ? (
                        <div className="wide-project-action-empty">
                          <span className="codicon codicon-history" aria-hidden="true" />
                          <span>No resumable sessions.</span>
                        </div>
                      ) : null}
                    </>
                  )}
                </div>
              </div>
            </>
          );
        })()}
      </>
    );
  };

  const renderWideProjectSessionNav = () => {
    return (
      <div className="wide-project-session-nav">
        {projects.length === 0 ? (
          <div className="chat-empty-hint">No projects available.</div>
        ) : null}
        {sessionSearchActive ? renderSessionSearchResults(false) : sortedProjectItems.map(projectItem => {
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
                      openWideProjectActionMenu(targetProjectId, 'new', event.currentTarget);
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
                      openWideProjectActionMenu(targetProjectId, 'resume', event.currentTarget);
                    }}
                  >
                    <span className="codicon codicon-history" />
                  </button>
                </div>
                {actionMenuOpen ? (
                  <div
                    ref={wideProjectActionMenuRef}
                    className="wide-project-action-popover"
                    style={wideProjectActionMenu.popover
                      ? {
                          top: `${wideProjectActionMenu.popover.top}px`,
                          left: `${wideProjectActionMenu.popover.left}px`,
                          width: `${wideProjectActionMenu.popover.width}px`,
                          maxHeight: `${wideProjectActionMenu.popover.maxHeight}px`,
                          transform: wideProjectActionMenu.popover.placement === 'above'
                            ? 'translateY(-100%)'
                            : undefined,
                        }
                      : undefined}
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
                            <span className="codicon codicon-circle-slash" aria-hidden="true" />
                            <span>No agents available.</span>
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
                            <span className="codicon codicon-loading codicon-modifier-spin" aria-hidden="true" />
                            <span>Loading sessions...</span>
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
                                <span>{resolveSessionDisplayTitle(session) || session.sessionId}</span>
                              </button>
                            ))
                          : null}
                        {!resumeLoading && resumeSessions.length === 0 ? (
                          <div className="wide-project-action-empty">
                            <span className="codicon codicon-history" aria-hidden="true" />
                            <span>No resumable sessions.</span>
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
                    const displaySessionAgent = normalizeAgentTypeName(sessionAgent);
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
                          onContextMenu={event => openProjectSessionContextMenu(targetProjectId, session.sessionId, event)}
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
                            {resolveSessionDisplayTitle(session) || session.sessionId}
                          </span>
                          {displaySessionAgent ? (
                            <span className={`wide-session-agent-tag ${tagVariantClass('wide-session-agent', sessionAgent)}`}>
                              {displaySessionAgent}
                            </span>
                          ) : null}
                          <span className="wide-session-time" title={session.updatedAt || ''}>
                            {formatCompactRelativeAge(session.updatedAt)}
                          </span>
                        </button>
                        {renderProjectSessionActionMenu(targetProjectId, session)}
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
    const chatSidebarTitleSearchOpen = tab === 'chat' && !sidebarSettingsOpen && sessionSearchHeaderExpanded;
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
                setSettingsDetailView(null);
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
          <div className={`sidebar-title-row${chatSidebarTitleSearchOpen ? ' search-open' : ''}`}>
            {chatSidebarTitleSearchOpen ? renderChatHeaderSearchControls(false) : (
              <>
                <span className="sidebar-title-text">{wideSidebarTitle}</span>
                {tab === 'chat' && !sidebarSettingsOpen ? (
                  <>
                    {renderChatHubSummary()}
                    {renderChatHeaderSearchControls(false)}
                  </>
                ) : null}
              </>
            )}
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
      {selectedFileIsHtml ? (
        <button
          type="button"
          className={`view-tool html-preview-toggle ${
            htmlPreviewEnabled ? 'active' : ''
          }`}
          onClick={() => setHtmlPreviewEnabled(value => !value)}
          title={
            htmlPreviewEnabled
              ? 'Switch to source mode'
              : 'Switch to HTML preview'
          }
          aria-label="Toggle HTML preview"
        >
          <span className="html-preview-toggle-text">HTML</span>
        </button>
      ) : null}
      {selectedFileIsHtml && htmlPreviewEnabled ? (
        <button
          type="button"
          className={`view-tool html-script-toggle ${
            htmlPreviewScriptsEnabled ? 'active' : ''
          }`}
          onClick={() => setHtmlPreviewScriptsEnabled(value => !value)}
          title={
            htmlPreviewScriptsEnabled
              ? 'Disable HTML scripts'
              : 'Enable HTML scripts'
          }
          aria-label="Toggle HTML scripts"
        >
          <span className="codicon codicon-run-all view-tool-icon" />
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
  const renderChatInlineCode = useCallback(
    ({ className, children }: { className?: string; children?: React.ReactNode }) => {
      const languageMatch = /language-([\w-]+)/.exec(className || '');
      const codeText = String(children ?? '').replace(/\n$/, '');
      const relayLocalUrl = parsePortRelayLocalHttpUrl(codeText);
      if (!languageMatch && !codeText.includes('\n') && relayLocalUrl) {
        return (
          <a
            className="chat-relay-link chat-relay-code-link"
            href={codeText.trim()}
            title="Open through Port Relay"
            onClick={event => {
              event.preventDefault();
              openChatPortRelayLink(relayLocalUrl).catch(() => undefined);
            }}
          >
            <code className={className}>{children}</code>
          </a>
        );
      }

      return markdownCodeRenderer({
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
      });
    },
    [
      themeMode,
      codeTheme,
      codeFont,
      codeFontSize,
      codeLineHeight,
      codeTabSize,
      openChatPortRelayLink,
    ],
  );

  const chatMarkdownComponents = useMemo<Components>(
    () => ({
      pre: markdownPreRenderer,
      code: renderChatInlineCode,
      img: ({ src, alt, ...rest }) => (
        <img
          {...rest}
          src={typeof src === 'string' ? src : undefined}
          alt={alt || ''}
          crossOrigin="anonymous"
        />
      ),
      a: ({ href, children, ...rest }) => {
        const linkHref = typeof href === 'string' ? href : '';
        const targetFile = linkHref ? resolveChatFileLink(linkHref) : null;
        const relayLocalUrl = parsePortRelayLocalHttpUrl(linkHref);
        const isFileLink = !!targetFile;
        const isWindowsLocalPath = /^\/?[a-zA-Z]:/.test(linkHref.trim());
        const linkText = collectReactText(children);
        const textLine = parseTrailingLineNumber(linkText);
        const jumpLine = targetFile?.line ?? textLine;
        const fallbackHref = linkHref || '#';

        return (
          <a
            {...rest}
            className={[rest.className, relayLocalUrl ? 'chat-relay-link' : ''].filter(Boolean).join(' ') || undefined}
            href={fallbackHref}
            target={isFileLink || relayLocalUrl ? undefined : '_blank'}
            rel={isFileLink || relayLocalUrl ? undefined : 'noreferrer'}
            title={
              isFileLink && jumpLine
                ? `${targetFile.path}:${jumpLine}`
                : relayLocalUrl
                  ? 'Open through Port Relay'
                : rest.title
            }
            onClick={event => {
              if (relayLocalUrl) {
                event.preventDefault();
                openChatPortRelayLink(relayLocalUrl).catch(() => undefined);
                return;
              }
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
      currentProject?.path,
      openChatPortRelayLink,
      renderChatInlineCode,
    ],
  );

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

  const exportPromptDoneMarkdownImage = async (doneTurnIndex: number) => {
    const result = buildPromptDoneCopyRange(selectedFullChatMessages, doneTurnIndex);
    if (!result.ok) {
      return;
    }
    markdownImageExportIdRef.current += 1;
    setError('');
    setMarkdownImageExportRequest({
      id: markdownImageExportIdRef.current,
      content: result.markdown,
      fileName: buildPromptMarkdownImageFileName(doneTurnIndex),
    });
  };

  const completeMarkdownImageExport = useCallback(() => {
    setMarkdownImageExportRequest(null);
  }, []);

  const failMarkdownImageExport = useCallback((message: string) => {
    setMarkdownImageExportRequest(null);
    setError(`Failed to export response image: ${message}`);
  }, []);

  const selectedChatHasOpenPromptTurn = selectedFullChatMessages.some(message =>
    isPromptStartMessage(message) &&
    resolvePromptTurnStatus(selectedFullChatMessages, message) === 'responding',
  );
  const selectedChatPromptRunning =
    !!selectedChatEncodedKey &&
    !selectedPendingPrompt &&
    (
      selectedChatSession?.running === true ||
      selectedChatHasOpenPromptTurn
    );
  const selectedChatPromptCancelling =
    !!selectedChatEncodedKey && chatCancellingRuntimeKey === selectedChatEncodedKey;

  const latestSelectableAssistantReply = (() => {
    if (selectedPendingPrompt) {
      return {
        messageKey: '',
        hasOptionReplies: false,
        confirmationReply: null as ChatConfirmationReply | null,
      };
    }
    const ordered = [...selectedFullChatMessages].sort((left, right) => (left.turnIndex ?? 0) - (right.turnIndex ?? 0));
    const latestUserTurnIndex = Math.max(
      0,
      ...ordered
        .filter(message => isPromptStartMessage(message))
        .map(message => Math.max(0, Math.trunc(message.turnIndex ?? 0))),
    );
    const latestAssistantMessage = [...ordered]
      .reverse()
      .find(message =>
        message.method === 'agent_message_chunk' &&
        Math.max(0, Math.trunc(message.turnIndex ?? 0)) > latestUserTurnIndex,
      );
    if (!latestAssistantMessage) {
      return {
        messageKey: '',
        hasOptionReplies: false,
        confirmationReply: null as ChatConfirmationReply | null,
      };
    }
    const text = msgText(latestAssistantMessage.method, latestAssistantMessage.param).trim();
    const latestOptionReplies = extractChatOptionReplies(text);
    return {
      messageKey: chatMessageDomKey(latestAssistantMessage),
      hasOptionReplies: latestOptionReplies.length > 0,
      confirmationReply: latestOptionReplies.length === 0 ? extractChatConfirmationReply(text) : null,
    };
  })();
  const latestSelectableOptionReplyMessageKey = latestSelectableAssistantReply.hasOptionReplies
    ? latestSelectableAssistantReply.messageKey
    : '';

  const renderChatMessageTurn = (message: RegistryChatMessage) => {
    const doneTurnIndex = message.turnIndex ?? 0;
    const copyRange = message.method === 'prompt_done'
      ? buildPromptDoneCopyRange(selectedFullChatMessages, doneTurnIndex)
      : null;
    const promptStatus = isPromptStartMessage(message)
      ? resolvePromptTurnStatus(selectedFullChatMessages, message)
      : null;
    const text = msgText(message.method, message.param).trim();
    const optionReplies =
      message.method === 'agent_message_chunk' &&
      chatMessageDomKey(message) === latestSelectableOptionReplyMessageKey
        ? extractChatOptionReplies(text)
        : [];
    const confirmationReply =
      message.method === 'agent_message_chunk' &&
      chatMessageDomKey(message) === latestSelectableAssistantReply.messageKey &&
      optionReplies.length === 0
        ? latestSelectableAssistantReply.confirmationReply
        : null;
    if (!shouldRenderChatTurn(message, hideToolCalls, promptStatus)) {
      return null;
    }
    const searchHighlighted =
      sessionSearchTargetTurn?.runtimeKey === selectedChatEncodedKey &&
      sessionSearchTargetTurn.turnIndex === (message.turnIndex ?? 0);
    return (
      <div
        key={`${selectedChatEncodedKey}:${message.turnIndex}:${message.method}`}
        data-chat-message-key={chatMessageDomKey(message)}
        className={searchHighlighted ? 'chat-turn-search-highlight' : undefined}
      >
        <ChatTurnView
          message={message}
          promptRequest={message.method === 'prompt_done' ? findPromptRequestForDone(doneTurnIndex) : undefined}
          promptStatus={promptStatus}
          hideToolCalls={hideToolCalls}
          markdownComponents={chatMarkdownComponents}
          markdownUrlTransform={chatMarkdownUrlTransform}
          copyDisabled={copyRange ? !copyRange.ok : true}
          optionReplies={optionReplies}
          optionRepliesDisabled={chatSending}
          confirmationReply={confirmationReply}
          onSelectOptionReply={label => sendDirectChatText(label).catch(() => undefined)}
          onSelectConfirmationReply={replyText => sendDirectChatText(replyText).catch(() => undefined)}
          onCopyPromptDone={
            message.method === 'prompt_done'
              ? () => copyPromptDoneMarkdown(doneTurnIndex).catch(() => undefined)
              : undefined
          }
          onExportPromptDoneImage={
            message.method === 'prompt_done'
              ? () => exportPromptDoneMarkdownImage(doneTurnIndex).catch(() => undefined)
              : undefined
          }
        />
      </div>
    );
  };
  const renderChatVirtuosoItem = (displayItem: typeof chatDisplayIndex.items[number]) => {
    const sourceMessage = displayItem.kind === 'turn'
      ? chatMessages[displayItem.sourceIndex]
      : undefined;
    const content = displayItem.kind === 'pending' && selectedPendingPrompt ? (
      <ChatTurnView
        message={buildPendingPromptMessage(selectedPendingPrompt)}
        promptStatus={selectedPendingPrompt.status}
        hideToolCalls={hideToolCalls}
        markdownComponents={chatMarkdownComponents}
        markdownUrlTransform={chatMarkdownUrlTransform}
        onRetryPendingPrompt={() => retryPendingChatPrompt(selectedChatEncodedKey)}
        onEditPendingPrompt={() => editPendingChatPrompt(selectedChatEncodedKey)}
      />
    ) : sourceMessage ? (
      renderChatMessageTurn(sourceMessage)
    ) : null;
    return content;
  };
  const renderPortRelayFrameSurface = (mode: 'desktop' | 'mobile') => (
    <div className={`port-relay-frame-surface ${mode}`}>
      <iframe
        title="Port Relay"
        src={portRelayFrameUrl}
        className="port-relay-frame"
        allow="clipboard-read; clipboard-write"
      />
    </div>
  );
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
    if (isWide && portRelayFrameOpen && portRelayFrameUrl) {
      return renderPortRelayFrameSurface('desktop');
    }
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
              setChatFileMentionMenuOpen(false);
              setChatConfigOverflowOpen(false);
              setChatConfigMenuOptionId(current => (current === option.id ? '' : option.id));
            }}
          >
            <span
              className={`codicon ${updating ? 'codicon-loading codicon-modifier-spin' : chatConfigIconClass(option)}`}
              aria-hidden="true"
            />
            <span className="chat-config-pill-value">{currentLabel}</span>
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
              <span className="title-text">
                CHAT - {selectedChatDisplayTitle || 'New Session'}
              </span>
            ) : (
              renderBreadcrumbTitle(chatBreadcrumbProjectName, chatBreadcrumbLabel)
            )}
          </div>
          <div
            className="chat-main"
            style={chatMainStyle}
          >
            <div
              ref={chatScrollRef}
              className="scroll-panel chat-block"
              onScroll={handleChatScroll}
              onWheel={event => { if (event.deltaY < 0) { markChatUserScrollIntent(); } }}
              onPointerDown={() => { chatPointerScrollingRef.current = true; }}
              onPointerUp={() => { chatPointerScrollingRef.current = false; }}
              onPointerCancel={() => { chatPointerScrollingRef.current = false; }}
              onTouchStart={() => { chatPointerScrollingRef.current = true; }}
              onTouchEnd={() => { chatPointerScrollingRef.current = false; }}
              onTouchCancel={() => { chatPointerScrollingRef.current = false; }}
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
              {chatDisplayIndex.items.length > 0 ? (
                <ChatVirtuosoTurnList
                  ref={chatVirtuosoListRef}
                  scrollRef={chatScrollRef}
                  displayIndex={chatDisplayIndex}
                  runtimeKey={selectedChatEncodedKey}
                  atBottomThreshold={CHAT_AUTO_SCROLL_BOTTOM_THRESHOLD}
                  onAtBottomChange={handleChatAtBottomChange}
                  shouldAutoscroll={shouldAutoscrollChat}
                  renderItem={renderChatVirtuosoItem}
                />
              ) : null}
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
              multiple
              style={{ display: 'none' }}
              onChange={handleChatFileChange}
            />
            <div
              className={`chat-composer-frame${chatComposerDragActive ? ' drag-over' : ''}`}
              onDragOver={event => {
                if (chatSending) {
                  return;
                }
                if (event.dataTransfer.types.includes('Files')) {
                  event.preventDefault();
                  setChatComposerDragActive(true);
                }
              }}
              onDragLeave={event => {
                if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
                  setChatComposerDragActive(false);
                }
              }}
              onDrop={event => {
                if (chatSending) {
                  setChatComposerDragActive(false);
                  return;
                }
                const files = chatFilesFromFileList(event.dataTransfer.files);
                if (files.length === 0) {
                  setChatComposerDragActive(false);
                  return;
                }
                event.preventDefault();
                setChatComposerDragActive(false);
                const attachmentDraftKey = currentChatDraftKeyRef.current;
                const attachmentDraftGeneration = getChatDraftGeneration(attachmentDraftKey);
                enqueueChatAttachmentFiles(files, attachmentDraftKey, attachmentDraftGeneration);
              }}
            >
              {chatAttachments.length > 0 ? (
                <div className="chat-attachment-preview-list">
                  {chatAttachments.map(attachment => {
                    const previewSrc = chatAttachmentPreviewSrc(attachment);
                    const pending = isChatAttachmentUploadPending(attachment);
                    return (
                      <div key={attachment.id} className={`chat-attachment-preview ${attachment.status}`}>
                        {previewSrc ? (
                          <img
                            className="chat-attachment-thumb"
                            src={previewSrc}
                            alt={attachment.name || 'attachment preview'}
                          />
                        ) : (
                          <div className="chat-attachment-thumb file" aria-hidden="true">
                            <span className="codicon codicon-file" />
                          </div>
                        )}
                        <div className="chat-attachment-meta">
                          <div className="chat-attachment-name">{attachment.name}</div>
                          <div className="chat-attachment-status">
                            {attachment.status === 'failed'
                              ? (attachment.error || 'Upload failed')
                              : attachment.status === 'completed'
                                ? formatChatAttachmentSize(attachment.size)
                                : attachment.status === 'queued'
                                  ? 'Ready'
                                  : `${attachment.progress}%`}
                          </div>
                          {pending ? (
                            <div className="chat-attachment-progress" aria-hidden="true">
                              <span style={{width: `${Math.max(4, attachment.progress)}%`}} />
                            </div>
                          ) : null}
                        </div>
                        {attachment.status === 'failed' ? (
                          <button
                            type="button"
                            className="chat-attachment-retry"
                            onClick={() => retryChatAttachment(attachment.id)}
                            title="Queue retry"
                            aria-label="Queue retry"
                          >
                            <span className="codicon codicon-refresh" />
                          </button>
                        ) : null}
                        <button
                          type="button"
                          className="chat-attachment-remove"
                          onClick={() => removeChatAttachment(attachment.id)}
                          disabled={pending}
                          title={pending ? 'Uploading' : 'Remove attachment'}
                          aria-label={pending ? 'Uploading' : 'Remove attachment'}
                        >
                          <span className="codicon codicon-close" />
                        </button>
                      </div>
                    );
                  })}
                </div>
              ) : null}
              <div className="chat-composer-input-row">
                <button
                  type="button"
                  className={`chat-composer-stop-trigger${selectedChatPromptRunning ? ' active' : ''}`}
                  onPointerDown={event => event.preventDefault()}
                  onClick={() => cancelSelectedChatPrompt().catch(() => undefined)}
                  disabled={!selectedChatPromptRunning || selectedChatPromptCancelling}
                  title={selectedChatPromptRunning ? 'Cancel prompt' : 'No prompt running'}
                  aria-label="Cancel prompt"
                >
                  <span
                    className={`codicon ${selectedChatPromptCancelling ? 'codicon-loading codicon-modifier-spin' : 'codicon-debug-stop'}`}
                    aria-hidden="true"
                  />
                </button>
                <div className="chat-composer-input-shell">
                  <textarea
                    ref={chatComposerTextareaRef}
                    rows={1}
                    className="chat-composer-input"
                    value={chatComposerText}
                    readOnly={chatSending}
                    onChange={event => updateChatComposerText(event.target.value)}
                    onPaste={event => {
                      if (chatSending) {
                        return;
                      }
                      if (!supportsChatClipboardFiles) {
                        return;
                      }
                      const attachmentDraftKey = currentChatDraftKeyRef.current;
                      const attachmentDraftGeneration = getChatDraftGeneration(attachmentDraftKey);
                      const files = chatFilesFromDataTransferItems(event.clipboardData?.items);
                      if (files.length === 0) {
                        return;
                      }
                      event.preventDefault();
                      enqueueChatAttachmentFiles(files, attachmentDraftKey, attachmentDraftGeneration);
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
                      if (chatSending || chatAttachmentUploadPending) {
                        return;
                      }
                      sendChatMessage().catch(() => undefined);
                    }}
                    placeholder="Send a message..."
                  />
                </div>
                <div className="chat-send-control">
                  <button
                    type="button"
                    className="chat-send-button"
                    onClick={() => sendChatMessage().catch(() => undefined)}
                    disabled={chatSending || chatAttachmentUploadPending}
                    title="Send"
                    aria-label="Send message"
                  >
                    <span className="codicon codicon-send" />
                  </button>
                </div>
              </div>
              {chatFileMentionMenuOpen ? (
                <div ref={chatFileMentionMenuRef} className="chat-file-mention-menu" role="menu" aria-label="File mentions">
                  <div className="chat-file-mention-empty">File mentions coming soon</div>
                </div>
              ) : null}
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
                    ref={chatPromptButtonRef}
                    className="chat-tool-button chat-slash-button"
                    onPointerDown={event => event.preventDefault()}
                    onClick={openChatPromptMenu}
                    title="Skills"
                    aria-label="Open skills"
                    aria-haspopup="listbox"
                    aria-expanded={chatPromptMenuOpen}
                  >
                    <span className="chat-slash-symbol">/</span>
                  </button>
                  <button
                    type="button"
                    ref={chatFileMentionButtonRef}
                    className="chat-tool-button chat-mention-button"
                    onPointerDown={event => event.preventDefault()}
                    onClick={openChatFileMentionMenu}
                    title="Mention files"
                    aria-label="Mention files"
                    aria-haspopup="menu"
                    aria-expanded={chatFileMentionMenuOpen}
                  >
                    <span className="chat-mention-symbol">@</span>
                  </button>
                  <button
                    type="button"
                    className="chat-tool-button chat-attach-button"
                    onClick={() => {
                      setChatPromptMenuOpen(false);
                      setChatFileMentionMenuOpen(false);
                      setChatConfigMenuOptionId('');
                      setChatConfigOverflowOpen(false);
                      chatFileInputRef.current?.click();
                    }}
                    disabled={chatSending}
                    title="Attach file"
                    aria-label="Attach file"
                  >
                    <span className="codicon codicon-new-file" />
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
                              setChatFileMentionMenuOpen(false);
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
              renderBreadcrumbTitle(breadcrumbProjectName, fileBreadcrumbLabel)
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
                  ) : selectedFileIsHtml && htmlPreviewEnabled ? (
                    <HtmlPreview
                      key={`${selectedFile}:${
                        htmlPreviewScriptsEnabled ? 'scripts' : 'static'
                      }`}
                      content={fileContent}
                      scriptsEnabled={htmlPreviewScriptsEnabled}
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
            renderBreadcrumbTitle(breadcrumbProjectName, gitBreadcrumbLabel)
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
        <DesktopTitleBar title="WheelMaker" />
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
            syncWorkspaceProject(projectItem.projectId, {reason: 'manual'}).catch(() => undefined)
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
  const isShortcutSettingsDetailActive = sidebarSettingsOpen && (
    settingsDetailView === 'update' ||
    settingsDetailView === 'skills' ||
    settingsDetailView === 'tokenStats' ||
    settingsDetailView === 'portRelay'
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
        <button
          type="button"
          className={`desktop-activity-button${sidebarSettingsOpen && settingsDetailView === 'portRelay' ? ' active' : ''}`}
          onClick={handleDesktopPortRelaySelect}
          title="Port Relay"
          aria-label="Port Relay"
        >
          <span className="codicon codicon-radio-tower" />
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
          className={`desktop-activity-button${sidebarSettingsOpen && settingsDetailView === 'update' ? ' active' : ''}`}
          onClick={() => openSettingsDetail('update')}
          title="Update"
          aria-label="Update"
        >
          <span className="codicon codicon-cloud-download" />
        </button>
        <button
          type="button"
          className={`desktop-activity-button${sidebarSettingsOpen && settingsDetailView === 'skills' ? ' active' : ''}`}
          onClick={() => openSettingsDetail('skills')}
          title="Skills"
          aria-label="Skills"
        >
          <span className="codicon codicon-extensions" />
        </button>
        <button
          type="button"
          className={`desktop-activity-button${sidebarSettingsOpen && settingsDetailView === 'tokenStats' ? ' active' : ''}`}
          onClick={() => openSettingsDetail('tokenStats')}
          title="Token Stats"
          aria-label="Token Stats"
        >
          <span className="codicon codicon-graph-line" />
        </button>
        <button
          type="button"
          className={`desktop-activity-button${sidebarSettingsOpen && !isShortcutSettingsDetailActive ? ' active' : ''}`}
          onClick={handleDesktopSettingsSelect}
          title="Settings"
          aria-label="Settings"
        >
          <span className="codicon codicon-settings-gear" />
        </button>
      </div>
    </nav>
  ) : null;

  const portRelayTargetMenu = portRelayTargetMenuOpen && mobilePortRelayFrameOpen ? (
    <div
      ref={portRelayTargetMenuRef}
      className="port-relay-target-switch-menu"
      role="menu"
      aria-label="Port Relay targets"
      onPointerDown={event => event.stopPropagation()}
    >
      {portRelayTargetMenuTargets.map(target => {
        const selected = samePortRelayTarget(activePortRelayTarget, target);
        const switching = samePortRelayTarget(portRelayMenuSwitchingTarget, target);
        return (
          <button
            key={portRelayTargetKey(target)}
            type="button"
            className="port-relay-target-switch-item"
            data-selected={selected}
            data-loading={switching}
            role="menuitemradio"
            aria-checked={selected}
            onClick={() => handleMobilePortRelayTargetMenuSelect(target).catch(() => undefined)}
          >
            <span className="port-relay-target-switch-check" aria-hidden="true">
              {switching ? (
                <span className="codicon codicon-loading codicon-modifier-spin" />
              ) : selected ? (
                <span className="codicon codicon-check" />
              ) : null}
            </span>
            <span className="port-relay-target-switch-label">{`${target.hubId}:${target.targetPort}`}</span>
          </button>
        );
      })}
    </div>
  ) : null;

  const chatQuickSwitchMenu = chatQuickSwitchMenuOpen && !mobilePortRelayFrameOpen ? (
    <div
      ref={chatQuickSwitchMenuRef}
      className="chat-quick-switch-menu"
      style={chatQuickSwitchMenuStyle}
      role="menu"
      aria-label="Recent chats"
      onPointerDown={event => event.stopPropagation()}
    >
      {mobileChatQuickSwitchSections.length === 0 ? (
        <div className="chat-quick-switch-empty">No chats</div>
      ) : (
        mobileChatQuickSwitchSections.map(section => (
          <div key={`chat-quick-switch-project:${section.projectId}`} className="chat-quick-switch-project">
            <div className="chat-quick-switch-project-name" title={section.projectName}>
              {section.projectName}
            </div>
            <div className="chat-quick-switch-session-list">
              {section.sessions.map(session => {
                const selected = selectedChatEncodedKey === buildChatRuntimeKey(section.projectId, session.sessionId);
                const unreadCount = session.unreadCount ?? 0;
                return (
                  <button
                    key={`chat-quick-switch-session:${section.projectId}:${session.sessionId}`}
                    type="button"
                    className="chat-quick-switch-item"
                    data-selected={selected}
                    role="menuitem"
                    onClick={() => handleMobileChatQuickSwitchSelect(section.projectId, session).catch(() => undefined)}
                  >
                    {renderSessionStateMarker(session, section.projectId)}
                    <span className="chat-quick-switch-title">
                      {resolveSessionDisplayTitle(session) || session.sessionId}
                    </span>
                    <span className="chat-quick-switch-time" title={session.updatedAt || ''}>
                      {formatCompactRelativeAge(session.updatedAt)}
                    </span>
                    {unreadCount > 0 ? (
                      <span className="chat-quick-switch-unread">{Math.min(99, unreadCount)}</span>
                    ) : null}
                    {selected ? (
                      <span className="chat-quick-switch-selected codicon codicon-check" aria-label="Current chat" />
                    ) : null}
                  </button>
                );
              })}
            </div>
          </div>
        ))
      )}
    </div>
  ) : null;

  const floatingControlStack = !isWide ? (
    <div className="floating-control-stack-layer">
      <div
        ref={floatingControlStackRef}
        className="floating-control-stack"
        data-drag-state={floatingDragVisualState}
        style={effectiveFloatingControlStackStyle}
        onPointerDown={gestureNavigation ? undefined : beginFloatingPress}
        onPointerMove={gestureNavigation ? handleGestureNavigationPointerMove : handleFloatingPointerMove}
        onPointerUp={event => {
          floatingIgnoreLostCaptureRef.current = true;
          if (gestureNavigation) {
            if (floatingDragStateRef.current?.pointerId === event.pointerId) {
              finishFloatingDrag(event.pointerId);
              return;
            }
            finishGestureNavigation(event.pointerId);
            return;
          }
          finishFloatingDrag(event.pointerId);
        }}
        onPointerCancel={event => {
          floatingIgnoreLostCaptureRef.current = true;
          if (gestureNavigation) {
            if (floatingDragStateRef.current?.pointerId === event.pointerId) {
              cancelFloatingDrag(event.pointerId);
              return;
            }
            cancelGestureNavigation(event.pointerId);
            return;
          }
          cancelFloatingDrag(event.pointerId);
        }}
        onLostPointerCapture={event => {
          if (floatingIgnoreLostCaptureRef.current) {
            floatingIgnoreLostCaptureRef.current = false;
            return;
          }
          if (gestureNavigation) {
            if (floatingDragStateRef.current?.pointerId === event.pointerId) {
              cancelFloatingDrag(event.pointerId);
              return;
            }
            cancelGestureNavigation(event.pointerId);
            return;
          }
          cancelFloatingDrag(event.pointerId);
        }}
      >
        {portRelayReady && portRelayFrameUrl ? (
          <button
            type="button"
            className="drawer-toggle-bubble port-relay-floating-bubble"
            data-active={portRelayFrameOpen}
            onPointerDown={handlePortRelayFloatingPointerDown}
            onPointerMove={handlePortRelayFloatingPointerMove}
            onPointerUp={finishPortRelayFloatingPress}
            onPointerCancel={finishPortRelayFloatingPress}
            onContextMenu={event => event.preventDefault()}
            onClick={handlePortRelayFloatingToggle}
            title={portRelayFrameOpen ? 'Close relay page' : 'Open relay page'}
            aria-label={portRelayFrameOpen ? 'Close relay page' : 'Open relay page'}
            aria-pressed={portRelayFrameOpen}
          >
            <span className="codicon codicon-radio-tower" />
          </button>
        ) : null}
        {portRelayTargetMenu}
        {chatQuickSwitchMenu}
        {mobilePortRelayFrameOpen ? null : gestureNavigation ? (
          <div
            className="gesture-nav-control"
            data-expanded={gestureNavigationExpanded}
            data-candidate={gestureNavState?.candidate ?? ''}
            aria-label="Gesture navigation"
          >
            {gestureNavigationExpanded ? (
              <>
                <button
                  type="button"
                  className="gesture-nav-button gesture-nav-option gesture-nav-option-chat"
                  data-gesture-nav-tab="chat"
                  data-active={tab === 'chat'}
                  data-candidate={gestureNavState?.candidate === 'chat'}
                  onClick={event => handleGestureNavigationOptionClick('chat', event)}
                  title="Chat"
                  aria-label="Chat"
                >
                  <span className="codicon codicon-comment-discussion" />
                </button>
                <button
                  type="button"
                  className="gesture-nav-button gesture-nav-option gesture-nav-option-file"
                  data-gesture-nav-tab="file"
                  data-active={tab === 'file'}
                  data-candidate={gestureNavState?.candidate === 'file'}
                  onClick={event => handleGestureNavigationOptionClick('file', event)}
                  title="File"
                  aria-label="File"
                >
                  <span className="codicon codicon-files" />
                </button>
                <button
                  type="button"
                  className="gesture-nav-button gesture-nav-option gesture-nav-option-git"
                  data-gesture-nav-tab="git"
                  data-active={tab === 'git'}
                  data-candidate={gestureNavState?.candidate === 'git'}
                  onClick={event => handleGestureNavigationOptionClick('git', event)}
                  title="Git"
                  aria-label="Git"
                >
                  <span className="codicon codicon-source-control" />
                </button>
              </>
            ) : null}
            <button
              type="button"
              className="gesture-nav-button gesture-nav-drawer-button"
              onPointerDown={handleGestureNavigationButtonPointerDown}
              onClick={handleFloatingDrawerToggle}
              title="Toggle drawer"
              aria-label="Toggle drawer"
              aria-expanded={drawerOpen}
            >
              <span className="codicon codicon-menu" />
              {!gestureNavigationExpanded ? (
                <span className="gesture-nav-badge" aria-hidden="true">
                  <span className={`codicon ${tabIconClass(tab)}`} />
                </span>
              ) : null}
            </button>
          </div>
        ) : (
          <>
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
                onPointerDown={handleChatQuickSwitchPointerDown}
                onPointerMove={handleChatQuickSwitchPointerMove}
                onPointerUp={finishChatQuickSwitchPress}
                onPointerCancel={finishChatQuickSwitchPress}
                onContextMenu={event => event.preventDefault()}
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
          </>
        )}
      </div>
    </div>
  ) : null;

  const mobileSettingsTitle = settingsDetailView
    ? settingsDetailTitle(settingsDetailView)
    : 'Settings';
  const mobileSettingsActions = settingsDetailView
    ? renderSettingsDetailActions(settingsDetailView)
    : <span className="mobile-settings-action-spacer" aria-hidden="true" />;

  const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (
    <div
      className="mobile-settings-screen"
      role="dialog"
      aria-modal="true"
      aria-label={mobileSettingsTitle}
    >
      <div className="mobile-settings-nav">
        <button
          type="button"
          className="mobile-settings-back"
          onClick={handleMobileSettingsBackButton}
          aria-label={settingsDetailView ? 'Back to settings' : 'Back to drawer'}
          title="Back"
        >
          <span className="codicon codicon-arrow-left" />
        </button>
        <div className="mobile-settings-title">{mobileSettingsTitle}</div>
        <div className="mobile-settings-actions">{mobileSettingsActions}</div>
      </div>
      <div className="mobile-settings-scroll">
        <div className="mobile-settings-group">
          {renderSettingsContent(false, { hideDetailHeader: true })}
        </div>
      </div>
    </div>
  ) : null;
  const portRelayMobileFrameOverlay = mobilePortRelayFrameOpen
    ? renderPortRelayFrameSurface('mobile')
    : null;

  const archiveTarget = confirmTarget?.kind === 'archive' ? confirmTarget : null;
  const deleteTarget = confirmTarget?.kind === 'delete' ? confirmTarget : null;
  const npmPackageTarget = confirmTarget?.kind === 'npmPackage' ? confirmTarget : null;
  const npmPackageHubUpdateTarget = confirmTarget?.kind === 'npmPackageHubUpdate' ? confirmTarget : null;
  const wheelMakerUpdateTarget = confirmTarget?.kind === 'wheelMakerUpdate' ? confirmTarget : null;
  const wheelMakerUpdateAllTarget = confirmTarget?.kind === 'wheelMakerUpdateAll' ? confirmTarget : null;
  const skillInstallConfirmTarget = confirmTarget?.kind === 'skillInstall' ? confirmTarget : null;
  const skillUninstallConfirmTarget = confirmTarget?.kind === 'skillUninstall' ? confirmTarget : null;
  const skillUpdateConfirmTarget = confirmTarget?.kind === 'skillUpdate' ? confirmTarget : null;
  const skillConfirmTarget = skillInstallConfirmTarget ?? skillUninstallConfirmTarget ?? skillUpdateConfirmTarget;
  const npmPackageConfirmPendingKey = npmPackageTarget
    ? agentPackageActionKey(npmPackageTarget.hubId, npmPackageTarget.packageName)
    : '';
  const skillConfirmPendingKey = skillConfirmTarget
    ? skillActionPendingKey({
        hubId: skillConfirmTarget.hubId,
        scope: skillConfirmTarget.scope,
        projectName: skillConfirmTarget.projectName,
        skillName: skillUninstallConfirmTarget?.skillName,
        action: skillConfirmTarget.kind,
      })
    : '';
  const confirmBusy = archiveTarget
    ? chatArchivingSessionId === archiveTarget.sessionId
    : deleteTarget
      ? chatDeletingSessionId === deleteTarget.sessionId
      : npmPackageTarget
        ? agentPackageActionPendingKey === npmPackageConfirmPendingKey
        : npmPackageHubUpdateTarget
          ? agentPackageHubUpdatePendingId === npmPackageHubUpdateTarget.hubId
          : wheelMakerUpdateTarget
            ? wheelMakerUpdatePendingHubId === wheelMakerUpdateTarget.hubId
            : wheelMakerUpdateAllTarget
              ? wheelMakerUpdateAllPending
              : skillConfirmTarget
                ? skillsPendingKey === skillConfirmPendingKey
                : false;
  const confirmTitle = confirmTarget?.kind === 'clearCache'
    ? 'Clear local cache?'
    : deleteTarget
      ? 'Delete session?'
      : npmPackageTarget
        ? `${agentPackageActionLabel(npmPackageTarget.action)} package?`
        : npmPackageHubUpdateTarget
          ? 'Update npm packages?'
          : wheelMakerUpdateTarget
            ? 'Update and publish WheelMaker?'
            : wheelMakerUpdateAllTarget
              ? 'Update all hubs?'
              : skillInstallConfirmTarget
                ? 'Install skills?'
                : skillUninstallConfirmTarget
                  ? 'Uninstall skill?'
                  : skillUpdateConfirmTarget
                    ? 'Update skills?'
                    : 'Archive session?';
  const confirmName = confirmTarget?.kind === 'clearCache'
    ? 'Token and server address will be preserved.'
    : deleteTarget
      ? deleteTarget.title || 'Untitled session'
      : npmPackageTarget
        ? npmPackageTarget.displayName || npmPackageTarget.packageName
        : npmPackageHubUpdateTarget
          ? `${npmPackageHubUpdateTarget.hubId} - ${npmPackageUpdateSummary(npmPackageHubUpdateTarget.packages.length)}`
          : wheelMakerUpdateTarget
            ? `Hub: ${wheelMakerUpdateTarget.hubId}`
            : wheelMakerUpdateAllTarget
              ? `${wheelMakerUpdateAllTarget.hubIds.length} hubs`
              : skillInstallConfirmTarget
                ? skillScopeLabel(skillInstallConfirmTarget)
                : skillUninstallConfirmTarget
                  ? skillUninstallConfirmTarget.skillName
                  : skillUpdateConfirmTarget
                    ? skillScopeLabel(skillUpdateConfirmTarget)
                    : archiveTarget?.title || 'Untitled session';
  const confirmCopy = confirmTarget?.kind === 'clearCache'
    ? 'The app will reload after local cached workspace data is cleared.'
    : deleteTarget
      ? 'This permanently deletes the session data from the Hub.'
      : npmPackageTarget
        ? `Hub: ${npmPackageTarget.hubId}. Package: ${npmPackageTarget.packageName}. Installed: ${npmPackageTarget.installedVersion || '-'}. Target: ${npmPackageTarget.action === 'uninstall' ? 'remove deprecated package' : npmPackageTarget.latestVersion || 'latest'}. Restart WheelMaker or start a new agent session for changes to take effect.`
        : npmPackageHubUpdateTarget
          ? `Runs latest install/update for ${npmPackageHubUpdateTarget.packages.map(pkg => pkg.displayName || pkg.packageName).join(', ')}. Restart WheelMaker or start a new agent session for changes to take effect.`
          : wheelMakerUpdateTarget
            ? `Current: ${shortGitSha(wheelMakerUpdateTarget.currentSha)}. Latest: ${shortGitSha(wheelMakerUpdateTarget.latestSha)}. ${wheelMakerUpdateTarget.behindCount > 0 ? `${wheelMakerUpdateTarget.behindCount} commits behind. ` : ''}This writes a full-update signal; updater will pull, build, publish Web, and restart Hub/Monitor. Updater itself is not restarted.`
            : wheelMakerUpdateAllTarget
              ? `This sends update-publish to ${wheelMakerUpdateAllTarget.hubIds.length} hubs. Each hub may pull, build, publish Web, and restart independently.`
              : skillInstallConfirmTarget
                ? `Source: ${skillInstallConfirmTarget.source}. Skills: ${skillInstallConfirmTarget.skills.join(', ')}.`
                : skillUninstallConfirmTarget
                  ? `Remove from ${skillScopeLabel(skillUninstallConfirmTarget)}.`
                  : skillUpdateConfirmTarget
                    ? skillUpdateConfirmTarget.includeProjects
                      ? 'Updates Hub Skills and online Project Skills on this Hub.'
                      : `Updates installed skills in ${skillScopeLabel(skillUpdateConfirmTarget)}.`
                    : 'Archived sessions leave the chat list.';
  const confirmIcon = confirmTarget?.kind === 'clearCache'
    ? 'codicon-trash'
    : deleteTarget
      ? 'codicon-trash'
      : npmPackageTarget
        ? npmPackageTarget.action === 'uninstall' ? 'codicon-trash' : 'codicon-cloud-download'
        : npmPackageHubUpdateTarget
          ? 'codicon-cloud-download'
          : wheelMakerUpdateTarget
            ? 'codicon-cloud-download'
            : wheelMakerUpdateAllTarget
              ? 'codicon-cloud-download'
              : skillInstallConfirmTarget
                ? 'codicon-cloud-download'
                : skillUninstallConfirmTarget
                  ? 'codicon-trash'
                  : skillUpdateConfirmTarget
                    ? 'codicon-sync'
                    : 'codicon-archive';
  const confirmPrimaryLabel = confirmTarget?.kind === 'clearCache'
    ? 'Clear Cache'
    : deleteTarget
      ? 'Delete'
      : npmPackageTarget
        ? agentPackageActionLabel(npmPackageTarget.action)
        : npmPackageHubUpdateTarget
          ? 'Update'
          : wheelMakerUpdateTarget
            ? 'Update'
            : wheelMakerUpdateAllTarget
              ? 'Update'
              : skillInstallConfirmTarget
                ? 'Install'
                : skillUninstallConfirmTarget
                  ? 'Uninstall'
                  : skillUpdateConfirmTarget
                    ? 'Update'
                    : 'Archive';
  const confirmPrimaryClassName = confirmTarget?.kind === 'clearCache' || !!deleteTarget || npmPackageTarget?.action === 'uninstall' || !!skillUninstallConfirmTarget
    ? 'app-confirm-btn primary danger'
    : 'app-confirm-btn primary';
  const confirmIconClassName = confirmTarget?.kind === 'clearCache' || !!deleteTarget || npmPackageTarget?.action === 'uninstall' || !!skillUninstallConfirmTarget
    ? 'app-confirm-icon danger'
    : 'app-confirm-icon';
  const handleConfirmPrimary = () => {
    if (!confirmTarget) {
      return;
    }
    if (confirmTarget.kind === 'clearCache') {
      clearLocalCache();
      return;
    }
    if (confirmTarget.kind === 'delete') {
      handleDeleteProjectSession(
        confirmTarget.projectId,
        confirmTarget.sessionId,
      ).catch(() => undefined);
      return;
    }
    if (confirmTarget.kind === 'npmPackage') {
      handleAgentPackageConfirmedAction(confirmTarget).catch(() => undefined);
      return;
    }
    if (confirmTarget.kind === 'npmPackageHubUpdate') {
      handleAgentPackageHubUpdateConfirmedAction(confirmTarget).catch(() => undefined);
      return;
    }
    if (confirmTarget.kind === 'wheelMakerUpdate') {
      handleWheelMakerUpdateConfirmedAction(confirmTarget).catch(() => undefined);
      return;
    }
    if (confirmTarget.kind === 'wheelMakerUpdateAll') {
      handleWheelMakerUpdateAllConfirmedAction(confirmTarget).catch(() => undefined);
      return;
    }
    if (confirmTarget.kind === 'skillInstall' || confirmTarget.kind === 'skillUninstall' || confirmTarget.kind === 'skillUpdate') {
      handleSkillConfirmedAction(confirmTarget).catch(() => undefined);
      return;
    }
    handleArchiveProjectSession(
      confirmTarget.projectId,
      confirmTarget.sessionId,
    ).catch(() => undefined);
  };
  const appConfirmDialog = confirmTarget ? (
    <div
      className="app-confirm-backdrop"
      role="presentation"
      onPointerDown={() => {
        if (!confirmBusy) {
          setConfirmError('');
          setConfirmTarget(null);
        }
      }}
    >
      <div
        className="app-confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="app-confirm-title"
        onPointerDown={event => event.stopPropagation()}
      >
        <div className={confirmIconClassName}>
          <span className={`codicon ${confirmIcon}`} />
        </div>
        <div className="app-confirm-content">
          <div id="app-confirm-title" className="app-confirm-title">
            {confirmTitle}
          </div>
          <div className="app-confirm-name">{confirmName}</div>
          <div className="app-confirm-copy">{confirmCopy}</div>
          {confirmError ? (
            <div className="app-confirm-error">{confirmError}</div>
          ) : null}
        </div>
        <div className="app-confirm-actions">
          <button
            type="button"
            className="app-confirm-btn secondary"
            disabled={confirmBusy}
            onClick={() => {
              setConfirmError('');
              setConfirmTarget(null);
            }}
          >
            Cancel
          </button>
          <button
            type="button"
            className={confirmPrimaryClassName}
            disabled={confirmBusy}
            onClick={handleConfirmPrimary}
          >
            <span
              className={`codicon ${
                confirmBusy
                  ? 'codicon-loading codicon-modifier-spin'
                  : confirmIcon
              }`}
            />
            {confirmPrimaryLabel}
          </button>
        </div>
      </div>
    </div>
  ) : null;
  const renameBusy = !!renameTarget && chatRenamingSessionId === renameTarget.sessionId;
  const submitRenameTarget = () => {
    if (!renameTarget || renameBusy) {
      return;
    }
    handleRenameProjectSession(
      renameTarget.projectId,
      renameTarget.sessionId,
      renameTitleDraft,
    ).catch(() => undefined);
  };
  const appRenameDialog = renameTarget ? (
    <div
      className="app-confirm-backdrop"
      role="presentation"
      onPointerDown={() => {
        if (!renameBusy) {
          setRenameError('');
          setRenameTarget(null);
          setRenameTitleDraft('');
        }
      }}
    >
      <div
        className="app-confirm-dialog app-rename-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="app-rename-title"
        onPointerDown={event => event.stopPropagation()}
      >
        <div className="app-confirm-icon">
          <span className="codicon codicon-edit" />
        </div>
        <div className="app-confirm-content">
          <div id="app-rename-title" className="app-confirm-title">
            Rename session
          </div>
          <div className="app-confirm-name">{renameTarget.title || renameTarget.sessionId}</div>
          <input
            className="app-rename-input"
            type="text"
            value={renameTitleDraft}
            maxLength={200}
            autoFocus
            disabled={renameBusy}
            onChange={event => setRenameTitleDraft(event.target.value)}
            onKeyDown={event => {
              if (event.key === 'Enter') {
                event.preventDefault();
                submitRenameTarget();
              }
              if (event.key === 'Escape' && !renameBusy) {
                setRenameError('');
                setRenameTarget(null);
                setRenameTitleDraft('');
              }
            }}
          />
          <div className="app-confirm-copy">Saving an empty title restores the automatic first prompt title.</div>
          {renameError ? (
            <div className="app-confirm-error">{renameError}</div>
          ) : null}
        </div>
        <div className="app-confirm-actions">
          <button
            type="button"
            className="app-confirm-btn secondary"
            disabled={renameBusy}
            onClick={() => {
              setRenameError('');
              setRenameTarget(null);
              setRenameTitleDraft('');
            }}
          >
            Cancel
          </button>
          <button
            type="button"
            className="app-confirm-btn primary"
            disabled={renameBusy}
            onClick={submitRenameTarget}
          >
            <span
              className={`codicon ${
                renameBusy
                  ? 'codicon-loading codicon-modifier-spin'
                  : 'codicon-check'
              }`}
            />
            Save
          </button>
        </div>
      </div>
    </div>
  ) : null;
  const registryDebugPanel = isWide && registryDebug && registryDebugPanelOpen ? (
    <RegistryDebugPanel
      records={registryDebugRecords}
      selectedRecordId={selectedRegistryDebugRecordId}
      onSelectedRecordIdChange={setSelectedRegistryDebugRecordId}
      selectedScope={selectedRegistryDebugScope}
      onSelectedScopeChange={setSelectedRegistryDebugScope}
      selectedSessionId={selectedRegistryDebugSessionId}
      onSelectedSessionIdChange={setSelectedRegistryDebugSessionId}
      sessionLabels={registryDebugSessionLabels}
      includeMultiSessionRecords={registryDebugIncludeMultiSessionRecords}
      onIncludeMultiSessionRecordsChange={setRegistryDebugIncludeMultiSessionRecords}
      onClear={() => registryDebugStore.clear()}
      onClose={() => setRegistryDebugPanelOpen(false)}
    />
  ) : null;

  return (
    <>
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
        drawerOpen={mobilePortRelayFrameOpen ? false : drawerOpen}
        onCloseDrawer={() => setDrawerOpen(false)}
      />
      {portRelayMobileFrameOverlay}
      {registryDebugPanel}
      {markdownImageExportRequest ? (
        <MarkdownImageExportSurface
          key={markdownImageExportRequest.id}
          request={markdownImageExportRequest}
          markdownComponents={chatMarkdownComponents}
          markdownUrlTransform={chatMarkdownUrlTransform}
          onComplete={completeMarkdownImageExport}
          onError={failMarkdownImageExport}
        />
      ) : null}
      {appRenameDialog}
      {appConfirmDialog}
    </>
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

installMobileViewportZoomGuard(document);

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




















