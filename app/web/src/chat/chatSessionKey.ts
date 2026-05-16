export type ChatSessionKey = {
  projectId: string;
  sessionId: string;
};

export const EMPTY_CHAT_SESSION_KEY = '';

const CHAT_SESSION_KEY_SEPARATOR = '\u001f';

export function chatSessionKeyFromParts(
  projectId: string,
  sessionId: string,
): ChatSessionKey | null {
  const normalizedProjectId = projectId.trim();
  const normalizedSessionId = sessionId.trim();
  if (!normalizedProjectId || !normalizedSessionId) {
    return null;
  }
  return {
    projectId: normalizedProjectId,
    sessionId: normalizedSessionId,
  };
}

export function encodeChatSessionKey(
  key: ChatSessionKey | null | undefined,
): string {
  const normalized = key
    ? chatSessionKeyFromParts(key.projectId, key.sessionId)
    : null;
  if (!normalized) {
    return EMPTY_CHAT_SESSION_KEY;
  }
  return [
    encodeURIComponent(normalized.projectId),
    encodeURIComponent(normalized.sessionId),
  ].join(CHAT_SESSION_KEY_SEPARATOR);
}

export function decodeChatSessionKey(encoded: string): ChatSessionKey | null {
  if (!encoded) {
    return null;
  }
  const parts = encoded.split(CHAT_SESSION_KEY_SEPARATOR);
  if (parts.length !== 2) {
    return null;
  }
  try {
    return chatSessionKeyFromParts(
      decodeURIComponent(parts[0]),
      decodeURIComponent(parts[1]),
    );
  } catch {
    return null;
  }
}

export function sameChatSessionKey(
  left: ChatSessionKey | null | undefined,
  right: ChatSessionKey | null | undefined,
): boolean {
  return encodeChatSessionKey(left) === encodeChatSessionKey(right);
}
