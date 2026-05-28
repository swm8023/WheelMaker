import {ForegroundConnectionSupervisor} from '../web/src/pwa/connection';

function createSupervisorEnv() {
  const documentListeners = new Map<string, () => void>();
  const windowListeners = new Map<string, () => void>();
  const env = {
    document: {
      hidden: false,
      addEventListener: (name: string, cb: () => void) => {
        documentListeners.set(name, cb);
      },
      removeEventListener: (name: string) => {
        documentListeners.delete(name);
      },
    },
    window: {
      addEventListener: (name: string, cb: () => void) => {
        windowListeners.set(name, cb);
      },
      removeEventListener: (name: string) => {
        windowListeners.delete(name);
      },
    },
    navigator: {onLine: true},
    setTimeoutImpl: setTimeout,
    clearTimeoutImpl: clearTimeout,
  };
  return {env, documentListeners, windowListeners};
}

describe('PWA foreground connection supervisor', () => {
  test('disconnects when the app enters background by default', () => {
    const {env, documentListeners} = createSupervisorEnv();
    const hooks = {
      connect: jest.fn(),
      disconnect: jest.fn(),
    };
    const supervisor = new ForegroundConnectionSupervisor(hooks, env);

    supervisor.start();
    env.document.hidden = true;
    documentListeners.get('visibilitychange')?.();

    expect(hooks.disconnect).toHaveBeenCalledWith('background');
  });

  test('keeps the registry connection during transient background while blocked', () => {
    const {env, documentListeners} = createSupervisorEnv();
    const hooks = {
      connect: jest.fn(),
      disconnect: jest.fn(),
    };
    const supervisor = new ForegroundConnectionSupervisor(hooks, env, {
      shouldDisconnectOnBackground: () => false,
    });

    supervisor.start();
    env.document.hidden = true;
    documentListeners.get('visibilitychange')?.();

    expect(hooks.disconnect).not.toHaveBeenCalledWith('background');
  });
});
