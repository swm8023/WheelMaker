//go:build windows

package main

import "testing"

type fakeDesktopWindowOps struct {
	rect      desktopWindowRect
	workArea  desktopWindowRect
	insets    desktopWindowFrameInsets
	maximized bool
	moves     []desktopWindowRect
	shows     []uintptr
}

func (f *fakeDesktopWindowOps) windowRect(uintptr) (desktopWindowRect, bool) {
	return f.rect, true
}

func (f *fakeDesktopWindowOps) monitorWorkArea(uintptr) (desktopWindowRect, bool) {
	return f.workArea, true
}

func (f *fakeDesktopWindowOps) moveWindow(_ uintptr, rect desktopWindowRect) {
	f.moves = append(f.moves, rect)
	f.rect = rect
}

func (f *fakeDesktopWindowOps) frameInsets() desktopWindowFrameInsets {
	return f.insets
}

func (f *fakeDesktopWindowOps) showWindow(_ uintptr, command uintptr) {
	f.shows = append(f.shows, command)
	if command == swRestore {
		f.maximized = false
	}
}

func (f *fakeDesktopWindowOps) isMaximized(uintptr) bool {
	return f.maximized
}

func TestDesktopMaximizeControllerUsesMonitorWorkArea(t *testing.T) {
	ops := &fakeDesktopWindowOps{
		rect:     desktopWindowRect{left: 100, top: 120, right: 900, bottom: 720},
		workArea: desktopWindowRect{left: 0, top: 0, right: 1920, bottom: 1040},
		insets:   desktopWindowFrameInsets{x: 8, y: 8},
	}
	controller := newDesktopMaximizeController(42, ops)

	controller.toggle()

	if len(ops.shows) != 0 {
		t.Fatalf("toggle should not use SW_MAXIMIZE, shows=%v", ops.shows)
	}
	if len(ops.moves) != 1 {
		t.Fatalf("moves len=%d, want 1", len(ops.moves))
	}
	want := desktopWindowRect{left: -8, top: -8, right: 1928, bottom: 1048}
	if got := ops.moves[0]; got != want {
		t.Fatalf("maximized rect=%+v, want frame-expanded work area %+v", got, want)
	}
}

func TestDesktopMaximizeControllerRestoresPreviousRect(t *testing.T) {
	ops := &fakeDesktopWindowOps{
		rect:     desktopWindowRect{left: 100, top: 120, right: 900, bottom: 720},
		workArea: desktopWindowRect{left: 0, top: 0, right: 1920, bottom: 1040},
	}
	controller := newDesktopMaximizeController(42, ops)
	original := ops.rect

	controller.toggle()
	controller.toggle()

	if len(ops.moves) != 2 {
		t.Fatalf("moves len=%d, want 2", len(ops.moves))
	}
	if got := ops.moves[1]; got != original {
		t.Fatalf("restored rect=%+v, want original %+v", got, original)
	}
}
