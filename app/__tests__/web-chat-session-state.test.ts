import { getChatSessionVisualState } from '../web/src/chatSessionState';

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
});
