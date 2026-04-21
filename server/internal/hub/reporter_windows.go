//go:build windows

package hub

import "syscall"

func isDirectoryReparsePoint(path string) bool {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		return false
	}
	const fileAttributeDirectory = uint32(0x10)
	const fileAttributeReparsePoint = uint32(0x400)
	return attrs&fileAttributeDirectory != 0 && attrs&fileAttributeReparsePoint != 0
}
