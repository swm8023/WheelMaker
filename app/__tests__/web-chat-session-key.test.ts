import {
  chatSessionKeyFromParts,
  decodeChatSessionKey,
  encodeChatSessionKey,
  sameChatSessionKey,
} from '../web/src/chat/chatSessionKey';

describe('chat session key helpers', () => {
  test('encodes a project-scoped session key distinctly from the session id', () => {
    const key = { projectId: 'project:one', sessionId: 'session/one' };

    const encoded = encodeChatSessionKey(key);

    expect(encoded).not.toBe(key.sessionId);
    expect(decodeChatSessionKey(encoded)).toEqual(key);
  });

  test('returns an empty encoded key when either part is missing', () => {
    expect(encodeChatSessionKey(null)).toBe('');
    expect(encodeChatSessionKey({ projectId: '', sessionId: 's1' })).toBe('');
    expect(encodeChatSessionKey({ projectId: 'p1', sessionId: '' })).toBe('');
    expect(decodeChatSessionKey('')).toBeNull();
  });

  test('trims boundary input and compares both project and session', () => {
    const key = chatSessionKeyFromParts(' p1 ', ' s1 ');

    expect(key).toEqual({ projectId: 'p1', sessionId: 's1' });
    expect(sameChatSessionKey(key, { projectId: 'p1', sessionId: 's1' })).toBe(true);
    expect(sameChatSessionKey(key, { projectId: 'p2', sessionId: 's1' })).toBe(false);
    expect(sameChatSessionKey(key, { projectId: 'p1', sessionId: 's2' })).toBe(false);
    expect(sameChatSessionKey(null, null)).toBe(true);
  });
});
