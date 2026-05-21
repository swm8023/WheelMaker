//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	gwlStyle = -16

	wsCaption     = 0x00c00000
	wsSysMenu     = 0x00080000
	wsThickFrame  = 0x00040000
	wsMinimizeBox = 0x00020000
	wsMaximizeBox = 0x00010000

	swpNoSize        = 0x0001
	swpNoMove        = 0x0002
	swpNoZOrder      = 0x0004
	swpNoOwnerZOrder = 0x0200
	swpFrameChanged  = 0x0020

	swMinimize = 6
	swMaximize = 3
	swRestore  = 9

	wmClose         = 0x0010
	wmNCLButtonDown = 0x00a1
	htCaption       = 2

	dwmwaUseImmersiveDarkMode = 20
	dwmwaBorderColor          = 34
	dwmwaCaptionColor         = 35

	smCXSizeFrame    = 32
	smCYSizeFrame    = 33
	smCXPaddedBorder = 92
)

var (
	user32                    = windows.NewLazySystemDLL("user32.dll")
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procGetWindowLongPtrW     = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW     = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procMonitorFromWindow     = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW       = user32.NewProc("GetMonitorInfoW")
	procReleaseCapture        = user32.NewProc("ReleaseCapture")
	procSendMessageW          = user32.NewProc("SendMessageW")
	procPostMessageW          = user32.NewProc("PostMessageW")
	procShowWindow            = user32.NewProc("ShowWindow")
	procIsZoomed              = user32.NewProc("IsZoomed")
	procGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const monitorDefaultToNearest = 2

type desktopWindowRect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

type desktopWindowFrameInsets struct {
	x int32
	y int32
}

func (r desktopWindowRect) expandedBy(insets desktopWindowFrameInsets) desktopWindowRect {
	return desktopWindowRect{
		left:   r.left - insets.x,
		top:    r.top - insets.y,
		right:  r.right + insets.x,
		bottom: r.bottom + insets.y,
	}
}

func (r desktopWindowRect) width() int32 {
	return r.right - r.left
}

func (r desktopWindowRect) height() int32 {
	return r.bottom - r.top
}

type monitorInfo struct {
	cbSize    uint32
	rcMonitor desktopWindowRect
	rcWork    desktopWindowRect
	dwFlags   uint32
}

func getWindowLongPtr(hwnd uintptr, index int32) uintptr {
	value, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(index))
	return value
}

func setWindowLongPtr(hwnd uintptr, index int32, value uintptr) {
	procSetWindowLongPtrW.Call(hwnd, uintptr(index), value)
}

func setWindowPos(hwnd uintptr, flags uintptr) {
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, flags)
}

func getWindowRect(hwnd uintptr) (desktopWindowRect, bool) {
	var rect desktopWindowRect
	result, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	return rect, result != 0
}

func getMonitorWorkArea(hwnd uintptr) (desktopWindowRect, bool) {
	monitor, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if monitor == 0 {
		return desktopWindowRect{}, false
	}
	info := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	result, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		return desktopWindowRect{}, false
	}
	return info.rcWork, true
}

func moveWindowToRect(hwnd uintptr, rect desktopWindowRect) {
	procSetWindowPos.Call(
		hwnd,
		0,
		uintptr(uint32(rect.left)),
		uintptr(uint32(rect.top)),
		uintptr(uint32(rect.width())),
		uintptr(uint32(rect.height())),
		swpNoZOrder|swpNoOwnerZOrder,
	)
}

func releaseCapture() {
	procReleaseCapture.Call()
}

func sendWindowMessage(hwnd uintptr, msg uintptr, wparam uintptr, lparam uintptr) {
	procSendMessageW.Call(hwnd, msg, wparam, lparam)
}

func postWindowMessage(hwnd uintptr, msg uintptr, wparam uintptr, lparam uintptr) {
	procPostMessageW.Call(hwnd, msg, wparam, lparam)
}

func showWindow(hwnd uintptr, command uintptr) {
	procShowWindow.Call(hwnd, command)
}

func isWindowMaximized(hwnd uintptr) bool {
	result, _, _ := procIsZoomed.Call(hwnd)
	return result != 0
}

func getWindowFrameInsets() desktopWindowFrameInsets {
	sizeFrameX := getSystemMetric(smCXSizeFrame)
	sizeFrameY := getSystemMetric(smCYSizeFrame)
	paddedBorder := getSystemMetric(smCXPaddedBorder)
	return desktopWindowFrameInsets{
		x: sizeFrameX + paddedBorder,
		y: sizeFrameY + paddedBorder,
	}
}

func getSystemMetric(index int32) int32 {
	result, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int32(result)
}

func setDwmWindowAttribute(hwnd uintptr, attribute uint32, value unsafe.Pointer, size uint32) error {
	result, _, err := procDwmSetWindowAttribute.Call(hwnd, uintptr(attribute), uintptr(value), uintptr(size))
	if result == 0 {
		return nil
	}
	if err != windows.Errno(0) {
		return err
	}
	return windows.Errno(result)
}
