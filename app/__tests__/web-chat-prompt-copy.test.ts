import { buildPromptAgentMarkdown } from '../web/src/chatPromptCopy';

describe('web chat prompt copy', () => {
  test('builds markdown lazily from agent messages only', () => {
    const markdown = buildPromptAgentMarkdown([
      {
        kind: 'thought',
        text: 'hidden reasoning',
      },
      {
        kind: 'tool',
        text: 'shell command',
      },
      {
        kind: 'message',
        text: 'First **markdown** paragraph.',
      },
      {
        kind: 'plan',
        text: 'not copied plan',
      },
      {
        kind: 'message',
        text: '- item one\n- item two',
      },
    ]);

    expect(markdown).toBe('First **markdown** paragraph.\n\n- item one\n- item two');
    expect(markdown).not.toContain('hidden reasoning');
    expect(markdown).not.toContain('shell command');
    expect(markdown).not.toContain('not copied plan');
  });
});
