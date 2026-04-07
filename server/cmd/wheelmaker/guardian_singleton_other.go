//go:build !windows

package main

func acquireGuardianSingleton() (func(), error) {
	return func() {}, nil
}
