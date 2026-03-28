import React, {useMemo, useState} from 'react';
import {Pressable, StatusBar, StyleSheet, Text, View} from 'react-native';
import {SafeAreaProvider, SafeAreaView} from 'react-native-safe-area-context';
import {WebView} from 'react-native-webview';
import {getNativeWebViewUri, getRemoteWebUrl} from './src/config/runtime';

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
