package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func pwaBasePath(path string) string {
	if strings.HasPrefix(path, "/monitor/") || path == "/monitor" {
		return "/monitor/"
	}
	return "/"
}

func pwaJoin(base, rel string) string {
	clean := strings.TrimLeft(rel, "/")
	if base == "/" {
		return "/" + clean
	}
	return base + clean
}

func handleManifest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := pwaBasePath(r.URL.Path)
		payload := map[string]any{
			"name":             "WheelMaker Monitor",
			"short_name":       "WM Monitor",
			"start_url":        base,
			"scope":            base,
			"display":          "standalone",
			"background_color": "#080c11",
			"theme_color":      "#0e1520",
			"icons": []map[string]any{
				{
					"src":     pwaJoin(base, "icons/icon.svg"),
					"sizes":   "512x512",
					"type":    "image/svg+xml",
					"purpose": "any maskable",
				},
			},
		}
		w.Header().Set("Content-Type", "application/manifest+json")
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func handleServiceWorker() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(serviceWorkerJS))
	}
}

func handleIcon() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(appIconSVG))
	}
}

const appIconSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#0e1520"/>
      <stop offset="100%" stop-color="#1a2535"/>
    </linearGradient>
  </defs>
  <rect x="24" y="24" width="464" height="464" rx="92" fill="url(#bg)"/>
  <rect x="96" y="112" width="320" height="288" rx="28" fill="#050810" stroke="#3b82f6" stroke-width="16"/>
  <path d="M156 214 L212 256 L156 298" fill="none" stroke="#22c55e" stroke-width="24" stroke-linecap="round" stroke-linejoin="round"/>
  <line x1="244" y1="304" x2="348" y2="304" stroke="#dde5ee" stroke-width="18" stroke-linecap="round"/>
</svg>
`

const serviceWorkerJS = `const CACHE_NAME = 'wheelmaker-monitor-pwa-v1';
const scopePath = new URL(self.registration.scope).pathname.replace(/\/?$/, '/');
const appShell = [
  scopePath,
  scopePath + 'manifest.webmanifest',
  scopePath + 'icons/icon.svg'
];
const apiPrefix = scopePath + 'api/';

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(appShell)).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.map((key) => {
      if (key !== CACHE_NAME) return caches.delete(key);
      return Promise.resolve();
    }))).then(() => self.clients.claim())
  );
});

async function networkFirst(request) {
  try {
    const fresh = await fetch(request);
    return fresh;
  } catch (_) {
    const cached = await caches.match(request);
    if (cached) return cached;
    if (request.mode === 'navigate') {
      const shell = await caches.match(scopePath);
      if (shell) return shell;
    }
    throw _;
  }
}

async function staleWhileRevalidate(request) {
  const cached = await caches.match(request);
  const fetchPromise = fetch(request).then((response) => {
    if (response && response.status === 200) {
      caches.open(CACHE_NAME).then((cache) => cache.put(request, response.clone()));
    }
    return response;
  }).catch(() => null);
  return cached || fetchPromise || fetch(request);
}

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;

  if (url.pathname.startsWith(apiPrefix) || req.mode === 'navigate') {
    event.respondWith(networkFirst(req));
    return;
  }
  event.respondWith(staleWhileRevalidate(req));
});
`
