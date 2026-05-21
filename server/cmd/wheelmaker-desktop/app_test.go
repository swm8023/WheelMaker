package main

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

type recordingLauncher struct {
	url  string
	opts desktopWindowOptions
	err  error
}

func (r *recordingLauncher) Launch(url string, opts desktopWindowOptions) error {
	r.url = url
	r.opts = opts
	return r.err
}

func TestRunDesktopAppLaunchesStableLoopbackStorageOrigin(t *testing.T) {
	launcher := &recordingLauncher{}
	err := runDesktopApp(fstest.MapFS{
		"index.html": {Data: []byte("<html>desktop</html>")},
	}, launcher)

	if err != nil {
		t.Fatalf("runDesktopApp: %v", err)
	}
	if launcher.url != "http://127.0.0.1:9632/" {
		t.Fatalf("url=%q should use stable desktop storage origin", launcher.url)
	}
}

func TestRunDesktopAppLaunchesWithCustomTitleBarAndIcon(t *testing.T) {
	launcher := &recordingLauncher{}
	err := runDesktopApp(fstest.MapFS{
		"index.html": {Data: []byte("<html>desktop</html>")},
	}, launcher)

	if err != nil {
		t.Fatalf("runDesktopApp: %v", err)
	}
	if launcher.opts.Title != "WheelMaker" {
		t.Fatalf("title=%q, want WheelMaker", launcher.opts.Title)
	}
	if !launcher.opts.CustomTitleBar {
		t.Fatal("expected custom title bar to be enabled")
	}
	if launcher.opts.IconID != desktopResourceIconID {
		t.Fatalf("IconID=%d, want %d", launcher.opts.IconID, desktopResourceIconID)
	}
	if launcher.opts.ThemeColor != desktopTitleBarThemeColor {
		t.Fatalf("ThemeColor=%q, want %q", launcher.opts.ThemeColor, desktopTitleBarThemeColor)
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
