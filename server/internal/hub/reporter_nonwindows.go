//go:build !windows

package hub

func isDirectoryReparsePoint(path string) bool {
	return false
}
