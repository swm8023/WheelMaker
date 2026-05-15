import {
  getChatSessionVisualState,
  isChatSessionRunningMessage,
} from '../web/src/chatSessionState';

describe('web chat session visual state', () => {
  test('prioritizes running, then unread failed, then unread completed', () => {
    expect(getChatSessionVisualState({
      sessionId: 's1',
      title: '',
      preview: '',
      updatedAt: '',
      messageCount: 0,
      running: true,
      lastDoneTurnIndex: 4,
      lastDoneSuccess: false,
      lastReadTurnIndex: 0,
    })).toBe('running');

    expect(getChatSessionVisualState({
      sessionId: 's2',
      title: '',
      preview: '',
      updatedAt: '',
      messageCount: 0,
      running: false,
      lastDoneTurnIndex: 4,
      lastDoneSuccess: false,
      lastReadTurnIndex: 3,
    })).toBe('failed-unviewed');

    expect(getChatSessionVisualState({
      sessionId: 's3',
      title: '',
      preview: '',
      updatedAt: '',
      messageCount: 0,
      running: false,
      lastDoneTurnIndex: 4,
      lastDoneSuccess: true,
      lastReadTurnIndex: 3,
    })).toBe('completed-unviewed');

    expect(getChatSessionVisualState({
      sessionId: 's4',
      title: '',
      preview: '',
      updatedAt: '',
      messageCount: 0,
      running: false,
      lastDoneTurnIndex: 4,
      lastDoneSuccess: true,
      lastReadTurnIndex: 4,
    })).toBe('idle');
  });

  test('marks a session running from prompt start and streaming turns', () => {
    expect(isChatSessionRunningMessage({
      sessionId: 's1',
      turnIndex: 1,
      method: 'prompt_request',
      param: {},
      finished: true,
    })).toBe(true);

    expect(isChatSessionRunningMessage({
      sessionId: 's1',
      turnIndex: 2,
      method: 'agent_message_chunk',
      param: {text: 'partial'},
      finished: false,
    })).toBe(true);

    expect(isChatSessionRunningMessage({
      sessionId: 's1',
      turnIndex: 3,
      method: 'prompt_done',
      param: {stopReason: 'end_turn'},
      finished: true,
    })).toBe(false);
  });
});
