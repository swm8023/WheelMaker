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
