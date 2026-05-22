export type DesktopWebSourcePreference = 'auto' | 'embedded';
export type DesktopWebSourceActual = 'embedded' | 'remote';

export type DesktopWebSourceState = {
  preference: DesktopWebSourcePreference;
  actualSource: DesktopWebSourceActual;
  displayTitle: string;
  displaySource: string;
  remoteUrl: string;
  remoteHost: string;
};

export type DesktopRemoteWebCandidate = {
  source: 'registry';
  registryAddress: string;
  remoteWebUrl: string;
};

export type DesktopWindowBridge = {
  enabled: true;
  startDrag?: () => Promise<void> | void;
  minimize?: () => Promise<void> | void;
  toggleMaximize?: () => Promise<void> | void;
  close?: () => Promise<void> | void;
  getWebSourceState?: () => Promise<DesktopWebSourceState> | DesktopWebSourceState;
  setWebSourcePreference?: (
    preference: DesktopWebSourcePreference,
  ) => Promise<DesktopWebSourceState> | DesktopWebSourceState;
  setRemoteWebCandidate?: (
    candidate: DesktopRemoteWebCandidate,
  ) => Promise<DesktopWebSourceState> | DesktopWebSourceState;
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
