# WheelMaker App

The `app` workspace now has two clear parts:

- Native shell (React Native): `App.native.tsx` + `android/` + `ios/`
- Web product (pure React): `web/`

## Directory Layout

- `web/src`: pure React UI entry and pages
- `web/public`: web static template and runtime config
- `scripts/sync_web_assets.ps1`: copy built web files into Android assets
- `scripts/export_web_release.ps1`: export web files for Nginx/static hosting

## Commands

- `npm run web`: start pure React web dev server on `:8080`
- `npm run build:web`: build web files to `dist/`
- `npm run build:web:release`: export deployable files to `dist-web/`
- `npm run build:web:native`: build web and sync to `android/app/src/main/assets/wheelmaker-web/`
- `npm run android:web`: build synced web assets then run Android

## Runtime Config

Web runtime config file:

- `web/public/runtime-config.js`

Fields:

- `defaultRegistryAddress`
- `defaultRegistryPort`
- `remoteWebUrl`

## Release Model

1. Browser release:
   - `npm run build:web:release`
   - Deploy `dist-web/*` to Nginx static root.

2. Native release with local WebView content:
   - `npm run build:web:native`
   - Build native package normally.
