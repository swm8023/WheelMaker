import type {RegistryChatMessage} from '../types/registry';
import type {ChatPromptStatus} from './chatPromptStatus';
import {
  splitChatConfirmationReplyText,
  splitChatOptionReplyText,
} from './chatOptionReplies';

export type ChatDisplayIndexItem = {
  kind: 'turn' | 'pending';
  key: string;
  turnIndex: number;
  sourceIndex: number;
  estimatedHeight: number;
};

export type ChatDisplayIndex = {
  items: ChatDisplayIndexItem[];
};

export type ChatTurnHeightMetrics = {
  contentWidth: number;
  textFontSize: number;
  textLineHeight: number;
  paragraphGap: number;
  promptFontSize: number;
  promptLineHeight: number;
  promptHorizontalPadding: number;
  promptVerticalPadding: number;
  promptGroupVerticalPadding: number;
  promptGroupGap: number;
  promptMaxWidth: number;
  toolLineHeight: number;
  thoughtCollapsedHeight: number;
  planLineHeight: number;
  planBlockVerticalPadding: number;
  planTitleHeight: number;
  planItemGap: number;
  optionButtonMinHeight: number;
  optionButtonMaxWidth: number;
  confirmationButtonMinHeight: number;
  confirmationButtonMaxWidth: number;
  imageStripHeight: number;
};

export type ChatTurnHeightContext = {
  layoutMetrics?: Partial<ChatTurnHeightMetrics>;
  promptStatus?: ChatPromptStatus;
};

export type ChatDisplayIndexOptions = {
  shouldRender?: (message: RegistryChatMessage, promptStatus: ChatPromptStatus) => boolean;
  hideToolCalls?: boolean;
  layoutMetrics?: Partial<ChatTurnHeightMetrics>;
  promptStatus?: (message: RegistryChatMessage) => ChatPromptStatus;
  pendingKey?: string;
  pendingEstimatedHeight?: number;
};

export const DEFAULT_CHAT_TURN_HEIGHT_METRICS: ChatTurnHeightMetrics = {
  contentWidth: 720,
  textFontSize: 14,
  textLineHeight: 22,
  paragraphGap: 7,
  promptFontSize: 13,
  promptLineHeight: 18,
  promptHorizontalPadding: 24,
  promptVerticalPadding: 16,
  promptGroupVerticalPadding: 22,
  promptGroupGap: 8,
  promptMaxWidth: 920,
  toolLineHeight: 18,
  thoughtCollapsedHeight: 28,
  planLineHeight: 19,
  planBlockVerticalPadding: 16,
  planTitleHeight: 23,
  planItemGap: 4,
  optionButtonMinHeight: 30,
  optionButtonMaxWidth: 560,
  confirmationButtonMinHeight: 32,
  confirmationButtonMaxWidth: 620,
  imageStripHeight: 232,
};

function positiveTurnIndex(message: RegistryChatMessage): number {
  const turnIndex = Number(message.turnIndex ?? 0);
  return Number.isFinite(turnIndex) ? Math.max(0, Math.trunc(turnIndex)) : 0;
}

function displayKey(message: RegistryChatMessage): string {
  return `${message.sessionId}:${positiveTurnIndex(message)}:${message.method}`;
}

function normalizeMetrics(input: Partial<ChatTurnHeightMetrics> | undefined): ChatTurnHeightMetrics {
  const base = DEFAULT_CHAT_TURN_HEIGHT_METRICS;
  const metrics = {...base, ...(input ?? {})};
  return {
    ...metrics,
    contentWidth: Math.max(240, Math.round(metrics.contentWidth)),
    textFontSize: Math.max(8, metrics.textFontSize),
    textLineHeight: Math.max(10, metrics.textLineHeight),
    paragraphGap: Math.max(0, metrics.paragraphGap),
    promptFontSize: Math.max(8, metrics.promptFontSize),
    promptLineHeight: Math.max(10, metrics.promptLineHeight),
  };
}

function clampHeight(value: number): number {
  return Math.max(24, Math.min(8000, Math.round(value)));
}

function isPromptStartMethod(method: string): boolean {
  return method === 'prompt_request' || method === 'user_message_chunk';
}

