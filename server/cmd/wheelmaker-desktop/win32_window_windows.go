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
)

var (
	user32                    = windows.NewLazySystemDLL("user32.dll")
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procGetWindowLongPtrW     = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW     = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procReleaseCapture        = user32.NewProc("ReleaseCapture")
	procSendMessageW          = user32.NewProc("SendMessageW")
	procPostMessageW          = user32.NewProc("PostMessageW")
	procShowWindow            = user32.NewProc("ShowWindow")
	procIsZoomed              = user32.NewProc("IsZoomed")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

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
