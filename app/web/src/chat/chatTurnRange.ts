import type {RegistryChatMessage} from '../types/registry';

function positiveTurnIndex(message: RegistryChatMessage): number {
  const turnIndex = Number(message.turnIndex);
  return Number.isFinite(turnIndex) ? Math.max(0, Math.trunc(turnIndex)) : 0;
}

export function hasContinuousTurnRange(
  turns: RegistryChatMessage[],
  startTurnIndex: number,
  endTurnIndex: number,
): boolean {
  const start = Math.max(0, Math.trunc(startTurnIndex));
  const end = Math.max(0, Math.trunc(endTurnIndex));
  if (start <= 0 || end <= 0 || end < start) {
    return false;
  }
  const indexes = new Set(
    turns
      .map(positiveTurnIndex)
      .filter(turnIndex => turnIndex >= start && turnIndex <= end),
  );
  for (let turnIndex = start; turnIndex <= end; turnIndex += 1) {
    if (!indexes.has(turnIndex)) {
      return false;
    }
  }
  return true;
}
