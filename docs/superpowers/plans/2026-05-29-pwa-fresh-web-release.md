# PWA Fresh Web Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure WheelMaker Web/PWA clients load the latest published frontend without manual browser cache clearing.

**Architecture:** Build JS/CSS with content hashes, publish a small `web-build.json` freshness probe, keep service worker out of app shell caching, and set HTTP cache headers in the desktop asset server. The React app checks the freshness probe on lifecycle events and reloads once when the published SHA changes.

**Tech Stack:** React 19, TypeScript, webpack, Jest, Go HTTP handlers, service worker Cache Storage.

---

### Task 1: Lock the Release Cache Contract

**Files:**
- Modify: `app/__tests__/web-setup.test.js`
- Modify: `app/__tests__/web-export-release-script.test.ts`
- Modify: `server/cmd/wheelmaker-desktop/app_test.go`

- [ ] **Step 1: Write failing tests for hashed production assets, freshness metadata, service worker cache exclusions, release export, and desktop cache headers.**
- [ ] **Step 2: Run focused tests and confirm they fail for the current fixed-bundle/service-worker-shell behavior.**

### Task 2: Implement Build and Publish Metadata

**Files:**
- Modify: `app/web/webpack.config.js`
- Modify: `app/web/public/index.html`
- Modify: `app/scripts/export_web_release.js`
- Modify: `app/scripts/export_web_release.ps1`
- Create: `app/web/src/types/build.d.ts`

- [ ] **Step 1: Add content-hash output for `bundle` JS/CSS while keeping `runtime-config.js` stable.**
- [ ] **Step 2: Inject build SHA/time constants into the Web bundle.**
- [ ] **Step 3: Generate `web-build.json` during release export with asset hashes.**
- [ ] **Step 4: Run focused release tests and confirm they pass.**

### Task 3: Implement Runtime Freshness Checking

**Files:**
- Create: `app/web/src/pwa/webFreshness.ts`
- Create: `app/__tests__/web-pwa-freshness.test.ts`
- Modify: `app/web/src/main.tsx`

- [ ] **Step 1: Write failing tests for no-store build probe fetch, changed-SHA reload, same-SHA no-op, and cache cleanup.**
- [ ] **Step 2: Implement the freshness checker and wire it from `main.tsx`.**
- [ ] **Step 3: Run focused PWA freshness tests and confirm they pass.**

### Task 4: Remove App Shell From Service Worker Cache

**Files:**
- Modify: `app/web/public/service-worker.js`

- [ ] **Step 1: Keep push/notification behavior and remove cached shell entries for `/`, `/index.html`, JS/CSS, runtime config, and build metadata.**
- [ ] **Step 2: Keep network-first navigation fallback for offline use.**
- [ ] **Step 3: Run focused setup tests and confirm they pass.**

### Task 5: Set Desktop Asset Cache Headers

**Files:**
- Modify: `server/cmd/wheelmaker-desktop/server.go`

- [ ] **Step 1: Add route-aware cache headers for embedded desktop assets.**
- [ ] **Step 2: Run focused desktop Go tests and confirm they pass.**

### Task 6: Documentation and Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the Nginx/PWA docs with the split cache header model.**
- [ ] **Step 2: Run app focused tests, app typecheck, and desktop Go tests.**
- [ ] **Step 3: Run the required final git add/commit/push sequence.**
