import { buildPromptAgentMarkdown } from '../chatPromptCopy';
import type { RegistryChatMessage } from '../types/registry';
import { hasContinuousTurnRange } from './chatTurnWindow';

export type ChatCopyRangeResult =
  | {
      ok: true;
      markdown: string;
      startTurnIndex: number;
      endTurnIndex: number;
    }
  | {
      ok: false;
      reason:
        | 'missing_done'
        | 'missing_request'
        | 'gap'
        | 'empty_agent_response';
    };

function textFromParam(param: Record<string, unknown>): string {
  if (typeof param.text === 'string') {
    return param.text;
  }
  if (typeof param.output === 'string') {
    return param.output;
  }
  if (Array.isArray(param.contentBlocks)) {
    return param.contentBlocks
      .map(item => {
        if (!item || typeof item !== 'object') {
          return '';
        }
        const block = item as Record<string, unknown>;
        return typeof block.text === 'string' ? block.text : '';
      })
      .filter(Boolean)
      .join('\n');
  }
  return '';
}

export function buildPromptDoneCopyRange(
  turns: RegistryChatMessage[],
  doneTurnIndex: number,
): ChatCopyRangeResult {
  const ordered = [...turns].sort((left, right) => (left.turnIndex ?? 0) - (right.turnIndex ?? 0));
  const doneIndex = ordered.findIndex(
    message =>
      message.method === 'prompt_done' &&
      Math.trunc(Number(message.turnIndex ?? 0)) === doneTurnIndex,
  );
  if (doneIndex < 0) {
    return { ok: false, reason: 'missing_done' };
  }

  let requestIndex = -1;
  for (let index = doneIndex - 1; index >= 0; index -= 1) {
    if (ordered[index].method === 'prompt_request') {
      requestIndex = index;
      break;
    }
  }
  if (requestIndex < 0) {
    return { ok: false, reason: 'missing_request' };
  }

  const startTurnIndex = Math.trunc(Number(ordered[requestIndex].turnIndex ?? 0));
  const endTurnIndex = Math.trunc(Number(ordered[doneIndex].turnIndex ?? 0));
  if (!hasContinuousTurnRange(ordered, startTurnIndex, endTurnIndex)) {
    return { ok: false, reason: 'gap' };
  }

  const markdown = buildPromptAgentMarkdown(
    ordered
      .slice(requestIndex + 1, doneIndex)
      .filter(message => message.method === 'agent_message_chunk')
      .map(message => ({
        kind: 'message',
        text: textFromParam(message.param),
      })),
  );
  if (!markdown) {
    return { ok: false, reason: 'empty_agent_response' };
  }

  return {
    ok: true,
    markdown,
    startTurnIndex,
    endTurnIndex,
  };
}
