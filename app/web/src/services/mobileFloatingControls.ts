import type { PersistedFloatingControlSide } from './workspacePersistence';

export const FLOATING_CONTROL_SIDE_HYSTERESIS_PX = 24;

export function resolveFloatingControlDragSide(
  currentSide: PersistedFloatingControlSide,
  pointerX: number,
  viewportWidth: number,
  hysteresisPx = FLOATING_CONTROL_SIDE_HYSTERESIS_PX,
): PersistedFloatingControlSide {
  const midpoint = viewportWidth / 2;
  if (currentSide === 'right') {
    return pointerX < midpoint - hysteresisPx ? 'left' : 'right';
  }
  return pointerX > midpoint + hysteresisPx ? 'right' : 'left';
}
