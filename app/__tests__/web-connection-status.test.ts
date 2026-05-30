import {
  resolveRegistryConnectionStatus,
  resolveVoiceCapabilityStatus,
  resolveWebResourceConnectionStatus,
} from '../web/src/settings/connectionStatus';

describe('connection settings status model', () => {
  test('describes local and remote Web resource sources', () => {
    expect(resolveWebResourceConnectionStatus({
      preference: 'auto',
      actualSource: 'embedded',
      displayTitle: 'WheelMaker - Embedded',
      displaySource: 'Embedded',
      remoteUrl: 'https://workspace.example.com/',
      remoteHost: 'workspace.example.com',
    })).toEqual({
      label: 'Local',
      detail: 'Embedded Web resources',
      remoteUrl: 'https://workspace.example.com/',
    });

    expect(resolveWebResourceConnectionStatus({
      preference: 'auto',
      actualSource: 'remote',
      displayTitle: 'WheelMaker - workspace.example.com',
      displaySource: 'workspace.example.com',
      remoteUrl: 'https://workspace.example.com/',
      remoteHost: 'workspace.example.com',
    })).toEqual({
      label: 'Remote',
      detail: 'workspace.example.com',
      remoteUrl: 'https://workspace.example.com/',
    });
  });

  test('describes browser fallback when no native Web source state exists', () => {
    expect(resolveWebResourceConnectionStatus(null)).toEqual({
      label: 'Browser',
      detail: 'Native Web source state is unavailable',
      remoteUrl: '',
    });
  });

  test('describes registry connection state', () => {
    expect(resolveRegistryConnectionStatus({
      connected: true,
      reconnecting: false,
      autoConnecting: false,
      address: 'ws://registry.example/ws',
    })).toEqual({
      label: 'Connected',
      detail: 'ws://registry.example/ws',
    });

    expect(resolveRegistryConnectionStatus({
      connected: false,
      reconnecting: true,
      autoConnecting: false,
      address: 'ws://registry.example/ws',
    }).label).toBe('Reconnecting');
  });

  test('describes voice capability source without falling back from Android native', () => {
    expect(resolveVoiceCapabilityStatus({
      speechEnabled: true,
      androidNativeHost: true,
      androidNativeAvailable: true,
    })).toEqual({
      label: 'Android Native',
      detail: 'APK microphone capture, direct Doubao connection',
    });

    expect(resolveVoiceCapabilityStatus({
      speechEnabled: true,
      androidNativeHost: true,
      androidNativeAvailable: false,
    })).toEqual({
      label: 'Unavailable',
      detail: 'Android native speech bridge is missing',
    });

    expect(resolveVoiceCapabilityStatus({
      speechEnabled: true,
      androidNativeHost: false,
      androidNativeAvailable: false,
    })).toEqual({
      label: 'Registry',
      detail: 'Registry speech bridge',
    });
  });
});
