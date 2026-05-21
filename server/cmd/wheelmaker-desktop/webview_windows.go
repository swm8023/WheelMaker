//go:build windows

package main

import (
	"strconv"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
)

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
			IconId: opts.IconID,
			Center: true,
		},
	})
	if w == nil {
		return errWebView2Unavailable
	}
	defer w.Destroy()
	hwnd := uintptr(w.Window())
	if hwnd != 0 {
		if opts.CustomTitleBar {
			applyCustomTitleBarFrame(hwnd)
		}
		applyDesktopWindowTheme(hwnd, opts.ThemeColor)
		if err := bindDesktopWindowBridge(w, hwnd); err != nil {
			return err
		}
		w.Init(desktopRuntimeInitScript())
	}
	w.Navigate(url)
	w.Run()
	return nil
}

func bindDesktopWindowBridge(w webview2.WebView, hwnd uintptr) error {
	maximizeController := newDesktopMaximizeController(hwnd, win32DesktopWindowOps{})
	bindings := []struct {
		name string
		fn   func() error
	}{
		{desktopStartDragBinding, func() error {
			startWindowDrag(hwnd)
			return nil
		}},
		{desktopMinimizeBinding, func() error {
			showWindow(hwnd, swMinimize)
			return nil
		}},
		{desktopToggleMaximizeBinding, func() error {
			maximizeController.toggle()
			return nil
		}},
		{desktopCloseBinding, func() error {
			postWindowClose(hwnd)
			return nil
		}},
	}
	for _, binding := range bindings {
		if err := w.Bind(binding.name, binding.fn); err != nil {
			return err
		}
	}
	return nil
}

func applyCustomTitleBarFrame(hwnd uintptr) {
	style := getWindowLongPtr(hwnd, gwlStyle)
	style &^= wsCaption
	style |= wsSysMenu | wsThickFrame | wsMinimizeBox | wsMaximizeBox
	setWindowLongPtr(hwnd, gwlStyle, style)
	setWindowPos(hwnd, swpNoMove|swpNoSize|swpNoZOrder|swpNoOwnerZOrder|swpFrameChanged)
}

func applyDesktopWindowTheme(hwnd uintptr, hexColor string) {
	var darkMode int32 = 1
	_ = setDwmWindowAttribute(hwnd, dwmwaUseImmersiveDarkMode, unsafe.Pointer(&darkMode), uint32(unsafe.Sizeof(darkMode)))
	if color, ok := parseColorRef(hexColor); ok {
		_ = setDwmWindowAttribute(hwnd, dwmwaCaptionColor, unsafe.Pointer(&color), uint32(unsafe.Sizeof(color)))
		_ = setDwmWindowAttribute(hwnd, dwmwaBorderColor, unsafe.Pointer(&color), uint32(unsafe.Sizeof(color)))
	}
}

func startWindowDrag(hwnd uintptr) {
	releaseCapture()
	sendWindowMessage(hwnd, wmNCLButtonDown, htCaption, 0)
}

func postWindowClose(hwnd uintptr) {
	postWindowMessage(hwnd, wmClose, 0, 0)
}

func parseColorRef(hexColor string) (uint32, bool) {
	if len(hexColor) != 7 || hexColor[0] != '#' {
		return 0, false
	}
	value, err := strconv.ParseUint(hexColor[1:], 16, 32)
	if err != nil {
		return 0, false
	}
	red := value >> 16 & 0xff
	green := value >> 8 & 0xff
	blue := value & 0xff
	return uint32(red | green<<8 | blue<<16), true
}
