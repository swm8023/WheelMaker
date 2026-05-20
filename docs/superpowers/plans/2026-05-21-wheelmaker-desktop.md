# WheelMaker Desktop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Windows-only `WheelMakerDesktop.exe` manual desktop distribution with embedded Workspace Web UI assets and a current-user desktop shortcut.

**Architecture:** Keep the existing Web release flow as the primary path. Add a separate desktop command that serves embedded Web assets from `embed.FS` through a loopback HTTP server and opens that URL with WebView2. Add a manual publish script that builds Web assets into the desktop embed staging directory, builds the desktop executable, writes a release manifest, and creates the shortcut without touching services.

**Tech Stack:** Go 1.26, `github.com/jchv/go-webview2`, Go `embed.FS`, PowerShell, Webpack, Jest.

---

## File Structure

- Modify `app/web/webpack.config.js`: allow `WHEELMAKER_WEB_TARGET` to override the Webpack output path while preserving the default `~/.wheelmaker/web`.
- Modify `app/__tests__/web-setup.test.js`: test the configurable Webpack output path.
- Create `server/cmd/wheelmaker-desktop/assets.go`: embed generated desktop Web assets.
- Create `server/cmd/wheelmaker-desktop/server.go`: loopback static server and SPA fallback.
- Create `server/cmd/wheelmaker-desktop/server_test.go`: tests for root serving, asset serving, SPA fallback, and missing asset 404.
- Create `server/cmd/wheelmaker-desktop/app.go`: app orchestration with injectable launcher.
- Create `server/cmd/wheelmaker-desktop/app_test.go`: tests for server startup URL and launcher error formatting.
- Create `server/cmd/wheelmaker-desktop/webview_windows.go`: Windows WebView2 launcher.
- Create `server/cmd/wheelmaker-desktop/webview_unsupported.go`: non-Windows unsupported launcher.
- Create `server/cmd/wheelmaker-desktop/main.go`: command entrypoint.
- Create `server/cmd/wheelmaker-desktop/webroot/.gitkeep`: committed placeholder so `embed` has a stable directory before publish.
- Modify `server/go.mod` and `server/go.sum`: add the WebView2 binding.
- Modify `.gitignore`: ignore generated desktop webroot content while keeping `.gitkeep`.
- Create `scripts/publish_desktop.ps1`: manual desktop publisher.
- Create `publish-desktop.bat`: root manual entrypoint.
- Create `scripts/test_publish_desktop_ps1.ps1`: static checks for the desktop publish script.
- Modify `scripts/test_deploy_bat.ps1`, `scripts/test_update_publish_bat.ps1`, and `scripts/test_refresh_server_ps1.ps1`: assert desktop publishing stays out of existing automated flows.

---

### Task 1: Configurable Webpack Desktop Staging

**Files:**
- Modify: `app/web/webpack.config.js`
- Modify: `app/__tests__/web-setup.test.js`

- [ ] **Step 1: Write the failing Jest test**

Append this test to `app/__tests__/web-setup.test.js`:

```js
  test('webpack output path can be redirected for desktop staging', () => {
    const projectRoot = path.join(__dirname, '..');
    const webpackConfigPath = path.join(projectRoot, 'web', 'webpack.config.js');
    const target = path.join(projectRoot, '..', 'server', 'cmd', 'wheelmaker-desktop', 'webroot');
    const previous = process.env.WHEELMAKER_WEB_TARGET;

    delete require.cache[require.resolve(webpackConfigPath)];
    process.env.WHEELMAKER_WEB_TARGET = target;
    const redirected = require(webpackConfigPath);

    if (previous === undefined) {
      delete process.env.WHEELMAKER_WEB_TARGET;
    } else {
      process.env.WHEELMAKER_WEB_TARGET = previous;
    }
    delete require.cache[require.resolve(webpackConfigPath)];
    const normal = require(webpackConfigPath);

    expect(redirected.output.path).toBe(path.resolve(target));
    expect(normal.output.path).toBe(path.join(require('os').homedir(), '.wheelmaker', 'web'));
  });
```