function isToolCallMethod(method: string): boolean {
  return method === 'tool_call';
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

function extractTextFromParam(param: unknown): string {
  if (typeof param === 'string') {
    return param.trim();
  }
  if (Array.isArray(param)) {
    return param
      .map(item => {
        if (!item || typeof item !== 'object') return '';
        const entry = item as Record<string, unknown>;
        return typeof entry.content === 'string' ? entry.content.trim() : '';
      })
      .filter(Boolean)
      .join('\n')
      .trim();
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

function messageText(message: RegistryChatMessage): string {
  const param = message.param ?? {};
  if (message.method === 'prompt_request') {
    const blockText = extractTextFromACPContent(param.contentBlocks);
    return blockText || extractTextFromParam(param);
  }
  if (message.method === 'prompt_done') {
    const stopReason = typeof param.stopReason === 'string' ? param.stopReason.trim() : '';
    const resultMessage = typeof param.message === 'string' ? param.message.trim() : '';
    return [stopReason, resultMessage].filter(Boolean).join('\n');
  }
  return extractTextFromParam(param);
}

function imageBlockCount(message: RegistryChatMessage): number {
  const blocks = message.param?.contentBlocks;
  if (!Array.isArray(blocks)) {
    return 0;
  }
  return blocks.filter(item => {
    if (!item || typeof item !== 'object') return false;
    return (item as Record<string, unknown>).type === 'image';
  }).length;
}

function textWidthUnits(text: string): number {
  let units = 0;
  for (const char of text) {
    if (/\s/.test(char)) {
      units += 0.35;
    } else if (/[\u3400-\u9fff\u3000-\u303f\uff00-\uffef]/.test(char)) {
      units += 1;
    } else if (/[\dA-Z]/.test(char)) {
      units += 0.66;
    } else if (/[il.,:;|!]/.test(char)) {
      units += 0.32;
    } else {
      units += 0.56;
    }
  }
  return units;
}

function stripMarkdownMarkers(line: string): string {
  return line
    .replace(/^\s{0,3}#{1,6}\s+/, '')
    .replace(/^\s*[-*+]\s+/, '')
    .replace(/^\s*\d+\.\s+/, '')
    .replace(/^\s*>\s?/, '')
    .replace(/[*_`~[\]()]/g, '');
}

function estimateWrappedRows(text: string, width: number, fontSize: number): number {
  const maxUnits = Math.max(8, width / Math.max(1, fontSize));
  const lines = text.split(/\r?\n/);
  return lines.reduce((sum, rawLine) => {
    const line = stripMarkdownMarkers(rawLine);
    if (!line.trim()) {
      return sum + 1;
    }
    return sum + Math.max(1, Math.ceil(textWidthUnits(line) / maxUnits));
  }, 0);
}

function estimateMarkdownTextHeight(text: string, metrics: ChatTurnHeightMetrics): number {
  const normalized = text.trim();
  if (!normalized) {
    return 0;
  }
  const blocks = normalized.split(/\r?\n\s*\r?\n/).filter(block => block.trim());
  if (blocks.length === 0) {
    return 0;
  }
  const rows = blocks.reduce(
    (sum, block) => sum + estimateWrappedRows(block, metrics.contentWidth, metrics.textFontSize),
    0,
  );
  return rows * metrics.textLineHeight + Math.max(0, blocks.length - 1) * metrics.paragraphGap;
}

function estimateOptionLineHeight(text: string, metrics: ChatTurnHeightMetrics): number {
  const width = Math.max(120, Math.min(metrics.contentWidth, metrics.optionButtonMaxWidth) - 42);
  const rows = estimateWrappedRows(text, width, metrics.textFontSize);
  return Math.max(metrics.optionButtonMinHeight, rows * metrics.textLineHeight * 0.9 + 12) + 6;
}

function estimateConfirmationLineHeight(text: string, metrics: ChatTurnHeightMetrics): number {
  const width = Math.max(120, Math.min(metrics.contentWidth, metrics.confirmationButtonMaxWidth) - 48);
  const rows = estimateWrappedRows(text, width, metrics.textFontSize);
  return Math.max(metrics.confirmationButtonMinHeight, rows * metrics.textLineHeight * 0.95 + 10) + 6;
}

function estimateAssistantTextHeight(text: string, metrics: ChatTurnHeightMetrics): number {
  const optionParts = splitChatOptionReplyText(text);
  if (optionParts.some(part => part.type === 'option')) {
    return optionParts.reduce((sum, part) => {
      if (part.type === 'markdown') {
        return sum + estimateMarkdownTextHeight(part.text, metrics);
      }
      return sum + estimateOptionLineHeight(part.reply.text, metrics);
    }, 0);
  }
  const confirmationParts = splitChatConfirmationReplyText(text);
  if (confirmationParts.some(part => part.type === 'confirmation')) {
    return confirmationParts.reduce((sum, part) => {
      if (part.type === 'markdown') {
        return sum + estimateMarkdownTextHeight(part.text, metrics);
      }
      return sum + estimateConfirmationLineHeight(part.reply.sentence, metrics);
    }, 0);
  }
  return estimateMarkdownTextHeight(text, metrics);
}

function estimatePromptStartHeight(
  message: RegistryChatMessage,
  context: ChatTurnHeightContext,
  metrics: ChatTurnHeightMetrics,
): number {
  const text = messageText(message);
  const promptWidth = Math.max(
    120,
    Math.min(metrics.contentWidth, metrics.promptMaxWidth) - metrics.promptHorizontalPadding - 2,
  );
  const textRows = text ? estimateWrappedRows(text, promptWidth, metrics.promptFontSize) : 0;
  const textHeight = textRows > 0
    ? textRows * metrics.promptLineHeight + metrics.promptVerticalPadding + 2
    : 0;
  const statusHeight = context.promptStatus ? 18 : 0;
  const rowHeight = Math.max(textHeight, statusHeight);
  const images = imageBlockCount(message);
  const imageRows = images > 0
    ? Math.ceil(images / Math.max(1, Math.floor(metrics.contentWidth / 288)))
    : 0;
  const imageHeight = imageRows > 0 ? imageRows * metrics.imageStripHeight : 0;
  const deliveryHeight = context.promptStatus === 'undelivered' ? 22 : 0;
  const visibleSections = [rowHeight, imageHeight, deliveryHeight].filter(height => height > 0);
  const gaps = Math.max(0, visibleSections.length - 1) * metrics.promptGroupGap;
  return clampHeight(
    metrics.promptGroupVerticalPadding +
      visibleSections.reduce((sum, height) => sum + height, 0) +
      gaps,
  );
}

function promptDoneHasResultLine(message: RegistryChatMessage): boolean {
  const stopReason = typeof message.param?.stopReason === 'string'
    ? message.param.stopReason.trim().toLowerCase()
    : '';
  return stopReason === 'cancelled' ||
    stopReason === 'canceled' ||
    stopReason === 'interrupted' ||
    stopReason === 'failed' ||
    stopReason === 'error';
}

function estimatePlanHeight(message: RegistryChatMessage, metrics: ChatTurnHeightMetrics): number {
  const text = messageText(message);
  const lines = text
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(Boolean);
  const entries = lines.length > 0 ? lines : ['Plan'];
  const entryWidth = Math.max(120, metrics.contentWidth - 44);
  const rows = entries.reduce(
    (sum, entry) => sum + estimateWrappedRows(entry, entryWidth, 13),
    0,
  );
  return clampHeight(
    metrics.planBlockVerticalPadding +
      metrics.planTitleHeight +
      rows * metrics.planLineHeight +
      Math.max(0, entries.length - 1) * metrics.planItemGap,
  );
}

export function estimateChatTurnHeight(
  message: RegistryChatMessage,
  context: ChatTurnHeightContext = {},
): number {
  const metrics = normalizeMetrics(context.layoutMetrics);
  if (isPromptStartMethod(message.method)) {
    return estimatePromptStartHeight(message, context, metrics);
  }
  if (message.method === 'prompt_done') {
    return clampHeight(38 + (promptDoneHasResultLine(message) ? 18 : 0));
  }
  if (isToolCallMethod(message.method)) {
    return clampHeight(metrics.toolLineHeight);
  }
  if (message.method === 'agent_thought_chunk') {
    return clampHeight(metrics.thoughtCollapsedHeight);
  }
  if (message.method === 'agent_plan') {
    return estimatePlanHeight(message, metrics);
  }
  const text = messageText(message);
  return clampHeight(estimateAssistantTextHeight(text, metrics));
}

export function buildChatDisplayIndex(
  messages: RegistryChatMessage[],
  options: ChatDisplayIndexOptions = {},
): ChatDisplayIndex {
  const sorted = messages
    .map((message, sourceIndex) => ({message, sourceIndex}))
    .filter(item => positiveTurnIndex(item.message) > 0)
    .sort((left, right) => positiveTurnIndex(left.message) - positiveTurnIndex(right.message));
  const items: ChatDisplayIndexItem[] = [];
  for (const item of sorted) {
    if (options.hideToolCalls && isToolCallMethod(item.message.method)) {
      continue;
    }
    const promptStatus = options.promptStatus?.(item.message) ?? null;
    if (options.shouldRender && !options.shouldRender(item.message, promptStatus)) {
      continue;
    }
    const estimatedHeight = estimateChatTurnHeight(item.message, {
      layoutMetrics: options.layoutMetrics,
      promptStatus,
    });
    items.push({
      kind: 'turn',
      key: displayKey(item.message),
      turnIndex: positiveTurnIndex(item.message),
      sourceIndex: item.sourceIndex,
      estimatedHeight,
    });
  }
  const pendingKey = options.pendingKey?.trim();
  if (pendingKey) {
    items.push({
      kind: 'pending',
      key: pendingKey,
      turnIndex: 0,
      sourceIndex: -1,
      estimatedHeight: Math.max(56, Math.trunc(options.pendingEstimatedHeight ?? 120)),
    });
  }
  return {items};
}
