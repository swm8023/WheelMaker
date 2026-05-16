import type { RegistryChatMessage } from '../types/registry';

export const DEFAULT_TURN_WINDOW_SIZE = 200;
export const TURN_WINDOW_STEP_SIZE = 100;
export const TURN_WINDOW_EDGE_THRESHOLD = 25;

export type ChatTurnWindow = {
  startIndex: number;
  endIndex: number;
};

function positiveTurnIndex(message: RegistryChatMessage): number {
  const turnIndex = Number(message.turnIndex);
  return Number.isFinite(turnIndex) ? Math.max(0, Math.trunc(turnIndex)) : 0;
}

function sortedTurns(turns: RegistryChatMessage[]): RegistryChatMessage[] {
  return [...turns].sort((left, right) => positiveTurnIndex(left) - positiveTurnIndex(right));
}

function orderedTurns(turns: RegistryChatMessage[]): RegistryChatMessage[] {
  return sortedTurns(turns).filter(message => positiveTurnIndex(message) > 0);
}

function normalizedWindowSize(size: number): number {
  return Math.max(1, Math.trunc(size));
}

function normalizedStep(size: number, step: number): number {
  return Math.max(1, Math.min(normalizedWindowSize(size), Math.trunc(step)));
}

export function createLatestTurnWindow(
  turns: RegistryChatMessage[],
  size = DEFAULT_TURN_WINDOW_SIZE,
  step = TURN_WINDOW_STEP_SIZE,
): ChatTurnWindow {
  const count = orderedTurns(turns).length;
  if (count === 0) {
    return { startIndex: 0, endIndex: 0 };
  }
  const normalizedSize = normalizedWindowSize(size);
  const normalizedStepSize = normalizedStep(normalizedSize, step);
  const startIndex = Math.max(0, count - normalizedStepSize);
  return {
    startIndex,
    endIndex: startIndex + normalizedSize,
  };
}

export function expandTurnWindowEarlier(
  turns: RegistryChatMessage[],
  window: ChatTurnWindow,
  step = TURN_WINDOW_STEP_SIZE,
  size = DEFAULT_TURN_WINDOW_SIZE,
): ChatTurnWindow {
  const count = orderedTurns(turns).length;
  if (count === 0 || window.endIndex <= 0) {
    return { startIndex: 0, endIndex: 0 };
  }
  const normalizedSize = normalizedWindowSize(size);
  const normalizedStepSize = normalizedStep(normalizedSize, step);
  const startIndex = Math.max(0, Math.min(window.startIndex, count) - normalizedStepSize);
  return {
    startIndex,
    endIndex: startIndex + normalizedSize,
  };
}

export function expandTurnWindowLater(
  turns: RegistryChatMessage[],
  window: ChatTurnWindow,
  step = TURN_WINDOW_STEP_SIZE,
  size = DEFAULT_TURN_WINDOW_SIZE,
): ChatTurnWindow {
  const count = orderedTurns(turns).length;
  if (count === 0 || window.endIndex <= 0) {
    return { startIndex: 0, endIndex: 0 };
  }
  const normalizedSize = normalizedWindowSize(size);
  const normalizedStepSize = normalizedStep(normalizedSize, step);
  const latestStartIndex = Math.max(0, count - normalizedStepSize);
  const startIndex = Math.min(
    latestStartIndex,
    Math.max(0, window.startIndex + normalizedStepSize),
  );
  return {
    startIndex,
    endIndex: startIndex + normalizedSize,
  };
}

export function followLatestTurnWindow(
  turns: RegistryChatMessage[],
  window: ChatTurnWindow,
  threshold = TURN_WINDOW_EDGE_THRESHOLD,
  size = DEFAULT_TURN_WINDOW_SIZE,
  step = TURN_WINDOW_STEP_SIZE,
): ChatTurnWindow {
  const count = orderedTurns(turns).length;
  if (count === 0) {
    return { startIndex: 0, endIndex: 0 };
  }
  const edgeThreshold = Math.max(0, Math.trunc(threshold));
  if (window.endIndex <= 0 || window.endIndex - count < edgeThreshold) {
    return createLatestTurnWindow(turns, size, step);
  }
  return window;
}

export function sliceTurnsForWindow(
  turns: RegistryChatMessage[],
  window: ChatTurnWindow,
): RegistryChatMessage[] {
  if (window.endIndex <= 0) {
    return [];
  }
  const startIndex = Math.max(0, Math.trunc(window.startIndex));
  const endIndex = Math.max(startIndex, Math.trunc(window.endIndex));
  return orderedTurns(turns).slice(startIndex, endIndex);
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