- [ ] **Step 2: Run the test to verify RED**

Run: `cd app && npm test -- web-setup.test.js --runInBand`

Expected: FAIL because `redirected.output.path` still points at `~/.wheelmaker/web`.

- [ ] **Step 3: Implement the Webpack output override**

Change `app/web/webpack.config.js` near the top to:

```js
const webTarget = process.env.WHEELMAKER_WEB_TARGET
  ? path.resolve(process.env.WHEELMAKER_WEB_TARGET)
  : path.join(os.homedir(), '.wheelmaker', 'web');
```

- [ ] **Step 4: Run the test to verify GREEN**

Run: `cd app && npm test -- web-setup.test.js --runInBand`

Expected: PASS.

---

### Task 2: Embedded Static Server

**Files:**
- Create: `server/cmd/wheelmaker-desktop/assets.go`
- Create: `server/cmd/wheelmaker-desktop/server.go`
- Create: `server/cmd/wheelmaker-desktop/server_test.go`
- Create: `server/cmd/wheelmaker-desktop/webroot/.gitkeep`
- Modify: `.gitignore`

- [ ] **Step 1: Write the failing Go tests**

Create `server/cmd/wheelmaker-desktop/server_test.go`:

```go
package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDesktopAssetHandlerServesRootIndex(t *testing.T) {
	handler := newDesktopAssetHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html>WheelMaker</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "WheelMaker") {
		t.Fatalf("body=%q should include index content", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type=%q should be text/html", got)
	}
}

func TestDesktopAssetHandlerServesStaticAsset(t *testing.T) {
	handler := newDesktopAssetHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html></html>")},
		"bundle.js":  {Data: []byte("console.log('wm')")},
	})

	req := httptest.NewRequest(http.MethodGet, "/bundle.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "console.log('wm')" {
		t.Fatalf("body=%q", got)
	}
}

func TestDesktopAssetHandlerFallsBackToIndexForWorkspaceRoute(t *testing.T) {
	handler := newDesktopAssetHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html>shell</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/settings/skills", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); !strings.Contains(got, "shell") {
		t.Fatalf("body=%q should be index fallback", got)
	}
}

func TestDesktopAssetHandlerDoesNotFallbackForMissingFileAsset(t *testing.T) {
	handler := newDesktopAssetHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html>shell</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStartDesktopAssetServerUsesLoopback(t *testing.T) {
	srv, err := startDesktopAssetServer(fstest.MapFS{
		"index.html": {Data: []byte("<html>loopback</html>")},
	})
	if err != nil {
		t.Fatalf("startDesktopAssetServer: %v", err)
	}
	defer srv.Close()

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatalf("GET server root: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want %d", resp.StatusCode, http.StatusOK)
	}
	if !strings.Contains(string(body), "loopback") {
		t.Fatalf("body=%q should include embedded index", string(body))
	}
	if !strings.HasPrefix(srv.URL(), "http://127.0.0.1:") {
		t.Fatalf("url=%q should use loopback", srv.URL())
	}
}
```

- [ ] **Step 2: Run the test to verify RED**

Run: `cd server && go test ./cmd/wheelmaker-desktop`

Expected: FAIL because the package and functions do not exist yet.

- [ ] **Step 3: Add generated webroot ignore rules and placeholder**

Modify `.gitignore`:

```gitignore
server/cmd/wheelmaker-desktop/webroot/*
!server/cmd/wheelmaker-desktop/webroot/.gitkeep
```

Create `server/cmd/wheelmaker-desktop/webroot/.gitkeep` as an empty placeholder.

- [ ] **Step 4: Implement embedded assets and static serving**

Create `server/cmd/wheelmaker-desktop/assets.go`:

