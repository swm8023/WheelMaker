import { getDesktopWindowBridge } from '../desktopRuntime';

export type DesktopChatQuickSwitchContextMenuStyle = {
  position: 'fixed';
  left: number;
  top: number;
  width: number;
  maxHeight: number;
};

export type DesktopChatQuickSwitchContextMenuResult =
  | { open: false }
  | {
      open: true;
      style: DesktopChatQuickSwitchContextMenuStyle;
    };

type ResolveDesktopChatQuickSwitchContextMenuInput = {
  desktopShell?: boolean;
  desktopLayout?: boolean;
  target: EventTarget | null;
  selectedText: string;
  clientX: number;
  clientY: number;
  viewportWidth: number;
  viewportHeight: number;
  preferredWidth?: number;
  preferredMaxHeight?: number;
  margin?: number;
};

const INTERACTIVE_CONTEXT_MENU_TARGET_SELECTOR = [
  'a',
  'button',
  'input',
  'textarea',
  'select',
  '[contenteditable="true"]',
  '[role="button"]',
  '[role="link"]',
  '[role="menuitem"]',
  '[data-chat-context-menu-native]',
].join(',');

type ClosestTarget = {
  closest?: (selector: string) => Element | null;
  parentElement?: ClosestTarget | null;
};
type ClosestElement = {
  closest: (selector: string) => Element | null;
  parentElement?: ClosestTarget | null;
};

export function resolveDesktopChatQuickSwitchContextMenu({
  desktopShell,
  desktopLayout,
  target,
  selectedText,
  clientX,
  clientY,
  viewportWidth,
  viewportHeight,
  preferredWidth = 320,
  preferredMaxHeight = 280,
  margin = 8,
}: ResolveDesktopChatQuickSwitchContextMenuInput): DesktopChatQuickSwitchContextMenuResult {
  const resolvedDesktopShell = desktopShell ?? (desktopLayout === true && getDesktopWindowBridge() !== null);
  if (!resolvedDesktopShell || selectedText.trim()) {
    return { open: false };
  }
  if (isInteractiveContextMenuTarget(target)) {
    return { open: false };
  }

  const safeMargin = Math.max(0, finiteNumberOr(margin, 8));
  const safeViewportWidth = Math.max(0, finiteNumberOr(viewportWidth, 0));
  const safeViewportHeight = Math.max(0, finiteNumberOr(viewportHeight, 0));
  const width = Math.max(0, Math.min(
    positiveNumberOr(preferredWidth, 320),
    safeViewportWidth - safeMargin * 2,
  ));
  const maxHeight = Math.max(0, Math.min(
    positiveNumberOr(preferredMaxHeight, 280),
    safeViewportHeight - safeMargin * 2,
  ));
  const left = clamp(
    finiteNumberOr(clientX, safeMargin),
    safeMargin,
    Math.max(safeMargin, safeViewportWidth - width - safeMargin),
  );
  const top = clamp(
    finiteNumberOr(clientY, safeMargin),
    safeMargin,
    Math.max(safeMargin, safeViewportHeight - maxHeight - safeMargin),
  );

  return {
    open: true,
    style: {
      position: 'fixed',
      left: Math.round(left),
      top: Math.round(top),
      width: Math.round(width),
      maxHeight: Math.round(maxHeight),
    },
  };
}

function isInteractiveContextMenuTarget(target: EventTarget | null): boolean {
  const element = targetElement(target);
  return Boolean(element?.closest(INTERACTIVE_CONTEXT_MENU_TARGET_SELECTOR));
}

function targetElement(target: EventTarget | null): ClosestElement | null {
  const candidate = target as ClosestTarget | null;
  if (typeof candidate?.closest === 'function') {
    return candidate as ClosestElement;
  }
  if (typeof candidate?.parentElement?.closest === 'function') {
    return candidate.parentElement as ClosestElement;
  }
  return null;
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
