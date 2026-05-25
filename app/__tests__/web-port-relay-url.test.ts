import {buildPortRelayOpenUrl, resolvePortRelayOpenUrl} from '../web/src/portRelayUrl';

describe('port relay URL helpers', () => {
  test('maps registry wss connection to https relay URL on the selected port', () => {
    expect(buildPortRelayOpenUrl('wss://relay.example.com:28800/ws', 28801)).toBe('https://relay.example.com:28801/');
  });

  test('prefers relay URL returned by registry snapshot', () => {
    expect(resolvePortRelayOpenUrl({
      relayUrl: 'https://relay.example.com:28801/',
      registryAddress: 'ws://127.0.0.1:9630/ws',
      listenPort: 28801,
    })).toBe('https://relay.example.com:28801/');
  });

  test('derives from external registry address when snapshot relay URL is loopback', () => {
    expect(resolvePortRelayOpenUrl({
      relayUrl: 'http://127.0.0.1:28801/',
      registryAddress: 'wss://vimernas.myqnapcloud.com/ws',
      listenPort: 28801,
    })).toBe('https://vimernas.myqnapcloud.com:28801/');
  });

  test('keeps loopback relay URL for local desktop registry connections', () => {
    expect(resolvePortRelayOpenUrl({
      relayUrl: 'http://127.0.0.1:28801/',
      registryAddress: 'ws://127.0.0.1:9630/ws',
      listenPort: 28801,
    })).toBe('http://127.0.0.1:28801/');
  });
});
