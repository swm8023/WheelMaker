import type {NativeWebSourceState} from '../shell/native/webSource';

export type ConnectionStatusLine = {
  label: string;
  detail: string;
};

export type WebResourceConnectionStatus = ConnectionStatusLine & {
  remoteUrl: string;
};

export function resolveWebResourceConnectionStatus(
  state: NativeWebSourceState | null,
): WebResourceConnectionStatus {
  if (!state) {
    return {
      label: 'Browser',
      detail: 'Native Web source state is unavailable',
      remoteUrl: '',
    };
  }
  if (state.actualSource === 'remote') {
    return {
      label: 'Remote',
      detail: state.remoteHost || state.displaySource || state.remoteUrl || 'Remote Web resources',
      remoteUrl: state.remoteUrl || '',
    };
  }
  return {
    label: 'Local',
    detail: 'Embedded Web resources',
    remoteUrl: state.remoteUrl || '',
  };
}

export function resolveRegistryConnectionStatus({
  connected,
  reconnecting,
  autoConnecting,
  address,
}: {
  connected: boolean;
  reconnecting: boolean;
  autoConnecting: boolean;
  address: string;
}): ConnectionStatusLine {
  if (connected) {
    return {
      label: 'Connected',
      detail: address || 'Registry address unavailable',
    };
  }
  if (reconnecting) {
    return {
      label: 'Reconnecting',
      detail: address || 'Registry address unavailable',
    };
  }
  if (autoConnecting) {
    return {
      label: 'Connecting',
      detail: address || 'Registry address unavailable',
    };
  }
  return {
    label: 'Disconnected',
    detail: address || 'Registry address unavailable',
  };
}

export function resolveVoiceCapabilityStatus({
  speechEnabled,
  androidNativeHost,
  androidNativeAvailable,
}: {
  speechEnabled: boolean;
  androidNativeHost: boolean;
  androidNativeAvailable: boolean;
}): ConnectionStatusLine {
  if (!speechEnabled) {
    return {
      label: 'Disabled',
      detail: 'Voice input is off',
    };
  }
  if (androidNativeAvailable) {
    return {
      label: 'Android Native',
      detail: 'APK microphone capture, direct Doubao connection',
    };
  }
  if (androidNativeHost) {
    return {
      label: 'Unavailable',
      detail: 'Android native speech bridge is missing',
    };
  }
  return {
    label: 'Registry',
    detail: 'Registry speech bridge',
  };
}
