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
