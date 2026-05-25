export function buildPortRelayOpenUrl(registryAddress: string, portRelayListenPort: string | number): string {
  const port = String(portRelayListenPort).trim();
  if (!port) return '';
  const rawAddress = registryAddress.trim() || '127.0.0.1';
  const addressWithScheme = /^[a-z]+:\/\//i.test(rawAddress) ? rawAddress : `http://${rawAddress}`;
  try {
    const url = new URL(addressWithScheme);
    if (url.protocol === 'ws:') url.protocol = 'http:';
    if (url.protocol === 'wss:') url.protocol = 'https:';
    url.port = port;
    url.pathname = '/';
    url.search = '';
    url.hash = '';
    return url.toString();
  } catch {
    return `http://127.0.0.1:${port}/`;
  }
}

export type PortRelayLocalHttpUrl = {
  targetPort: number;
  path: string;
};

const PORT_RELAY_AUTO_AUTH_QUERY = '__wm_relay_code';

function isLoopbackHost(hostname: string): boolean {
  const value = hostname.trim().toLowerCase().replace(/^\[/, '').replace(/\]$/, '');
  return value === 'localhost' || value === '127.0.0.1' || value === '::1';
}

function isLocalHttpRelayHost(hostname: string): boolean {
  const value = hostname.trim().toLowerCase().replace(/^\[/, '').replace(/\]$/, '');
  return value === 'localhost' || value === '127.0.0.1';
}

function urlHostIsLoopback(rawUrl: string): boolean {
  try {
    return isLoopbackHost(new URL(rawUrl).hostname);
  } catch {
    return false;
  }
}

export function resolvePortRelayOpenUrl(args: {
  relayUrl?: string;
  registryAddress: string;
  listenPort: string | number;
}): string {
  const relayUrl = args.relayUrl?.trim();
  const derivedUrl = buildPortRelayOpenUrl(args.registryAddress, args.listenPort);
  if (relayUrl) {
    if (urlHostIsLoopback(relayUrl) && derivedUrl && !urlHostIsLoopback(derivedUrl)) {
      return derivedUrl;
    }
    return relayUrl;
  }
  return derivedUrl;
}

export function parsePortRelayLocalHttpUrl(value: string): PortRelayLocalHttpUrl | null {
  let url: URL;
  try {
    url = new URL(value.trim());
  } catch {
    return null;
  }
  if (url.protocol !== 'http:' || !isLocalHttpRelayHost(url.hostname) || !url.port) {
    return null;
  }
  const targetPort = Number(url.port);
  if (!Number.isInteger(targetPort) || targetPort < 1 || targetPort > 65535) {
    return null;
  }
  return {
    targetPort,
    path: `${url.pathname || '/'}${url.search}${url.hash}`,
  };
}

export function appendPortRelayOpenPath(baseUrl: string, path: string): string {
  const normalizedPath = path.trim();
  if (!normalizedPath || normalizedPath === '/') {
    return baseUrl;
  }
  try {
    const url = new URL(baseUrl);
    const pathUrl = new URL(normalizedPath, 'http://relay.local');
    url.pathname = pathUrl.pathname || '/';
    url.search = pathUrl.search;
    url.hash = pathUrl.hash;
    return url.toString();
  } catch {
    return baseUrl;
  }
}

export function appendPortRelayAutoAuthCode(openUrl: string, accessCode: string): string {
  const code = accessCode.trim();
  if (!/^\d{6}$/.test(code)) {
    return openUrl;
  }
  try {
    const url = new URL(openUrl);
    url.searchParams.set(PORT_RELAY_AUTO_AUTH_QUERY, code);
    return url.toString();
  } catch {
    return openUrl;
  }
}
