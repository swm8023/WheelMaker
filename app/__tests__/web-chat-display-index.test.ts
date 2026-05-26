import fs from 'fs';
import path from 'path';
import {buildChatDisplayIndex, resolveChatDisplayScrollIndex} from '../web/src/chat/chatDisplayIndex';
import type {RegistryChatMessage} from '../web/src/types/registry';

function message(
  turnIndex: number,
  method: string,
  text: string,
  finished = true,
): RegistryChatMessage {
  return {
    sessionId: 'sess-1',
    turnIndex,
    method,
    param: {text},
    finished,
  };
}

describe('chat display index', () => {
  test('stores lightweight sorted render metadata without copying message content', () => {
    const source = [
      message(3, 'prompt_done', 'done'),
      message(1, 'prompt_request', 'hello'),
      message(2, 'agent_message_chunk', 'assistant answer'),
    ];

    const index = buildChatDisplayIndex(source);

    expect(index.items.map(item => item.turnIndex)).toEqual([1, 2, 3]);
    expect(index.items.map(item => item.sourceIndex)).toEqual([1, 2, 0]);
    expect(Object.keys(index.items[0]).sort()).toEqual([
      'estimatedHeight',
      'key',
      'kind',
      'sourceIndex',
      'turnIndex',
    ]);
  });

  test('filters hidden turns and appends pending turn metadata', () => {
    const source = [
      message(1, 'prompt_request', 'hello'),
      message(2, 'tool_result', 'tool output'),
    ];

    const index = buildChatDisplayIndex(source, {
      shouldRender: turn => turn.method !== 'tool_result',
      pendingKey: 'pending-1',
      pendingEstimatedHeight: 88,
    });

    expect(index.items.map(item => item.kind)).toEqual(['turn', 'pending']);
    expect(index.items.map(item => item.key)).toEqual([
      'sess-1:1:prompt_request',
      'pending-1',
    ]);
  });

  test('uses turn type and layout width when estimating dynamic chat heights', () => {
    const assistantText = [
      'This is a long assistant response that should wrap differently depending on the chat column width.',
      '',
      'A second paragraph keeps markdown-style block spacing in the estimate.',
    ].join('\n');
    const wide = buildChatDisplayIndex([message(1, 'agent_message_chunk', assistantText)], {
      layoutMetrics: {contentWidth: 900},
    });
    const narrow = buildChatDisplayIndex([message(1, 'agent_message_chunk', assistantText)], {
      layoutMetrics: {contentWidth: 320},
    });

    expect(narrow.items[0].estimatedHeight).toBeGreaterThan(wide.items[0].estimatedHeight);
  });

  test('keeps tool calls compact when visible and removes them when hidden', () => {
    const source = [
      message(1, 'prompt_request', 'hello'),
      message(2, 'tool_call', 'x'.repeat(2000)),
      message(3, 'agent_message_chunk', 'answer'),
    ];
    const visible = buildChatDisplayIndex(source, {hideToolCalls: false});
    const hidden = buildChatDisplayIndex(source, {hideToolCalls: true});

    expect(visible.items.map(item => item.turnIndex)).toEqual([1, 2, 3]);
    expect(hidden.items.map(item => item.turnIndex)).toEqual([1, 3]);
    expect(visible.items[1].estimatedHeight).toBeLessThan(48);
  });

  test('resolves turn jump to exact or nearest visible display item', () => {
    const source = [
      message(1, 'prompt_request', 'hello'),
      message(2, 'tool_call', 'hidden tool'),
      message(3, 'agent_message_chunk', 'answer'),
    ];
    const index = buildChatDisplayIndex(source, {hideToolCalls: true});

    expect(resolveChatDisplayScrollIndex(index, 1)).toBe(0);
    expect(resolveChatDisplayScrollIndex(index, 2)).toBe(1);
    expect(resolveChatDisplayScrollIndex(index, 4)).toBe(1);
    expect(resolveChatDisplayScrollIndex({items: []}, 1)).toBe(null);
  });

  test('accounts for prompt status and explicit user newlines', () => {
    const compactPrompt = message(1, 'prompt_request', 'one line');
    compactPrompt.param = {contentBlocks: [{type: 'text', text: 'one line'}]};
    const multilinePrompt = message(2, 'prompt_request', 'line one\nline two\nline three');
    multilinePrompt.param = {contentBlocks: [{type: 'text', text: 'line one\nline two\nline three'}]};

    const compact = buildChatDisplayIndex([compactPrompt], {
      layoutMetrics: {contentWidth: 360},
      promptStatus: () => null,
    });
    const multiline = buildChatDisplayIndex([multilinePrompt], {
      layoutMetrics: {contentWidth: 360},
      promptStatus: () => 'responding',
    });

    expect(multiline.items[0].estimatedHeight).toBeGreaterThan(compact.items[0].estimatedHeight);
  });

  test('reserves visible prompt height for attachment-only content blocks', () => {
    const attachmentOnlyPrompt = message(1, 'prompt_request', '');
    attachmentOnlyPrompt.param = {
      contentBlocks: [
        {
          type: 'resource_link',
          uri: 'file:///D:/Code/WheelMaker/docs/spec.pdf',
          name: 'spec.pdf',
          mimeType: 'application/pdf',
          size: 42_000,
        },
      ],
    };
    const emptyPrompt = message(2, 'prompt_request', '');
    emptyPrompt.param = {contentBlocks: []};

    const index = buildChatDisplayIndex([attachmentOnlyPrompt, emptyPrompt], {
      layoutMetrics: {contentWidth: 360},
      promptStatus: () => null,
    });

    expect(index.items[0].estimatedHeight).toBeGreaterThan(index.items[1].estimatedHeight);
  });

  test('does not keep a manual virtual range implementation', () => {
    const source = fs.readFileSync(
      path.join(__dirname, '..', 'web', 'src', 'chat', 'chatDisplayIndex.ts'),
      'utf8',
    );

    expect(source).not.toContain('getChatDisplayIndexRange');
    expect(source).not.toContain('ChatDisplayRange');
    expect(source).not.toContain('paddingTop');
    expect(source).not.toContain('paddingBottom');
    expect(source).not.toContain('totalEstimatedHeight');
  });
});
