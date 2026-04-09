//go:build !linux
// +build !linux

package utils

import "errors"

// Reflink is a stub for non-Linux platforms.
func Reflink(src, dst string) error {
	return errors.New("reflink is not supported on this platform")
}
