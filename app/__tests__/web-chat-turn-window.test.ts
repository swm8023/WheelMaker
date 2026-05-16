import {
  createLatestTurnWindow,
  expandTurnWindowEarlier,
  expandTurnWindowLater,
  followLatestTurnWindow,
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
  test('creates the latest bounded window by message offset with future buffer', () => {
    const turns = Array.from({ length: 250 }, (_, index) => turn(index + 1));

    expect(createLatestTurnWindow(turns)).toEqual({
      startIndex: 150,
      endIndex: 350,
    });
  });

  test('moves the fixed-size window earlier', () => {
    const turns = Array.from({ length: 450 }, (_, index) => turn(index + 1));
    const latest = createLatestTurnWindow(turns);

    const expanded = expandTurnWindowEarlier(turns, latest);
    const expandedAgain = expandTurnWindowEarlier(turns, expanded);

    expect(latest).toEqual({ startIndex: 350, endIndex: 550 });
    expect(expanded).toEqual({ startIndex: 250, endIndex: 450 });
    expect(expandedAgain).toEqual({ startIndex: 150, endIndex: 350 });
  });

  test('moves the fixed-size window later toward latest', () => {
    const turns = Array.from({ length: 450 }, (_, index) => turn(index + 1));

    const moved = expandTurnWindowLater(turns, {startIndex: 150, endIndex: 350});
    const latest = expandTurnWindowLater(turns, moved);

    expect(moved).toEqual({startIndex: 250, endIndex: 450});
    expect(latest).toEqual({startIndex: 350, endIndex: 550});
  });

  test('follow mode refreshes latest only when future buffer is low', () => {
    const turns = Array.from({ length: 250 }, (_, index) => turn(index + 1));

    expect(followLatestTurnWindow(turns, {startIndex: 50, endIndex: 300})).toEqual({
      startIndex: 50,
      endIndex: 300,
    });
    expect(followLatestTurnWindow(turns, {startIndex: 25, endIndex: 274})).toEqual({
      startIndex: 150,
      endIndex: 350,
    });
  });

  test('slices visible turns by message offset and sorts by turn index', () => {
    const visible = sliceTurnsForWindow([turn(4), turn(2), turn(3), turn(1)], {
      startIndex: 1,
      endIndex: 3,
    });

    expect(visible.map(item => item.turnIndex)).toEqual([2, 3]);
  });

  test('detects continuous and gapped ranges', () => {
    expect(hasContinuousTurnRange([turn(1), turn(2), turn(3)], 1, 3)).toBe(true);
    expect(hasContinuousTurnRange([turn(1), turn(3)], 1, 3)).toBe(false);
    expect(hasContinuousTurnRange([turn(3), turn(4)], 1, 4)).toBe(false);
  });
});
