import type {RegistryChatMessage} from '../types/registry';

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

export type ChatDisplayIndexOptions = {
  shouldRender?: (message: RegistryChatMessage) => boolean;
  pendingKey?: string;
  pendingEstimatedHeight?: number;
};

function positiveTurnIndex(message: RegistryChatMessage): number {
  const turnIndex = Number(message.turnIndex ?? 0);
  return Number.isFinite(turnIndex) ? Math.max(0, Math.trunc(turnIndex)) : 0;
}

function displayKey(message: RegistryChatMessage): string {
  return `${message.sessionId}:${positiveTurnIndex(message)}:${message.method}`;
}

function messageTextLength(message: RegistryChatMessage): number {
  const param = message.param ?? {};
  const text = (param as {text?: unknown}).text;
  if (typeof text === 'string') {
    return text.length;
  }
  try {
    return JSON.stringify(param).length;
  } catch {
    return 0;
  }
}

export function estimateChatTurnHeight(message: RegistryChatMessage): number {
  const textLength = messageTextLength(message);
  const textRows = Math.ceil(textLength / 88);
  const base =
    message.method === 'prompt_request'
      ? 120
      : message.method === 'prompt_done'
        ? 72
        : message.method.startsWith('tool_')
          ? 84
          : 104;
  return Math.max(56, Math.min(1200, base + textRows * 22));
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
    if (options.shouldRender && !options.shouldRender(item.message)) {
      continue;
    }
    const estimatedHeight = estimateChatTurnHeight(item.message);
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
