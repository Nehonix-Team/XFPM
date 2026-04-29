//go:build !windows
// +build !windows

package utils

import "errors"

// CreateJunction is a stub for non-Windows platforms.
func CreateJunction(target, link string) error {
	return errors.New("junctions are only supported on Windows")
}
