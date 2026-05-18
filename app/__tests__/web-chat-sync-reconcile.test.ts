import {
  getLatestSessionReadCursor,
  reconcileCachedSessionReadCursor,
  needsPromptTurnRefresh,
  mergeIncomingSessionMessage,
  reconcileSessionReadMessages,
  replaceSessionMessages,
  sanitizeCachedSessionMessages,
  shouldRequestSessionReadForIncomingTurn,
} from '../web/src/chatSync';
import {
  decodeSessionTurnToMessage,
  normalizeSessionMessagePayload,
  normalizeSessionReadPayload,
  normalizeSessionWireTurn,
} from '../web/src/chat/chatWire';
import type { RegistryChatMessage } from '../web/src/types/registry';

const message = (text: string): RegistryChatMessage => ({
  sessionId: 'sess-1',
  turnIndex: 2,
  method: 'agent_message_chunk',
  param: { text },
  finished: false,
});

describe('chat session read reconciliation', () => {
  test('normalizes raw session.message payload and rejects legacy flat payload', () => {
    const payload = {
      sessionId: 'sess-1',
      turn: {
        turnIndex: 4,
        content: '{"method":"agent_message_chunk","param":{"text":"raw"}}',
        finished: false,
      },
    };

    expect(normalizeSessionMessagePayload(payload)).toEqual({
      sessionId: 'sess-1',
      turn: {
        turnIndex: 4,
        content: '{"method":"agent_message_chunk","param":{"text":"raw"}}',
        finished: false,
      },
    });
    expect(normalizeSessionMessagePayload({
      sessionId: 'sess-1',
      turnIndex: 4,
      content: '{}',
      finished: true,
    })).toBeNull();
  });

  test('normalizes raw session.read payload only when top-level session id matches', () => {
    const payload = {
      sessionId: 'sess-1',
      latestTurnIndex: 7,
      session: {sessionId: 'sess-1', title: 'Task', updatedAt: '', messageCount: 0},
      turns: [
        {turnIndex: 6, content: '{"method":"agent_message_chunk","param":{"text":"ok"}}', finished: true},
      ],
    };

    expect(normalizeSessionReadPayload(payload, 'sess-1')?.turns).toEqual([
      {turnIndex: 6, content: '{"method":"agent_message_chunk","param":{"text":"ok"}}', finished: true},
    ]);
    expect(normalizeSessionReadPayload({...payload, sessionId: 'other'}, 'sess-1')).toBeNull();
  });

  test('raw turn normalization does not parse content but decode can parse for rendering', () => {
    const raw = normalizeSessionWireTurn({
      turnIndex: 2,
      content: '{not valid json',
      finished: false,
    });
    expect(raw).toEqual({turnIndex: 2, content: '{not valid json', finished: false});

    expect(decodeSessionTurnToMessage('sess-1', {
      turnIndex: 2,
      content: '{"method":"agent_message_chunk","param":{"text":"hello"}}',
      finished: true,
    })).toEqual({
      sessionId: 'sess-1',
      turnIndex: 2,
      method: 'agent_message_chunk',
      param: {text: 'hello'},
      finished: true,
    });
  });

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

  test('repairs cached session messages before they drive cursors and windows', () => {
    const repaired = sanitizeCachedSessionMessages(
      [
        { sessionId: 'sess-1', turnIndex: 3, method: 'prompt_done', param: {}, finished: true },
        { sessionId: 'other', turnIndex: 2, method: 'agent_message_chunk', param: {}, finished: true },
        { sessionId: 'sess-1', turnIndex: 0, method: 'agent_message_chunk', param: {}, finished: true },
        { sessionId: 'sess-1', turnIndex: 2, method: 'agent_message_chunk', param: { text: 'stale' }, finished: true },
        { sessionId: 'sess-1', turnIndex: 1, method: 'prompt_request', param: {}, finished: true },
        { sessionId: 'sess-1', turnIndex: 2, method: 'agent_message_chunk', param: { text: 'fresh' }, finished: true },
        null,
      ],
      'sess-1',
    );

    expect(repaired).toEqual([
      { sessionId: 'sess-1', turnIndex: 1, method: 'prompt_request', param: {}, finished: true },
      { sessionId: 'sess-1', turnIndex: 2, method: 'agent_message_chunk', param: { text: 'fresh' }, finished: true },
      { sessionId: 'sess-1', turnIndex: 3, method: 'prompt_done', param: {}, finished: true },
    ]);
    expect(reconcileCachedSessionReadCursor({ turnIndex: 8 }, repaired)).toEqual({ turnIndex: 3 });
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

  test('does not request a read when unfinished local turns are contiguous', () => {
    const local = {
      cursor: { turnIndex: 3 },
      messages: [
        {
          sessionId: 'sess-1',
          turnIndex: 4,
          method: 'prompt_request',
          param: {},
          finished: false,
        },
      ],
    };

    expect(shouldRequestSessionReadForIncomingTurn(local, {
      sessionId: 'sess-1',
      turnIndex: 5,
      method: 'agent_message_chunk',
      param: {},
      finished: false,
    })).toBeNull();
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

  test('requests a gap read from the latest contiguous local turn', () => {
    const local = {
      cursor: { turnIndex: 3 },
      messages: [
        {
          sessionId: 'sess-1',
          turnIndex: 4,
          method: 'prompt_request',
          param: {},
          finished: false,
        },
      ],
    };

    expect(shouldRequestSessionReadForIncomingTurn(local, {
      sessionId: 'sess-1',
      turnIndex: 6,
      method: 'agent_message_chunk',
      param: {},
      finished: false,
    })).toEqual({ turnIndex: 4 });
  });

  test('keeps an incoming live turn when also requesting a gap read', () => {
    const incoming: RegistryChatMessage = {
      sessionId: 'sess-1',
      turnIndex: 5,
      method: 'agent_message_chunk',
      param: { text: 'live turn' },
      finished: false,
    };

    const result = mergeIncomingSessionMessage({
      cursor: { turnIndex: 3 },
      messages: [
        { sessionId: 'sess-1', turnIndex: 1, method: 'prompt_request', param: {}, finished: true },
        { sessionId: 'sess-1', turnIndex: 2, method: 'agent_message_chunk', param: { text: 'done' }, finished: true },
        { sessionId: 'sess-1', turnIndex: 3, method: 'prompt_done', param: { stopReason: 'end_turn' }, finished: true },
      ],
    }, incoming);

    expect(result.gapReadCursor).toEqual({ turnIndex: 3 });
    expect(result.messages).toContainEqual(incoming);
    expect(result.cursor).toEqual({ turnIndex: 3 });
  });
});
