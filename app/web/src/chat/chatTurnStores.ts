import type {RegistrySessionTurn} from '../types/registry';
import type {SessionReadCursor} from '../chatSync';

export type ChatTurnStoreState = {
  finished: RegistrySessionTurn[];
  live: RegistrySessionTurn[];
  cursor: SessionReadCursor;
};

function cloneTurn(turn: RegistrySessionTurn): RegistrySessionTurn {
  return {
    turnIndex: Math.trunc(turn.turnIndex),
    content: turn.content,
    finished: turn.finished === true,
  };
}

function normalizeTurn(turn: RegistrySessionTurn): RegistrySessionTurn | null {
  const turnIndex = Math.trunc(turn.turnIndex ?? 0);
  if (turnIndex <= 0 || typeof turn.content !== 'string' || turn.content === '') {
    return null;
  }
  return {
    turnIndex,
    content: turn.content,
    finished: turn.finished === true,
  };
}

function sortTurns(turns: RegistrySessionTurn[]): RegistrySessionTurn[] {
  return [...turns].sort((a, b) => a.turnIndex - b.turnIndex);
}

function upsertTurn(turns: RegistrySessionTurn[], incoming: RegistrySessionTurn): RegistrySessionTurn[] {
  const index = turns.findIndex(item => item.turnIndex === incoming.turnIndex);
  if (index < 0) return sortTurns([...turns, cloneTurn(incoming)]);
  const next = [...turns];
  next[index] = cloneTurn(incoming);
  return sortTurns(next);
}

export function createEmptyChatTurnStore(): ChatTurnStoreState {
  return {finished: [], live: [], cursor: {turnIndex: 0}};
}

export function resetChatTurnStore(state: ChatTurnStoreState): ChatTurnStoreState {
  state.finished = [];
  state.live = [];
  state.cursor = {turnIndex: 0};
  return state;
}

export function isStaleSessionReadResult(afterTurnIndex: number, latestTurnIndex: number): boolean {
  const after = Math.max(0, Math.trunc(afterTurnIndex ?? 0));
  const latest = Number.isFinite(latestTurnIndex)
    ? Math.max(0, Math.trunc(latestTurnIndex))
    : 0;
  return after > 0 && latest < after;
}

export function getFinishedCursor(turns: RegistrySessionTurn[]): SessionReadCursor {
  const finished = new Set<number>();
  for (const raw of turns) {
    const turn = normalizeTurn(raw);
    if (turn?.finished) {
      finished.add(turn.turnIndex);
    }
  }
  let turnIndex = 0;
  while (finished.has(turnIndex + 1)) {
    turnIndex += 1;
  }
  return {turnIndex};
}

export function hydrateFinishedStore(turns: RegistrySessionTurn[]): ChatTurnStoreState {
  const normalized = sortTurns(
    turns
      .map(item => normalizeTurn(item))
      .filter((item): item is RegistrySessionTurn => !!item && item.finished),
  );
  const cursor = getFinishedCursor(normalized);
  return {
    finished: getDurableTurnPrefix(normalized, cursor),
    live: [],
    cursor,
  };
}

export function getDurableTurnPrefix(
  turns: RegistrySessionTurn[],
  cursor: SessionReadCursor,
): RegistrySessionTurn[] {
  const maxTurnIndex = Math.max(0, Math.trunc(cursor.turnIndex ?? 0));
  const byIndex = new Map<number, RegistrySessionTurn>();
  for (const raw of turns) {
    const turn = normalizeTurn(raw);
    if (turn?.finished) {
      byIndex.set(turn.turnIndex, turn);
    }
  }
  const out: RegistrySessionTurn[] = [];
  for (let turnIndex = 1; turnIndex <= maxTurnIndex; turnIndex += 1) {
    const turn = byIndex.get(turnIndex);
    if (!turn) break;
    out.push(cloneTurn(turn));
  }
  return out;
}

export function buildMergedRawTurns(state: ChatTurnStoreState): RegistrySessionTurn[] {
  const byIndex = new Map<number, RegistrySessionTurn>();
  for (const turn of state.live) {
    byIndex.set(turn.turnIndex, cloneTurn(turn));
  }
  for (const turn of state.finished) {
    byIndex.set(turn.turnIndex, cloneTurn(turn));
  }
  return sortTurns(Array.from(byIndex.values()));
}

export function shouldReadRepairForIncomingTurn(
  state: ChatTurnStoreState,
  incoming: RegistrySessionTurn,
): SessionReadCursor | null {
  const turn = normalizeTurn(incoming);
  if (!turn) return null;
  const cursorTurnIndex = Math.max(0, Math.trunc(state.cursor.turnIndex ?? 0));
  if (turn.turnIndex > cursorTurnIndex + 1) {
    return {turnIndex: cursorTurnIndex};
  }
  return null;
}

export function mergeRealtimeTurn(
  state: ChatTurnStoreState,
  incoming: RegistrySessionTurn,
): ChatTurnStoreState {
  const turn = normalizeTurn(incoming);
  if (!turn) return state;
  if (turn.finished) {
    state.finished = upsertTurn(state.finished, turn);
    state.live = state.live.filter(item => item.turnIndex !== turn.turnIndex);
    state.cursor = getFinishedCursor(state.finished);
    state.finished = getDurableTurnPrefix(state.finished, state.cursor);
    return state;
  }
  if (!state.finished.some(item => item.turnIndex === turn.turnIndex)) {
    state.live = upsertTurn(state.live, turn);
  }
  return state;
}

export function applySessionReadResult(
  state: ChatTurnStoreState,
  afterTurnIndex: number,
  turns: RegistrySessionTurn[],
  latestTurnIndex: number,
): ChatTurnStoreState {
  const after = Math.max(0, Math.trunc(afterTurnIndex ?? 0));
  if (isStaleSessionReadResult(after, latestTurnIndex)) {
    return resetChatTurnStore(state);
  }
  const latest = Math.max(after, Math.trunc(latestTurnIndex ?? after));
  const normalized = sortTurns(
    turns
      .map(item => normalizeTurn(item))
      .filter((item): item is RegistrySessionTurn => !!item),
  );
  for (let index = 0; index < normalized.length; index += 1) {
    const turn = normalized[index];
    const isLast = index === normalized.length - 1;
    if (!turn.finished && !isLast) {
      throw new Error('invalid session.read result: unfinished tail must be last');
    }
  }
  state.finished = state.finished.filter(turn => turn.turnIndex <= after || turn.turnIndex > latest);
  state.live = state.live.filter(turn => turn.turnIndex <= after || turn.turnIndex > latest);
  for (const turn of normalized) {
    if (turn.finished) {
      state.finished = upsertTurn(state.finished, turn);
    } else {
      state.live = upsertTurn(state.live, turn);
    }
  }
  state.cursor = getFinishedCursor(state.finished);
  state.finished = getDurableTurnPrefix(state.finished, state.cursor);
  return state;
}
