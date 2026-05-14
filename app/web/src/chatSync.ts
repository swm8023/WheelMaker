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

function isTextTurnMessage(message: RegistryChatMessage): boolean {
  return message.method === 'agent_message_chunk' || message.method === 'agent_thought_chunk';
}

export function replacePromptMessages(
  list: RegistryChatMessage[],
  nextMessages: RegistryChatMessage[],
  _promptIndex: number,
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
  messages: RegistryChatMessage[],
  promptDone: RegistryChatMessage,
): boolean {
  void messages;
  void promptDone;
  return false;
}

export function getLatestSessionReadCursor(messages: RegistryChatMessage[]): SessionReadCursor {
  return messages.reduce(
    (latest, message) => {
      if (!isFinishedChatMessage(message)) {
        return latest;
      }
      const turnIndex = message.turnIndex ?? 0;
      if (turnIndex > latest.turnIndex) {
        return { turnIndex };
      }
      return latest;
    },
    { turnIndex: 0 },
  );
}

export function shouldRequestSessionReadForIncomingTurn(
  local: {cursor: SessionReadCursor; terminalPrompts: ReadonlySet<number>},
  incoming: RegistryChatMessage,
): SessionReadCursor | null {
  const cursor = local.cursor;
  const currentTurnIndex = cursor.turnIndex ?? 0;
  const incomingTurnIndex = incoming.turnIndex ?? 0;
  if (incomingTurnIndex <= 0) {
    return null;
  }
  if (incomingTurnIndex > currentTurnIndex + 1) {
    return cursor;
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
