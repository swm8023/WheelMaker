import {
  getDesktopWindowBridge,
  type DesktopRemoteWebCandidate,
  type DesktopWebSourceState,
} from '../desktopRuntime';

function isLoopbackHost(hostname: string): boolean {
  const value = hostname.toLowerCase();
  return value === 'localhost' || value === '127.0.0.1' || value === '::1' || value === '[::1]';
}

export function inferDesktopRemoteWebCandidate(registryAddress: string): DesktopRemoteWebCandidate | null {
  let parsed: URL;
  try {
    parsed = new URL(registryAddress.trim());
  } catch {
    return null;
  }
  if ((parsed.protocol !== 'wss:' && parsed.protocol !== 'https:') || !parsed.host) {
    return null;
  }
  if (isLoopbackHost(parsed.hostname)) {
    return null;
  }
  return {
    source: 'registry',
    registryAddress: registryAddress.trim(),
    remoteWebUrl: `https://${parsed.host}/`,
  };
}

export function readDesktopWebSourceState(): Promise<DesktopWebSourceState | null> {
  const bridge = getDesktopWindowBridge();
  const read = bridge?.getWebSourceState;
  if (!read) {
    return Promise.resolve(null);
  }
  return Promise.resolve(read()).catch(() => null);
}

export async function setDesktopWebSourcePreference(preference: 'auto' | 'embedded'): Promise<DesktopWebSourceState | null> {
  const bridge = getDesktopWindowBridge();
  const setPreference = bridge?.setWebSourcePreference;
  if (!setPreference) {
    return null;
  }
  return Promise.resolve(setPreference(preference)).catch(() => null);
}

export function submitDesktopRemoteWebCandidate(registryAddress: string): void {
  const bridge = getDesktopWindowBridge();
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
