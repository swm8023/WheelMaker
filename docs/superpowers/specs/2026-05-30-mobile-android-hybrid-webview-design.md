# Mobile Android Hybrid WebView Design

Date: 2026-05-30
Status: Approved

## Goal

Add an Android APK/AAB distribution for WheelMaker without rewriting the Workspace product or embedding the Go runtime in the APK.

The Android app is a native Kotlin WebView shell. It opens the existing Workspace Web UI, starts from an embedded Web snapshot on first launch, then prefers the remote self-hosted Web release after the user configures the Registry address in the existing Web settings.

## Product Language

- **Workspace Web UI**: the existing React app under `app/`.
- **WheelMaker Android**: the Android native shell under `mobile/android/`.
- **Remote Web**: the self-hosted Web release served by Machine A, normally `https://<host>:28800/`.
- **Embedded Web**: the Web UI snapshot packaged inside the APK.
- **Go services**: Hub, Registry, Monitor, Updater, and agent adapters under `server/`.

## Decisions

1. Android uses native Kotlin, Gradle, and Android WebView.
2. Android does not use Go, gomobile, React Native, Capacitor, Cordova, or Flutter in the first version.
3. Android starts with Embedded Web on first launch.
4. After the user enters the Registry address in existing Web settings, the Web UI infers a Remote Web candidate and submits it to the Android shell.
5. Android persists the Remote Web candidate and then prefers Remote Web in auto mode.
6. If Remote Web is unavailable, Android falls back to Embedded Web.
7. Repo-local generated files are not allowed. Web build output, Gradle build output, Gradle cache, APK/AAB files, release manifests, and temporary assets all live outside the git worktree.
8. The Go services remain external runtime dependencies. The APK does not start, supervise, or package Hub, Registry, Monitor, or Updater.
9. Android always navigates WebView to a stable app-owned origin; the native request handler chooses Remote Web or Embedded Web per resource.
10. The first implementation only builds a local APK. It does not expose APK downloads through Update, Monitor, Nginx, or the Web publish root.

## Non-Goals

- Do not rewrite Chat, File, Git, Settings, or session logic in Android.
- Do not move Registry or Hub into the APK.
- Do not replace the browser/PWA/Desktop Web release path.
- Do not implement native Android speech recognition or direct Doubao access in the first Android shell slice.
- Do not bypass TLS errors in release builds.
- Do not publish to Play Store in the first slice.
- Do not add service discovery or QR pairing in the first slice.
- Do not add an APK download card to the Update screen in the first slice.
- Do not copy Android APK artifacts into `~/.wheelmaker/web`.
- Do not change Nginx or Monitor routes for Android artifact download in the first slice.

## Repository Layout

Add source and configuration only:

```text
mobile/
  android/
    settings.gradle.kts
    build.gradle.kts
    app/
      build.gradle.kts
      src/main/AndroidManifest.xml
      src/main/java/.../MainActivity.kt
      src/main/res/...
```

Do not write generated Web assets to `mobile/android/app/src/main/assets/`.

Defensive ignore rules should still reject accidental Android-generated paths:

```text
mobile/android/.gradle/
mobile/android/**/build/
mobile/android/local.properties
mobile/android/app/src/main/assets/
```

The scripts should not rely on those ignore rules during normal publishing; they should avoid creating those paths at all.

## Build And Publish Directories

Use `~/.wheelmaker` for Android publish output and build workspace:

```text
~/.wheelmaker/mobile/android/
  WheelMakerAndroid.apk
  WheelMakerAndroid.aab
  android-release.json

~/.wheelmaker/build/mobile/android/
  webroot/
  gradle-build/
  gradle-cache/
  gradle-home/
```

`~/.wheelmaker/web` remains the normal browser/PWA Remote Web publish directory. Android publish builds a separate Embedded Web snapshot and does not overwrite `~/.wheelmaker/web`.

The first Android slice stops at local build output. `android-release.json` records what was built, but no service exposes it for download.

