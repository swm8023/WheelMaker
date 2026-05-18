import type {
  RegistrySessionMessage,
  RegistrySessionMessageEventPayload,
  RegistrySessionReadResponse,
  RegistrySessionSummary,
  RegistrySessionTurn,
} from '../types/registry';

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null;
}

function normalizeTurnIndex(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.trunc(value)
    : 0;
}

export function normalizeSessionWireTurn(raw: unknown): RegistrySessionTurn | null {
  const input = asRecord(raw);
  if (!input) return null;
  const turnIndex = normalizeTurnIndex(input.turnIndex);
  if (turnIndex <= 0) return null;
  const content = typeof input.content === 'string' ? input.content : '';
  if (content === '') return null;
  return {
    turnIndex,
    content,
    finished: input.finished === true,
  };
}

export function normalizeSessionMessagePayload(raw: unknown): RegistrySessionMessageEventPayload | null {
  const input = asRecord(raw);
  if (!input) return null;
  const sessionId = typeof input.sessionId === 'string' ? input.sessionId.trim() : '';
  if (!sessionId) return null;
  if ('turnIndex' in input || 'content' in input || 'finished' in input) {
    return null;
  }
  const turn = normalizeSessionWireTurn(input.turn);
  if (!turn) return null;
  return {sessionId, turn};
}

export function normalizeSessionReadPayload(
  raw: unknown,
  expectedSessionId: string,
  normalizeSummary?: (raw: unknown) => RegistrySessionSummary | null,
): RegistrySessionReadResponse | null {
  const input = asRecord(raw);
  if (!input) return null;
  const sessionId = typeof input.sessionId === 'string' ? input.sessionId.trim() : '';
  if (!sessionId || sessionId !== expectedSessionId.trim()) {
    return null;
  }
  const latestTurnIndex = normalizeTurnIndex(input.latestTurnIndex);
  const turns = (Array.isArray(input.turns) ? input.turns : [])
    .map(item => normalizeSessionWireTurn(item))
    .filter((item): item is RegistrySessionTurn => !!item)
    .sort((a, b) => a.turnIndex - b.turnIndex);
  const session = normalizeSummary?.(input.session) ?? undefined;
  return {
    sessionId,
    ...(session ? {session} : {}),
    turns,
    messages: turns
      .map(turn => decodeSessionTurnToMessage(sessionId, turn))
      .filter((item): item is RegistrySessionMessage => !!item),
    latestTurnIndex: Math.max(0, latestTurnIndex),
  };
}

export function decodeSessionTurnToMessage(
  sessionId: string,
  turn: RegistrySessionTurn,
): RegistrySessionMessage | null {
  const normalizedSessionId = sessionId.trim();
  if (!normalizedSessionId || !turn || turn.turnIndex <= 0) return null;
  try {
    const doc = JSON.parse(turn.content) as Record<string, unknown>;
    const method = typeof doc.method === 'string' ? doc.method.trim() : '';
    const param = asRecord(doc.param) ?? {};
    if (!method) return null;
    if (method === 'user_message_chunk') {
      const text = typeof param.text === 'string' ? param.text : '';
      if (/^<(command-name|command-message|command-args|local-command-caveat|local-command-stdout)[\s>]/.test(text)) {
        return null;
      }
    }
    return {
      sessionId: normalizedSessionId,
      turnIndex: turn.turnIndex,
      method,
      param,
      finished: turn.finished === true,
    };
  } catch {
    return {
      sessionId: normalizedSessionId,
      turnIndex: turn.turnIndex,
      method: 'system',
      param: {text: turn.content},
      finished: turn.finished === true,
    };
  }
}
