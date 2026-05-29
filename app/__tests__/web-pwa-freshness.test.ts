import {
  checkForFreshWebBuild,
  isFreshWebBuild,
  type WebBuildInfo,
} from '../web/src/pwa/webFreshness';

describe('pwa web freshness', () => {
  test('treats an empty or matching server build as fresh', () => {
    const current: WebBuildInfo = {sha: 'abc', builtAt: '2026-05-29T00:00:00.000Z'};

    expect(isFreshWebBuild(current, null)).toBe(true);
    expect(isFreshWebBuild(current, {sha: '', builtAt: ''})).toBe(true);
    expect(isFreshWebBuild(current, {sha: 'abc', builtAt: '2026-05-29T00:00:00.000Z'})).toBe(true);
  });

  test('detects a newer server build by sha', () => {
    expect(isFreshWebBuild(
      {sha: 'old-sha', builtAt: '2026-05-29T00:00:00.000Z'},
      {sha: 'new-sha', builtAt: '2026-05-29T01:00:00.000Z'},
    )).toBe(false);
  });

  test('fetches the build probe without cache and reloads after clearing pwa caches', async () => {
    const deleted: string[] = [];
    const reload = jest.fn();
    const waiting = {postMessage: jest.fn()};
    const registration = {
      waiting,
      update: jest.fn(async () => undefined),
    };
    const fetchImpl = jest.fn(async () => ({
      ok: true,
      json: async () => ({schemaVersion: 1, sha: 'new-sha', builtAt: '2026-05-29T01:00:00.000Z'}),
    }));

    const refreshed = await checkForFreshWebBuild({
      currentBuild: {sha: 'old-sha', builtAt: '2026-05-29T00:00:00.000Z'},
      fetchImpl,
      serviceWorker: {
        getRegistration: jest.fn(async () => registration),
      },
      caches: {
        keys: jest.fn(async () => ['wheelmaker-web-pwa-v6', 'other-cache']),
        delete: jest.fn(async (key: string) => {
          deleted.push(key);
          return true;
        }),
      },
      reload,
    });

    expect(refreshed).toBe(true);
    expect(fetchImpl).toHaveBeenCalledWith('/web-build.json', {cache: 'no-store'});
    expect(registration.update).toHaveBeenCalled();
    expect(waiting.postMessage).toHaveBeenCalledWith('SKIP_WAITING');
    expect(deleted).toEqual(['wheelmaker-web-pwa-v6']);
    expect(reload).toHaveBeenCalledTimes(1);
  });

  test('does not reload when the server build matches', async () => {
    const reload = jest.fn();
    const fetchImpl = jest.fn(async () => ({
      ok: true,
      json: async () => ({schemaVersion: 1, sha: 'same-sha', builtAt: '2026-05-29T00:00:00.000Z'}),
    }));

    const refreshed = await checkForFreshWebBuild({
      currentBuild: {sha: 'same-sha', builtAt: '2026-05-29T00:00:00.000Z'},
      fetchImpl,
      reload,
    });

    expect(refreshed).toBe(false);
    expect(reload).not.toHaveBeenCalled();
  });
});
