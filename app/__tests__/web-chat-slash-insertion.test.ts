import {insertChatSlashCommandText} from '../web/src/chat/chatSlashInsertion';

describe('chat slash skill insertion', () => {
  test('inserts a skill command into existing composer text without replacing it', () => {
    expect(insertChatSlashCommandText('Please review this', '/grill-me', 6, 6)).toEqual({
      text: 'Please /grill-me review this',
      selectionStart: 17,
      selectionEnd: 17,
    });
  });

  test('replaces an active slash query instead of appending a duplicate command', () => {
    expect(insertChatSlashCommandText('/gri', '/grill-me', 4, 4)).toEqual({
      text: '/grill-me ',
      selectionStart: 10,
      selectionEnd: 10,
    });
  });
});
