import type { RegistryChatMessage } from './types/registry';

export type SessionReadCursor = {
  turnIndex: number;
};

function chatMessageKey(message: RegistryChatMessage): string {
  return `${message.sessionId}:${message.turnIndex}`;
}

function upsertChatMessage(
  list: RegistryChatMessage[],
  next: RegistryChatMessage,
): RegistryChatMessage[] {
  const key = chatMessageKey(next);
  const index = list.findIndex(item => chatMessageKey(item) === key);
  if (index < 0) {
    return [...list, next].sort((a, b) => {
      return (a.turnIndex ?? 0) - (b.turnIndex ?? 0);
    });
  }
  const copy = [...list];
  copy[index] = next;
  return copy;
}

function sameChatMessage(a: RegistryChatMessage | undefined, b: RegistryChatMessage): boolean {
  if (!a) return false;
  return (
    a.sessionId === b.sessionId &&
    a.turnIndex === b.turnIndex &&
    a.method === b.method &&
    isFinishedChatMessage(a) === isFinishedChatMessage(b) &&
    JSON.stringify(a.param ?? {}) === JSON.stringify(b.param ?? {})
  );
}

export function isFinishedChatMessage(message: RegistryChatMessage): boolean {
  return message.finished === true;
}

export function replaceSessionMessages(
  list: RegistryChatMessage[],
  nextMessages: RegistryChatMessage[],
  checkpointTurnIndex?: number,
): RegistryChatMessage[] {
  const base = list.filter(item => {
    if (checkpointTurnIndex == null || checkpointTurnIndex <= 0) return false;
    return (item.turnIndex ?? 0) <= checkpointTurnIndex;
  });
  return nextMessages.reduce(
    (items, message) => upsertChatMessage(items, message),
    base,
  );
}

export function needsPromptTurnRefresh(
  _messages: RegistryChatMessage[],
  _promptDone: RegistryChatMessage,
): boolean {
  return false;
}

export function getLatestSessionReadCursor(messages: RegistryChatMessage[]): SessionReadCursor {
  const finishedTurns = new Set<number>();
  for (const message of messages) {
    if (!isFinishedChatMessage(message)) {
      continue;
    }
    const turnIndex = Math.trunc(message.turnIndex ?? 0);
    if (turnIndex > 0) {
      finishedTurns.add(turnIndex);
    }
  }
  let turnIndex = 0;
  while (finishedTurns.has(turnIndex + 1)) {
    turnIndex += 1;
  }
  return {turnIndex};
}

export function reconcileCachedSessionReadCursor(
  _storedCursor: SessionReadCursor | undefined,
  cachedMessages: RegistryChatMessage[] | null | undefined,
): SessionReadCursor {
  if (!cachedMessages || cachedMessages.length === 0) {
    return {turnIndex: 0};
  }
  return getLatestSessionReadCursor(cachedMessages);
}

export function shouldRequestSessionReadForIncomingTurn(
  local: {cursor: SessionReadCursor; messages?: RegistryChatMessage[]},
  incoming: RegistryChatMessage,
): SessionReadCursor | null {
  const cursor = local.cursor;
  const currentTurnIndex = cursor.turnIndex ?? 0;
  const incomingTurnIndex = incoming.turnIndex ?? 0;
  if (incomingTurnIndex <= 0) {
    return null;
  }
  const localTurnIndexes = new Set(
    (local.messages ?? [])
      .map(message => Math.trunc(message.turnIndex ?? 0))
      .filter(turnIndex => turnIndex > currentTurnIndex),
  );
  let contiguousTurnIndex = currentTurnIndex;
  while (localTurnIndexes.has(contiguousTurnIndex + 1)) {
    contiguousTurnIndex += 1;
  }
  if (incomingTurnIndex > contiguousTurnIndex + 1) {
    return {turnIndex: contiguousTurnIndex};
  }
  return null;
}

export function reconcileSessionReadMessages(
  readMessages: RegistryChatMessage[],
  freshStoreMessages: RegistryChatMessage[],
  existingMessages: RegistryChatMessage[],
): RegistryChatMessage[] {
  let nextMessages = [...readMessages];
  const existingByKey = new Map(existingMessages.map(message => [chatMessageKey(message), message]));
  for (const message of freshStoreMessages) {
    const existing = existingByKey.get(chatMessageKey(message));
    if (!sameChatMessage(existing, message)) {
      nextMessages = upsertChatMessage(nextMessages, message);
    }
  }
  return nextMessages;
}
