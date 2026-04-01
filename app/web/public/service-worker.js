const CACHE_NAME = 'wheelmaker-web-pwa-v1';
const SHELL = ['/', '/index.html', '/runtime-config.js', '/manifest.webmanifest', '/icons/icon.svg'];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME).then(cache => cache.addAll(SHELL)).then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches
      .keys()
      .then(keys => Promise.all(keys.map(key => (key !== CACHE_NAME ? caches.delete(key) : Promise.resolve()))))
      .then(() => self.clients.claim()),
  );
});

async function networkFirst(request) {
  try {
    return await fetch(request);
  } catch (error) {
    const cached = await caches.match(request);
    if (cached) return cached;
    if (request.mode === 'navigate') {
      const shell = await caches.match('/index.html');
      if (shell) return shell;
    }
    throw error;
  }
}

self.addEventListener('fetch', event => {
  const req = event.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/ws')) return;

  event.respondWith(networkFirst(req));
});
