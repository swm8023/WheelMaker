import React from 'react';
import { SafeAreaView, StatusBar, StyleSheet, Text, View } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';

function App() {
  return (
    <SafeAreaProvider>
      <StatusBar barStyle="light-content" />
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.container}>
          <Text style={styles.title}>WheelMaker RN Bootstrap</Text>
          <Text style={styles.subtitle}>
            Foundation only: Observe WebSocket client + repositories are ready in
            src/services.
          </Text>
        </View>
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: '#111111',
  },
  container: {
    flex: 1,
    paddingHorizontal: 20,
    paddingTop: 24,
  },
  title: {
    color: '#f2f2f2',
    fontSize: 24,
    fontWeight: '700',
  },
  subtitle: {
    marginTop: 12,
    color: '#c8c8c8',
    fontSize: 14,
    lineHeight: 22,
  },
});

export default App;
