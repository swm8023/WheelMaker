export const FLOATING_CONTROL_SIDE_HYSTERESIS_PX = 24;
export const FLOATING_CONTROL_DEFAULT_Y_RATIO = 0.25;
export const FLOATING_CONTROL_COMPOSER_GAP_PX = 12;

export type LegacyFloatingControlSlot =
  | 'upper'
  | 'upper-middle'
  | 'center'
  | 'lower-middle'
  | 'lower';

type FloatingControlSide = 'left' | 'right';

export function resolveFloatingControlDragSide(
  currentSide: FloatingControlSide,
  pointerX: number,
  viewportWidth: number,
  hysteresisPx = FLOATING_CONTROL_SIDE_HYSTERESIS_PX,
): FloatingControlSide {
  const midpoint = viewportWidth / 2;
  if (currentSide === 'right') {
    return pointerX < midpoint - hysteresisPx ? 'left' : 'right';
  }
  return pointerX > midpoint + hysteresisPx ? 'right' : 'left';
}

export function sanitizeFloatingControlYRatio(
  value: unknown,
  fallback = FLOATING_CONTROL_DEFAULT_Y_RATIO,
): number {
  const numeric = typeof value === 'number' && Number.isFinite(value) ? value : fallback;
  return Math.min(1, Math.max(0, numeric));
}

export function floatingControlYRatioFromLegacySlot(value: unknown): number | null {
  switch (value) {
    case 'upper':
      return 0;
    case 'upper-middle':
      return 0.25;
    case 'center':
      return 0.5;
    case 'lower-middle':
      return 0.75;
    case 'lower':
      return 1;
    default:
      return null;
  }
}

export function floatingControlTopFromYRatio(
  ratio: number,
  minTop: number,
  maxTop: number,
): number {
  const clampedRatio = sanitizeFloatingControlYRatio(ratio);
  return Math.round(minTop + (maxTop - minTop) * clampedRatio);
}

export function floatingControlYRatioFromTop(
  top: number,
  minTop: number,
  maxTop: number,
): number {
  if (maxTop <= minTop) {
    return FLOATING_CONTROL_DEFAULT_Y_RATIO;
  }
  return sanitizeFloatingControlYRatio((top - minTop) / (maxTop - minTop));
}

export function resolveFloatingControlYRatioForStableTop({
  previousTop,
  minTop,
  maxTop,
  fallbackRatio = FLOATING_CONTROL_DEFAULT_Y_RATIO,
}: {
  previousTop: number;
  minTop: number;
  maxTop: number;
  fallbackRatio?: number;
}): number {
  if (maxTop <= minTop) {
    return sanitizeFloatingControlYRatio(fallbackRatio);
  }
  const clampedTop = Math.min(maxTop, Math.max(minTop, previousTop));
  return floatingControlYRatioFromTop(clampedTop, minTop, maxTop);
}

export function resolveFloatingControlVerticalBounds({
  viewportHeight,
  keyboardOffset,
  stackHeight,
  safeAreaTopInset,
  safeAreaBottomInset,
  composerTop,
  composerGap = FLOATING_CONTROL_COMPOSER_GAP_PX,
}: {
  viewportHeight: number;
  keyboardOffset: number;
  stackHeight: number;
  safeAreaTopInset: number;
  safeAreaBottomInset: number;
  composerTop: number | null;
  composerGap?: number;
}): {minTop: number; maxTop: number} {
  const minTop = Math.max(safeAreaTopInset + 6, 6);
  const bottomInset = Math.max(safeAreaBottomInset + 6, 6);
  const viewportMaxTop = viewportHeight - keyboardOffset - stackHeight - bottomInset;
  const composerMaxTop = composerTop === null
    ? viewportMaxTop
    : composerTop - stackHeight - composerGap;
  const maxTop = Math.max(
    minTop,
    Math.min(viewportMaxTop, composerMaxTop),
  );
  return {minTop, maxTop};
}
