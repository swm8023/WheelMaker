//go:build windows

package main

type desktopWindowOps interface {
	windowRect(hwnd uintptr) (desktopWindowRect, bool)
	monitorWorkArea(hwnd uintptr) (desktopWindowRect, bool)
	moveWindow(hwnd uintptr, rect desktopWindowRect)
	frameInsets() desktopWindowFrameInsets
	showWindow(hwnd uintptr, command uintptr)
	isMaximized(hwnd uintptr) bool
}

type win32DesktopWindowOps struct{}

func (win32DesktopWindowOps) windowRect(hwnd uintptr) (desktopWindowRect, bool) {
	return getWindowRect(hwnd)
}

func (win32DesktopWindowOps) monitorWorkArea(hwnd uintptr) (desktopWindowRect, bool) {
	return getMonitorWorkArea(hwnd)
}

func (win32DesktopWindowOps) moveWindow(hwnd uintptr, rect desktopWindowRect) {
	moveWindowToRect(hwnd, rect)
}

func (win32DesktopWindowOps) frameInsets() desktopWindowFrameInsets {
	return getWindowFrameInsets()
}

func (win32DesktopWindowOps) showWindow(hwnd uintptr, command uintptr) {
	showWindow(hwnd, command)
}

func (win32DesktopWindowOps) isMaximized(hwnd uintptr) bool {
	return isWindowMaximized(hwnd)
}

type desktopMaximizeController struct {
	hwnd       uintptr
	ops        desktopWindowOps
	maximized  bool
	restore    desktopWindowRect
	hasRestore bool
}

func newDesktopMaximizeController(hwnd uintptr, ops desktopWindowOps) *desktopMaximizeController {
	return &desktopMaximizeController{hwnd: hwnd, ops: ops}
}

func (c *desktopMaximizeController) toggle() {
	if c.maximized {
		c.restoreWindow()
		return
	}
	if c.ops.isMaximized(c.hwnd) {
		c.ops.showWindow(c.hwnd, swRestore)
		return
	}
	c.maximizeToWorkArea()
}

func (c *desktopMaximizeController) maximizeToWorkArea() {
	current, hasCurrent := c.ops.windowRect(c.hwnd)
	workArea, hasWorkArea := c.ops.monitorWorkArea(c.hwnd)
	if !hasCurrent || !hasWorkArea {
		c.ops.showWindow(c.hwnd, swMaximize)
		return
	}
	c.restore = current
	c.hasRestore = true
	c.maximized = true
	c.ops.moveWindow(c.hwnd, workArea.expandedBy(c.ops.frameInsets()))
}

func (c *desktopMaximizeController) restoreWindow() {
	c.maximized = false
	if !c.hasRestore {
		c.ops.showWindow(c.hwnd, swRestore)
		return
	}
	c.ops.moveWindow(c.hwnd, c.restore)
}
