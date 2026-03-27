import React, {useEffect, useMemo, useState} from 'react';
import {Platform, Pressable, StyleSheet, Text, TextInput, View} from 'react-native';
import type {AppTheme} from '../theme';
import {loadLastRegistryAddress, saveLastRegistryAddress} from '../services/preferences';

type ConnectScreenProps = {
  onConnect: (ipOrAddress: string, token: string) => Promise<void>;
  theme: AppTheme;
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

function defaultWebRegistryWsUrl(): string {
  if (Platform.OS !== 'web') {
    return '';
  }
  const win = globalThis as unknown as {
    location?: {search?: string; protocol?: string; host?: string};
  };
  const location = win.location;
  if (!location) {
    return '';
  }
  try {
    const params = new URLSearchParams(location.search ?? '');
    const wsOverride = params.get('ws')?.trim();
    if (wsOverride) {
      return wsOverride;
    }
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${location.host}/ws`;
  } catch {
    return '';
  }
}

export function ConnectScreen({onConnect, theme}: ConnectScreenProps) {
  const isWeb = Platform.OS === 'web';
  const webDefaultAddress = defaultWebRegistryWsUrl();
  const [ipOrAddress, setIpOrAddress] = useState(isWeb ? webDefaultAddress : '127.0.0.1');
  const [token, setToken] = useState('');
  const [manualMode, setManualMode] = useState(!isWeb);
  const [submitting, setSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [autoTried, setAutoTried] = useState(false);

  const disabled = useMemo(() => submitting || !ipOrAddress.trim(), [submitting, ipOrAddress]);

  useEffect(() => {
    let mounted = true;
    (async () => {
      if (isWeb && !manualMode) {
        return;
      }
      const lastAddress = await loadLastRegistryAddress();
      if (mounted && lastAddress) {
        setIpOrAddress(lastAddress);
      }
    })();
    return () => {
      mounted = false;
    };
  }, [isWeb, manualMode]);

  useEffect(() => {
    if (!isWeb || manualMode || autoTried || !ipOrAddress.trim()) {
      return;
    }
    setAutoTried(true);
    setSubmitting(true);
    setErrorMessage('');
    (async () => {
      try {
        await onConnect(ipOrAddress.trim(), token.trim());
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setErrorMessage(message || 'connect failed');
      } finally {
        setSubmitting(false);
      }
    })();
  }, [autoTried, ipOrAddress, isWeb, manualMode, onConnect, token]);

  const submit = async () => {
    if (disabled) {
      return;
    }
    setSubmitting(true);
    setErrorMessage('');
    try {
      const input = ipOrAddress.trim();
      await saveLastRegistryAddress(input);
      const wsUrl = toRegistryWsUrl(input);
      await onConnect(wsUrl, token.trim());
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setErrorMessage(message || 'connect failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <View style={[styles.page, {backgroundColor: theme.colors.background}]}>
      <View style={[styles.card, {borderColor: theme.colors.border, backgroundColor: theme.colors.panel}]}>
        <Text style={[styles.title, {color: theme.colors.text}]}>Connect to WheelMaker</Text>
        <Text style={[styles.hint, {color: theme.colors.textMuted}]}>Input server IP/Host and optional token.</Text>

        {isWeb && !manualMode ? (
          <View
            style={[
              styles.webAutoInfo,
              {borderColor: theme.colors.border, backgroundColor: theme.colors.inputBackground},
            ]}>
            <Text style={{color: theme.colors.textMuted}}>Auto address</Text>
            <Text selectable style={{color: theme.colors.text}}>
              {ipOrAddress}
            </Text>
          </View>
        ) : (
          <TextInput
            style={[
              styles.input,
              {
                borderColor: theme.colors.border,
                backgroundColor: theme.colors.inputBackground,
                color: theme.colors.text,
              },
            ]}
            value={ipOrAddress}
            onChangeText={setIpOrAddress}
            placeholder="127.0.0.1 or ws://127.0.0.1:9630/ws"
            placeholderTextColor={theme.colors.textMuted}
            autoCapitalize="none"
            autoCorrect={false}
          />
        )}
        <TextInput
          style={[
            styles.input,
            {
              borderColor: theme.colors.border,
              backgroundColor: theme.colors.inputBackground,
              color: theme.colors.text,
            },
          ]}
          value={token}
          onChangeText={setToken}
          placeholder="Token (optional)"
          placeholderTextColor={theme.colors.textMuted}
          autoCapitalize="none"
          autoCorrect={false}
        />

        {errorMessage ? <Text style={[styles.errorText, {color: theme.colors.error}]}>{errorMessage}</Text> : null}

        <Pressable
          style={[
            styles.button,
            {borderColor: theme.colors.border, backgroundColor: theme.colors.panelSecondary},
            disabled && styles.buttonDisabled,
          ]}
          onPress={submit}
          disabled={disabled}>
          <Text style={{color: theme.colors.text}}>
            {submitting ? 'Connecting...' : isWeb && !manualMode ? 'Retry' : 'Connect'}
          </Text>
        </Pressable>

        {isWeb ? (
          <Pressable style={styles.advancedButton} onPress={() => setManualMode(value => !value)}>
            <Text style={{color: theme.colors.accent}}>{manualMode ? 'Use Auto Mode' : 'Advanced (Manual Input)'}</Text>
          </Pressable>
        ) : null}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  page: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: 16,
  },
  card: {
    width: '100%',
    maxWidth: 420,
    borderWidth: 1,
    borderRadius: 10,
    padding: 16,
  },
  title: {
    fontSize: 18,
    fontWeight: '600',
    marginBottom: 6,
  },
  hint: {
    marginBottom: 12,
  },
  input: {
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 10,
    marginBottom: 10,
  },
  errorText: {
    marginBottom: 10,
  },
  button: {
    borderWidth: 1,
    borderRadius: 8,
    minHeight: 40,
    alignItems: 'center',
    justifyContent: 'center',
  },
  buttonDisabled: {
    opacity: 0.6,
  },
  webAutoInfo: {
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 10,
    marginBottom: 10,
    gap: 4,
  },
  advancedButton: {
    marginTop: 10,
    alignSelf: 'flex-start',
  },
});

export {toRegistryWsUrl};
