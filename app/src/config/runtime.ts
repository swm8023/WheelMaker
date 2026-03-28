import {Platform} from 'react-native';

export type WebviewSourceMode = 'local' | 'remote';

type RuntimeConfig = {
  defaultRegistryAddress?: string;
  defaultRegistryPort?: number;
  webviewSourceMode?: WebviewSourceMode;
  remoteWebUrl?: string;
  localWebAssetPathAndroid?: string;
  webDevUrl?: string;
};

type WebLocationLike = {
  hostname?: string;
  host?: string;
  protocol?: string;
};

type GlobalLike = {
  __WHEELMAKER_RUNTIME_CONFIG__?: RuntimeConfig;
  location?: WebLocationLike;
  window?: {
    __WHEELMAKER_RUNTIME_CONFIG__?: RuntimeConfig;
    location?: WebLocationLike;
  };
};

const DEFAULT_REGISTRY_PORT = 9630;
const DEFAULT_WEB_DEV_URL = 'http://127.0.0.1:8080';
const DEFAULT_ANDROID_LOCAL_WEB_URI = 'file:///android_asset/wheelmaker-web/index.html';

function loadRuntimeConfig(): RuntimeConfig {
  const globalLike = globalThis as unknown as GlobalLike;
  if (globalLike.__WHEELMAKER_RUNTIME_CONFIG__) {
    return globalLike.__WHEELMAKER_RUNTIME_CONFIG__;
  }
  if (globalLike.window?.__WHEELMAKER_RUNTIME_CONFIG__) {
    return globalLike.window.__WHEELMAKER_RUNTIME_CONFIG__;
  }
  return {};
}

function buildWebDefaultRegistryAddress(): string {
  const globalLike = globalThis as unknown as GlobalLike;
  const location = globalLike.location ?? globalLike.window?.location;
  if (!location) return `127.0.0.1:${DEFAULT_REGISTRY_PORT}`;
  const {hostname, host, protocol} = location;
  if (hostname === '127.0.0.1') {
    return 'ws://127.0.0.1:6930/ws';
  }
  if (host) {
    const wsProtocol = protocol === 'https:' ? 'wss:' : 'ws:';
    return `${wsProtocol}//${host}/ws`;
  }
  return '';
}

export function getDefaultRegistryAddress(): string {
  const config = loadRuntimeConfig();
  if (config.defaultRegistryAddress?.trim()) {
    return config.defaultRegistryAddress.trim();
  }
  if (Platform.OS === 'web') {
    return buildWebDefaultRegistryAddress();
  }
  return `127.0.0.1:${config.defaultRegistryPort ?? DEFAULT_REGISTRY_PORT}`;
}

export function getNativeWebViewUri(): string {
  const config = loadRuntimeConfig();
  if (__DEV__) {
    return config.webDevUrl?.trim() || DEFAULT_WEB_DEV_URL;
  }
  if (config.webviewSourceMode === 'remote' && config.remoteWebUrl?.trim()) {
    return config.remoteWebUrl.trim();
  }
  if (Platform.OS === 'android') {
    return config.localWebAssetPathAndroid?.trim() || DEFAULT_ANDROID_LOCAL_WEB_URI;
  }
  return config.remoteWebUrl?.trim() || DEFAULT_WEB_DEV_URL;
}

export function getRemoteWebUrl(): string {
  const config = loadRuntimeConfig();
  if (config.remoteWebUrl?.trim()) {
    return config.remoteWebUrl.trim();
  }
  const globalLike = globalThis as unknown as GlobalLike;
  const location = globalLike.location ?? globalLike.window?.location;
  if (location?.host) {
    return `${location.protocol}//${location.host}/`;
  }
  return DEFAULT_WEB_DEV_URL;
}
