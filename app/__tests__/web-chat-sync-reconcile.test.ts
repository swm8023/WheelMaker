import {
  getLatestSessionReadCursor,
  needsPromptTurnRefresh,
  reconcileSessionReadMessages,
  replacePromptMessages,
} from '../web/src/chatSync';
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

    expect(cursor).toEqual({ promptIndex: 1, turnIndex: 1 });
  });

  test('advances the persisted read cursor after a text turn is done', () => {
    const cursor = getLatestSessionReadCursor([
      {
        sessionId: 'sess-1',
        promptIndex: 1,
        turnIndex: 1,
        method: 'prompt_request',
        param: { contentBlocks: [] },
      },
      { ...message('complete'), done: true },
    ]);

    expect(cursor).toEqual({ promptIndex: 1, turnIndex: 2 });
  });

  test('detects open or missing text turns when prompt_done arrives', () => {
    const promptDone: RegistryChatMessage = {
      sessionId: 'sess-1',
      promptIndex: 1,
      turnIndex: 4,
      method: 'prompt_done',
      param: { stopReason: 'end_turn' },
    };

    expect(needsPromptTurnRefresh([message('partial')], promptDone)).toBe(true);
    expect(needsPromptTurnRefresh([{ ...message('complete'), done: true }], promptDone)).toBe(true);
    expect(
      needsPromptTurnRefresh(
        [
          { sessionId: 'sess-1', promptIndex: 1, turnIndex: 1, method: 'prompt_request', param: {} },
          { ...message('complete'), done: true },
          {
            sessionId: 'sess-1',
            promptIndex: 1,
            turnIndex: 3,
            method: 'agent_thought_chunk',
            param: { text: 'done thinking' },
            done: true,
          },
        ],
        promptDone,
      ),
    ).toBe(false);
  });

  test('replaces only the requested prompt when refreshing prompt turns', () => {
    const refreshed = replacePromptMessages(
      [
        { sessionId: 'sess-1', promptIndex: 1, turnIndex: 1, method: 'prompt_request', param: {} },
        message('stale'),
        { sessionId: 'sess-1', promptIndex: 1, turnIndex: 3, method: 'prompt_done', param: { stopReason: 'end_turn' } },
        { sessionId: 'sess-1', promptIndex: 2, turnIndex: 1, method: 'prompt_request', param: {} },
      ],
      [
        { sessionId: 'sess-1', promptIndex: 1, turnIndex: 1, method: 'prompt_request', param: {} },
        { ...message('complete'), done: true },
      ],
      1,
    );

    expect(refreshed).toEqual([
      { sessionId: 'sess-1', promptIndex: 1, turnIndex: 1, method: 'prompt_request', param: {} },
      { ...message('complete'), done: true },
      { sessionId: 'sess-1', promptIndex: 1, turnIndex: 3, method: 'prompt_done', param: { stopReason: 'end_turn' } },
      { sessionId: 'sess-1', promptIndex: 2, turnIndex: 1, method: 'prompt_request', param: {} },
    ]);
  });
});
