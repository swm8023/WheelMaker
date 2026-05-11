import { reconcileSessionReadMessages } from '../web/src/chatSync';
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
});