## Android Publish Flow

Add a root entrypoint:

```text
publish-android.bat
```

It calls:

```text
scripts/publish_android.ps1
```

The script:

1. Resolves the repo root.
2. Creates or cleans `~/.wheelmaker/build/mobile/android/webroot`.
3. Runs the Web build with `WHEELMAKER_WEB_TARGET` set to that external `webroot`.
4. Runs the existing Web public asset export so the embedded snapshot contains `index.html`, `runtime-config.js`, `web-build.json`, `manifest.webmanifest`, `service-worker.js`, icons, and hashed bundle assets.
5. Runs Gradle from `mobile/android/` with external build/cache directories.
6. Passes the external Web root into Gradle as `-PwheelmakerWebAssetsDir=<webroot>`.
7. Writes APK/AAB and `android-release.json` to `~/.wheelmaker/mobile/android`.

Suggested Gradle invocation shape:

```text
gradlew assembleRelease
  --project-cache-dir ~/.wheelmaker/build/mobile/android/gradle-cache
  -g ~/.wheelmaker/build/mobile/android/gradle-home
  -PwheelmakerBuildRoot=~/.wheelmaker/build/mobile/android/gradle-build
  -PwheelmakerWebAssetsDir=~/.wheelmaker/build/mobile/android/webroot
```

Gradle configuration must set root and subproject build directories under `wheelmakerBuildRoot`, not under the repo.

Do not generate `mobile/android/local.properties`; SDK discovery should use `ANDROID_HOME` or `ANDROID_SDK_ROOT`.

## Stable WebView Origin

Android should always navigate the WebView to a stable HTTPS-like app origin, for example:

```text
https://appassets.androidplatform.net/
```

This mirrors Desktop's stable `127.0.0.1:9632` storage-origin goal without opening a TCP listener on Android.

The stable origin must serve the Web build at root paths because the current Web output uses absolute paths such as `/bundle.<hash>.js`, `/runtime-config.js`, `/service-worker.js`, and `/web-build.json`.

The WebView should prefer AndroidX WebKit `WebViewAssetLoader` where practical. A focused `WebViewClient.shouldInterceptRequest` fallback is acceptable if it gives better control over root-path mapping, content types, and cache headers.

Remote Web is selected by the native resource handler, not by navigating the WebView to the remote URL. In auto mode:

```text
https://appassets.androidplatform.net/index.html
  -> remote resource when Remote Web is configured and healthy
  -> embedded APK asset otherwise
```

This keeps IndexedDB, localStorage, Cache Storage, and Service Worker state attached to one Android app origin across Remote Web and Embedded Web fallback.

## Web Source Runtime

Android mirrors the Desktop Web source model, but implemented in Kotlin:

- Preference: `auto` or `embedded`.
- Remote candidate: saved `remoteWebUrl` and the Registry origin that produced it.
- Actual source: `remote` or `embedded`.
- State exposed to Web: preference, actual source, display title/source, remote URL, remote host.

Startup behavior:

1. Load persisted source config.
2. Navigate WebView to the stable Android app origin.
3. If preference is `embedded` or no valid remote URL exists, serve Embedded Web resources.
4. If preference is `auto` and a remote URL exists, probe Remote Web.
5. If the probe succeeds, serve root-path resources by proxying Remote Web through the stable origin.
6. If the probe fails, serve Embedded Web resources through the same stable origin.

Remote probing should request the remote root or `index.html` with a short timeout. A 2xx response is healthy. Network errors, TLS errors, invalid URL, and unsupported scheme all fall back to Embedded Web.

The Android Web source runtime should keep the current source decision in native memory and expose it to Web through the bridge. It should not make the Web app responsible for loading remote assets directly.

## Native Bridge

Expose an Android-specific bridge:

```ts
window.WheelMakerAndroid = {
  enabled: true,
  getWebSourceState(): Promise<NativeWebSourceState>,
  setWebSourcePreference(preference: 'auto' | 'embedded'): Promise<NativeWebSourceState>,
  setRemoteWebCandidate(candidate: NativeRemoteWebCandidate): Promise<NativeWebSourceState>,
}
```

The Web UI should not duplicate Desktop and Android remote-candidate logic. Extract a small generic native Web source adapter:

```text
app/web/src/shell/native/
  webSource.ts
  runtime.ts
```

Desktop can keep `window.WheelMakerDesktop`; Android uses `window.WheelMakerAndroid`. The shared Web helper should detect either bridge and submit the same remote candidate shape.

## Registry Address Handling

Android first launch opens Embedded Web, so the default Web origin is not the real Registry host. The Web runtime must not infer `wss://appassets.androidplatform.net/ws` as a useful default Registry address.

The initial flow remains:

1. User opens Embedded Web.
2. User enters the Registry address in the existing settings screen.
3. Web connects to Registry through the current `RegistryClient`.
4. Web infers `remoteWebUrl` from the non-loopback Registry address.
5. Web submits that remote candidate to `window.WheelMakerAndroid`.
6. Next Android app launch can prefer Remote Web.

Loopback Registry addresses should not become Remote Web candidates, matching Desktop behavior.

## Local Storage Model

The first Android version continues using the existing Web persistence stack:

- IndexedDB for Workspace data, settings, Registry address/token, UI state, session and file caches, and speech settings.
- localStorage/sessionStorage only where the existing Web app already uses them.
- Android SharedPreferences or DataStore only for native shell state: Web source preference, Remote Web URL, Registry-origin metadata for the remote candidate, and lightweight shell flags.

Do not migrate Workspace business state into Android SQLite, Room, DataStore, or files in the first slice.

Clearing Chrome or Edge browser cache does not affect the APK WebView data directory. Clearing WheelMaker Android app storage or uninstalling the APK removes WebView IndexedDB, WebStorage, Cache Storage, and native shell preferences. Clearing only the app cache should not be treated as a supported Workspace reset path.

If a future in-app reset is added, it should explicitly delete WebView storage and native shell preferences under a clear user action.

## Platform Responsibilities

Native Android owns platform primitives:

- WebView lifecycle.
- Android back handling.
- WebView permission prompts.
- Microphone permission pass-through.
- File chooser and image upload.
- Notification permission and display plumbing where WebView/PWA support is insufficient.
- Download handling.
- External URL handling.
- Foreground/background lifecycle and WebView resume.

Web owns product interaction state:

- Chat composer and voice button behavior.
- transcript insertion/cancel/restore policy.
- Chat, File, Git, Settings, Port Relay UI.
- Registry connection and Workspace persistence.

Go owns server protocol and provider integration:

- Registry `/ws` protocol.
- Hub routing.
- Session, File, Git, command, relay, and speech server handlers.
- Existing Volcengine/Doubao proxy for Web/PWA/Desktop.

## Speech Iteration Path

First Android version does not duplicate speech logic. It keeps the current Web speech flow:

```text
Web getUserMedia
-> Web voice controller
-> Registry speech.*
-> Go Registry proxies Doubao
-> transcript events return to Web
```

Later Android speech work should proceed in stages:

1. **Native audio capture only**: Android captures microphone PCM and passes chunks through a JS bridge to the existing Web voice controller. Registry speech and Web transcript insertion stay unchanged.
2. **Native Doubao provider**: Android captures PCM and connects directly to Doubao. Android emits the same provider events to Web:

```ts
speech.transcript { text: string, final: boolean }
speech.error { code: string, message: string, retryable: boolean }
speech.state { phase: string }
```

Do not duplicate the Chat composer voice state machine in Kotlin. Android may replace provider adapters, but Web remains the owner of input state, cancel semantics, and transcript insertion.

The intended Web abstraction is:

