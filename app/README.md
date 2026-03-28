# WheelMaker App

This app now uses a single Web UI codepath:

- Browser mode: serve built static files.
- Native mode: React Native shell + `react-native-webview` loads Web UI.

## Runtime Config

Web runtime config is injected from `public/runtime-config.js` and available as:

`window.__WHEELMAKER_RUNTIME_CONFIG__`

Fields:

- `defaultRegistryAddress`
- `defaultRegistryPort`
- `webviewSourceMode` (`local` or `remote`)
- `remoteWebUrl`
- `localWebAssetPathAndroid`
- `webDevUrl`

## Build Commands

- `npm run web`: local web dev server
- `npm run build:web`: build static files to `dist/`
- `npm run build:web:release`: copy `dist/` to `dist-web/` for Nginx deployment
- `npm run build:web:native`: copy `dist/` to `android/app/src/main/assets/wheelmaker-web/`
- `npm run android:web`: build local static web assets, then run Android app

## Release Flow

### Remote Web (Browser)

1. Run `npm run build:web:release`
2. Deploy `dist-web/*` to your Nginx static root
3. Optionally override `runtime-config.js` at deploy time

### Native App With Local Web

1. Run `npm run build:web:native`
2. Build app package as usual (`npm run android` / Android Studio)
3. App loads local static UI from Android assets by default
