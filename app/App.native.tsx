import React, {useMemo, useState} from 'react';
import {Pressable, StatusBar, StyleSheet, Text, View} from 'react-native';
import {SafeAreaProvider, SafeAreaView} from 'react-native-safe-area-context';
import {WebView} from 'react-native-webview';

type RuntimeConfig = {
  webviewSourceMode?: 'local' | 'remote';
  remoteWebUrl?: string;
  localWebAssetPathAndroid?: string;
  webDevUrl?: string;
};

type GlobalLike = {
  __WHEELMAKER_RUNTIME_CONFIG__?: RuntimeConfig;
};

const DEFAULT_WEB_DEV_URL = 'http://127.0.0.1:8080';
const DEFAULT_ANDROID_LOCAL_WEB_URI = 'file:///android_asset/wheelmaker-web/index.html';

function loadRuntimeConfig(): RuntimeConfig {
  const globalLike = globalThis as unknown as GlobalLike;
  return globalLike.__WHEELMAKER_RUNTIME_CONFIG__ ?? {};
}

function getNativeWebViewUri(): string {
  const config = loadRuntimeConfig();
  if (__DEV__) {
    return config.webDevUrl?.trim() || DEFAULT_WEB_DEV_URL;
  }
  if (config.webviewSourceMode === 'remote' && config.remoteWebUrl?.trim()) {
    return config.remoteWebUrl.trim();
  }
  return config.localWebAssetPathAndroid?.trim() || DEFAULT_ANDROID_LOCAL_WEB_URI;
}

function getRemoteWebUrl(): string {
  const config = loadRuntimeConfig();
  return config.remoteWebUrl?.trim() || DEFAULT_WEB_DEV_URL;
}

function initialUri(): string {
  return getNativeWebViewUri();
}

function App(): React.JSX.Element {
  const [uri, setUri] = useState(initialUri);
  const [loadFailed, setLoadFailed] = useState(false);
  const fallbackUri = useMemo(() => getRemoteWebUrl(), []);
  const canFallbackToRemote = useMemo(() => uri !== fallbackUri, [fallbackUri, uri]);

  return (
    <SafeAreaProvider>
      <StatusBar barStyle="light-content" />
      <SafeAreaView style={styles.root}>
        <WebView
          source={{uri}}
          style={styles.webview}
          onError={() => setLoadFailed(true)}
          onHttpError={() => setLoadFailed(true)}
          allowsBackForwardNavigationGestures
          originWhitelist={['*']}
          javaScriptEnabled
          domStorageEnabled
          allowFileAccess
          allowingReadAccessToURL="file:///"
        />
        {loadFailed ? (
          <View style={styles.errorBanner}>
            <Text style={styles.errorText}>Web UI load failed: {uri}</Text>
            {canFallbackToRemote ? (
              <Pressable
                style={styles.retryButton}
                onPress={() => {
                  setLoadFailed(false);
                  setUri(fallbackUri);
                }}>
                <Text style={styles.retryText}>Use Remote URL</Text>
              </Pressable>
            ) : null}
          </View>
        ) : null}
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
    backgroundColor: '#1e1e1e',
  },
  webview: {
    flex: 1,
  },
  errorBanner: {
    position: 'absolute',
    left: 10,
    right: 10,
    bottom: 10,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: '#3c3c3c',
    backgroundColor: '#252526',
    paddingHorizontal: 12,
    paddingVertical: 10,
    gap: 8,
  },
  errorText: {
    color: '#f48771',
    fontSize: 12,
  },
  retryButton: {
    alignSelf: 'flex-start',
    borderWidth: 1,
    borderColor: '#3c3c3c',
    backgroundColor: '#333333',
    borderRadius: 6,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  retryText: {
    color: '#d4d4d4',
    fontSize: 12,
  },
});

export default App;
