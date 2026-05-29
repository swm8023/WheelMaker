# PWA Fresh Web Release Design

## Goal

WheelMaker Web and installed PWA clients should load the latest published frontend after each update without requiring users to manually clear browser or WebView cache.

## Resource Model

The Web release uses a split cache model:

- Entry and freshness resources stay revalidatable:
  - `/` and `/index.html`: `no-cache, must-revalidate`
  - `/service-worker.js`: `no-cache, must-revalidate`
  - `/runtime-config.js`: `no-store`
  - `/web-build.json`: `no-store`
- Versioned assets stay strongly cacheable:
  - `bundle.<contenthash>.js`: `public, max-age=31536000, immutable`
  - `bundle.<contenthash>.css`: `public, max-age=31536000, immutable`
  - font and icon assets: long cache where their file names are versioned or stable enough

The app keeps cache efficiency for large JS/CSS files because unchanged builds keep the same content hash names. A new build changes the referenced asset names in `index.html`, so clients cannot keep running an old bundle after receiving a fresh entry document.

## Build Metadata

The build injects current Web build metadata into the app bundle and writes `web-build.json` during release export:

```json
{
  "schemaVersion": 1,
  "sha": "18708677bd740bcd241bc64f6f69f8caf57d26c3",
  "builtAt": "2026-05-29T00:00:00.000Z",
  "assets": {
    "bundle.js": "sha256:...",
    "bundle.css": "sha256:..."
  }
}
```

`sha` comes from `WHEELMAKER_WEB_BUILD_SHA` when set, otherwise from `git rev-parse HEAD`. `builtAt` comes from `WHEELMAKER_WEB_BUILD_TIME` when set, otherwise the current UTC time.

## Runtime Freshness Check

On load, focus, and visibility resume, the app fetches `/web-build.json` with `cache: 'no-store'`.

If the server build SHA is non-empty and differs from the currently running build SHA, the app activates the latest service worker when available, clears old WheelMaker PWA Cache Storage entries, and reloads once. If no new build is present, the check is only a small JSON request and does not re-download hashed JS/CSS.

## Service Worker Policy

The service worker keeps notification and push support but does not cache `/`, `/index.html`, `bundle*.js`, `bundle*.css`, `runtime-config.js`, or `web-build.json`. It may cache icons for install and notification display.

Navigation requests stay network-first and may fall back to `/index.html` only while offline.

## Deployment Notes

The published web directory must include `service-worker.js`, `manifest.webmanifest`, icons, hashed bundle assets, `runtime-config.js`, and `web-build.json`.

Nginx deployments should set matching headers for entry/freshness resources and hashed assets so browser and PWA behavior is consistent outside the desktop embedded asset server.
