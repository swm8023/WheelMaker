import {extractChatOptionReplies, splitChatOptionReplyText} from '../web/src/chat/chatOptionReplies';

describe('chat option reply extraction', () => {
  test('extracts contiguous A/B/C line-start choices', () => {
    expect(extractChatOptionReplies([
      'Next step?',
      '',
      'A. Refresh current project',
      'B. Refresh every project',
      'C. Use local cache first',
    ].join('\n'))).toEqual([
      {label: 'A', text: 'Refresh current project'},
      {label: 'B', text: 'Refresh every project'},
      {label: 'C', text: 'Use local cache first'},
    ]);
  });

  test('rejects summaries and plan labels that are not active choices', () => {
    expect(extractChatOptionReplies([
      'Samples:',
      '- `A. Direct send / B. Pick from list / C. Insert only`',
      '',
      '方案 A，推荐：左上按钮变成快捷语句菜单。',
      '方案 B：直接铺开 5 个快捷语句按钮。',
    ].join('\n'))).toEqual([]);
  });

  test('ignores code fences and requires at least A and B', () => {
    expect(extractChatOptionReplies([
      '```text',
      'A. Not a choice',
      'B. Still code',
      '```',
      '',
      'A. Only one visible choice',
    ].join('\n'))).toEqual([]);
  });

  test('splits the latest choice block so the original option lines can be replaced inline', () => {
    expect(splitChatOptionReplyText([
      'Pick one:',
      '',
      'A. Apply the small change',
      'B. Keep the existing behavior',
      '',
      'Only reply with the letter.',
    ].join('\n'))).toEqual([
      {type: 'markdown', text: 'Pick one:\n\n'},
      {type: 'option', reply: {label: 'A', text: 'Apply the small change'}},
      {type: 'option', reply: {label: 'B', text: 'Keep the existing behavior'}},
      {type: 'markdown', text: '\n\nOnly reply with the letter.'},
    ]);
  });

  test('splits only the latest valid choice block in a message', () => {
    expect(splitChatOptionReplyText([
      'Previous notes:',
      'A. Old option',
      'B. Old alternative',
      '',
      'Current choice:',
      'A. Send A',
      'B. Send B',
    ].join('\n'))).toEqual([
      {type: 'markdown', text: 'Previous notes:\nA. Old option\nB. Old alternative\n\nCurrent choice:\n'},
      {type: 'option', reply: {label: 'A', text: 'Send A'}},
      {type: 'option', reply: {label: 'B', text: 'Send B'}},
    ]);
  });
});
