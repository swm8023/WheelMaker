import {
  createLatestTurnWindow,
  expandTurnWindowEarlier,
  hasContinuousTurnRange,
  sliceTurnsForWindow,
} from '../web/src/chat/chatTurnWindow';
import type { RegistryChatMessage } from '../web/src/types/registry';

function turn(turnIndex: number): RegistryChatMessage {
  return {
    sessionId: 's1',
    turnIndex,
    method: turnIndex % 2 === 0 ? 'agent_message_chunk' : 'prompt_request',
    param: { text: `turn ${turnIndex}` },
    finished: true,
  };
}

describe('chat turn window helpers', () => {
  test('creates the latest bounded window by raw turn index', () => {
    const turns = Array.from({ length: 250 }, (_, index) => turn(index + 1));

    expect(createLatestTurnWindow(turns)).toEqual({
      startTurnIndex: 51,
      endTurnIndex: 250,
    });
  });

  test('expands the window earlier without changing the end boundary', () => {
    const turns = Array.from({ length: 450 }, (_, index) => turn(index + 1));
    const latest = createLatestTurnWindow(turns);

    const expanded = expandTurnWindowEarlier(turns, latest);
    const expandedAgain = expandTurnWindowEarlier(turns, expanded);

    expect(latest).toEqual({ startTurnIndex: 251, endTurnIndex: 450 });
    expect(expanded).toEqual({ startTurnIndex: 51, endTurnIndex: 450 });
    expect(expandedAgain).toEqual({ startTurnIndex: 1, endTurnIndex: 450 });
  });

  test('slices visible turns inclusively and sorts by turn index', () => {
    const visible = sliceTurnsForWindow([turn(4), turn(2), turn(3), turn(1)], {
      startTurnIndex: 2,
      endTurnIndex: 3,
    });

    expect(visible.map(item => item.turnIndex)).toEqual([2, 3]);
  });

  test('detects continuous and gapped ranges', () => {
    expect(hasContinuousTurnRange([turn(1), turn(2), turn(3)], 1, 3)).toBe(true);
    expect(hasContinuousTurnRange([turn(1), turn(3)], 1, 3)).toBe(false);
    expect(hasContinuousTurnRange([turn(3), turn(4)], 1, 4)).toBe(false);
  });
});
