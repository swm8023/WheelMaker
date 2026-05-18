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

export type ChatVirtualScrollDirection = 'forward' | 'backward' | null;

export function shouldAdjustChatVirtualItemSizeChange(input: {
  isScrolling: boolean;
  itemEnd: number;
  itemStart: number;
  scrollDirection: ChatVirtualScrollDirection;
  scrollOffset: number | null | undefined;
}): boolean {
  const scrollOffset = Math.max(0, input.scrollOffset ?? 0);
  if (input.isScrolling && input.scrollDirection === 'backward') {
    return input.itemEnd <= scrollOffset;
  }
  return input.itemStart < scrollOffset;
}

export function resolveChatBottomScrollTop(input: {
  scrollHeight: number;
  clientHeight: number;
}): number {
  return Math.max(0, input.scrollHeight - input.clientHeight);
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