```go
package main

import (
	"embed"
	"io/fs"
)

//go:embed all:webroot
var embeddedWebRoot embed.FS

func embeddedAssets() (fs.FS, error) {
	return fs.Sub(embeddedWebRoot, "webroot")
}
```

Create `server/cmd/wheelmaker-desktop/server.go`:

```go
package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
)

type desktopAssetServer struct {
	server *http.Server
	ln     net.Listener
	url    string
}

func (s *desktopAssetServer) URL() string {
	return s.url
}

func (s *desktopAssetServer) Close() error {
	return s.server.Close()
}

func startDesktopAssetServer(assets fs.FS) (*desktopAssetServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen desktop asset server: %w", err)
	}
	handler := newDesktopAssetHandler(assets)
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	out := &desktopAssetServer{
		server: srv,
		ln:     ln,
		url:    "http://" + ln.Addr().String() + "/",
	}
	go func() {
		err := srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			// The desktop app has no background logger yet; startup path reports listen errors synchronously.
		}
	}()
	return out, nil
}

func newDesktopAssetHandler(assets fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := cleanAssetPath(r.URL.Path)
		if name == "" {
			name = "index.html"
		}
		data, err := fs.ReadFile(assets, name)
		if err != nil {
			if isWorkspaceRoute(name) {
				data, err = fs.ReadFile(assets, "index.html")
				name = "index.html"
			}
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		if contentType == "" && name == "index.html" {
			contentType = "text/html; charset=utf-8"
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

func cleanAssetPath(raw string) string {
	clean := path.Clean("/" + raw)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "." {
		return ""
	}
	return clean
}

func isWorkspaceRoute(name string) bool {
	return path.Ext(name) == ""
}
```

- [ ] **Step 5: Run the test to verify GREEN**

Run: `cd server && go test ./cmd/wheelmaker-desktop`

Expected: PASS.

---

### Task 3: Desktop App Orchestration and WebView2 Launcher

**Files:**
- Create: `server/cmd/wheelmaker-desktop/app.go`
- Create: `server/cmd/wheelmaker-desktop/app_test.go`
- Create: `server/cmd/wheelmaker-desktop/webview_windows.go`
- Create: `server/cmd/wheelmaker-desktop/webview_unsupported.go`
- Create: `server/cmd/wheelmaker-desktop/main.go`
- Modify: `server/go.mod`
- Modify: `server/go.sum`

- [ ] **Step 1: Write the failing orchestration tests**

Create `server/cmd/wheelmaker-desktop/app_test.go`:

```go
package main

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

type recordingLauncher struct {
	url string
	err error
}

func (r *recordingLauncher) Launch(url string, opts desktopWindowOptions) error {
	r.url = url
	return r.err
}

func TestRunDesktopAppStartsServerAndLaunchesLoopbackURL(t *testing.T) {
	launcher := &recordingLauncher{}
	err := runDesktopApp(fstest.MapFS{
		"index.html": {Data: []byte("<html>desktop</html>")},
	}, launcher)

	if err != nil {
		t.Fatalf("runDesktopApp: %v", err)
	}
	if !strings.HasPrefix(launcher.url, "http://127.0.0.1:") {
		t.Fatalf("url=%q should use loopback", launcher.url)
	}
}

func TestRunDesktopAppReturnsActionableWebViewError(t *testing.T) {
	launcher := &recordingLauncher{err: errWebView2Unavailable}
	err := runDesktopApp(fstest.MapFS{
		"index.html": {Data: []byte("<html>desktop</html>")},
	}, launcher)

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Microsoft Edge WebView2 Runtime") {
		t.Fatalf("error=%q should mention WebView2 runtime", err.Error())
	}
}

func TestRunDesktopAppReportsMissingIndex(t *testing.T) {
	launcher := &recordingLauncher{}
	err := runDesktopApp(fstest.MapFS{}, launcher)

	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error=%v should wrap fs.ErrNotExist", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify RED**

Run: `cd server && go test ./cmd/wheelmaker-desktop`

Expected: FAIL because app orchestration types and functions do not exist yet.

- [ ] **Step 3: Add WebView2 dependency**

Run: `cd server && go get github.com/jchv/go-webview2@v0.0.0-20260205173254-56598839c808`

Expected: `server/go.mod` and `server/go.sum` update with `github.com/jchv/go-webview2` and its transitive dependencies.

- [ ] **Step 4: Implement app orchestration**

Create `server/cmd/wheelmaker-desktop/app.go`:

```go
package main

