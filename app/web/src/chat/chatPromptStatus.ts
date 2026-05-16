import type { RegistryChatMessage } from '../types/registry';

export type ChatPromptStatus = 'responding' | null;

function positiveTurnIndex(message: RegistryChatMessage): number {
  const turnIndex = Number(message.turnIndex);
  return Number.isFinite(turnIndex) ? Math.max(0, Math.trunc(turnIndex)) : 0;
}

function isPromptStart(message: RegistryChatMessage): boolean {
  return message.method === 'prompt_request' || message.method === 'user_message_chunk';
}

export function resolvePromptTurnStatus(
  turns: RegistryChatMessage[],
  promptTurn: RegistryChatMessage,
): ChatPromptStatus {
  if (!isPromptStart(promptTurn)) {
    return null;
  }
  const promptTurnIndex = positiveTurnIndex(promptTurn);
  if (promptTurnIndex <= 0) {
    return 'responding';
  }
  const ordered = [...turns]
    .filter(message => message.sessionId === promptTurn.sessionId)
    .sort((left, right) => positiveTurnIndex(left) - positiveTurnIndex(right));

  for (const message of ordered) {
    const turnIndex = positiveTurnIndex(message);
    if (turnIndex <= promptTurnIndex) {
      continue;
    }
    if (isPromptStart(message)) {
      break;
    }
    if (message.method === 'prompt_done') {
      return null;
    }
  }
  return 'responding';
}
