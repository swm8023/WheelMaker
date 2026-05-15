import type { RegistryChatSession } from './types/registry';

export type ChatSessionVisualState =
  | 'idle'
  | 'running'
  | 'completed-unviewed'
  | 'failed-unviewed';

function nonNegativeInteger(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(0, Math.trunc(value))
    : 0;
}

export function getChatSessionVisualState(session: Pick<
  RegistryChatSession,
  'running' | 'lastDoneTurnIndex' | 'lastDoneSuccess' | 'lastReadTurnIndex'
>): ChatSessionVisualState {
  if (session.running === true) {
    return 'running';
  }
  const lastDoneTurnIndex = nonNegativeInteger(session.lastDoneTurnIndex);
  const lastReadTurnIndex = nonNegativeInteger(session.lastReadTurnIndex);
  if (lastDoneTurnIndex <= 0 || lastDoneTurnIndex <= lastReadTurnIndex) {
    return 'idle';
  }
  return session.lastDoneSuccess === false
    ? 'failed-unviewed'
    : 'completed-unviewed';
}