import (
	"errors"
	"fmt"
	"io/fs"
)

var errWebView2Unavailable = errors.New("Microsoft Edge WebView2 Runtime is required to run WheelMaker Desktop")

type desktopWindowOptions struct {
	Title  string
	Width  uint
	Height uint
	Debug  bool
}

type desktopLauncher interface {
	Launch(url string, opts desktopWindowOptions) error
}

func runDesktopApp(assets fs.FS, launcher desktopLauncher) error {
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		return fmt.Errorf("desktop web assets missing index.html: %w", err)
	}
	srv, err := startDesktopAssetServer(assets)
	if err != nil {
		return err
	}
	defer srv.Close()

	opts := desktopWindowOptions{
		Title:  "WheelMaker Desktop",
		Width:  1280,
		Height: 840,
	}
	if err := launcher.Launch(srv.URL(), opts); err != nil {
		if errors.Is(err, errWebView2Unavailable) {
			return fmt.Errorf("%w. Install it from https://developer.microsoft.com/microsoft-edge/webview2/", err)
		}
		return err
	}
	return nil
}
```

- [ ] **Step 5: Implement platform launchers and command entrypoint**

Create `server/cmd/wheelmaker-desktop/webview_windows.go`:

```go
//go:build windows

package main

import webview2 "github.com/jchv/go-webview2"

type webView2Launcher struct{}

func newWebView2Launcher() desktopLauncher {
	return webView2Launcher{}
}

func (webView2Launcher) Launch(url string, opts desktopWindowOptions) error {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     opts.Debug,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  opts.Title,
			Width:  opts.Width,
			Height: opts.Height,
			Center: true,
		},
	})
	if w == nil {
		return errWebView2Unavailable
	}
	defer w.Destroy()
	w.Navigate(url)
	w.Run()
	return nil
}
```

Create `server/cmd/wheelmaker-desktop/webview_unsupported.go`:

```go
//go:build !windows

package main

type unsupportedLauncher struct{}

func newWebView2Launcher() desktopLauncher {
	return unsupportedLauncher{}
}

func (unsupportedLauncher) Launch(_ string, _ desktopWindowOptions) error {
	return errWebView2Unavailable
}
```

Create `server/cmd/wheelmaker-desktop/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "WheelMakerDesktop: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	assets, err := embeddedAssets()
	if err != nil {
		return err
	}
	return runDesktopApp(assets, newWebView2Launcher())
}
```

- [ ] **Step 6: Run the tests to verify GREEN**

Run: `cd server && go test ./cmd/wheelmaker-desktop`

Expected: PASS.

---

### Task 4: Manual Desktop Publish Scripts

**Files:**
- Create: `scripts/publish_desktop.ps1`
- Create: `publish-desktop.bat`
- Create: `scripts/test_publish_desktop_ps1.ps1`
- Modify: `scripts/test_deploy_bat.ps1`
- Modify: `scripts/test_update_publish_bat.ps1`
- Modify: `scripts/test_refresh_server_ps1.ps1`

- [ ] **Step 1: Write failing script tests**

Create `scripts/test_publish_desktop_ps1.ps1`:

```powershell
$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$scriptPath = Join-Path $repoRoot "scripts\publish_desktop.ps1"
$batPath = Join-Path $repoRoot "publish-desktop.bat"

