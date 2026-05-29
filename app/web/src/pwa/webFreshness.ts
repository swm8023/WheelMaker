export type WebBuildInfo = {
  schemaVersion?: number;
  sha?: string;
  builtAt?: string;
  assets?: Record<string, string>;
};

type BuildProbeResponse = {
  ok: boolean;
  json: () => Promise<unknown>;
};

type FetchBuildProbe = (input: RequestInfo | URL, init?: RequestInit) => Promise<BuildProbeResponse>;

type ServiceWorkerRegistrationLike = {
  waiting?: {postMessage: (message: unknown) => void} | null;
  update?: () => Promise<unknown>;
};

type ServiceWorkerContainerLike = {
  getRegistration?: () => Promise<ServiceWorkerRegistrationLike | undefined>;
};

type CacheStorageLike = {
  keys: () => Promise<string[]>;
  delete: (key: string) => Promise<boolean>;
};

export type WebFreshnessCheckOptions = {
  currentBuild: WebBuildInfo;
  buildUrl?: string;
  fetchImpl?: FetchBuildProbe;
  serviceWorker?: ServiceWorkerContainerLike;
  caches?: CacheStorageLike;
  reload?: () => void;
  sessionStorage?: Storage;
};

export type WebFreshnessInstallOptions = WebFreshnessCheckOptions & {
  windowRef?: Window;
  documentRef?: Document;
};

const DEFAULT_BUILD_URL = '/web-build.json';
const PWA_CACHE_PREFIX = 'wheelmaker-web-pwa-';
const RELOAD_SHA_KEY = 'wheelmaker.webFreshness.reloadSha';

function cleanSha(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function normalizeBuildInfo(raw: unknown): WebBuildInfo | null {
  if (!raw || typeof raw !== 'object') {
    return null;
  }
  const input = raw as Record<string, unknown>;
  const sha = cleanSha(input.sha);
  const builtAt = typeof input.builtAt === 'string' ? input.builtAt : '';
  const assets = input.assets && typeof input.assets === 'object'
    ? input.assets as Record<string, string>
    : undefined;
  return {
    schemaVersion: typeof input.schemaVersion === 'number' ? input.schemaVersion : undefined,
    sha,
    builtAt,
    assets,
  };
}

export function isFreshWebBuild(current: WebBuildInfo, latest: WebBuildInfo | null): boolean {
  const currentSha = cleanSha(current.sha);
  const latestSha = cleanSha(latest?.sha);
  if (!currentSha || !latestSha) {
    return true;
  }
  return currentSha === latestSha;
}

async function clearWheelMakerPWACaches(cacheStorage?: CacheStorageLike): Promise<void> {
  if (!cacheStorage) {
    return;
  }
  const keys = await cacheStorage.keys();
  await Promise.all(
    keys
      .filter(key => key.startsWith(PWA_CACHE_PREFIX))
      .map(key => cacheStorage.delete(key).catch(() => false)),
  );
}

function defaultFetch(): FetchBuildProbe | null {
  if (typeof fetch !== 'function') {
    return null;
  }
  return fetch.bind(globalThis) as FetchBuildProbe;
}

function defaultServiceWorker(): ServiceWorkerContainerLike | undefined {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) {
    return undefined;
  }
  return navigator.serviceWorker;
}

function defaultCaches(): CacheStorageLike | undefined {
  if (typeof caches === 'undefined') {
    return undefined;
  }
  return caches;
}

function defaultReload(): void {
  if (typeof window !== 'undefined') {
    window.location.reload();
  }
}

function markReloadForSha(storage: Storage | undefined, sha: string): boolean {
  if (!storage || !sha) {
    return true;
  }
  try {
    if (storage.getItem(RELOAD_SHA_KEY) === sha) {
      return false;
    }
    storage.setItem(RELOAD_SHA_KEY, sha);
    return true;
  } catch {
    return true;
  }
}

function clearReloadMarker(storage: Storage | undefined, sha: string): void {
  if (!storage || !sha) {
    return;
  }
  try {
    if (storage.getItem(RELOAD_SHA_KEY) === sha) {
      storage.removeItem(RELOAD_SHA_KEY);
    }
  } catch {
    // Ignore restricted storage.
  }
}

async function activateLatestWebBuild(options: WebFreshnessCheckOptions, latestSha: string): Promise<void> {
  if (!markReloadForSha(options.sessionStorage, latestSha)) {
    return;
  }

  const serviceWorker = options.serviceWorker ?? defaultServiceWorker();
  const registration = await serviceWorker?.getRegistration?.().catch(() => undefined);
  await registration?.update?.().catch(() => undefined);
  registration?.waiting?.postMessage('SKIP_WAITING');
  await clearWheelMakerPWACaches(options.caches ?? defaultCaches());
  (options.reload ?? defaultReload)();
}

export async function checkForFreshWebBuild(options: WebFreshnessCheckOptions): Promise<boolean> {
  const fetchBuild = options.fetchImpl ?? defaultFetch();
  if (!fetchBuild) {
    return false;
  }

  try {
    const response = await fetchBuild(options.buildUrl ?? DEFAULT_BUILD_URL, {cache: 'no-store'});
    if (!response.ok) {
      return false;
    }
    const latest = normalizeBuildInfo(await response.json());
    if (isFreshWebBuild(options.currentBuild, latest)) {
      clearReloadMarker(options.sessionStorage, cleanSha(latest?.sha));
      return false;
    }
    await activateLatestWebBuild(options, cleanSha(latest?.sha));
    return true;
  } catch {
    return false;
  }
}

export function installWebFreshnessAutoRefresh(options: WebFreshnessInstallOptions): () => void {
  const windowRef = options.windowRef ?? (typeof window !== 'undefined' ? window : undefined);
  const documentRef = options.documentRef ?? (typeof document !== 'undefined' ? document : undefined);
  if (!windowRef) {
    return () => undefined;
  }

  let checking = false;
  const runCheck = () => {
    if (checking) {
      return;
    }
    checking = true;
    void checkForFreshWebBuild({
      ...options,
      sessionStorage: options.sessionStorage ?? windowRef.sessionStorage,
      serviceWorker: options.serviceWorker ?? windowRef.navigator.serviceWorker,
      caches: options.caches ?? windowRef.caches,
      reload: options.reload ?? (() => windowRef.location.reload()),
    }).finally(() => {
      checking = false;
    });
  };
  const handleVisibility = () => {
    if (!documentRef || documentRef.visibilityState === 'visible') {
      runCheck();
    }
  };

  windowRef.setTimeout(runCheck, 0);
  windowRef.addEventListener('load', runCheck);
  windowRef.addEventListener('focus', runCheck);
  documentRef?.addEventListener('visibilitychange', handleVisibility);

  return () => {
    windowRef.removeEventListener('load', runCheck);
    windowRef.removeEventListener('focus', runCheck);
    documentRef?.removeEventListener('visibilitychange', handleVisibility);
  };
}
