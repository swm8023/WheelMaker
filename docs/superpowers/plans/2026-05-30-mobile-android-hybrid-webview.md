# Mobile Android Hybrid WebView Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local Android APK distribution that wraps the existing Workspace Web UI in a native Kotlin WebView shell with a stable app origin, Remote Web preference, Embedded Web fallback, and zero repo-local generated artifacts.

**Architecture:** Add a `mobile/android/` Kotlin/Gradle project that always loads `https://appassets.androidplatform.net/`. Android intercepts root-path Web requests and serves Remote Web resources when healthy, otherwise APK-embedded Web assets. Web product state stays in IndexedDB; Android stores only native shell Web source settings.

**Tech Stack:** React 19 / TypeScript / Jest, Kotlin, Android Gradle Plugin 8.13.2, AndroidX WebKit 1.15.0, Android WebView, PowerShell publish script, Gradle CLI from PATH.

---

## Scope

This plan implements only local APK build output:

- Output APK and manifest to `~/.wheelmaker/mobile/android/`.
- Build all temporary Web and Gradle files under `~/.wheelmaker/build/mobile/android/`.
- Do not modify Nginx.
- Do not modify Go server or Monitor routes.
- Do not add APK download to the Update screen.
- Do not copy APK artifacts into `~/.wheelmaker/web`.

## File Structure

- `docs/superpowers/specs/2026-05-30-mobile-android-hybrid-webview-design.md` - already approved design; this plan keeps it as the source of truth.
- `.gitignore` - add defensive Android generated-output ignores.
- `app/__tests__/cli-setup.test.js` - allow `mobile/android/` native shell while continuing to reject React Native scaffolding under `app/`.
- `app/__tests__/web-native-web-source.test.ts` - tests for shared Desktop/Android native Web source helpers.
- `app/__tests__/web-runtime.test.ts` - tests for ignoring Android appassets origin as a Registry default.
- `app/web/src/runtime.ts` - do not infer Registry URL from `appassets.androidplatform.net`.
- `app/web/src/shell/native/webSource.ts` - shared remote Web candidate inference and native bridge submission for Desktop and Android.
- `app/web/src/shell/desktop/webSource.ts` - delegate to shared native Web source helper while preserving exported Desktop API names.
- `app/web/src/shell/desktopRuntime.ts` - extend bridge types only if needed for the shared helper.
- `mobile/android/settings.gradle.kts` - Android project settings with external build directory wiring.
- `mobile/android/build.gradle.kts` - plugin versions and repository configuration.
- `mobile/android/app/build.gradle.kts` - app module, external `buildDirectory`, assets source from `wheelmakerWebAssetsDir`, dependencies.
- `mobile/android/app/src/main/AndroidManifest.xml` - permissions and single Activity declaration.
- `mobile/android/app/src/main/java/com/wheelmaker/android/*.kt` - WebView shell, Web source runtime, stable-origin request handler, bridge, file chooser and permission handling.
- `mobile/android/app/src/main/res/values/*.xml` - app labels and themes.
- `mobile/android/app/src/test/java/com/wheelmaker/android/*.kt` - JVM tests for source config, source runtime, and asset path handling.
- `publish-android.bat` - root entrypoint.
- `scripts/publish_android.ps1` - local APK build script.
- `scripts/test_publish_android_ps1.ps1` - script structure tests.
- `README.md` and `app/README.md` - local Android build usage and output paths.

## Task 1: Guardrails And Existing Test Updates

**Files:**
- Modify: `.gitignore`
- Modify: `app/__tests__/cli-setup.test.js`
- Create: `scripts/test_publish_android_ps1.ps1`

- [ ] **Step 1: Write failing tests for Android shell allowance and publish-script guardrails**

Modify `app/__tests__/cli-setup.test.js` so the native path test allows the new `mobile/android/` source tree but still rejects React Native scaffolding under `app/`.

Use this replacement for the third test:

```js
  test('does not keep React Native shell project files in the web app workspace', () => {
    const nativePaths = [
      path.join('android'),
      path.join('ios'),
      'App.tsx',
      'App.native.tsx',
      'index.js',
      'metro.config.js',
      'babel.config.js',
      'app.json',
      path.join('scripts', 'sync_web_assets.ps1'),
    ];
    const existingPaths = nativePaths.filter(nativePath =>
      fs.existsSync(path.join(projectRoot, nativePath)),
    );

    expect(existingPaths).toEqual([]);
  });
```

Create `scripts/test_publish_android_ps1.ps1`:

```powershell
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\publish_android.ps1"
$batPath = Join-Path $repoRoot "publish-android.bat"

function Assert-Contains {
  param(
    [Parameter(Mandatory = $true)][string]$Label,
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$Needle
  )
  if (-not $Text.Contains($Needle)) {
    throw "$Label missing expected text: $Needle"
  }
}

function Assert-NotContains {
  param(
    [Parameter(Mandatory = $true)][string]$Label,
    [Parameter(Mandatory = $true)][string]$Text,
    [Parameter(Mandatory = $true)][string]$Needle
  )
  if ($Text.Contains($Needle)) {
    throw "$Label should not contain text: $Needle"
  }
}

if (-not (Test-Path -LiteralPath $scriptPath)) {
  throw "publish_android.ps1 is missing"
}
if (-not (Test-Path -LiteralPath $batPath)) {
  throw "publish-android.bat is missing"
}

$script = Get-Content -LiteralPath $scriptPath -Raw
$bat = Get-Content -LiteralPath $batPath -Raw

Assert-Contains -Label "publish_android.ps1" -Text $script -Needle ".wheelmaker"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "build\mobile\android"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "mobile\android"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "WHEELMAKER_WEB_TARGET"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "wheelmakerWebAssetsDir"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "wheelmakerBuildRoot"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "--project-cache-dir"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "gradle-home"
Assert-Contains -Label "publish_android.ps1" -Text $script -Needle "android-release.json"
Assert-NotContains -Label "publish_android.ps1" -Text $script -Needle ".wheelmaker\web\mobile\android"
Assert-NotContains -Label "publish_android.ps1" -Text $script -Needle "mobile\android\app\src\main\assets\wheelmaker-web"

Assert-Contains -Label "publish-android.bat" -Text $bat -Needle "scripts\publish_android.ps1"
Assert-Contains -Label "publish-android.bat" -Text $bat -Needle "powershell"

Write-Host "publish_android.ps1 checks passed"
```

- [ ] **Step 2: Run tests and verify they fail for missing publish script**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/cli-setup.test.js
cd ..
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_publish_android_ps1.ps1
```

Expected:

- `cli-setup.test.js` passes after the test wording update because it only checks existing web workspace constraints.
- `test_publish_android_ps1.ps1` fails with `publish_android.ps1 is missing`.

- [ ] **Step 3: Add defensive ignore rules**

Modify `.gitignore` by replacing the old Android asset ignore line:

```gitignore
app/android/app/src/main/assets/wheelmaker-web/
```

with:

```gitignore
mobile/android/.gradle/
mobile/android/**/build/
mobile/android/local.properties
mobile/android/app/src/main/assets/
```

- [ ] **Step 4: Re-run guardrail tests**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/cli-setup.test.js
cd ..
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_publish_android_ps1.ps1
```

Expected:

- `cli-setup.test.js` passes.
- `test_publish_android_ps1.ps1` still fails with `publish_android.ps1 is missing`; this is the expected red state for the future script task.

- [ ] **Step 5: Commit guardrails**

Run:

```powershell
git add .gitignore app/__tests__/cli-setup.test.js scripts/test_publish_android_ps1.ps1
git commit -m "test: add android publish guardrails"
```

## Task 2: Shared Native Web Source Helpers

**Files:**
- Create: `app/__tests__/web-native-web-source.test.ts`
- Create: `app/web/src/shell/native/webSource.ts`
- Modify: `app/web/src/shell/desktop/webSource.ts`

- [ ] **Step 1: Write failing tests for native Web source inference and bridge selection**

Create `app/__tests__/web-native-web-source.test.ts`:

