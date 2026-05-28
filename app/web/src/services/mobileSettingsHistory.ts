export type MobileSettingsHistoryDetail =
  | 'update'
  | 'skills'
  | 'tokenStats'
  | 'ccSwitch'
  | 'database'
  | 'portRelay'
  | 'debugLogs';

export type MobileSettingsHistoryState = {
  wheelMakerHistory: 'mobile-settings';
  detail: MobileSettingsHistoryDetail | null;
};

export type MobileSettingsPopAction = 'back-to-list' | 'close-settings' | 'none';

const MOBILE_SETTINGS_HISTORY_MARKER = 'mobile-settings';

export function createMobileSettingsHistoryState(
  detail: MobileSettingsHistoryDetail | null,
): MobileSettingsHistoryState {
  return {
    wheelMakerHistory: MOBILE_SETTINGS_HISTORY_MARKER,
    detail,
  };
}

export function isMobileSettingsHistoryState(input: unknown): input is MobileSettingsHistoryState {
  if (!input || typeof input !== 'object') {
    return false;
  }
  const state = input as Partial<MobileSettingsHistoryState>;
  return state.wheelMakerHistory === MOBILE_SETTINGS_HISTORY_MARKER
    && (
      state.detail === null
      || state.detail === 'update'
      || state.detail === 'skills'
      || state.detail === 'tokenStats'
      || state.detail === 'ccSwitch'
      || state.detail === 'database'
      || state.detail === 'portRelay'
      || state.detail === 'debugLogs'
    );
}

export function mobileSettingsHistoryKey(detail: MobileSettingsHistoryDetail | null): string {
  return `mobile-settings:${detail ?? 'root'}`;
}

export function resolveMobileSettingsPopAction({
  settingsOpen,
  settingsDetailView,
  nextState,
}: {
  settingsOpen: boolean;
  settingsDetailView: MobileSettingsHistoryDetail | null;
  nextState: unknown;
}): MobileSettingsPopAction {
  if (!settingsOpen) {
    return 'none';
  }
  if (settingsDetailView !== null) {
    return isMobileSettingsHistoryState(nextState) ? 'back-to-list' : 'close-settings';
  }
  return isMobileSettingsHistoryState(nextState) ? 'none' : 'close-settings';
}
