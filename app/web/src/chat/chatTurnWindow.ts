import type { RegistryChatMessage } from '../types/registry';

export const DEFAULT_TURN_WINDOW_SIZE = 200;
export const TURN_WINDOW_EXPAND_SIZE = 200;

export type ChatTurnWindow = {
  startTurnIndex: number;
  endTurnIndex: number;
};

function positiveTurnIndex(message: RegistryChatMessage): number {
  const turnIndex = Number(message.turnIndex);
  return Number.isFinite(turnIndex) ? Math.max(0, Math.trunc(turnIndex)) : 0;
}

function sortedTurns(turns: RegistryChatMessage[]): RegistryChatMessage[] {
  return [...turns].sort((left, right) => positiveTurnIndex(left) - positiveTurnIndex(right));
}

export function createLatestTurnWindow(
  turns: RegistryChatMessage[],
  size = DEFAULT_TURN_WINDOW_SIZE,
): ChatTurnWindow {
  const ordered = sortedTurns(turns).filter(message => positiveTurnIndex(message) > 0);
  if (ordered.length === 0) {
    return { startTurnIndex: 0, endTurnIndex: 0 };
  }
  const normalizedSize = Math.max(1, Math.trunc(size));
  const latest = positiveTurnIndex(ordered[ordered.length - 1]);
  const first = positiveTurnIndex(ordered[0]);
  return {
    startTurnIndex: Math.max(first, latest - normalizedSize + 1),
    endTurnIndex: latest,
  };
}

export function expandTurnWindowEarlier(
  turns: RegistryChatMessage[],
  window: ChatTurnWindow,
  size = TURN_WINDOW_EXPAND_SIZE,
): ChatTurnWindow {
  const ordered = sortedTurns(turns).filter(message => positiveTurnIndex(message) > 0);
  if (ordered.length === 0 || window.endTurnIndex <= 0) {
    return { startTurnIndex: 0, endTurnIndex: 0 };
  }
  const first = positiveTurnIndex(ordered[0]);
  const normalizedSize = Math.max(1, Math.trunc(size));
  return {
    startTurnIndex: Math.max(first, window.startTurnIndex - normalizedSize),
    endTurnIndex: window.endTurnIndex,
  };
}

export function sliceTurnsForWindow(
  turns: RegistryChatMessage[],
  window: ChatTurnWindow,
): RegistryChatMessage[] {
  if (window.startTurnIndex <= 0 || window.endTurnIndex <= 0) {
    return [];
  }
  return sortedTurns(turns).filter(message => {
    const turnIndex = positiveTurnIndex(message);
    return turnIndex >= window.startTurnIndex && turnIndex <= window.endTurnIndex;
  });
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
