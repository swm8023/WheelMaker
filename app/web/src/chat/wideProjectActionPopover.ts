export type WideProjectActionPopoverRect = {
  top: number;
  right: number;
  bottom: number;
};

export type WideProjectActionPopoverPlacement = {
  placement: 'above' | 'below';
  top: number;
  left: number;
  width: number;
  maxHeight: number;
};

type WideProjectActionPopoverInput = {
  anchorRect: WideProjectActionPopoverRect | null;
  viewportWidth: number;
  viewportHeight: number;
  preferredWidth?: number;
  preferredMaxHeight?: number;
  margin?: number;
  gap?: number;
};

export function resolveWideProjectActionPopoverPlacement(
  input: WideProjectActionPopoverInput,
): WideProjectActionPopoverPlacement | null {
  const anchor = input.anchorRect;
  if (!anchor) {
    return null;
  }
  const margin = positiveNumberOr(input.margin, 8);
  const gap = positiveNumberOr(input.gap, 4);
  const viewportWidth = Math.max(0, finiteNumberOr(input.viewportWidth, 0));
  const viewportHeight = Math.max(0, finiteNumberOr(input.viewportHeight, 0));
  const preferredWidth = positiveNumberOr(input.preferredWidth, 260);
  const preferredMaxHeight = positiveNumberOr(input.preferredMaxHeight, 280);
  const width = Math.max(0, Math.min(preferredWidth, viewportWidth - margin * 2));
  const left = clamp(anchor.right - width, margin, Math.max(margin, viewportWidth - width - margin));
  const availableBelow = Math.max(0, viewportHeight - anchor.bottom - gap - margin);
  const availableAbove = Math.max(0, anchor.top - gap - margin);
  const openAbove = availableBelow < Math.min(160, preferredMaxHeight) && availableAbove > availableBelow;
  const viewportMaxHeight = Math.max(0, viewportHeight - margin * 2);
  const maxHeight = Math.min(
    preferredMaxHeight,
    viewportMaxHeight,
    Math.max(80, openAbove ? availableAbove : availableBelow),
  );
  const top = openAbove
    ? Math.max(margin + maxHeight, anchor.top - gap)
    : Math.min(anchor.bottom + gap, Math.max(margin, viewportHeight - margin - maxHeight));

  return {
    placement: openAbove ? 'above' : 'below',
    top: Math.round(top),
    left: Math.round(left),
    width: Math.round(width),
    maxHeight: Math.round(maxHeight),
  };
}

function finiteNumberOr(value: number | undefined, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function positiveNumberOr(value: number | undefined, fallback: number): number {
  const finite = finiteNumberOr(value, fallback);
  return finite > 0 ? finite : fallback;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