if (-not (Test-Path $scriptPath)) { throw "publish_desktop.ps1 is missing" }
if (-not (Test-Path $batPath)) { throw "publish-desktop.bat is missing" }

$script = Get-Content -LiteralPath $scriptPath -Raw -Encoding UTF8
$bat = Get-Content -LiteralPath $batPath -Raw -Encoding UTF8

function Assert-Contains {
  param([string]$Label, [string]$Text, [string]$Needle)
  if (-not $Text.Contains($Needle)) {
    throw "$Label does not contain expected text: $Needle"
  }
}

function Assert-NotContains {
  param([string]$Label, [string]$Text, [string]$Needle)
  if ($Text.Contains($Needle)) {
    throw "$Label should not contain text: $Needle"
  }
}

Assert-Contains "publish_desktop.ps1" $script "WHEELMAKER_WEB_TARGET"
Assert-Contains "publish_desktop.ps1" $script "npm run build:web"
Assert-Contains "publish_desktop.ps1" $script "node scripts/export_web_release.js"
Assert-Contains "publish_desktop.ps1" $script "go build"
Assert-Contains "publish_desktop.ps1" $script "WheelMakerDesktop.exe"
Assert-Contains "publish_desktop.ps1" $script "desktop-release.json"
Assert-Contains "publish_desktop.ps1" $script "CreateShortcut"
Assert-Contains "publish_desktop.ps1" $script "Desktop"
Assert-NotContains "publish_desktop.ps1" $script "Restart-Services"
Assert-NotContains "publish_desktop.ps1" $script "update-now.signal"
Assert-Contains "publish-desktop.bat" $bat "scripts\publish_desktop.ps1"

Write-Host "desktop publish script checks passed"
```

Append these assertions to the existing script tests:

```powershell
Assert-NotContains "publish_desktop.ps1"
Assert-NotContains "publish-desktop.bat"
```

Use each test file's existing helper signatures when appending to `scripts/test_deploy_bat.ps1`, `scripts/test_update_publish_bat.ps1`, and `scripts/test_refresh_server_ps1.ps1`.

- [ ] **Step 2: Run script tests to verify RED**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_publish_desktop_ps1.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_bat.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_update_publish_bat.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_ps1.ps1
```

Expected: `test_publish_desktop_ps1.ps1` fails because the scripts do not exist.

- [ ] **Step 3: Implement `publish-desktop.bat`**

Create `publish-desktop.bat`:

```bat
@echo off
setlocal
title WheelMaker Desktop Publish

where pwsh >nul 2>&1
if %errorlevel% equ 0 (
  set "_PS=pwsh"
) else (
  set "_PS=powershell"
)

%_PS% -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\publish_desktop.ps1"
set "_EXIT=%errorlevel%"
if not "%_EXIT%"=="0" (
  echo.
  echo [FAILED] desktop publish exited with code %_EXIT%
  pause
  exit /b %_EXIT%
)

echo.
echo [OK] WheelMaker Desktop publish complete
pause
exit /b 0
```

- [ ] **Step 4: Implement `scripts/publish_desktop.ps1`**

Create `scripts/publish_desktop.ps1` with these core functions:

