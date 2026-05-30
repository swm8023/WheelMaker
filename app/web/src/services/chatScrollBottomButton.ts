export const CHAT_SCROLL_BOTTOM_COMPOSER_GAP_PX = 10;
export const CHAT_SCROLL_BOTTOM_FALLBACK_OFFSET_PX = 92;

function normalizeNonNegativePx(value: number): number {
  return Number.isFinite(value) ? Math.max(0, Math.round(value)) : 0;
}

export function resolveChatScrollBottomButtonOffset({
  composerHeight,
  keyboardInset,
  composerGap = CHAT_SCROLL_BOTTOM_COMPOSER_GAP_PX,
  fallbackOffset = CHAT_SCROLL_BOTTOM_FALLBACK_OFFSET_PX,
}: {
  composerHeight: number;
  keyboardInset: number;
  composerGap?: number;
  fallbackOffset?: number;
}): number {
  const normalizedComposerHeight = normalizeNonNegativePx(composerHeight);
  const normalizedKeyboardInset = normalizeNonNegativePx(keyboardInset);
  const normalizedGap = normalizeNonNegativePx(composerGap);
  const normalizedFallbackOffset = Number.isFinite(fallbackOffset)
    ? Math.max(0, Math.round(fallbackOffset))
    : CHAT_SCROLL_BOTTOM_FALLBACK_OFFSET_PX;

  return normalizedKeyboardInset + (
    normalizedComposerHeight > 0
      ? normalizedComposerHeight + normalizedGap
      : normalizedFallbackOffset
  );
}