```ts
import {
  getNativeWebSourceBridge,
  inferNativeRemoteWebCandidate,
  submitNativeRemoteWebCandidate,
} from '../web/src/shell/native/webSource';

describe('native Web source helpers', () => {
  afterEach(() => {
    delete (globalThis as {window?: unknown}).window;
  });

  test('infers remote Web URL from secure Registry address', () => {
    expect(inferNativeRemoteWebCandidate('wss://workspace.example.com/ws')).toEqual({
      source: 'registry',
      registryAddress: 'wss://workspace.example.com/ws',
      remoteWebUrl: 'https://workspace.example.com/',
    });
  });

  test('infers plain remote Web URL from plain Registry address', () => {
    expect(inferNativeRemoteWebCandidate('ws://47.86.63.26:28800/ws')).toEqual({
      source: 'registry',
      registryAddress: 'ws://47.86.63.26:28800/ws',
      remoteWebUrl: 'http://47.86.63.26:28800/',
    });
  });

  test('rejects loopback Registry candidates', () => {
    expect(inferNativeRemoteWebCandidate('ws://127.0.0.1:9630/ws')).toBeNull();
    expect(inferNativeRemoteWebCandidate('ws://localhost:9630/ws')).toBeNull();
    expect(inferNativeRemoteWebCandidate('ws://[::1]:9630/ws')).toBeNull();
  });

  test('prefers Android bridge when present', () => {
    const bridge = {
      enabled: true,
      setRemoteWebCandidate: jest.fn(),
    };
    (globalThis as {window?: unknown}).window = {
      WheelMakerAndroid: bridge,
      WheelMakerDesktop: {
        enabled: true,
        setRemoteWebCandidate: jest.fn(),
      },
    };

    expect(getNativeWebSourceBridge()).toBe(bridge);
  });

  test('falls back to Desktop bridge when Android bridge is absent', () => {
    const bridge = {
      enabled: true,
      setRemoteWebCandidate: jest.fn(),
    };
    (globalThis as {window?: unknown}).window = {
      WheelMakerDesktop: bridge,
    };

    expect(getNativeWebSourceBridge()).toBe(bridge);
  });

  test('submits empty candidate when Registry address is not usable for remote Web', () => {
    const setRemoteWebCandidate = jest.fn();
    (globalThis as {window?: unknown}).window = {
      WheelMakerAndroid: {
        enabled: true,
        setRemoteWebCandidate,
      },
    };

    submitNativeRemoteWebCandidate('ws://127.0.0.1:9630/ws');

    expect(setRemoteWebCandidate).toHaveBeenCalledWith({
      source: 'registry',
      registryAddress: 'ws://127.0.0.1:9630/ws',
      remoteWebUrl: '',
    });
  });
});
```

- [ ] **Step 2: Run test and verify it fails because the module is missing**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-native-web-source.test.ts
```

Expected: FAIL with a module resolution error for `../web/src/shell/native/webSource`.

- [ ] **Step 3: Create shared native Web source helper**

Create `app/web/src/shell/native/webSource.ts`:

```ts
export type NativeWebSourcePreference = 'auto' | 'embedded';

export type NativeWebSourceState = {
  preference: NativeWebSourcePreference;
  actualSource: 'embedded' | 'remote';
  displayTitle: string;
  displaySource: string;
  remoteUrl: string;
  remoteHost: string;
};

export type NativeRemoteWebCandidate = {
  source: 'registry';
  registryAddress: string;
  remoteWebUrl: string;
};

export type NativeWebSourceBridge = {
  enabled?: boolean;
  getWebSourceState?: () => Promise<NativeWebSourceState> | NativeWebSourceState;
  setWebSourcePreference?: (preference: NativeWebSourcePreference) => Promise<NativeWebSourceState> | NativeWebSourceState;
  setRemoteWebCandidate?: (candidate: NativeRemoteWebCandidate) => Promise<NativeWebSourceState> | NativeWebSourceState;
};

type NativeWindow = Window & {
  WheelMakerAndroid?: NativeWebSourceBridge;
  WheelMakerDesktop?: NativeWebSourceBridge;
};

function isLoopbackHost(hostname: string): boolean {
  const value = hostname.toLowerCase();
  return value === 'localhost' || value === '127.0.0.1' || value === '::1' || value === '[::1]';
}

export function inferNativeRemoteWebCandidate(registryAddress: string): NativeRemoteWebCandidate | null {
  let parsed: URL;
  try {
    parsed = new URL(registryAddress.trim());
  } catch {
    return null;
  }
  if (
    parsed.protocol !== 'ws:' &&
    parsed.protocol !== 'wss:' &&
    parsed.protocol !== 'http:' &&
    parsed.protocol !== 'https:'
  ) {
    return null;
  }
  if (!parsed.host || isLoopbackHost(parsed.hostname)) {
    return null;
  }
  const remoteProtocol = parsed.protocol === 'ws:' || parsed.protocol === 'http:' ? 'http:' : 'https:';
  return {
    source: 'registry',
    registryAddress: registryAddress.trim(),
    remoteWebUrl: `${remoteProtocol}//${parsed.host}/`,
  };
}

export function getNativeWebSourceBridge(): NativeWebSourceBridge | null {
  if (typeof window === 'undefined') {
    return null;
  }
  const nativeWindow = window as NativeWindow;
  return nativeWindow.WheelMakerAndroid ?? nativeWindow.WheelMakerDesktop ?? null;
}

export function submitNativeRemoteWebCandidate(registryAddress: string): void {
  const bridge = getNativeWebSourceBridge();
  const submit = bridge?.setRemoteWebCandidate;
  if (!submit) {
    return;
  }
  const candidate = inferNativeRemoteWebCandidate(registryAddress);
  void Promise.resolve(submit(candidate ?? {
    source: 'registry',
    registryAddress: registryAddress.trim(),
    remoteWebUrl: '',
  })).catch(() => undefined);
}
```

- [ ] **Step 4: Delegate Desktop helper to shared native helper**

Replace `app/web/src/shell/desktop/webSource.ts` with:

```ts
import {
  getNativeWebSourceBridge,
  inferNativeRemoteWebCandidate,
  type NativeRemoteWebCandidate,
  type NativeWebSourceState,
} from '../native/webSource';

export type DesktopRemoteWebCandidate = NativeRemoteWebCandidate;
export type DesktopWebSourceState = NativeWebSourceState;

export function inferDesktopRemoteWebCandidate(registryAddress: string): DesktopRemoteWebCandidate | null {
  return inferNativeRemoteWebCandidate(registryAddress);
}

export function readDesktopWebSourceState(): Promise<DesktopWebSourceState | null> {
  const bridge = getNativeWebSourceBridge();
  const read = bridge?.getWebSourceState;
  if (!read) {
    return Promise.resolve(null);
  }
  return Promise.resolve(read()).catch(() => null);
}

export async function setDesktopWebSourcePreference(preference: 'auto' | 'embedded'): Promise<DesktopWebSourceState | null> {
  const bridge = getNativeWebSourceBridge();
  const setPreference = bridge?.setWebSourcePreference;
  if (!setPreference) {
    return null;
  }
  return Promise.resolve(setPreference(preference)).catch(() => null);
}

export function submitDesktopRemoteWebCandidate(registryAddress: string): void {
  const bridge = getNativeWebSourceBridge();
  const submit = bridge?.setRemoteWebCandidate;
  if (!submit) {
    return;
  }
  const candidate = inferNativeRemoteWebCandidate(registryAddress);
  void Promise.resolve(submit(candidate ?? {
    source: 'registry',
    registryAddress: registryAddress.trim(),
    remoteWebUrl: '',
  })).catch(() => undefined);
}
```

- [ ] **Step 5: Run native Web source tests**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-native-web-source.test.ts __tests__/web-desktop-web-source.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit shared native Web source helper**

Run:

```powershell
git add app/__tests__/web-native-web-source.test.ts app/web/src/shell/native/webSource.ts app/web/src/shell/desktop/webSource.ts
git commit -m "refactor: share native web source helpers"
```

## Task 3: Android Stable Origin Registry Default

**Files:**
- Create: `app/__tests__/web-runtime.test.ts`
- Modify: `app/web/src/runtime.ts`

- [ ] **Step 1: Write failing runtime test**

Create `app/__tests__/web-runtime.test.ts`:

```ts
import {getDefaultRegistryAddress, toRegistryWsUrl} from '../web/src/runtime';