```powershell
param(
  [string]$RepoRoot = "",
  [string]$OutputDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\desktop"),
  [switch]$WhatIf
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Step { param([string]$Text) Write-Host ("==> {0}" -f $Text) }
function Assert-Command {
  param([string]$Name, [string]$Hint = "")
  if (Get-Command $Name -ErrorAction SilentlyContinue) { return }
  if ([string]::IsNullOrWhiteSpace($Hint)) { throw ("required command not found in PATH: {0}" -f $Name) }
  throw ("required command not found in PATH: {0}. {1}" -f $Name, $Hint)
}
function Invoke-Checked {
  param([string]$FilePath, [string[]]$Arguments = @(), [string]$FailureMessage = "")
  & $FilePath @Arguments
  if ($LASTEXITCODE -eq 0) { return }
  if ([string]::IsNullOrWhiteSpace($FailureMessage)) {
    throw ("command failed: {0} {1} (exit={2})" -f $FilePath, ($Arguments -join " "), $LASTEXITCODE)
  }
  throw ("{0} (exit={1})" -f $FailureMessage, $LASTEXITCODE)
}
function Get-GitValue {
  param([string[]]$Arguments)
  Push-Location $script:RepoRoot
  try {
    $value = ((& git @Arguments) | Select-Object -First 1)
    if ($LASTEXITCODE -ne 0) { throw ("git {0} failed (exit={1})" -f ($Arguments -join " "), $LASTEXITCODE) }
    return ([string]$value).Trim()
  } finally { Pop-Location }
}
function Reset-DesktopWebRoot {
  $resolved = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($script:DesktopWebRoot)
  $expected = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath((Join-Path $script:RepoRoot "server\cmd\wheelmaker-desktop\webroot"))
  if (-not [String]::Equals($resolved, $expected, [StringComparison]::OrdinalIgnoreCase)) {
    throw ("refusing to clean unexpected desktop webroot: {0}" -f $resolved)
  }
  if ($WhatIf) { Write-Host ("[whatif] clean {0}" -f $resolved); return }
  New-Item -ItemType Directory -Path $resolved -Force | Out-Null
  Get-ChildItem -LiteralPath $resolved -Force | Where-Object { $_.Name -ne ".gitkeep" } | Remove-Item -Recurse -Force
}
function Build-DesktopWeb {
  Assert-Command -Name "npm" -Hint "Install Node.js 22+."
  Reset-DesktopWebRoot
  $previousTarget = $env:WHEELMAKER_WEB_TARGET
  $env:WHEELMAKER_WEB_TARGET = $script:DesktopWebRoot
  Push-Location $script:AppRoot
  try {
    Write-Step "build embedded Workspace Web UI"
    if ($WhatIf) {
      Write-Host ("[whatif] WHEELMAKER_WEB_TARGET={0} npm run build:web" -f $script:DesktopWebRoot)
      Write-Host "[whatif] node scripts/export_web_release.js"
      return
    }
    Invoke-Checked -FilePath "npm" -Arguments @("run", "build:web") -FailureMessage "desktop web build failed"
    Invoke-Checked -FilePath "node" -Arguments @("scripts/export_web_release.js") -FailureMessage "desktop web public asset export failed"
  } finally {
    if ($null -ne $previousTarget) { $env:WHEELMAKER_WEB_TARGET = $previousTarget } else { Remove-Item Env:WHEELMAKER_WEB_TARGET -ErrorAction SilentlyContinue }
    Pop-Location
  }
}
function Build-DesktopBinary {
  Assert-Command -Name "go" -Hint "Install Go 1.26+."
  New-Item -ItemType Directory -Path $script:OutputDir -Force | Out-Null
  Push-Location $script:ServerRoot
  try {
    Write-Step ("build WheelMakerDesktop.exe: {0}" -f $script:DesktopExe)
    if ($WhatIf) { Write-Host ("[whatif] go build -ldflags -H windowsgui -o {0} ./cmd/wheelmaker-desktop/" -f $script:DesktopExe); return }
    Invoke-Checked -FilePath "go" -Arguments @("build", "-ldflags", "-H windowsgui", "-o", $script:DesktopExe, "./cmd/wheelmaker-desktop/") -FailureMessage "desktop binary build failed"
  } finally { Pop-Location }
}
function Write-DesktopReleaseManifest {
  Assert-Command -Name "git" -Hint "Install Git and ensure git.exe is available."
  $manifest = [ordered]@{
    "schemaVersion" = 1
    "repo" = $script:RepoRoot
    "branch" = Get-GitValue -Arguments @("branch", "--show-current")
    "sha" = Get-GitValue -Arguments @("rev-parse", "HEAD")
    "builtAt" = (Get-Date).ToUniversalTime().ToString("o")
    "desktopExe" = $script:DesktopExe
    "embeddedWebRoot" = $script:DesktopWebRoot
  }
  if ($WhatIf) { Write-Host ("[whatif] write {0}" -f $script:ManifestPath); return }
  $json = $manifest | ConvertTo-Json -Depth 4
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($script:ManifestPath, $json, $utf8NoBom)
}
function New-DesktopShortcut {
  $desktop = [Environment]::GetFolderPath("Desktop")
  $shortcutPath = Join-Path $desktop "WheelMaker Desktop.lnk"
  Write-Step ("create desktop shortcut: {0}" -f $shortcutPath)
  if ($WhatIf) { Write-Host ("[whatif] CreateShortcut {0} -> {1}" -f $shortcutPath, $script:DesktopExe); return }
  $shell = New-Object -ComObject WScript.Shell
  $shortcut = $shell.CreateShortcut($shortcutPath)
  $shortcut.TargetPath = $script:DesktopExe
  $shortcut.WorkingDirectory = $script:OutputDir
  $shortcut.IconLocation = $script:DesktopExe
  $shortcut.Save()
}

$script:RepoRoot = if ([string]::IsNullOrWhiteSpace($RepoRoot)) { (Resolve-Path (Join-Path $PSScriptRoot "..")).Path } else { (Resolve-Path $RepoRoot).Path }
$script:AppRoot = Join-Path $script:RepoRoot "app"
$script:ServerRoot = Join-Path $script:RepoRoot "server"
$script:DesktopWebRoot = Join-Path $script:RepoRoot "server\cmd\wheelmaker-desktop\webroot"
$script:OutputDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputDir)
$script:DesktopExe = Join-Path $script:OutputDir "WheelMakerDesktop.exe"
$script:ManifestPath = Join-Path $script:OutputDir "desktop-release.json"

Build-DesktopWeb
Build-DesktopBinary
Write-DesktopReleaseManifest
New-DesktopShortcut
Write-Step "desktop publish complete"
```

