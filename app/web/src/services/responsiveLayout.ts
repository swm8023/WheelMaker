export const LAYOUT_MODE_BREAKPOINT_PX = 900;

export type LayoutMode = 'desktop' | 'mobile';

export function resolveLayoutMode(windowWidth: number): LayoutMode {
  return windowWidth >= LAYOUT_MODE_BREAKPOINT_PX ? 'desktop' : 'mobile';
}