describe('runtime Registry address resolution', () => {
  const originalLocation = window.location;

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    });
    delete (window as unknown as {__WHEELMAKER_RUNTIME_CONFIG__?: unknown}).__WHEELMAKER_RUNTIME_CONFIG__;
  });

  function setLocation(input: {hostname: string; host: string; protocol: string}) {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: input,
    });
  }

  test('does not infer Registry address from Android appassets origin', () => {
    setLocation({
      hostname: 'appassets.androidplatform.net',
      host: 'appassets.androidplatform.net',
      protocol: 'https:',
    });
    (window as unknown as {__WHEELMAKER_RUNTIME_CONFIG__?: {defaultRegistryPort?: number}}).__WHEELMAKER_RUNTIME_CONFIG__ = {
      defaultRegistryPort: 9630,
    };

    expect(getDefaultRegistryAddress()).toBe('127.0.0.1:9630');
  });

  test('still infers same-origin WebSocket for normal HTTPS host', () => {
    setLocation({
      hostname: 'workspace.example.com',
      host: 'workspace.example.com:28800',
      protocol: 'https:',
    });

    expect(getDefaultRegistryAddress()).toBe('wss://workspace.example.com:28800/ws');
  });

  test('converts host and port to ws URL', () => {
    expect(toRegistryWsUrl('workspace.example.com:28800')).toBe('ws://workspace.example.com:28800/ws');
  });
});
```

- [ ] **Step 2: Run test and verify appassets case fails**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-runtime.test.ts
```

Expected: FAIL because `getDefaultRegistryAddress()` returns `wss://appassets.androidplatform.net/ws`.

- [ ] **Step 3: Update runtime default handling**

Modify `app/web/src/runtime.ts` by adding:

```ts
function isNativeAppAssetHost(hostname: string): boolean {
  return hostname.toLowerCase() === 'appassets.androidplatform.net';
}
```

Then change `getDefaultRegistryAddress()` so it checks the appassets host before same-origin inference:

```ts
export function getDefaultRegistryAddress(): string {
  const cfg = getConfig();
  if (cfg.defaultRegistryAddress?.trim()) {
    return cfg.defaultRegistryAddress.trim();
  }
  const host = window.location.hostname;
  if (host === '127.0.0.1') {
    return 'ws://127.0.0.1:9630/ws';
  }
  if (isNativeAppAssetHost(host)) {
    return `127.0.0.1:${cfg.defaultRegistryPort ?? 9630}`;
  }
  if (window.location.host) {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}/ws`;
  }
  return `127.0.0.1:${cfg.defaultRegistryPort ?? 9630}`;
}
```

- [ ] **Step 4: Run runtime tests**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/web-runtime.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit runtime stable origin handling**

Run:

```powershell
git add app/__tests__/web-runtime.test.ts app/web/src/runtime.ts
git commit -m "fix: ignore android app origin for registry default"
```

## Task 4: Android Gradle Project Skeleton

**Files:**
- Create: `mobile/android/settings.gradle.kts`
- Create: `mobile/android/build.gradle.kts`
- Create: `mobile/android/app/build.gradle.kts`
- Create: `mobile/android/app/src/main/AndroidManifest.xml`
- Create: `mobile/android/app/src/main/res/values/strings.xml`
- Create: `mobile/android/app/src/main/res/values/styles.xml`
- Create: `mobile/android/app/src/main/java/com/wheelmaker/android/MainActivity.kt`

- [ ] **Step 1: Write failing Gradle project smoke command**

Run:

```powershell
gradle -p mobile/android projects `
  --project-cache-dir "$HOME\.wheelmaker\build\mobile\android\gradle-cache" `
  -g "$HOME\.wheelmaker\build\mobile\android\gradle-home" `
  -PwheelmakerBuildRoot="$HOME\.wheelmaker\build\mobile\android\gradle-build"
```

Expected: FAIL because `mobile/android/` does not exist.

- [ ] **Step 2: Add Android settings**

Create `mobile/android/settings.gradle.kts`:

```kotlin
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "WheelMakerMobile"
include(":app")

val externalBuildRoot = providers.gradleProperty("wheelmakerBuildRoot")
    .orElse(System.getenv("WHEELMAKER_ANDROID_BUILD_ROOT") ?: "")
    .get()

if (externalBuildRoot.isNotBlank()) {
    layout.buildDirectory.set(file(externalBuildRoot).resolve("root"))
    gradle.beforeProject {
        layout.buildDirectory.set(file(externalBuildRoot).resolve(path.removePrefix(":").replace(':', '_').ifBlank { "root" }))
    }
}
```

Create `mobile/android/build.gradle.kts`:

```kotlin
plugins {
    id("com.android.application") version "8.13.2" apply false
    id("org.jetbrains.kotlin.android") version "2.2.21" apply false
}
```

- [ ] **Step 3: Add app module build file**

Create `mobile/android/app/build.gradle.kts`:

```kotlin
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val webAssetsDir = providers.gradleProperty("wheelmakerWebAssetsDir")
    .orElse(System.getenv("WHEELMAKER_ANDROID_WEB_ASSETS") ?: "")
    .get()

android {
    namespace = "com.wheelmaker.android"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.wheelmaker.android"
        minSdk = 23
        targetSdk = 36
        versionCode = 1
        versionName = "0.0.1"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    sourceSets {
        getByName("main") {
            if (webAssetsDir.isNotBlank()) {
                assets.srcDir(webAssetsDir)
            }
        }
    }

    testOptions {
        unitTests.isIncludeAndroidResources = true
    }
}

dependencies {
    implementation("androidx.webkit:webkit:1.15.0")
    testImplementation("junit:junit:4.13.2")
}
```

- [ ] **Step 4: Add manifest and resources**

Create `mobile/android/app/src/main/AndroidManifest.xml`:

```xml
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.RECORD_AUDIO" />
    <uses-permission android:name="android.permission.POST_NOTIFICATIONS" />

    <application
        android:allowBackup="true"
        android:label="@string/app_name"
        android:theme="@style/AppTheme"
        android:usesCleartextTraffic="false">
        <activity
            android:name=".MainActivity"
            android:configChanges="keyboard|keyboardHidden|orientation|screenSize"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
```

Create `mobile/android/app/src/main/res/values/strings.xml`:

```xml
<resources>
    <string name="app_name">WheelMaker</string>
</resources>
```

Create `mobile/android/app/src/main/res/values/styles.xml`:

```xml
<resources>
    <style name="AppTheme" parent="android:style/Theme.Material.NoActionBar">
        <item name="android:windowLightStatusBar">false</item>
        <item name="android:windowLightNavigationBar">false</item>
        <item name="android:windowActionModeOverlay">true</item>
        <item name="android:fontFamily">sans</item>
        <item name="android:colorAccent">#007acc</item>
    </style>
</resources>
```

- [ ] **Step 5: Add minimal Activity**

Create `mobile/android/app/src/main/java/com/wheelmaker/android/MainActivity.kt`:

```kotlin
package com.wheelmaker.android

import android.app.Activity
import android.os.Bundle
import android.webkit.WebView

class MainActivity : Activity() {
    private lateinit var webView: WebView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webView = WebView(this)
        webView.settings.javaScriptEnabled = true
        webView.settings.domStorageEnabled = true
        setContentView(webView)
        webView.loadUrl("https://appassets.androidplatform.net/")
    }
}
```

- [ ] **Step 6: Run Gradle project smoke command**

Run:

```powershell
gradle -p mobile/android projects `
  --project-cache-dir "$HOME\.wheelmaker\build\mobile\android\gradle-cache" `
  -g "$HOME\.wheelmaker\build\mobile\android\gradle-home" `
  -PwheelmakerBuildRoot="$HOME\.wheelmaker\build\mobile\android\gradle-build"
