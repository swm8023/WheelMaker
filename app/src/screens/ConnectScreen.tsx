import React, {useMemo, useState} from 'react';
import {Pressable, StyleSheet, Text, TextInput, View} from 'react-native';

type ConnectScreenProps = {
  onConnect: (ipOrAddress: string, token: string) => Promise<void>;
};

function toRegistryWsUrl(ipOrAddress: string): string {
  const input = ipOrAddress.trim();
  if (!input) {
    throw new Error('IP is required');
  }

  if (/^wss?:\/\//i.test(input)) {
    return input;
  }
  if (/^https?:\/\//i.test(input)) {
    return input.replace(/^http:\/\//i, 'ws://').replace(/^https:\/\//i, 'wss://');
  }

  const hasPort = /:\d+$/.test(input);
  const host = hasPort ? input : `${input}:9630`;
  return `ws://${host}/ws`;
}

export function ConnectScreen({onConnect}: ConnectScreenProps) {
  const [ipOrAddress, setIpOrAddress] = useState('127.0.0.1');
  const [token, setToken] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');

  const disabled = useMemo(() => submitting || !ipOrAddress.trim(), [submitting, ipOrAddress]);

  const submit = async () => {
    if (disabled) {
      return;
    }
    setSubmitting(true);
    setErrorMessage('');
    try {
      const wsUrl = toRegistryWsUrl(ipOrAddress);
      await onConnect(wsUrl, token.trim());
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setErrorMessage(message || 'connect failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <View style={styles.page}>
      <View style={styles.card}>
        <Text style={styles.title}>Connect to WheelMaker</Text>
        <Text style={styles.hint}>Input server IP/Host and optional token.</Text>

        <TextInput
          style={styles.input}
          value={ipOrAddress}
          onChangeText={setIpOrAddress}
          placeholder="127.0.0.1 or ws://127.0.0.1:9630/ws"
          autoCapitalize="none"
          autoCorrect={false}
        />
        <TextInput
          style={styles.input}
          value={token}
          onChangeText={setToken}
          placeholder="Token (optional)"
          autoCapitalize="none"
          autoCorrect={false}
        />

        {errorMessage ? <Text style={styles.errorText}>{errorMessage}</Text> : null}

        <Pressable
          style={[styles.button, disabled && styles.buttonDisabled]}
          onPress={submit}
          disabled={disabled}>
          <Text>{submitting ? 'Connecting...' : 'Connect'}</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  page: {
    flex: 1,
    backgroundColor: '#fff',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 16,
  },
  card: {
    width: '100%',
    maxWidth: 420,
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 10,
    padding: 16,
  },
  title: {
    fontSize: 18,
    fontWeight: '600',
    marginBottom: 6,
  },
  hint: {
    color: '#666',
    marginBottom: 12,
  },
  input: {
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 10,
    marginBottom: 10,
  },
  errorText: {
    color: '#b11',
    marginBottom: 10,
  },
  button: {
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 8,
    minHeight: 40,
    alignItems: 'center',
    justifyContent: 'center',
  },
  buttonDisabled: {
    opacity: 0.6,
  },
});

export {toRegistryWsUrl};
