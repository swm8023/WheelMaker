describe('web chat session title display', () => {
  test('resolves title facts with first prompt by default and latest when requested', () => {
    const {
      resolveChatSessionTitle,
    } = require('../web/src/chat/chatSessionTitle');

    const facts = JSON.stringify({ first: 'first prompt', last: 'latest prompt' });

    expect(resolveChatSessionTitle(facts, false)).toBe('first prompt');
    expect(resolveChatSessionTitle(facts, true)).toBe('latest prompt');
  });

  test('keeps legacy and incomplete titles readable', () => {
    const {
      resolveChatSessionTitle,
    } = require('../web/src/chat/chatSessionTitle');

    expect(resolveChatSessionTitle('legacy title', false)).toBe('legacy title');
    expect(resolveChatSessionTitle('legacy title', true)).toBe('legacy title');
    expect(resolveChatSessionTitle(JSON.stringify({ last: 'latest only' }), false)).toBe('latest only');
    expect(resolveChatSessionTitle(JSON.stringify({ first: 'first only' }), true)).toBe('first only');
    expect(resolveChatSessionTitle('{bad json', true)).toBe('{bad json');
  });
});