```

Expected: PASS and lists root project `WheelMakerMobile` with project `:app`.

- [ ] **Step 7: Confirm no repo build output was created**

Run:

```powershell
Test-Path mobile/android/.gradle
Test-Path mobile/android/build
Test-Path mobile/android/app/build
Test-Path mobile/android/local.properties
```

Expected: all four commands print `False`.

- [ ] **Step 8: Commit Android skeleton**

Run:

```powershell
git add mobile/android
git commit -m "feat: add android project skeleton"
```

## Task 5: Android Web Source Runtime And Stable-Origin Request Handler

**Files:**
- Create: `mobile/android/app/src/main/java/com/wheelmaker/android/WebSourceModels.kt`
- Create: `mobile/android/app/src/main/java/com/wheelmaker/android/WebSourceRuntime.kt`
- Create: `mobile/android/app/src/main/java/com/wheelmaker/android/StableOriginWebViewClient.kt`
- Create: `mobile/android/app/src/test/java/com/wheelmaker/android/WebSourceRuntimeTest.kt`
- Create: `mobile/android/app/src/test/java/com/wheelmaker/android/StableOriginPathTest.kt`
- Modify: `mobile/android/app/src/main/java/com/wheelmaker/android/MainActivity.kt`

- [ ] **Step 1: Write failing Kotlin tests**

Create `mobile/android/app/src/test/java/com/wheelmaker/android/WebSourceRuntimeTest.kt`:

```kotlin
package com.wheelmaker.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class WebSourceRuntimeTest {
    @Test
    fun rejectsLoopbackRemoteCandidates() {
        val candidate = RemoteWebCandidate(
            source = "registry",
            registryAddress = "ws://127.0.0.1:9630/ws",
            remoteWebUrl = "http://127.0.0.1:9630/"
        )

        assertFalse(normalizeRemoteWebCandidate(candidate).accepted)
    }

    @Test
    fun acceptsRemoteCandidateWhenRegistryAndRemoteHostsMatch() {
        val candidate = RemoteWebCandidate(
            source = "registry",
            registryAddress = "wss://workspace.example.com/ws",
            remoteWebUrl = "https://workspace.example.com/"
        )

        val normalized = normalizeRemoteWebCandidate(candidate)

        assertTrue(normalized.accepted)
        assertEquals("https://workspace.example.com/", normalized.remoteWebUrl)
        assertEquals("wss://workspace.example.com", normalized.registryOrigin)
    }

    @Test
    fun stateFallsBackToEmbeddedWhenRemoteUrlIsEmpty() {
        val runtime = WebSourceRuntime(InMemoryWebSourceStore(WebSourceConfig()))

        val state = runtime.state()

        assertEquals("auto", state.preference)
        assertEquals("embedded", state.actualSource)
        assertEquals("", state.remoteUrl)
    }
}
```

Create `mobile/android/app/src/test/java/com/wheelmaker/android/StableOriginPathTest.kt`:

```kotlin
package com.wheelmaker.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class StableOriginPathTest {
    @Test
    fun mapsRootToIndexHtml() {
        assertEquals("index.html", assetNameForStablePath("/"))
    }

    @Test
    fun stripsLeadingSlashForAssets() {
        assertEquals("bundle.abc.js", assetNameForStablePath("/bundle.abc.js"))
    }

    @Test
    fun treatsWorkspaceRoutesAsIndexFallbackCandidates() {
        assertTrue(isWorkspaceRoute("settings/update"))
    }
}
```

- [ ] **Step 2: Run tests and verify they fail because classes are missing**

Run:

```powershell
gradle -p mobile/android :app:testDebugUnitTest `
  --project-cache-dir "$HOME\.wheelmaker\build\mobile\android\gradle-cache" `
  -g "$HOME\.wheelmaker\build\mobile\android\gradle-home" `
  -PwheelmakerBuildRoot="$HOME\.wheelmaker\build\mobile\android\gradle-build"
```

Expected: FAIL with unresolved references such as `RemoteWebCandidate`.

- [ ] **Step 3: Add Web source models**

Create `mobile/android/app/src/main/java/com/wheelmaker/android/WebSourceModels.kt`:

```kotlin
package com.wheelmaker.android

data class WebSourceConfig(
    val webSourcePreference: String = WEB_SOURCE_AUTO,
    val remoteWebUrl: String = "",
    val remoteWebRegistryOrigin: String = ""
)

data class WebSourceState(
    val preference: String,
    val actualSource: String,
    val displayTitle: String,
    val displaySource: String,
    val remoteUrl: String,
    val remoteHost: String
)

data class RemoteWebCandidate(
    val source: String,
    val registryAddress: String,
    val remoteWebUrl: String
)

data class NormalizedRemoteCandidate(
    val accepted: Boolean,
    val remoteWebUrl: String = "",
    val registryOrigin: String = ""
)

const val WEB_SOURCE_AUTO = "auto"
const val WEB_SOURCE_EMBEDDED = "embedded"
const val WEB_ACTUAL_EMBEDDED = "embedded"
const val WEB_ACTUAL_REMOTE = "remote"
const val ANDROID_APP_ORIGIN = "https://appassets.androidplatform.net/"
```

- [ ] **Step 4: Add Web source runtime**

Create `mobile/android/app/src/main/java/com/wheelmaker/android/WebSourceRuntime.kt`:

```kotlin
package com.wheelmaker.android

import android.content.Context
import android.net.Uri
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import java.util.Locale

interface WebSourceStore {
    fun load(): WebSourceConfig
    fun save(config: WebSourceConfig)
}

class SharedPreferencesWebSourceStore(context: Context) : WebSourceStore {
    private val prefs = context.getSharedPreferences("wheelmaker_web_source", Context.MODE_PRIVATE)

    override fun load(): WebSourceConfig {
        return WebSourceConfig(
            webSourcePreference = prefs.getString("webSourcePreference", WEB_SOURCE_AUTO) ?: WEB_SOURCE_AUTO,
            remoteWebUrl = prefs.getString("remoteWebUrl", "") ?: "",
            remoteWebRegistryOrigin = prefs.getString("remoteWebRegistryOrigin", "") ?: ""
        ).sanitize()
    }

    override fun save(config: WebSourceConfig) {
        val clean = config.sanitize()
        prefs.edit()
            .putString("webSourcePreference", clean.webSourcePreference)
            .putString("remoteWebUrl", clean.remoteWebUrl)
            .putString("remoteWebRegistryOrigin", clean.remoteWebRegistryOrigin)
            .apply()
    }
}

class InMemoryWebSourceStore(private var config: WebSourceConfig) : WebSourceStore {
    override fun load(): WebSourceConfig = config
    override fun save(config: WebSourceConfig) {
        this.config = config
    }
}

class WebSourceRuntime(private val store: WebSourceStore) {
    private var config: WebSourceConfig = store.load().sanitize()
    private var actualSource: String = actualSourceForConfig(config)

    @Synchronized
    fun state(): WebSourceState = stateForConfig(config, actualSource)

    @Synchronized
    fun setPreference(preference: String): WebSourceState {
        val cleanPreference = if (preference == WEB_SOURCE_EMBEDDED) WEB_SOURCE_EMBEDDED else WEB_SOURCE_AUTO
        config = config.copy(webSourcePreference = cleanPreference).sanitize()
        actualSource = actualSourceForConfig(config)
        store.save(config)
        return state()
    }

    @Synchronized
    fun setRemoteCandidate(candidate: RemoteWebCandidate): WebSourceState {
        val normalized = normalizeRemoteWebCandidate(candidate)
        config = if (normalized.accepted) {
            config.copy(
                remoteWebUrl = normalized.remoteWebUrl,
                remoteWebRegistryOrigin = normalized.registryOrigin
            )
        } else {
            config.copy(remoteWebUrl = "", remoteWebRegistryOrigin = "")
        }.sanitize()
        actualSource = actualSourceForConfig(config)
        store.save(config)
        return state()
    }

    @Synchronized
    fun refreshActualSource(): WebSourceState {
        actualSource = if (config.webSourcePreference == WEB_SOURCE_AUTO && config.remoteWebUrl.isNotBlank() && probeRemote(config.remoteWebUrl)) {
            WEB_ACTUAL_REMOTE
        } else {
            WEB_ACTUAL_EMBEDDED
        }
        return state()
    }

    @Synchronized
    fun remoteBaseForRequest(): String {
        return if (actualSource == WEB_ACTUAL_REMOTE && config.webSourcePreference == WEB_SOURCE_AUTO) config.remoteWebUrl else ""
    }

    private fun probeRemote(remoteWebUrl: String): Boolean {
        return try {
            val connection = URL(remoteWebUrl).openConnection() as HttpURLConnection
            connection.connectTimeout = 3000
            connection.readTimeout = 3000
            connection.requestMethod = "GET"
            connection.instanceFollowRedirects = true
            connection.responseCode in 200..299
        } catch (_: Exception) {
            false
        }
    }
}

fun WebSourceConfig.sanitize(): WebSourceConfig {
    val cleanPreference = if (webSourcePreference == WEB_SOURCE_EMBEDDED) WEB_SOURCE_EMBEDDED else WEB_SOURCE_AUTO
    val cleanRemote = normalizeRemoteWebUrl(remoteWebUrl)
    return copy(
        webSourcePreference = cleanPreference,
        remoteWebUrl = cleanRemote,
        remoteWebRegistryOrigin = if (cleanRemote.isBlank()) "" else remoteWebRegistryOrigin
    )
}

fun actualSourceForConfig(config: WebSourceConfig): String {
    return if (config.webSourcePreference == WEB_SOURCE_AUTO && config.remoteWebUrl.isNotBlank()) WEB_ACTUAL_REMOTE else WEB_ACTUAL_EMBEDDED
}

fun stateForConfig(config: WebSourceConfig, actualSource: String): WebSourceState {
    val remoteHost = runCatching { Uri.parse(config.remoteWebUrl).host ?: "" }.getOrDefault("")
    val cleanActual = if (actualSource == WEB_ACTUAL_REMOTE && config.remoteWebUrl.isNotBlank()) WEB_ACTUAL_REMOTE else WEB_ACTUAL_EMBEDDED
    val displaySource = if (cleanActual == WEB_ACTUAL_REMOTE && remoteHost.isNotBlank()) remoteHost else "Embedded"
    return WebSourceState(
        preference = config.webSourcePreference,
        actualSource = cleanActual,
        displayTitle = "WheelMaker - $displaySource",
        displaySource = displaySource,
        remoteUrl = config.remoteWebUrl,
        remoteHost = remoteHost
    )
}

fun normalizeRemoteWebCandidate(candidate: RemoteWebCandidate): NormalizedRemoteCandidate {
    val registryUri = Uri.parse(candidate.registryAddress.trim())
    val remoteUri = Uri.parse(candidate.remoteWebUrl.trim())
    if (!isAllowedRegistryScheme(registryUri.scheme) || !isAllowedRemoteScheme(remoteUri.scheme)) return NormalizedRemoteCandidate(false)
    if (registryUri.host.isNullOrBlank() || remoteUri.host.isNullOrBlank()) return NormalizedRemoteCandidate(false)
    if (isLoopbackHost(registryUri.host ?: "") || isLoopbackHost(remoteUri.host ?: "")) return NormalizedRemoteCandidate(false)
    if (!registryUri.encodedAuthority.equals(remoteUri.encodedAuthority, ignoreCase = true)) return NormalizedRemoteCandidate(false)
    if (!remoteUri.query.isNullOrBlank() || !remoteUri.fragment.isNullOrBlank()) return NormalizedRemoteCandidate(false)
    val path = remoteUri.path ?: ""
    if (path.isNotBlank() && path != "/") return NormalizedRemoteCandidate(false)
    return NormalizedRemoteCandidate(true, "${remoteUri.scheme}://${remoteUri.encodedAuthority}/", registryOriginForUri(registryUri))
}

fun normalizeRemoteWebUrl(raw: String): String {
    if (raw.isBlank()) return ""
    val uri = Uri.parse(raw.trim())
    if (!isAllowedRemoteScheme(uri.scheme) || uri.host.isNullOrBlank() || isLoopbackHost(uri.host ?: "")) return ""
    if (!uri.query.isNullOrBlank() || !uri.fragment.isNullOrBlank()) return ""
    val path = uri.path ?: ""
    if (path.isNotBlank() && path != "/") return ""
    return "${uri.scheme}://${uri.encodedAuthority}/"
}

private fun registryOriginForUri(uri: Uri): String {
    val scheme = when (uri.scheme) {
        "http" -> "ws"
        "https" -> "wss"
        else -> uri.scheme ?: "ws"
    }
    return "$scheme://${uri.encodedAuthority}"
}

private fun isAllowedRegistryScheme(scheme: String?): Boolean = scheme == "ws" || scheme == "wss" || scheme == "http" || scheme == "https"
private fun isAllowedRemoteScheme(scheme: String?): Boolean = scheme == "http" || scheme == "https"

private fun isLoopbackHost(host: String): Boolean {
    val value = host.lowercase(Locale.US).trim('[', ']')
    return value == "localhost" || value == "127.0.0.1" || value == "::1"
}

fun webSourceStateToJson(state: WebSourceState): String {
    return JSONObject()
        .put("preference", state.preference)
        .put("actualSource", state.actualSource)
        .put("displayTitle", state.displayTitle)
        .put("displaySource", state.displaySource)
        .put("remoteUrl", state.remoteUrl)
        .put("remoteHost", state.remoteHost)
        .toString()
}
```

- [ ] **Step 5: Add stable-origin request handler**

Create `mobile/android/app/src/main/java/com/wheelmaker/android/StableOriginWebViewClient.kt`:

```kotlin
package com.wheelmaker.android

import android.content.Context
import android.net.Uri
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import java.io.ByteArrayInputStream
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.URL

class StableOriginWebViewClient(
    private val context: Context,
    private val webSourceRuntime: WebSourceRuntime
) : WebViewClient() {
    override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
        val uri = request.url ?: return null
        if (uri.scheme != "https" || uri.host != "appassets.androidplatform.net") {
            return null
        }
        val assetName = assetNameForStablePath(uri.path ?: "/")
        if (assetName == "ws") {
            return notFoundResponse()
        }
        return remoteResponse(assetName) ?: embeddedResponse(assetName) ?: embeddedResponse("index.html") ?: notFoundResponse()
    }

    private fun remoteResponse(assetName: String): WebResourceResponse? {
        val remoteBase = webSourceRuntime.remoteBaseForRequest()
        if (remoteBase.isBlank()) return null
        return try {
            val url = URL(remoteBase + if (assetName == "index.html") "" else assetName)
            val connection = url.openConnection() as HttpURLConnection
            connection.connectTimeout = 5000
            connection.readTimeout = 15000
            connection.instanceFollowRedirects = true
            val status = connection.responseCode
            if (status !in 200..299) return null
            val contentType = connection.contentType ?: contentTypeForAsset(assetName)
            val headers = responseHeadersForAsset(assetName).toMutableMap()
            connection.getHeaderField("Cache-Control")?.let { headers["Cache-Control"] = it }
            WebResourceResponse(contentType, null, status, "OK", headers, connection.inputStream)
        } catch (_: Exception) {
            null
        }
    }

    private fun embeddedResponse(assetName: String): WebResourceResponse? {
        val stream = openEmbeddedAsset(assetName) ?: return null
        return WebResourceResponse(
            contentTypeForAsset(assetName),
            null,
            200,
            "OK",
            responseHeadersForAsset(assetName),
            stream
        )
    }

    private fun openEmbeddedAsset(assetName: String): InputStream? {
        return try {
            context.assets.open(assetName)
        } catch (_: Exception) {
            null
        }
    }

    private fun notFoundResponse(): WebResourceResponse {
        return WebResourceResponse(
            "text/plain",
            "utf-8",
            404,
            "Not Found",
            mapOf("Cache-Control" to "no-store"),
            ByteArrayInputStream("not found".toByteArray())
        )
    }
}

fun assetNameForStablePath(path: String): String {
    val clean = path.trimStart('/').ifBlank { "index.html" }
    return clean.replace("../", "").replace("..\\", "")
}

fun isWorkspaceRoute(assetName: String): Boolean = !assetName.substringAfterLast('/').contains('.')

fun contentTypeForAsset(assetName: String): String {
    return when {
        assetName.endsWith(".html") -> "text/html"
        assetName.endsWith(".js") -> "application/javascript"
        assetName.endsWith(".css") -> "text/css"
        assetName.endsWith(".json") -> "application/json"
        assetName.endsWith(".webmanifest") -> "application/manifest+json"
        assetName.endsWith(".svg") -> "image/svg+xml"
        assetName.endsWith(".png") -> "image/png"
        assetName.endsWith(".woff2") -> "font/woff2"
        assetName.endsWith(".woff") -> "font/woff"
        else -> "application/octet-stream"
    }
}

fun responseHeadersForAsset(assetName: String): Map<String, String> {
    val baseName = assetName.substringAfterLast('/')
    val cacheControl = when {
        baseName == "index.html" || baseName == "service-worker.js" -> "no-cache, must-revalidate"
        baseName == "runtime-config.js" || baseName == "web-build.json" -> "no-store"
        baseName.startsWith("bundle.") && (baseName.endsWith(".js") || baseName.endsWith(".css")) -> "public, max-age=31536000, immutable"
        baseName.endsWith(".svg") || baseName.endsWith(".woff") || baseName.endsWith(".woff2") -> "public, max-age=31536000, immutable"
        else -> "no-cache"
    }
    return mapOf("Cache-Control" to cacheControl)
}
```

