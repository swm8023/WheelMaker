import {detectPWACapabilities, type PWACapabilities} from './capabilities';
import {ForegroundConnectionSupervisor} from './connection';
import {PWAPushDemo} from './push';
import {createPWAStorageAdapter, type PWAStorageAdapter} from './storage';

export type PWAFoundation = {
  capabilities: PWACapabilities;
  storageReady: Promise<PWAStorageAdapter>;
  pushDemo: PWAPushDemo;
  createConnectionSupervisor: typeof createConnectionSupervisor;
};

function createConnectionSupervisor(
  hooks: ConstructorParameters<typeof ForegroundConnectionSupervisor>[0],
  options?: ConstructorParameters<typeof ForegroundConnectionSupervisor>[2],
): ForegroundConnectionSupervisor {
  return new ForegroundConnectionSupervisor(hooks, undefined, options);
}

function attachFoundation(foundation: PWAFoundation): void {
  if (typeof window === 'undefined') {
    return;
  }
  window.__WHEELMAKER_PWA__ = foundation;
}

let cachedFoundation: PWAFoundation | null = null;

export function initializePWAFoundation(): PWAFoundation {
  if (cachedFoundation) {
    return cachedFoundation;
  }

  const capabilities = detectPWACapabilities();
  const pushDemo = new PWAPushDemo();
  const storageReady = createPWAStorageAdapter('wheelmaker.pwa');

  const foundation: PWAFoundation = {
    capabilities,
    storageReady,
    pushDemo,
    createConnectionSupervisor,
  };

  attachFoundation(foundation);

  void storageReady.then(async storage => {
    await storage.set('foundation.readyAt', new Date().toISOString());
  }).catch(() => {
    // Keep runtime resilient when browser storage is restricted.
  });

  cachedFoundation = foundation;
  return foundation;
}

export {detectPWACapabilities} from './capabilities';
export {ForegroundConnectionSupervisor} from './connection';
export {PWAPushDemo} from './push';
export {createPWAStorageAdapter} from './storage';