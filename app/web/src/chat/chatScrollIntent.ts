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

export type ChatScrollToBottomVisibility = {
  atBottom: boolean;
  showScrollToBottom: boolean;
};

export function resolveChatScrollBottomTop(input: {
  scrollHeight: number;
  clientHeight: number;
}): number {
  const scrollHeight = Number.isFinite(input.scrollHeight) ? Math.max(0, input.scrollHeight) : 0;
  const clientHeight = Number.isFinite(input.clientHeight) ? Math.max(0, input.clientHeight) : 0;
  return Math.max(0, scrollHeight - clientHeight);
}

export function resolveChatScrollToBottomVisibility(input: {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
  threshold: number;
}): ChatScrollToBottomVisibility {
  const scrollTop = Number.isFinite(input.scrollTop) ? Math.max(0, input.scrollTop) : 0;
  const scrollHeight = Number.isFinite(input.scrollHeight) ? Math.max(0, input.scrollHeight) : 0;
  const clientHeight = Number.isFinite(input.clientHeight) ? Math.max(0, input.clientHeight) : 0;
  const threshold = Number.isFinite(input.threshold) ? Math.max(0, input.threshold) : 0;
  const distanceFromBottom = Math.max(
    0,
    resolveChatScrollBottomTop({scrollHeight, clientHeight}) - scrollTop,
  );
  const scrollable = scrollHeight > clientHeight + 1;
  const atBottom = !scrollable || distanceFromBottom <= threshold;
  return {
    atBottom,
    showScrollToBottom: !atBottom,
  };
}

export type ChatBottomFollowAction = 'scrollToBottom' | 'autoscrollToBottom';

export function resolveChatBottomFollowAction(input: {
  itemCount: number;
  previousItemCount: number;
}): ChatBottomFollowAction {
  return input.itemCount === input.previousItemCount ? 'autoscrollToBottom' : 'scrollToBottom';
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
