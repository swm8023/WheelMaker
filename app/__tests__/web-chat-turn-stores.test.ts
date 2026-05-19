import {
  applySessionReadResult,
  buildMergedRawTurns,
  createEmptyChatTurnStore,
  getDurableTurnPrefix,
  getFinishedCursor,
  hydrateFinishedStore,
  mergeRealtimeTurn,
  shouldReadRepairForIncomingTurn,
} from '../web/src/chat/chatTurnStores';
import type {RegistrySessionTurn} from '../web/src/types/registry';

const turn = (turnIndex: number, finished = true, text = `turn-${turnIndex}`): RegistrySessionTurn => ({
  turnIndex,
  content: JSON.stringify({method: 'agent_message_chunk', param: {text}}),
  finished,
});

describe('raw chat turn stores', () => {
  test('finished cursor ignores holes and durable prefix stops before the hole', () => {
    const turns = [turn(1), turn(2), turn(4)];

    expect(getFinishedCursor(turns)).toEqual({turnIndex: 2});
    expect(getDurableTurnPrefix(turns, {turnIndex: 9})).toEqual([turn(1), turn(2)]);
  });

  test('live turns do not advance finished cursor', () => {
    const state = createEmptyChatTurnStore();
    mergeRealtimeTurn(state, turn(1, true));
    mergeRealtimeTurn(state, turn(2, false));

    expect(state.cursor).toEqual({turnIndex: 1});
    expect(buildMergedRawTurns(state)).toEqual([turn(1, true), turn(2, false)]);
  });

  test('same-index live turn updates and finished turn absorbs live', () => {
    const state = createEmptyChatTurnStore();
    mergeRealtimeTurn(state, turn(1, true));
    mergeRealtimeTurn(state, turn(2, false, 'partial'));
    mergeRealtimeTurn(state, turn(2, false, 'partial updated'));
    expect(buildMergedRawTurns(state)).toEqual([turn(1, true), turn(2, false, 'partial updated')]);

    mergeRealtimeTurn(state, turn(2, true, 'done'));
    expect(state.live).toEqual([]);
    expect(state.finished).toEqual([turn(1, true), turn(2, true, 'done')]);
    expect(state.cursor).toEqual({turnIndex: 2});
  });

  test('gap read trigger uses finished cursor and allows next unfinished tail', () => {
    const state = hydrateFinishedStore([turn(1), turn(2), turn(3), turn(4), turn(5), turn(6), turn(7), turn(8), turn(9), turn(10)]);

    expect(shouldReadRepairForIncomingTurn(state, turn(12, false))).toEqual({turnIndex: 10});
    expect(shouldReadRepairForIncomingTurn(state, turn(11, false))).toBeNull();
    mergeRealtimeTurn(state, turn(11, false));
    expect(shouldReadRepairForIncomingTurn(state, turn(12, true))).toEqual({turnIndex: 10});
  });

  test('read response replaces covered range and rejects middle unfinished turns', () => {
    const state = hydrateFinishedStore([turn(1), turn(2)]);
    mergeRealtimeTurn(state, turn(4, false, 'live'));

    applySessionReadResult(state, 2, [turn(3, true), turn(4, false, 'server live')], 4);
    expect(state.finished).toEqual([turn(1), turn(2), turn(3)]);
    expect(state.live).toEqual([turn(4, false, 'server live')]);
    expect(state.cursor).toEqual({turnIndex: 3});

    expect(() => applySessionReadResult(state, 3, [turn(4, false), turn(5, true)], 5)).toThrow(/unfinished tail/);
  });

  test('stale read response resets local turns when server latest is behind cursor', () => {
    const state = hydrateFinishedStore([turn(1), turn(2), turn(3)]);
    mergeRealtimeTurn(state, turn(4, false, 'dirty live'));

    applySessionReadResult(state, 3, [], 2);

    expect(state.finished).toEqual([]);
    expect(state.live).toEqual([]);
    expect(state.cursor).toEqual({turnIndex: 0});
  });
});
