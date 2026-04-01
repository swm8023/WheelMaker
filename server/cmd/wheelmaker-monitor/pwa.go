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
      <stop offset="0%" stop-color="#0b1220"/>
      <stop offset="100%" stop-color="#13253a"/>
    </linearGradient>
    <linearGradient id="glass" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#12243a"/>
      <stop offset="100%" stop-color="#0b1828"/>
    </linearGradient>
  </defs>
  <rect x="24" y="24" width="464" height="464" rx="96" fill="url(#bg)"/>
  <rect x="102" y="132" width="308" height="220" rx="24" fill="#060b12" stroke="#3b82f6" stroke-width="12"/>
  <rect x="118" y="148" width="276" height="188" rx="14" fill="url(#glass)"/>
  <path d="M130 260 L166 260 L186 232 L210 282 L238 214 L266 260 L296 244 L324 260 L382 260"
        fill="none" stroke="#22c55e" stroke-width="14" stroke-linecap="round" stroke-linejoin="round"/>
  <circle cx="140" cy="174" r="8" fill="#22c55e"/>
  <circle cx="166" cy="174" r="8" fill="#eab308"/>
  <circle cx="192" cy="174" r="8" fill="#ef4444"/>
  <rect x="180" y="376" width="152" height="22" rx="11" fill="#0c1422" stroke="#2a3a4f" stroke-width="7"/>
  <rect x="156" y="400" width="200" height="18" rx="9" fill="#0a111d" stroke="#223248" stroke-width="7"/>
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
