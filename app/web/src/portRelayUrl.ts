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

function isLoopbackHost(hostname: string): boolean {
  const value = hostname.trim().toLowerCase().replace(/^\[/, '').replace(/\]$/, '');
  return value === 'localhost' || value === '127.0.0.1' || value === '::1';
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
