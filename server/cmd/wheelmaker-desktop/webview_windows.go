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
