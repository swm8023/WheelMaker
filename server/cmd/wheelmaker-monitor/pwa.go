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
    <linearGradient id="bridge" x1="0" y1="0" x2="1" y2="0">
      <stop offset="0%" stop-color="#22c55e"/>
      <stop offset="50%" stop-color="#3b82f6"/>
      <stop offset="100%" stop-color="#22c55e"/>
    </linearGradient>
  </defs>
  <rect x="24" y="24" width="464" height="464" rx="96" fill="url(#bg)"/>
  <rect x="98" y="126" width="130" height="108" rx="18" fill="#07101b" stroke="#2f4667" stroke-width="9"/>
  <rect x="284" y="278" width="130" height="108" rx="18" fill="#07101b" stroke="#2f4667" stroke-width="9"/>
  <path d="M138 176 h48 M138 198 h74 M138 220 h38" stroke="#9fb3cc" stroke-width="9" stroke-linecap="round"/>
  <path d="M324 328 h48 M324 350 h74 M324 372 h38" stroke="#9fb3cc" stroke-width="9" stroke-linecap="round"/>
  <path d="M228 234 L284 278" stroke="url(#bridge)" stroke-width="18" stroke-linecap="round"/>
  <path d="M260 246 L228 234 L238 264" fill="none" stroke="#3b82f6" stroke-width="10" stroke-linecap="round" stroke-linejoin="round"/>
  <path d="M270 248 L284 278 L254 268" fill="none" stroke="#22c55e" stroke-width="10" stroke-linecap="round" stroke-linejoin="round"/>
  <circle cx="216" cy="224" r="13" fill="#3b82f6"/>
  <circle cx="296" cy="288" r="13" fill="#22c55e"/>
  <circle cx="356" cy="144" r="24" fill="#0b1726" stroke="#3b82f6" stroke-width="9"/>
  <path d="M346 144 h20 M356 134 v20" stroke="#dbe7f6" stroke-width="7" stroke-linecap="round"/>
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