- [ ] **Step 6: Wire runtime and client into MainActivity**

Replace `mobile/android/app/src/main/java/com/wheelmaker/android/MainActivity.kt` with:

```kotlin
package com.wheelmaker.android

import android.app.Activity
import android.os.Bundle
import android.webkit.WebView

class MainActivity : Activity() {
    private lateinit var webView: WebView
    private lateinit var webSourceRuntime: WebSourceRuntime

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webSourceRuntime = WebSourceRuntime(SharedPreferencesWebSourceStore(this))
        webSourceRuntime.refreshActualSource()
        webView = WebView(this)
        webView.settings.javaScriptEnabled = true
        webView.settings.domStorageEnabled = true
        webView.webViewClient = StableOriginWebViewClient(this, webSourceRuntime)
        setContentView(webView)
        webView.loadUrl(ANDROID_APP_ORIGIN)
    }
}
```

- [ ] **Step 7: Run Android unit tests**

Run:

```powershell
gradle -p mobile/android :app:testDebugUnitTest `
  --project-cache-dir "$HOME\.wheelmaker\build\mobile\android\gradle-cache" `
  -g "$HOME\.wheelmaker\build\mobile\android\gradle-home" `
  -PwheelmakerBuildRoot="$HOME\.wheelmaker\build\mobile\android\gradle-build"
```

Expected: PASS.

- [ ] **Step 8: Confirm no repo build output was created**

Run:

```powershell
Test-Path mobile/android/.gradle
Test-Path mobile/android/build
Test-Path mobile/android/app/build
```

Expected: all three commands print `False`.

- [ ] **Step 9: Commit Web source runtime**

Run:

```powershell
git add mobile/android/app/src/main/java/com/wheelmaker/android mobile/android/app/src/test/java/com/wheelmaker/android
git commit -m "feat: add android stable web source runtime"
```

## Task 6: Android JavaScript Bridge And WebView Platform Wiring

**Files:**
- Create: `mobile/android/app/src/main/java/com/wheelmaker/android/WheelMakerBridge.kt`
- Modify: `mobile/android/app/src/main/java/com/wheelmaker/android/MainActivity.kt`

- [ ] **Step 1: Add bridge smoke test by extending MainActivity expectations**

Run:

```powershell
Select-String -Path mobile/android/app/src/main/java/com/wheelmaker/android/MainActivity.kt -Pattern "addJavascriptInterface"
```

Expected: no output, proving bridge wiring is missing before implementation.

- [ ] **Step 2: Add JavaScript bridge**

Create `mobile/android/app/src/main/java/com/wheelmaker/android/WheelMakerBridge.kt`:

```kotlin
package com.wheelmaker.android

import android.webkit.JavascriptInterface
import org.json.JSONObject

class WheelMakerBridge(private val webSourceRuntime: WebSourceRuntime) {
    @JavascriptInterface
    fun getWebSourceState(): String = webSourceStateToJson(webSourceRuntime.state())

    @JavascriptInterface
    fun setWebSourcePreference(preference: String): String {
        return webSourceStateToJson(webSourceRuntime.setPreference(preference))
    }

    @JavascriptInterface
    fun setRemoteWebCandidate(rawJson: String): String {
        val input = JSONObject(rawJson)
        val candidate = RemoteWebCandidate(
            source = input.optString("source"),
            registryAddress = input.optString("registryAddress"),
            remoteWebUrl = input.optString("remoteWebUrl")
        )
        return webSourceStateToJson(webSourceRuntime.setRemoteCandidate(candidate))
    }
}
```

- [ ] **Step 3: Wire bridge and WebView platform settings**

Replace `mobile/android/app/src/main/java/com/wheelmaker/android/MainActivity.kt` with:

```kotlin
package com.wheelmaker.android

import android.Manifest
import android.app.Activity
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Bundle
import android.webkit.PermissionRequest
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebSettings
import android.webkit.WebView
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat

class MainActivity : Activity() {
    private lateinit var webView: WebView
    private lateinit var webSourceRuntime: WebSourceRuntime
    private var fileChooserCallback: ValueCallback<Array<Uri>>? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webSourceRuntime = WebSourceRuntime(SharedPreferencesWebSourceStore(this))
        webSourceRuntime.refreshActualSource()

        webView = WebView(this)
        configureWebView(webView)
        setContentView(webView)
        webView.loadUrl(ANDROID_APP_ORIGIN)
    }

    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
            return
        }
        super.onBackPressed()
    }

    private fun configureWebView(target: WebView) {
        target.settings.javaScriptEnabled = true
        target.settings.domStorageEnabled = true
        target.settings.databaseEnabled = true
        target.settings.cacheMode = WebSettings.LOAD_DEFAULT
        target.settings.mediaPlaybackRequiresUserGesture = false
        target.webViewClient = StableOriginWebViewClient(this, webSourceRuntime)
        target.webChromeClient = object : WebChromeClient() {
            override fun onPermissionRequest(request: PermissionRequest) {
                if (request.resources.contains(PermissionRequest.RESOURCE_AUDIO_CAPTURE)) {
                    if (ContextCompat.checkSelfPermission(this@MainActivity, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED) {
                        request.grant(arrayOf(PermissionRequest.RESOURCE_AUDIO_CAPTURE))
                    } else {
                        ActivityCompat.requestPermissions(this@MainActivity, arrayOf(Manifest.permission.RECORD_AUDIO), 1001)
                        request.deny()
                    }
                    return
                }
                request.deny()
            }

            override fun onShowFileChooser(
                webView: WebView,
                filePathCallback: ValueCallback<Array<Uri>>,
                fileChooserParams: FileChooserParams
            ): Boolean {
                fileChooserCallback?.onReceiveValue(null)
                fileChooserCallback = filePathCallback
                startActivityForResult(fileChooserParams.createIntent(), 1002)
                return true
            }
        }
        target.addJavascriptInterface(WheelMakerBridge(webSourceRuntime), "WheelMakerAndroidNative")
        target.evaluateJavascript(androidBridgeBootstrapScript(), null)
    }

    private fun androidBridgeBootstrapScript(): String {
        return """
            (() => {
              if (window.WheelMakerAndroid) return;
              const native = window.WheelMakerAndroidNative;
              window.WheelMakerAndroid = Object.freeze({
                enabled: true,
                getWebSourceState: () => Promise.resolve(JSON.parse(native.getWebSourceState())),
                setWebSourcePreference: preference => Promise.resolve(JSON.parse(native.setWebSourcePreference(preference))),
                setRemoteWebCandidate: candidate => Promise.resolve(JSON.parse(native.setRemoteWebCandidate(JSON.stringify(candidate)))),
              });
            })();
        """.trimIndent()
    }
}
```

- [ ] **Step 4: Add AndroidX core dependency for permission helpers**

Modify `mobile/android/app/build.gradle.kts` dependencies block to include:

```kotlin
implementation("androidx.core:core-ktx:1.17.0")
```

- [ ] **Step 5: Run bridge smoke check and Gradle tests**

Run:

```powershell
Select-String -Path mobile/android/app/src/main/java/com/wheelmaker/android/MainActivity.kt -Pattern "addJavascriptInterface"
gradle -p mobile/android :app:testDebugUnitTest `
  --project-cache-dir "$HOME\.wheelmaker\build\mobile\android\gradle-cache" `
  -g "$HOME\.wheelmaker\build\mobile\android\gradle-home" `
  -PwheelmakerBuildRoot="$HOME\.wheelmaker\build\mobile\android\gradle-build"
```

