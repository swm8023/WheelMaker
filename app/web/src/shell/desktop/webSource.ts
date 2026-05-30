import {
  getNativeWebSourceBridge,
  inferNativeRemoteWebCandidate,
  type NativeRemoteWebCandidate,
  type NativeWebSourceState,
} from '../native/webSource';

export type DesktopRemoteWebCandidate = NativeRemoteWebCandidate;
export type DesktopWebSourceState = NativeWebSourceState;

export function inferDesktopRemoteWebCandidate(registryAddress: string): DesktopRemoteWebCandidate | null {
  return inferNativeRemoteWebCandidate(registryAddress);
}

export function readDesktopWebSourceState(): Promise<DesktopWebSourceState | null> {
  const bridge = getNativeWebSourceBridge();
  const read = bridge?.getWebSourceState;
  if (!read) {
    return Promise.resolve(null);
  }
  return Promise.resolve(read()).catch(() => null);
}

export async function setDesktopWebSourcePreference(preference: 'auto' | 'embedded'): Promise<DesktopWebSourceState | null> {
  const bridge = getNativeWebSourceBridge();
  const setPreference = bridge?.setWebSourcePreference;
  if (!setPreference) {
    return null;
  }
  return Promise.resolve(setPreference(preference)).catch(() => null);
}

export function submitDesktopRemoteWebCandidate(registryAddress: string): void {
  const bridge = getNativeWebSourceBridge();
  const submit = bridge?.setRemoteWebCandidate;
  if (!submit) {
    return;
  }
  const candidate = inferDesktopRemoteWebCandidate(registryAddress);
  void Promise.resolve(submit(candidate ?? {
    source: 'registry',
    registryAddress: registryAddress.trim(),
    remoteWebUrl: '',
  })).catch(() => undefined);
}
