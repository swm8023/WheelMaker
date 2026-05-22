describe('web chat session title display', () => {
  test('resolves manual title before automatic title facts', () => {
    const {
      resolveChatSessionTitle,
    } = require('../web/src/chat/chatSessionTitle');

    const facts = JSON.stringify({ first: 'first prompt', last: 'latest prompt', manual: 'manual title' });

    expect(resolveChatSessionTitle(facts)).toBe('manual title');
  });

  test('uses first prompt by default and keeps legacy and incomplete titles readable', () => {
    const {
      resolveChatSessionTitle,
    } = require('../web/src/chat/chatSessionTitle');

    expect(resolveChatSessionTitle(JSON.stringify({ first: 'first prompt', last: 'latest prompt' }))).toBe('first prompt');
    expect(resolveChatSessionTitle('legacy title')).toBe('legacy title');
    expect(resolveChatSessionTitle(JSON.stringify({ last: 'latest only' }))).toBe('latest only');
    expect(resolveChatSessionTitle(JSON.stringify({ first: 'first only' }))).toBe('first only');
    expect(resolveChatSessionTitle('{bad json')).toBe('{bad json');
  });
});
