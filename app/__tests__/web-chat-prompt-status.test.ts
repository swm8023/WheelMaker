import {
  resolvePromptDoneStatus,
  resolvePromptTurnStatus,
  type ChatPromptStatus,
} from '../web/src/chat/chatPromptStatus';
import type { RegistryChatMessage } from '../web/src/types/registry';

function message(turnIndex: number, method: string, sessionId = 's1'): RegistryChatMessage {
  return {
    sessionId,
    turnIndex,
    method,
    param: {},
    finished: true,
  };
}

describe('web chat prompt status', () => {
  test('shows responding dots for an unfinished prompt turn', () => {
    const status: ChatPromptStatus = resolvePromptTurnStatus([
      message(1, 'prompt_request'),
      message(2, 'agent_message_chunk'),
    ], message(1, 'prompt_request'));

    expect(status).toBe('responding');
  });

  test('hides responding dots once the prompt has a done turn', () => {
    const status = resolvePromptTurnStatus([
      message(1, 'prompt_request'),
      message(2, 'agent_message_chunk'),
      message(3, 'prompt_done'),
    ], message(1, 'prompt_request'));

    expect(status).toBe(null);
  });

  test('does not let the next prompt complete the previous prompt', () => {
    const status = resolvePromptTurnStatus([
      message(1, 'prompt_request'),
      message(2, 'prompt_request'),
      message(3, 'prompt_done'),
    ], message(1, 'prompt_request'));

    expect(status).toBe('responding');
  });

  test('maps non-normal prompt stop reasons to compact status labels', () => {
    expect(resolvePromptDoneStatus({stopReason: 'cancelled'})).toEqual({
      kind: 'cancelled',
      label: 'Cancelled',
      message: '',
    });
    expect(resolvePromptDoneStatus({stopReason: 'canceled'})).toEqual({
      kind: 'cancelled',
      label: 'Cancelled',
      message: '',
    });
    expect(resolvePromptDoneStatus({stopReason: 'interrupted'})).toEqual({
      kind: 'interrupted',
      label: 'Interrupted',
      message: '',
    });
    expect(resolvePromptDoneStatus({stopReason: 'failed', message: 'agent crashed'})).toEqual({
      kind: 'failed',
      label: 'Failed',
      message: 'agent crashed',
    });
    expect(resolvePromptDoneStatus({stopReason: 'end_turn'})).toBe(null);
  });
});
