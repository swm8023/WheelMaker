import {
  type ChatSessionKey,
  chatSessionKeyFromParts,
  encodeChatSessionKey,
} from './chatSessionKey';

export type ChatListSelectionResolution = {
  sessionId: string;
  canMutateSelection: boolean;
};

export function shouldApplyPreservedChatLoad(
  currentKey: ChatSessionKey | null | undefined,
  selectionSnapshot: string,
): boolean {
  const snapshot = selectionSnapshot.trim();
  return !snapshot || encodeChatSessionKey(currentKey) === snapshot;
}

export function resolveChatListSelection(input: {
  activeProjectId: string;
  allowMissingSelection?: boolean;
  availableSessionIds: string[];
  currentKey: ChatSessionKey | null | undefined;
  legacySelectionId?: string;
  persistedKey: ChatSessionKey | null | undefined;
  preferredSelection?: string;
}): ChatListSelectionResolution {
  const activeProjectId = input.activeProjectId.trim();
  const availableSessionIds = input.availableSessionIds
    .map(sessionId => sessionId.trim())
    .filter(Boolean);
  const firstAvailableSessionId = availableSessionIds[0] ?? '';
  const currentKey = input.currentKey ?? null;

  if (currentKey && currentKey.projectId !== activeProjectId) {
    return {sessionId: '', canMutateSelection: false};
  }

  const currentSelection =
    currentKey?.projectId === activeProjectId ? currentKey.sessionId : '';
  const persistedSelection =
    input.persistedKey?.projectId === activeProjectId
      ? input.persistedKey.sessionId
      : '';
  const preferredSelection =
    chatSessionKeyFromParts(activeProjectId, input.preferredSelection ?? '')
      ?.sessionId ?? '';
  const legacySelection = (input.legacySelectionId ?? '').trim();

  let sessionId =
    currentSelection ||
    preferredSelection ||
    persistedSelection ||
    legacySelection ||
    '';

  if (
    sessionId &&
    !input.allowMissingSelection &&
    !availableSessionIds.includes(sessionId)
  ) {
    sessionId = firstAvailableSessionId;
  }
  sessionId = sessionId || firstAvailableSessionId;

  return {sessionId, canMutateSelection: true};
}
