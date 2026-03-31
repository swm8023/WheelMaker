//go:build !windows

package main

import "net/http"

func runAsWindowsServiceIfNeeded(_ string, _ http.Handler) (bool, error) {
	return false, nil
}
