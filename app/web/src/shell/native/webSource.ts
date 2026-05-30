export type NativeWebSourcePreference = 'auto' | 'embedded';
export type NativeWebSourceActual = 'embedded' | 'remote';

export type NativeWebSourceState = {
  preference: NativeWebSourcePreference;
  actualSource: NativeWebSourceActual;
  displayTitle: string;
  displaySource: string;
  remoteUrl: string;
  remoteHost: string;
};

export type NativeRemoteWebCandidate = {
  source: 'registry';
  registryAddress: string;
  remoteWebUrl: string;
};

export type NativeWebSourceBridge = {
  enabled?: boolean;
  getWebSourceState?: () => Promise<NativeWebSourceState> | NativeWebSourceState;
  setWebSourcePreference?: (
    preference: NativeWebSourcePreference,
  ) => Promise<NativeWebSourceState> | NativeWebSourceState;
  setRemoteWebCandidate?: (
    candidate: NativeRemoteWebCandidate,
  ) => Promise<NativeWebSourceState> | NativeWebSourceState;
};

type AndroidNativeBridge = {
  getWebSourceState?: () => string;
  setWebSourcePreference?: (preference: NativeWebSourcePreference) => string;
  setRemoteWebCandidate?: (candidateJson: string) => string;
};

type NativeWindow = Window & {
  WheelMakerAndroid?: NativeWebSourceBridge;
  WheelMakerDesktop?: NativeWebSourceBridge;
  WheelMakerAndroidNative?: AndroidNativeBridge;
};

function parseNativeState(value: string | undefined): NativeWebSourceState {
  return JSON.parse(value ?? '{}') as NativeWebSourceState;
}

function isLoopbackHost(hostname: string): boolean {
  const value = hostname.toLowerCase();
  return value === 'localhost' || value === '127.0.0.1' || value === '::1' || value === '[::1]';
}

function wrapAndroidNativeBridge(native: AndroidNativeBridge): NativeWebSourceBridge {
  return {
    enabled: true,
    getWebSourceState: native.getWebSourceState
      ? () => Promise.resolve(parseNativeState(native.getWebSourceState?.()))
      : undefined,
    setWebSourcePreference: native.setWebSourcePreference
      ? preference => Promise.resolve(parseNativeState(native.setWebSourcePreference?.(preference)))
      : undefined,
    setRemoteWebCandidate: native.setRemoteWebCandidate
      ? candidate => Promise.resolve(parseNativeState(native.setRemoteWebCandidate?.(JSON.stringify(candidate))))
      : undefined,
  };
}

export function inferNativeRemoteWebCandidate(registryAddress: string): NativeRemoteWebCandidate | null {
  let parsed: URL;
  const trimmed = registryAddress.trim();
  try {
    parsed = new URL(trimmed);
  } catch {
    return null;
  }
  if (
    parsed.protocol !== 'ws:' &&
    parsed.protocol !== 'wss:' &&
    parsed.protocol !== 'http:' &&
    parsed.protocol !== 'https:'
  ) {
    return null;
  }
  if (!parsed.host) {
    return null;
  }
  if (isLoopbackHost(parsed.hostname)) {
    return null;
  }
  const remoteProtocol = parsed.protocol === 'ws:' || parsed.protocol === 'http:' ? 'http:' : 'https:';
  return {
    source: 'registry',
    registryAddress: trimmed,
    remoteWebUrl: `${remoteProtocol}//${parsed.host}/`,
  };
}

export function getNativeWebSourceBridge(): NativeWebSourceBridge | null {
  if (typeof window === 'undefined') {
    return null;
  }
  const nativeWindow = window as NativeWindow;
  if (nativeWindow.WheelMakerAndroid) {
    return nativeWindow.WheelMakerAndroid;
  }
  if (nativeWindow.WheelMakerAndroidNative) {
    return wrapAndroidNativeBridge(nativeWindow.WheelMakerAndroidNative);
  }
  return nativeWindow.WheelMakerDesktop ?? null;
}

export function submitNativeRemoteWebCandidate(registryAddress: string): void {
  const bridge = getNativeWebSourceBridge();
  const submit = bridge?.setRemoteWebCandidate;
  if (!submit) {
    return;
  }
  const trimmed = registryAddress.trim();
  const candidate = inferNativeRemoteWebCandidate(trimmed);
  void Promise.resolve(submit(candidate ?? {
    source: 'registry',
    registryAddress: trimmed,
    remoteWebUrl: '',
  })).catch(() => undefined);
}