Expected:

- `Select-String` prints the `addJavascriptInterface` line.
- Gradle unit tests pass.

- [ ] **Step 6: Commit bridge wiring**

Run:

```powershell
git add mobile/android/app
git commit -m "feat: add android webview bridge"
```

## Task 7: Android Publish Script

**Files:**
- Create: `publish-android.bat`
- Create: `scripts/publish_android.ps1`
- Modify: `scripts/test_publish_android_ps1.ps1`

- [ ] **Step 1: Re-run publish script test and verify it fails**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_publish_android_ps1.ps1
```

Expected: FAIL with `publish_android.ps1 is missing`.

- [ ] **Step 2: Add root batch entrypoint**

Create `publish-android.bat`:

```bat
@echo off
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\publish_android.ps1" %*
exit /b %ERRORLEVEL%
```

- [ ] **Step 3: Add PowerShell publish script**

Create `scripts/publish_android.ps1`:

```powershell
param(
  [string]$RepoRoot = "",
  [string]$OutputDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\mobile\android"),
  [switch]$WhatIf
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Step {
  param([string]$Text)
  Write-Host ("==> {0}" -f $Text)
}

function Assert-Command {
  param([Parameter(Mandatory = $true)][string]$Name, [string]$Hint = "")
  if (Get-Command $Name -ErrorAction SilentlyContinue) { return }
  if ([string]::IsNullOrWhiteSpace($Hint)) { throw ("required command not found in PATH: {0}" -f $Name) }
  throw ("required command not found in PATH: {0}. {1}" -f $Name, $Hint)
}

function Invoke-Checked {
  param(
    [Parameter(Mandatory = $true)][string]$FilePath,
    [string[]]$Arguments = @(),
    [string]$FailureMessage = ""
  )
  & $FilePath @Arguments
  if ($LASTEXITCODE -eq 0) { return }
  if ([string]::IsNullOrWhiteSpace($FailureMessage)) {
    throw ("command failed: {0} {1} (exit={2})" -f $FilePath, ($Arguments -join " "), $LASTEXITCODE)
  }
  throw ("{0} (exit={1})" -f $FailureMessage, $LASTEXITCODE)
}

function Get-GitValue {
  param([Parameter(Mandatory = $true)][string[]]$Arguments)
  Push-Location $script:RepoRoot
  try {
    $value = ((& git @Arguments) | Select-Object -First 1)
    if ($LASTEXITCODE -ne 0) { throw ("git {0} failed (exit={1})" -f ($Arguments -join " "), $LASTEXITCODE) }
    return ([string]$value).Trim()
  } finally {
    Pop-Location
  }
}

function New-CleanDirectory {
  param([Parameter(Mandatory = $true)][string]$Path)
  $resolved = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Path)
  $root = [System.IO.Path]::GetFullPath($script:BuildRoot)
  if (-not $root.EndsWith([System.IO.Path]::DirectorySeparatorChar)) {
    $root = $root + [System.IO.Path]::DirectorySeparatorChar
  }
  $target = [System.IO.Path]::GetFullPath($resolved)
  if (-not $target.StartsWith($root, [StringComparison]::OrdinalIgnoreCase)) {
    throw ("refusing to clean path outside Android build root: {0}" -f $target)
  }
  if ($WhatIf) {
    Write-Host ("[whatif] clean directory {0}" -f $target)
    return
  }
  if (Test-Path -LiteralPath $target) {
    Remove-Item -LiteralPath $target -Recurse -Force
  }
  New-Item -ItemType Directory -Path $target -Force | Out-Null
}

function Build-AndroidWeb {
  Assert-Command -Name "npm" -Hint "Install Node.js 22+."
  Assert-Command -Name "node" -Hint "Install Node.js 22+."
  New-CleanDirectory -Path $script:WebRoot
  $previousTarget = $env:WHEELMAKER_WEB_TARGET
  $env:WHEELMAKER_WEB_TARGET = $script:WebRoot
  Push-Location $script:AppRoot
  try {
    Write-Step "build embedded Android Workspace Web UI"
    if ($WhatIf) {
      Write-Host ("[whatif] WHEELMAKER_WEB_TARGET={0} npm run build:web" -f $script:WebRoot)
      Write-Host "[whatif] node scripts/export_web_release.js"
      return
    }
    Invoke-Checked -FilePath "npm" -Arguments @("run", "build:web") -FailureMessage "Android Web build failed"
    Invoke-Checked -FilePath "node" -Arguments @("scripts/export_web_release.js") -FailureMessage "Android Web public asset export failed"
    if (-not (Test-Path -LiteralPath (Join-Path $script:WebRoot "index.html"))) {
      throw ("Android Web build missing index.html: {0}" -f $script:WebRoot)
    }
  } finally {
    if ($null -ne $previousTarget) {
      $env:WHEELMAKER_WEB_TARGET = $previousTarget
    } else {
      Remove-Item Env:WHEELMAKER_WEB_TARGET -ErrorAction SilentlyContinue
    }
    Pop-Location
  }
}

function Build-AndroidApk {
  Assert-Command -Name "gradle" -Hint "Install Gradle or use Android Studio's Gradle command in PATH."
  New-CleanDirectory -Path $script:GradleBuildRoot
  New-CleanDirectory -Path $script:GradleCacheDir
  New-CleanDirectory -Path $script:GradleHomeDir
  New-Item -ItemType Directory -Path $script:OutputDir -Force | Out-Null

  Push-Location $script:AndroidRoot
  try {
    Write-Step "build WheelMakerAndroid APK"
    $args = @(
      "assembleRelease",
      "--project-cache-dir", $script:GradleCacheDir,
      "-g", $script:GradleHomeDir,
      "-PwheelmakerBuildRoot=$script:GradleBuildRoot",
      "-PwheelmakerWebAssetsDir=$script:WebRoot"
    )
    if ($WhatIf) {
      Write-Host ("[whatif] gradle {0}" -f ($args -join " "))
      return
    }
    Invoke-Checked -FilePath "gradle" -Arguments $args -FailureMessage "Android APK build failed"
  } finally {
    Pop-Location
  }
}

function Copy-AndroidOutputs {
  $apk = Get-ChildItem -LiteralPath $script:GradleBuildRoot -Recurse -Filter "*.apk" |
    Where-Object { $_.FullName -match "\\outputs\\apk\\release\\" } |
    Select-Object -First 1
  if ($null -eq $apk) {
    throw ("release APK was not produced under {0}" -f $script:GradleBuildRoot)
  }
  $target = Join-Path $script:OutputDir "WheelMakerAndroid.apk"
  Write-Step ("copy APK: {0}" -f $target)
  if ($WhatIf) {
    Write-Host ("[whatif] copy {0} -> {1}" -f $apk.FullName, $target)
    return
  }
  Copy-Item -LiteralPath $apk.FullName -Destination $target -Force
}

