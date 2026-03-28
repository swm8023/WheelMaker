import React, {useMemo, useState} from 'react';
import {Platform, Pressable, StatusBar, StyleSheet, Text, View} from 'react-native';
import {SafeAreaProvider, SafeAreaView} from 'react-native-safe-area-context';
import {WebView} from 'react-native-webview';

const ANDROID_LOCAL_WEB_URI = 'file:///android_asset/wheelmaker-web/index.html';
const DEV_WEB_URI = 'http://127.0.0.1:8080';

function initialUri(): string {
  if (__DEV__) {
    return DEV_WEB_URI;
  }
  if (Platform.OS === 'android') {
    return ANDROID_LOCAL_WEB_URI;
  }
  return DEV_WEB_URI;
}

function App(): React.JSX.Element {
  const [uri, setUri] = useState(initialUri);
  const [loadFailed, setLoadFailed] = useState(false);
  const canFallbackToDev = useMemo(() => uri !== DEV_WEB_URI, [uri]);

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
            {canFallbackToDev ? (
              <Pressable
                style={styles.retryButton}
                onPress={() => {
                  setLoadFailed(false);
                  setUri(DEV_WEB_URI);
                }}>
                <Text style={styles.retryText}>Use Dev URL</Text>
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
