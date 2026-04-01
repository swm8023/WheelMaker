//go:build !windows

package winsvc

import "context"

type Runner func(context.Context) error
type IgnoreError func(error) bool

func RunIfWindowsService(_ string, _ Runner, _ IgnoreError) (bool, error) {
	return false, nil
}

