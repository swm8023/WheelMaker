export type GestureNavigationTab = 'chat' | 'file' | 'git';

export type GesturePressIntent = 'pressing' | 'expand' | 'neutral';

export const GESTURE_LONG_PRESS_MS = 350;
export const GESTURE_LONG_PRESS_CANCEL_PX = 12;
export const GESTURE_MOVE_LONG_PRESS_MS = 2000;
export const GESTURE_SELECTION_PX = 42;

export function resolveGesturePressIntent({
  distancePx,
  elapsedMs,
}: {
  distancePx: number;
  elapsedMs: number;
}): GesturePressIntent {
  if (distancePx < GESTURE_LONG_PRESS_CANCEL_PX) {
    return elapsedMs >= GESTURE_LONG_PRESS_MS ? 'expand' : 'pressing';
  }
  return 'neutral';
}

export function shouldStartGestureMove({
  elapsedMs,
  candidate,
}: {
  elapsedMs: number;
  candidate: GestureNavigationTab | null;
}): boolean {
  return elapsedMs >= GESTURE_MOVE_LONG_PRESS_MS && candidate === null;
}

export function resolveGestureDirectionCandidate({
  deltaX,
  deltaY,
}: {
  deltaX: number;
  deltaY: number;
}): GestureNavigationTab | null {
  const distance = Math.hypot(deltaX, deltaY);
  if (distance < GESTURE_SELECTION_PX) {
    return null;
  }
  const absX = Math.abs(deltaX);
  const absY = Math.abs(deltaY);
  if (absY >= absX) {
    return deltaY < 0 ? 'chat' : 'git';
  }
  return deltaX < 0 ? 'file' : null;
}