- [ ] **Step 5: Run script tests to verify GREEN**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_publish_desktop_ps1.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_deploy_bat.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_update_publish_bat.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test_refresh_server_ps1.ps1
```

Expected: PASS.

---

### Task 5: Publish Dry Run and Build Verification

**Files:**
- Verify: `scripts/publish_desktop.ps1`
- Verify: `server/cmd/wheelmaker-desktop`

- [ ] **Step 1: Run desktop publisher dry run**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/publish_desktop.ps1 -WhatIf`

Expected: PASS, prints the Web build, Go build, manifest, and shortcut actions without writing release output.

- [ ] **Step 2: Run desktop package tests**

Run: `cd server && go test ./cmd/wheelmaker-desktop`

Expected: PASS.

- [ ] **Step 3: Build the desktop command**

Run: `cd server && go build -o ..\.tmp\WheelMakerDesktop-test.exe ./cmd/wheelmaker-desktop/`

Expected: PASS and produces `.tmp/WheelMakerDesktop-test.exe`.

- [ ] **Step 4: Run broader verification**

Run:

```powershell
cd server; go test ./...
cd ..\app; npm run tsc:web
npm test -- --runInBand
```

Expected: PASS.

---

## Self-Review

- Spec coverage: plan covers Windows-only desktop command, embedded Web assets, loopback server, WebView2 runtime dependency, manual publish script, desktop shortcut, no service lifecycle management, no automatic updater integration, and preservation of the normal Web release path.
- Placeholder scan: no unresolved implementation placeholders are left in the plan.
- Type consistency: `desktopLauncher`, `desktopWindowOptions`, `runDesktopApp`, `startDesktopAssetServer`, and `newDesktopAssetHandler` are defined before later tasks depend on them.
