export type DesktopWindowBridge = {
  enabled: true;
  startDrag?: () => Promise<void> | void;
  minimize?: () => Promise<void> | void;
  toggleMaximize?: () => Promise<void> | void;
  close?: () => Promise<void> | void;
};

declare global {
  interface Window {
    WheelMakerDesktop?: DesktopWindowBridge;
  }
}

export function getDesktopWindowBridge(): DesktopWindowBridge | null {
  if (typeof window === 'undefined') {
    return null;
  }
  const bridge = window.WheelMakerDesktop;
  return bridge?.enabled === true ? bridge : null;
}
