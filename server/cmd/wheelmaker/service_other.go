//go:build !windows

package main

func runAsWindowsServiceIfNeeded(_ []string) (bool, error) {
	return false, nil
}
