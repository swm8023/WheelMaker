import {
  buildChatDisplayIndex,
  getChatDisplayIndexRange,
} from '../web/src/chat/chatDisplayIndex';
import type {RegistryChatMessage} from '../web/src/types/registry';

function message(
  turnIndex: number,
  method: string,
  text: string,
  finished = true,
): RegistryChatMessage {
  return {
    sessionId: 'sess-1',
    turnIndex,
    method,
    param: {text},
    finished,
  };
}

describe('chat display index', () => {
  test('stores lightweight sorted render metadata without copying message content', () => {
    const source = [
      message(3, 'prompt_done', 'done'),
      message(1, 'prompt_request', 'hello'),
      message(2, 'agent_message_chunk', 'assistant answer'),
    ];

    const index = buildChatDisplayIndex(source);

    expect(index.items.map(item => item.turnIndex)).toEqual([1, 2, 3]);
    expect(index.items.map(item => item.sourceIndex)).toEqual([1, 2, 0]);
    expect(index.totalEstimatedHeight).toBeGreaterThan(0);
    expect(Object.keys(index.items[0]).sort()).toEqual([
      'estimatedHeight',
      'key',
      'kind',
      'sourceIndex',
      'turnIndex',
    ]);
  });

  test('filters hidden turns and appends pending turn metadata', () => {
    const source = [
      message(1, 'prompt_request', 'hello'),
      message(2, 'tool_result', 'tool output'),
    ];

    const index = buildChatDisplayIndex(source, {
      shouldRender: turn => turn.method !== 'tool_result',
      pendingKey: 'pending-1',
      pendingEstimatedHeight: 88,
    });

    expect(index.items.map(item => item.kind)).toEqual(['turn', 'pending']);
    expect(index.items.map(item => item.key)).toEqual([
      'sess-1:1:prompt_request',
      'pending-1',
    ]);
  });

  test('calculates visible range from estimated heights and overscan', () => {
    const index = buildChatDisplayIndex([
      message(1, 'agent_message_chunk', 'a'.repeat(240)),
      message(2, 'agent_message_chunk', 'b'.repeat(240)),
      message(3, 'agent_message_chunk', 'c'.repeat(240)),
    ]);

    const range = getChatDisplayIndexRange(index, {
      scrollOffset: index.items[0].estimatedHeight + 1,
      viewportHeight: 1,
      overscan: 0,
    });

    expect(range.startIndex).toBe(1);
    expect(range.endIndex).toBe(2);
    expect(range.paddingTop).toBe(index.items[0].estimatedHeight);
    expect(range.paddingBottom).toBe(index.items[2].estimatedHeight);
  });
});
