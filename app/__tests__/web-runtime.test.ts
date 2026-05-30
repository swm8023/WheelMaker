import {getDefaultRegistryAddress, toRegistryWsUrl} from '../web/src/runtime';

type TestWindow = {
  __WHEELMAKER_RUNTIME_CONFIG__?: {
    defaultRegistryAddress?: string;
    defaultRegistryPort?: number;
  };
  location: {
    hostname: string;
    host: string;
    protocol: string;
  };
};

describe('runtime Registry address resolution', () => {
  const originalWindow = (globalThis as {window?: unknown}).window;

  afterEach(() => {
    if (typeof originalWindow === 'undefined') {
      Reflect.deleteProperty(globalThis, 'window');
      return;
    }
    Object.defineProperty(globalThis, 'window', {
      configurable: true,
      value: originalWindow,
    });
  });

  function setWindow(input: TestWindow) {
    Object.defineProperty(globalThis, 'window', {
      configurable: true,
      value: input,
    });
  }

  test('uses registry port 9630 for localhost ws default', () => {
    setWindow({
      location: {
        hostname: '127.0.0.1',
        host: '127.0.0.1:8080',
        protocol: 'http:',
      },
    });

    expect(getDefaultRegistryAddress()).toBe('ws://127.0.0.1:9630/ws');
  });

  test('does not infer Registry address from Android appassets origin', () => {
    setWindow({
      __WHEELMAKER_RUNTIME_CONFIG__: {
        defaultRegistryPort: 9630,
      },
      location: {
        hostname: 'appassets.androidplatform.net',
        host: 'appassets.androidplatform.net',
        protocol: 'https:',
      },
    });

    expect(getDefaultRegistryAddress()).toBe('127.0.0.1:9630');
  });

  test('still infers same-origin WebSocket for normal HTTPS host', () => {
    setWindow({
      location: {
        hostname: 'workspace.example.com',
        host: 'workspace.example.com:28800',
        protocol: 'https:',
      },
    });

    expect(getDefaultRegistryAddress()).toBe('wss://workspace.example.com:28800/ws');
  });

  test('converts host and port to ws URL', () => {
    expect(toRegistryWsUrl('workspace.example.com:28800')).toBe('ws://workspace.example.com:28800/ws');
  });
});
