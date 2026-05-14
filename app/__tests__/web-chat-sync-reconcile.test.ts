import {
  getLatestSessionReadCursor,
  reconcileCachedSessionReadCursor,
  needsPromptTurnRefresh,
  reconcileSessionReadMessages,
  replaceSessionMessages,
  shouldRequestSessionReadForIncomingTurn,
} from '../web/src/chatSync';
import type { RegistryChatMessage } from '../web/src/types/registry';

const message = (text: string): RegistryChatMessage => ({
  sessionId: 'sess-1',
  turnIndex: 2,
  method: 'agent_message_chunk',
  param: { text },
  finished: false,
});

describe('chat session read reconciliation', () => {
  test('keeps a live same-turn update that arrives while session.read is in flight', () => {
    const existing = [message('partial')];
    const readResult = [message('partial')];
    const freshStore = [message('partial complete')];

    const reconciled = reconcileSessionReadMessages(readResult, freshStore, existing);

    expect(reconciled).toEqual([message('partial complete')]);
  });

  test('advances cursor only for finished turns', () => {
    const cursor = getLatestSessionReadCursor([
      {
        sessionId: 'sess-1',
        turnIndex: 1,
        method: 'prompt_request',
        param: { contentBlocks: [] },
        finished: true,
      },
      {
        ...message('partial'),
        finished: false,
      },
    ]);

    expect(cursor).toEqual({ turnIndex: 1 });
  });

  test('treats prompt_done as a normal finished cursor turn', () => {
    const cursor = getLatestSessionReadCursor([
      {
        sessionId: 'sess-1',
        turnIndex: 1,
        method: 'prompt_request',
        param: { contentBlocks: [] },
        finished: true,
      },
      {
        sessionId: 'sess-1',
        turnIndex: 2,
        method: 'prompt_done',
        param: { stopReason: 'end_turn' },
        finished: true,
      },
    ]);

    expect(cursor).toEqual({ turnIndex: 2 });
  });

  test('advances the persisted read cursor after a text turn is done', () => {
    const cursor = getLatestSessionReadCursor([
      {
        sessionId: 'sess-1',
        turnIndex: 1,
        method: 'prompt_request',
        param: { contentBlocks: [] },
        finished: true,
      },
      { ...message('complete'), finished: true },
    ]);

    expect(cursor).toEqual({ turnIndex: 2 });
  });

  test('restores cursor from contiguous cached content instead of trusting stale session index', () => {
    expect(
      reconcileCachedSessionReadCursor(
        { turnIndex: 5 },
        [
          { sessionId: 'sess-1', turnIndex: 1, method: 'prompt_request', param: {}, finished: true },
          { sessionId: 'sess-1', turnIndex: 3, method: 'prompt_done', param: {}, finished: true },
        ],
      ),
    ).toEqual({ turnIndex: 1 });

    expect(reconcileCachedSessionReadCursor({ turnIndex: 5 }, [])).toEqual({ turnIndex: 0 });
  });

  test('does not request prompt reconstruction in turn-only protocol', () => {
    const promptDone: RegistryChatMessage = {
      sessionId: 'sess-1',
      turnIndex: 4,
      method: 'prompt_done',
      param: { stopReason: 'end_turn' },
      finished: true,
    };

    expect(needsPromptTurnRefresh([message('partial')], promptDone)).toBe(false);
    expect(needsPromptTurnRefresh([{ ...message('complete'), finished: true }], promptDone)).toBe(false);
    expect(
      needsPromptTurnRefresh(
        [
          { sessionId: 'sess-1', turnIndex: 1, method: 'prompt_request', param: {}, finished: true },
          { ...message('complete'), finished: true },
          {
            sessionId: 'sess-1',
            turnIndex: 3,
            method: 'agent_thought_chunk',
            param: { text: 'done thinking' },
            finished: true,
          },
        ],
        promptDone,
      ),
    ).toBe(false);
  });

  test('keeps checkpointed turns and merges returned turn delta', () => {
    const refreshed = replaceSessionMessages(
      [
        { sessionId: 'sess-1', turnIndex: 1, method: 'prompt_request', param: {}, finished: true },
        message('stale'),
        { sessionId: 'sess-1', turnIndex: 3, method: 'prompt_done', param: { stopReason: 'end_turn' }, finished: true },
        { sessionId: 'sess-1', turnIndex: 4, method: 'prompt_request', param: {}, finished: true },
      ],
      [
        { ...message('complete'), finished: true },
        { sessionId: 'sess-1', turnIndex: 3, method: 'prompt_done', param: { stopReason: 'end_turn' }, finished: true },
        { sessionId: 'sess-1', turnIndex: 4, method: 'prompt_request', param: {}, finished: true },
      ],
      1,
    );

    expect(refreshed).toEqual([
      { sessionId: 'sess-1', turnIndex: 1, method: 'prompt_request', param: {}, finished: true },
      { ...message('complete'), finished: true },
      { sessionId: 'sess-1', turnIndex: 3, method: 'prompt_done', param: { stopReason: 'end_turn' }, finished: true },
      { sessionId: 'sess-1', turnIndex: 4, method: 'prompt_request', param: {}, finished: true },
    ]);
  });

  test('does not request a read for the next contiguous turn', () => {
    const local = {
      cursor: { turnIndex: 3 },
    };
    const incoming: RegistryChatMessage = {
      sessionId: 'sess-1',
      turnIndex: 4,
      method: 'prompt_request',
      param: {},
      finished: true,
    };

    expect(shouldRequestSessionReadForIncomingTurn(local, incoming)).toBeNull();
  });

  test('requests a read when a turn gap is detected', () => {
    const local = {
      cursor: { turnIndex: 3 },
    };

    expect(shouldRequestSessionReadForIncomingTurn(local, {
      sessionId: 'sess-1',
      turnIndex: 5,
      method: 'agent_message_chunk',
      param: {},
      finished: true,
    })).toEqual({ turnIndex: 3 });
    expect(shouldRequestSessionReadForIncomingTurn(local, {
      sessionId: 'sess-1',
      turnIndex: 4,
      method: 'agent_message_chunk',
      param: {},
      finished: true,
    })).toBeNull();
  });
});
