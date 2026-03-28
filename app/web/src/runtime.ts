type RuntimeConfig = {
  defaultRegistryAddress?: string;
  defaultRegistryPort?: number;
};

type GlobalLike = {
  __WHEELMAKER_RUNTIME_CONFIG__?: RuntimeConfig;
  location?: {hostname?: string; host?: string; protocol?: string};
};

function getConfig(): RuntimeConfig {
  const globalLike = window as unknown as GlobalLike;
  return globalLike.__WHEELMAKER_RUNTIME_CONFIG__ ?? {};
}

export function getDefaultRegistryAddress(): string {
  const cfg = getConfig();
  if (cfg.defaultRegistryAddress?.trim()) {
    return cfg.defaultRegistryAddress.trim();
  }
  const host = window.location.hostname;
  if (host === '127.0.0.1') {
    return 'ws://127.0.0.1:6930/ws';
  }
  if (window.location.host) {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}/ws`;
  }
  return `127.0.0.1:${cfg.defaultRegistryPort ?? 9630}`;
}

export function toRegistryWsUrl(address: string): string {
  const input = address.trim();
  if (!input) {
    throw new Error('Address is required');
  }
  if (/^wss?:\/\//i.test(input)) return input;
  if (/^https?:\/\//i.test(input)) {
    return input.replace(/^http:\/\//i, 'ws://').replace(/^https:\/\//i, 'wss://');
  }
  const hasPort = /:\d+$/.test(input);
  const host = hasPort ? input : `${input}:9630`;
  return `ws://${host}/ws`;
}
