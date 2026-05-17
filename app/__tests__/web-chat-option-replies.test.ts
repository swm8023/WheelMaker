import {extractChatOptionReplies} from '../web/src/chat/chatOptionReplies';

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
});
