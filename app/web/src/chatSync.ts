import type { RegistryChatMessage } from './types/registry';

function chatMessageKey(message: RegistryChatMessage): string {
  return `${message.sessionId}:${message.promptIndex}:${message.turnIndex}`;
}

function upsertChatMessage(
  list: RegistryChatMessage[],
  next: RegistryChatMessage,
): RegistryChatMessage[] {
  const key = chatMessageKey(next);
  const index = list.findIndex(item => chatMessageKey(item) === key);
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

function sameChatMessage(a: RegistryChatMessage | undefined, b: RegistryChatMessage): boolean {
  if (!a) return false;
  return (
    a.sessionId === b.sessionId &&
    a.promptIndex === b.promptIndex &&
    a.turnIndex === b.turnIndex &&
    a.method === b.method &&
    JSON.stringify(a.param ?? {}) === JSON.stringify(b.param ?? {})
  );
}

export function getLatestSessionReadCursor(messages: RegistryChatMessage[]): {
  promptIndex: number;
  turnIndex: number;
} {
  return messages.reduce(
    (latest, message) => {
      if (message.method === 'prompt_done') {
        return latest;
      }
      const promptIndex = message.promptIndex ?? 0;
      const turnIndex = message.turnIndex ?? 0;
      if (
        promptIndex > latest.promptIndex ||
        (promptIndex === latest.promptIndex && turnIndex > latest.turnIndex)
      ) {
        return { promptIndex, turnIndex };
      }
      return latest;
    },
    { promptIndex: 0, turnIndex: 0 },
  );
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
