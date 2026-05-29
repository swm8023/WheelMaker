# WheelMaker App

The `app` workspace contains the Workspace Web UI. Browser builds and the
Windows `WheelMakerDesktop` executable both use this same React/webpack output.

## Directory Layout

- `web/src`: pure React UI entry and pages
- `web/public`: web static template and runtime config
- `scripts/export_web_release.js`: export web files for static hosting

## Commands

- `npm run web`: start pure React web dev server on `:8080`
- `npm run start`: alias for `npm run web`
- `npm run build:web`: build web files to `~/.wheelmaker/web` by default
- `npm run build:web:release`: build hashed web assets and export deployable files with `web-build.json`
- `npm run tsc:web`: type-check the web code

## Runtime Config

Web runtime config file:

- `web/public/runtime-config.js`

Fields:

- `defaultRegistryAddress`
- `defaultRegistryPort`
- `remoteWebUrl`

## Release Model

1. Browser/static release:
   - `npm run build:web:release`
   - Serve the exported web root.
   - Keep `/`, `/index.html`, `/service-worker.js`, `/runtime-config.js`, and `/web-build.json` revalidatable or uncached.
   - Serve `bundle.<contenthash>.js` and `bundle.<contenthash>.css` with long immutable cache headers.

2. Desktop release:
   - Run `publish-desktop.bat` from the repository root.
   - The desktop publisher embeds this web output into the Go/WebView2 desktop executable.
