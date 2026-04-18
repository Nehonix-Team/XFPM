//go:build !windows
// +build !windows

package utils

func initTerminal() {
	// Virtual terminal processing is not needed on Unix-like systems.
}