```ts
interface SpeechProvider {
  start(config: SpeechStartConfig): Promise<void>;
  finish(): Promise<void>;
  cancel(reason: string): Promise<void>;
  onTranscript(callback: (event: SpeechTranscriptEvent) => void): () => void;
  onError(callback: (event: SpeechErrorEvent) => void): () => void;
}
```

Providers:

- `registrySpeechProvider`: Web/PWA/Desktop/default Android first version.
- `androidNativeSpeechProvider`: future APK-only provider.

Shared contracts should lock audio format and event semantics:

```text
16000Hz / 16-bit / mono / 200ms PCM chunks
```

This deliberately limits duplicated code to the lowest-level provider adapter.

## Security

- Release builds must not ignore TLS errors.
- Allowing cleartext HTTP Remote Web should be an explicit debug or user-configured decision, not the release default.
- The Registry token remains in Web persistence and is not copied into Android native storage.
- Android should not log tokens, speech API keys, audio chunks, transcript contents in native debug logs, or full WebView URLs containing sensitive query parameters.
- Embedded Web is a fallback snapshot, not a trusted replacement for Registry authentication.
- Native bridge methods must validate URL schemes and reject loopback Remote Web candidates.

## Testing

Web tests:

- Native bridge detection supports Android and Desktop without duplicating candidate inference.
- Embedded Android origin does not become the default Registry WebSocket address.
- Remote Web candidate inference rejects loopback addresses and accepts valid public `ws`, `wss`, `http`, and `https` Registry addresses.
- Existing `cli-setup` tests are updated to allow `mobile/android/` while still rejecting React Native scaffolding.
- Android-specific Web source state is kept outside Workspace IndexedDB; Workspace data remains in existing Web persistence.

Android unit/instrumentation tests:

- Web source config sanitizes invalid URLs and unsupported schemes.
- Auto mode prefers Remote Web when probe succeeds.
- Auto mode falls back to Embedded Web when probe fails.
- Embedded asset handler serves root-path Web files with expected content types.
- Remote assets are proxied through the stable app origin instead of navigating to the remote origin.
- JS bridge returns and mutates source state.
- Back handling delegates to Web history before closing the Activity.

Script tests:

- `publish_android.ps1 -WhatIf` does not write generated files under the repo.
- Web assets are built under `~/.wheelmaker/build/mobile/android/webroot`.
- Gradle build/cache/home paths are outside the repo.
- APK and `android-release.json` are written under `~/.wheelmaker/mobile/android`.
- Android publish does not copy APK artifacts into `~/.wheelmaker/web`.

Manual verification:

- Fresh install opens Embedded Web.
- Entering a public Registry address connects and submits a Remote Web candidate.
- Restart serves Remote Web through the stable app origin.
- Blocking Remote Web falls back to Embedded Web while preserving existing IndexedDB settings.
- Microphone prompt, voice input, file upload, image upload, Android back, keyboard resize, and foreground/background reconnect work on a real Android device.

## Rollout

1. Add Android project skeleton with external build directories and no repo-generated artifacts.
2. Add publish script that builds Web snapshot into `~/.wheelmaker/build/mobile/android/webroot`.
3. Add WebView shell and Embedded Web loading.
4. Add Android Web source runtime and JS bridge.
5. Extract shared native Web source helper in Web UI and wire Android candidate submission.
6. Add platform permission handling for microphone and file chooser.
7. Add focused tests and README instructions for local APK build output.
8. Defer Update-screen APK download, artifact hosting, and native Android speech provider to separate designs after the shell is stable.

## Self-Review

- No placeholders or unresolved TBDs.
- The design keeps Go out of the APK and avoids Go service changes for the first Android shell.
- The build model keeps generated files outside the repo.
- The first slice stops at local APK build output and does not require Nginx or Monitor route changes.
- The stable Android app origin preserves IndexedDB across Remote Web and Embedded Web fallback.
- The future speech path limits duplication to provider adapters and keeps product state in Web.
- The Remote Web / Embedded Web source model matches the existing Desktop direction while using Android-native APIs.
