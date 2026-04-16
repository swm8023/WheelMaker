export type PWAPlatform = 'ios' | 'android' | 'desktop' | 'unknown';

export type PWACapabilities = {
  platform: PWAPlatform;
  isSecureContext: boolean;
  isStandalone: boolean;
  supportsServiceWorker: boolean;
  supportsNotifications: boolean;
  supportsWebPush: boolean;
  supportsForegroundWebSocket: boolean;
  supportsIndexedDB: boolean;
  supportsCacheStorage: boolean;
  supportsOPFS: boolean;
  supportsFileSystemAccess: boolean;
  supportsBackgroundSync: boolean;
  supportsPeriodicBackgroundSync: boolean;
  supportsBadging: boolean;
  supportsPersistentBackgroundConnection: false;
};

type CapabilityEnv = {
  isSecureContext?: boolean;
  navigator?: {
    userAgent?: string;
    maxTouchPoints?: number;
    standalone?: boolean;
    serviceWorker?: unknown;
    storage?: {
      getDirectory?: unknown;
    };
    setAppBadge?: unknown;
    clearAppBadge?: unknown;
  };
  matchMedia?: (query: string) => {matches: boolean};
  Notification?: unknown;
  PushManager?: unknown;
  SyncManager?: unknown;
  PeriodicSyncManager?: unknown;
  WebSocket?: unknown;
  indexedDB?: unknown;
  caches?: unknown;
  showOpenFilePicker?: unknown;
  showSaveFilePicker?: unknown;
};

function detectPlatform(userAgent: string, maxTouchPoints: number): PWAPlatform {
  const ua = userAgent.toLowerCase();
  const isIOS = /iphone|ipad|ipod/.test(ua) || (ua.includes('macintosh') && maxTouchPoints > 1);
  if (isIOS) return 'ios';
  if (/android/.test(ua)) return 'android';
  if (ua) return 'desktop';
  return 'unknown';
}

export function detectPWACapabilities(env: CapabilityEnv = globalThis as CapabilityEnv): PWACapabilities {
  const nav = env.navigator;
  const userAgent = nav?.userAgent || '';
  const maxTouchPoints = Number(nav?.maxTouchPoints || 0);
  const platform = detectPlatform(userAgent, maxTouchPoints);
  const isStandalone = !!nav?.standalone || !!env.matchMedia?.('(display-mode: standalone)')?.matches;
  const supportsServiceWorker = !!nav?.serviceWorker;
  const supportsNotifications = typeof env.Notification !== 'undefined';
  const supportsWebPush = supportsServiceWorker && supportsNotifications && typeof env.PushManager !== 'undefined';

  return {
    platform,
    isSecureContext: !!env.isSecureContext,
    isStandalone,
    supportsServiceWorker,
    supportsNotifications,
    supportsWebPush,
    supportsForegroundWebSocket: typeof env.WebSocket !== 'undefined',
    supportsIndexedDB: typeof env.indexedDB !== 'undefined',
    supportsCacheStorage: typeof env.caches !== 'undefined',
    supportsOPFS: typeof nav?.storage?.getDirectory === 'function',
    supportsFileSystemAccess:
      typeof env.showOpenFilePicker === 'function' && typeof env.showSaveFilePicker === 'function',
    supportsBackgroundSync: typeof env.SyncManager !== 'undefined',
    supportsPeriodicBackgroundSync: typeof env.PeriodicSyncManager !== 'undefined',
    supportsBadging:
      typeof nav?.setAppBadge === 'function' && typeof nav?.clearAppBadge === 'function',
    supportsPersistentBackgroundConnection: false,
  };
}