function Write-AndroidReleaseManifest {
  Assert-Command -Name "git" -Hint "Install Git and ensure git.exe is available."
  $apkPath = Join-Path $script:OutputDir "WheelMakerAndroid.apk"
  $apkHash = ""
  $apkSize = 0
  if (-not $WhatIf -and (Test-Path -LiteralPath $apkPath)) {
    $apkHash = (Get-FileHash -LiteralPath $apkPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $apkSize = (Get-Item -LiteralPath $apkPath).Length
  }
  $manifest = [ordered]@{
    "schemaVersion" = 1
    "platform" = "android"
    "repo" = $script:RepoRoot
    "branch" = Get-GitValue -Arguments @("branch", "--show-current")
    "sha" = Get-GitValue -Arguments @("rev-parse", "HEAD")
    "builtAt" = (Get-Date).ToUniversalTime().ToString("o")
    "apk" = [ordered]@{
      "fileName" = "WheelMakerAndroid.apk"
      "path" = $apkPath
      "sha256" = $apkHash
      "size" = $apkSize
    }
    "webRoot" = $script:WebRoot
    "gradleBuildRoot" = $script:GradleBuildRoot
  }
  if ($WhatIf) {
    Write-Host ("[whatif] write {0}" -f $script:ManifestPath)
    return
  }
  New-Item -ItemType Directory -Path $script:OutputDir -Force | Out-Null
  $json = $manifest | ConvertTo-Json -Depth 8
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($script:ManifestPath, $json, $utf8NoBom)
}

$script:RepoRoot = if ([string]::IsNullOrWhiteSpace($RepoRoot)) { (Resolve-Path (Join-Path $PSScriptRoot "..")).Path } else { (Resolve-Path $RepoRoot).Path }
$script:AppRoot = Join-Path $script:RepoRoot "app"
$script:AndroidRoot = Join-Path $script:RepoRoot "mobile\android"
$script:WheelMakerHome = Join-Path $HOME ".wheelmaker"
$script:BuildRoot = Join-Path $script:WheelMakerHome "build\mobile\android"
$script:WebRoot = Join-Path $script:BuildRoot "webroot"
$script:GradleBuildRoot = Join-Path $script:BuildRoot "gradle-build"
$script:GradleCacheDir = Join-Path $script:BuildRoot "gradle-cache"
$script:GradleHomeDir = Join-Path $script:BuildRoot "gradle-home"
$script:OutputDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputDir)
$script:ManifestPath = Join-Path $script:OutputDir "android-release.json"

Build-AndroidWeb
Build-AndroidApk
Copy-AndroidOutputs
Write-AndroidReleaseManifest
Write-Step "Android APK publish complete"
```

- [ ] **Step 4: Run script structure test**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_publish_android_ps1.ps1
```

Expected: PASS.

- [ ] **Step 5: Run WhatIf publish**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/publish_android.ps1 -WhatIf
```

Expected: prints Web build, Gradle build, copy, and manifest WhatIf steps without creating repo-local generated files.

- [ ] **Step 6: Confirm no repo build output was created by WhatIf**

Run:

```powershell
Test-Path mobile/android/.gradle
Test-Path mobile/android/build
Test-Path mobile/android/app/build
Test-Path mobile/android/local.properties
Test-Path mobile/android/app/src/main/assets
```

Expected: all five commands print `False`.

- [ ] **Step 7: Commit publish script**

Run:

```powershell
git add publish-android.bat scripts/publish_android.ps1 scripts/test_publish_android_ps1.ps1
git commit -m "feat: add android publish script"
```

## Task 8: Documentation And Verification

**Files:**
- Modify: `README.md`
- Modify: `app/README.md`

- [ ] **Step 1: Add README Android build instructions**

In `README.md`, add a subsection after the Desktop/PWA install area:

```markdown
### Build Android APK

WheelMaker Android is a native Kotlin WebView shell under `mobile/android/`.
It packages an embedded Workspace Web snapshot and uses a stable Android app origin so Web IndexedDB data survives Remote Web and Embedded Web fallback.

Requirements:

- Android SDK with `ANDROID_HOME` or `ANDROID_SDK_ROOT`
- Gradle in `PATH`
- Node.js 22+

Build from the repository root:

```bat
publish-android.bat
```

Outputs:

```text
~/.wheelmaker/mobile/android/WheelMakerAndroid.apk
~/.wheelmaker/mobile/android/android-release.json
```

Build workspace:

```text
~/.wheelmaker/build/mobile/android/
```

The Android build does not write generated Web assets, Gradle output, APK files, or release manifests into the git worktree.
The first Android slice only builds a local APK. It does not publish APK downloads through Nginx, Monitor, or the Update screen.
```
```

- [ ] **Step 2: Add app README note**

In `app/README.md`, add this to the release model section:

```markdown
3. Android APK embedded snapshot:
   - Run `publish-android.bat` from the repository root.
   - The publisher builds Web assets into `~/.wheelmaker/build/mobile/android/webroot`.
   - Gradle packages that external Web root into the APK assets.
   - APK output is written to `~/.wheelmaker/mobile/android`.
   - No Android-generated Web assets are written under `app/` or `mobile/android/`.
```

- [ ] **Step 3: Run focused verification**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/cli-setup.test.js __tests__/web-native-web-source.test.ts __tests__/web-runtime.test.ts __tests__/web-desktop-web-source.test.ts
npm run tsc:web
cd ..
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_publish_android_ps1.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/publish_android.ps1 -WhatIf
gradle -p mobile/android :app:testDebugUnitTest `
  --project-cache-dir "$HOME\.wheelmaker\build\mobile\android\gradle-cache" `
  -g "$HOME\.wheelmaker\build\mobile\android\gradle-home" `
  -PwheelmakerBuildRoot="$HOME\.wheelmaker\build\mobile\android\gradle-build"
```

Expected:

- Jest focused tests pass.
- TypeScript check passes.
- Publish script structure test passes.
- Publish script WhatIf passes.
- Android unit tests pass.

- [ ] **Step 4: Run local APK build if Android SDK and Gradle are installed**

Run:

```powershell
publish-android.bat
```

Expected:

- `~/.wheelmaker/mobile/android/WheelMakerAndroid.apk` exists.
- `~/.wheelmaker/mobile/android/android-release.json` exists.
- No files are created under `mobile/android/.gradle`, `mobile/android/build`, `mobile/android/app/build`, `mobile/android/local.properties`, or `mobile/android/app/src/main/assets`.

If Android SDK or Gradle is not installed, record the missing prerequisite and do not claim APK build verification.

- [ ] **Step 5: Commit docs**

Run:

```powershell
git add README.md app/README.md
git commit -m "docs: document android apk build"
```

## Task 9: Final Gate

**Files:**
- All files changed by previous tasks.

- [ ] **Step 1: Check git status**

Run:

```powershell
git status --short
```

Expected: no untracked generated Android output under the repo. Source/doc changes should already be committed.

- [ ] **Step 2: Run final verification set**

Run:

```powershell
cd app
npm test -- --runTestsByPath __tests__/cli-setup.test.js __tests__/web-native-web-source.test.ts __tests__/web-runtime.test.ts __tests__/web-desktop-web-source.test.ts
npm run tsc:web
cd ..
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_publish_android_ps1.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/publish_android.ps1 -WhatIf
```

Expected: all commands pass.

- [ ] **Step 3: Run Android tests if Gradle is available**

Run:

```powershell
gradle -p mobile/android :app:testDebugUnitTest `
  --project-cache-dir "$HOME\.wheelmaker\build\mobile\android\gradle-cache" `
  -g "$HOME\.wheelmaker\build\mobile\android\gradle-home" `
  -PwheelmakerBuildRoot="$HOME\.wheelmaker\build\mobile\android\gradle-build"
```

Expected: PASS if Android SDK and Gradle are installed. If prerequisites are missing, report the exact missing command or SDK error.

- [ ] **Step 4: Push branch**

Run:

```powershell
git push origin main
```

Expected: push succeeds.

## Self-Review

- Spec coverage: plan covers local APK build, `mobile/android/`, stable app origin, Remote Web proxy/fallback, IndexedDB preservation, Android shell preferences, zero repo-local generated output, no Go/Nginx/Monitor changes, and no Update-screen APK download.
- Marker scan: no task uses unresolved markers, open-ended "add tests" instructions, or missing file paths.
- Type consistency: Web native types use `remoteWebUrl` for candidates and `remoteUrl` for state, matching Desktop. Android Kotlin uses the same JSON property names exposed through the bridge.
