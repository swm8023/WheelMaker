import {
  inferDesktopRemoteWebCandidate,
  submitDesktopRemoteWebCandidate,
} from '../web/src/shell/desktop/webSource';

describe('desktop web source', () => {
  test('infers secure remote web root from registry websocket URL', () => {
    expect(inferDesktopRemoteWebCandidate('wss://workspace.example.com/ws')).toEqual({
      source: 'registry',
      registryAddress: 'wss://workspace.example.com/ws',
      remoteWebUrl: 'https://workspace.example.com/',
    });
  });

  test('does not infer remote web URL for local or insecure registry addresses', () => {
    expect(inferDesktopRemoteWebCandidate('ws://127.0.0.1:9630/ws')).toBeNull();
    expect(inferDesktopRemoteWebCandidate('ws://workspace.example.com/ws')).toBeNull();
    expect(inferDesktopRemoteWebCandidate('workspace.example.com:9630')).toBeNull();
  });

  test('submits an empty remote candidate so the desktop shell can clear stale remote URL', () => {
    const originalWindow = (global as typeof globalThis & { window?: unknown }).window;
    const setRemoteWebCandidate = jest.fn();
    (global as typeof globalThis & { window?: unknown }).window = {
      WheelMakerDesktop: {
        enabled: true,
        setRemoteWebCandidate,
      },
    };
    try {
      submitDesktopRemoteWebCandidate('ws://127.0.0.1:9630/ws');
      expect(setRemoteWebCandidate).toHaveBeenCalledWith({
        source: 'registry',
        registryAddress: 'ws://127.0.0.1:9630/ws',
        remoteWebUrl: '',
      });
    } finally {
      (global as typeof globalThis & { window?: unknown }).window = originalWindow;
    }
  });
});
