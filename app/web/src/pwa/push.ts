export type PushDemoOptions = {
  serviceWorkerPath?: string;
};

function urlBase64ToArrayBuffer(value: string): ArrayBuffer {
  const padding = '='.repeat((4 - (value.length % 4)) % 4);
  const base64 = (value + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(base64);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i += 1) {
    bytes[i] = raw.charCodeAt(i);
  }
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
}

export class PWAPushDemo {
  private readonly serviceWorkerPath: string;

  constructor(options: PushDemoOptions = {}) {
    this.serviceWorkerPath = options.serviceWorkerPath ?? '/service-worker.js';
  }

  isSupported(): boolean {
    return (
      typeof window !== 'undefined' &&
      window.isSecureContext &&
      'serviceWorker' in navigator &&
      'PushManager' in window &&
      typeof Notification !== 'undefined'
    );
  }

  async ensureServiceWorkerRegistration(): Promise<ServiceWorkerRegistration | null> {
    if (!this.isSupported()) {
      return null;
    }
    const existing = await navigator.serviceWorker.getRegistration(this.serviceWorkerPath);
    if (existing) {
      return existing;
    }
    return navigator.serviceWorker.register(this.serviceWorkerPath);
  }

  async requestPermission(): Promise<NotificationPermission | 'unsupported'> {
    if (typeof Notification === 'undefined') {
      return 'unsupported';
    }
    return Notification.requestPermission();
  }

  async getSubscription(): Promise<PushSubscription | null> {
    const registration = await this.ensureServiceWorkerRegistration();
    if (!registration) {
      return null;
    }
    return registration.pushManager.getSubscription();
  }

  async subscribe(vapidPublicKey: string): Promise<PushSubscription | null> {
    const registration = await this.ensureServiceWorkerRegistration();
    if (!registration) {
      return null;
    }
    const permission = await this.requestPermission();
    if (permission !== 'granted') {
      return null;
    }
    return registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToArrayBuffer(vapidPublicKey),
    });
  }

  async unsubscribe(): Promise<boolean> {
    const sub = await this.getSubscription();
    if (!sub) {
      return false;
    }
    return sub.unsubscribe();
  }

  async showDemoNotification(title = 'WheelMaker PWA', body = 'PWA push demo ready'): Promise<boolean> {
    const permission = await this.requestPermission();
    if (permission !== 'granted') {
      return false;
    }

    const registration = await this.ensureServiceWorkerRegistration();
    if (registration?.active) {
      registration.active.postMessage({
        type: 'WM_PWA_DEMO_NOTIFY',
        payload: {
          title,
          body,
          url: '/',
        },
      });
      return true;
    }

    if (typeof Notification !== 'undefined') {
      // Fallback when worker is not ready yet.
      new Notification(title, {body});
      return true;
    }

    return false;
  }
}