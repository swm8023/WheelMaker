import {
  type ChatSessionKey,
  chatSessionKeyFromParts,
  encodeChatSessionKey,
} from './chatSessionKey';

export type ChatListSelectionResolution = {
  sessionId: string;
  canMutateSelection: boolean;
};

export type SelectedChatVisibilityRecovery = 'none' | 'restore-cache' | 'read-session';

export function shouldApplyPreservedChatLoad(
  currentKey: ChatSessionKey | null | undefined,
  selectionSnapshot: string,
): boolean {
  const snapshot = selectionSnapshot.trim();
  return !snapshot || encodeChatSessionKey(currentKey) === snapshot;
}

export function shouldApplyLoadedChatSelection(
  currentKey: ChatSessionKey | null | undefined,
  loadedKey: ChatSessionKey | null | undefined,
): boolean {
  const loaded = encodeChatSessionKey(loadedKey);
  if (!loaded) {
    return false;
  }
  const current = encodeChatSessionKey(currentKey);
  return !current || current === loaded;
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
  const preservesLiveSelection = Boolean(currentSelection);

  if (
    sessionId &&
    !preservesLiveSelection &&
    !input.allowMissingSelection &&
    !availableSessionIds.includes(sessionId)
  ) {
    sessionId = firstAvailableSessionId;
  }
  sessionId = sessionId || firstAvailableSessionId;

  return {sessionId, canMutateSelection: true};
}

export function resolveSelectedChatVisibilityRecovery(input: {
  tab: string;
  connected: boolean;
  chatLoading: boolean;
  selectedRuntimeKey: string;
  visibleRuntimeKey: string;
  visibleMessageCount: number;
  cachedMessageCount: number;
  attemptedRuntimeKey: string;
}): SelectedChatVisibilityRecovery {
  if (
    input.tab !== 'chat' ||
    !input.connected ||
    input.chatLoading ||
    !input.selectedRuntimeKey
  ) {
    return 'none';
  }

  const visibleBelongsToSelection = input.visibleRuntimeKey === input.selectedRuntimeKey;
  const selectedVisible = visibleBelongsToSelection && input.visibleMessageCount > 0;
  if (selectedVisible) {
    return 'none';
  }
  if (input.cachedMessageCount > 0) {
    return 'restore-cache';
  }
  if (input.attemptedRuntimeKey === input.selectedRuntimeKey) {
    return 'none';
  }
  return 'read-session';
}
