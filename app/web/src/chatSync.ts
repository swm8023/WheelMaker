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
    (a.done === true) === (b.done === true) &&
    JSON.stringify(a.param ?? {}) === JSON.stringify(b.param ?? {})
  );
}

function isTextTurnMessage(message: RegistryChatMessage): boolean {
  return message.method === 'agent_message_chunk' || message.method === 'agent_thought_chunk';
}

export function replacePromptMessages(
  list: RegistryChatMessage[],
  nextMessages: RegistryChatMessage[],
  promptIndex: number,
  checkpointTurnIndex?: number,
): RegistryChatMessage[] {
  const base = list.filter(item => {
    const itemPromptIndex = item.promptIndex ?? 0;
    if (promptIndex <= 0) return false;
    if (itemPromptIndex < promptIndex) return true;
    if (itemPromptIndex === promptIndex && item.method === 'prompt_done') return true;
    if (
      itemPromptIndex === promptIndex &&
      checkpointTurnIndex != null &&
      checkpointTurnIndex > 0
    ) {
      return (item.turnIndex ?? 0) <= checkpointTurnIndex;
    }
    return itemPromptIndex > promptIndex;
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
  if (promptDone.method !== 'prompt_done') {
    return false;
  }
  const sessionId = promptDone.sessionId;
  const promptIndex = promptDone.promptIndex ?? 0;
  if (!sessionId || promptIndex <= 0) {
    return false;
  }
  let maxTurnIndex = 0;
  for (const message of messages) {
    if (message.sessionId !== sessionId || (message.promptIndex ?? 0) !== promptIndex) {
      continue;
    }
    if (message.method === 'prompt_done') {
      continue;
    }
    maxTurnIndex = Math.max(maxTurnIndex, message.turnIndex ?? 0);
    if (isTextTurnMessage(message) && message.done !== true) {
      return true;
    }
  }
  const expectedMaxTurnIndex = Math.max(0, (promptDone.turnIndex ?? 0) - 1);
  return expectedMaxTurnIndex > maxTurnIndex;
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
      if (isTextTurnMessage(message) && message.done !== true) {
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
