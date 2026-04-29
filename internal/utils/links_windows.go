//go:build windows
// +build windows

package utils

import (
	"github.com/nyaosorg/go-windows-junction"
)

// CreateJunction creates a directory junction on Windows using native Win32 API.
// This is significantly faster than spawning 'cmd /c mklink /j'.
func CreateJunction(target, link string) error {
	return junction.Create(target, link)
}
