export const CHAT_USER_SCROLL_LOCK_MS = 320;

export function nextChatUserScrollLockUntil(
  now = Date.now(),
  durationMs = CHAT_USER_SCROLL_LOCK_MS,
): number {
  return now + Math.max(0, durationMs);
}

export function isChatUserScrollLocked(lockUntil: number, now = Date.now()): boolean {
  return lockUntil > now;
}

export function shouldAutoScrollChatToBottom(input: {
  force: boolean;
  followsLatest: boolean;
  pointerScrolling: boolean;
  userScrollLocked: boolean;
}): boolean {
  return input.force || (input.followsLatest && !input.pointerScrolling && !input.userScrollLocked);
}

export function shouldHandleChatVirtualWindowScroll(programmaticScroll: boolean): boolean {
  return !programmaticScroll;
}

export type ChatSessionReadWindowUpdate = {
  resetToLatest?: true;
  followLatest?: boolean;
};

export function resolveChatSessionReadWindowUpdate(input: {
  useIncremental: boolean;
  followsLatest: boolean;
}): ChatSessionReadWindowUpdate {
  if (input.useIncremental) {
    return {followLatest: input.followsLatest};
  }
  return {resetToLatest: true};
}
