import { buildPromptDoneCopyRange } from '../web/src/chat/chatCopyRange';
import type { RegistryChatMessage } from '../web/src/types/registry';

function message(
  turnIndex: number,
  method: string,
  text = '',
): RegistryChatMessage {
  return {
    sessionId: 's1',
    turnIndex,
    method,
    param: text ? { text } : {},
    finished: true,
  };
}

describe('chat prompt-done copy range', () => {
  test('copies only agent message turns from the complete prompt range', () => {
    const result = buildPromptDoneCopyRange([
      message(1, 'prompt_request', 'question'),
      message(2, 'agent_thought_chunk', 'hidden thought'),
      message(3, 'agent_message_chunk', 'First paragraph.'),
      message(4, 'tool_call', 'tool output'),
      message(5, 'agent_plan', 'plan item'),
      message(6, 'agent_message_chunk', '- item one\n- item two'),
      message(7, 'prompt_done', 'end_turn'),
    ], 7);

    expect(result).toEqual({
      ok: true,
      markdown: 'First paragraph.\n\n- item one\n- item two',
      startTurnIndex: 1,
      endTurnIndex: 7,
    });
  });

  test('disables copy when the request turn is missing', () => {
    expect(buildPromptDoneCopyRange([
      message(3, 'agent_message_chunk', 'answer'),
      message(4, 'prompt_done', 'end_turn'),
    ], 4)).toEqual({ ok: false, reason: 'missing_request' });
  });

  test('disables copy when the full store has a gap in the range', () => {
    expect(buildPromptDoneCopyRange([
      message(1, 'prompt_request', 'question'),
      message(3, 'agent_message_chunk', 'answer'),
      message(4, 'prompt_done', 'end_turn'),
    ], 4)).toEqual({ ok: false, reason: 'gap' });
  });

  test('disables copy when no agent message turn is present', () => {
    expect(buildPromptDoneCopyRange([
      message(1, 'prompt_request', 'question'),
      message(2, 'agent_thought_chunk', 'hidden'),
      message(3, 'tool_call', 'tool'),
      message(4, 'prompt_done', 'end_turn'),
    ], 4)).toEqual({ ok: false, reason: 'empty_agent_response' });
  });
});
