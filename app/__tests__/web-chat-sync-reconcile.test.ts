import { getLatestSessionReadCursor, reconcileSessionReadMessages } from '../web/src/chatSync';
import type { RegistryChatMessage } from '../web/src/types/registry';

const message = (text: string): RegistryChatMessage => ({
  sessionId: 'sess-1',
  promptIndex: 1,
  turnIndex: 2,
  method: 'agent_message_chunk',
  param: { text },
});

describe('chat session read reconciliation', () => {
  test('keeps a live same-turn update that arrives while session.read is in flight', () => {
    const existing = [message('partial')];
    const readResult = [message('partial')];
    const freshStore = [message('partial complete')];

    const reconciled = reconcileSessionReadMessages(readResult, freshStore, existing);

    expect(reconciled).toEqual([message('partial complete')]);
  });

  test('ignores prompt_done when advancing the persisted read cursor', () => {
    const cursor = getLatestSessionReadCursor([
      {
        sessionId: 'sess-1',
        promptIndex: 1,
        turnIndex: 1,
        method: 'prompt_request',
        param: { contentBlocks: [] },
      },
      message('partial'),
      {
        sessionId: 'sess-1',
        promptIndex: 1,
        turnIndex: 3,
        method: 'prompt_done',
        param: { stopReason: 'end_turn' },
      },
    ]);

    expect(cursor).toEqual({ promptIndex: 1, turnIndex: 2 });
  });
});
